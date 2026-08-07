package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// ─────────────────────────────────────────────────────────────
// 单归档文件分块并发下载器
//
// 对齐 JHentai archive_download_service.dart 的多 Isolate（线程）分片下载：
//   - 一个归档 zip 文件被划分为若干块，由 N 个 worker（线程）并发通过 Range 请求下载
//   - 块按顺序从共享队列领取（takeChunk），天然支持运行中动态增减 worker（线程数）
//   - 位图记录每块完成状态，.part 文件截断到「连续完成前缀」实现可靠断点续传
//   - context 取消支持暂停/取消（立即中断进行中的 Range 请求），暂停保留 .part 供恢复
//   - 配额/限流检测复用 downloadZip 的锁定语义（lockHints）
// ─────────────────────────────────────────────────────────────

// archiveChunkDownloader 单归档文件分块并发下载器
type archiveChunkDownloader struct {
	g      *archiveDownloader
	url    string
	total  int64 // 文件总大小（Content-Length）
	chunk  int64 // 每块大小（字节）
	count  int64 // 总块数
	part   string
	client *http.Client

	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	nextIdx   int64    // 下一个待领取块索引
	prefix    int64    // 连续完成前缀（块索引），其前所有块均已完成
	doneBits  []uint64 // 块完成位图
	doneBytes int64    // 已下载字节数（含前缀之后的高位块，用于进度展示）
	failed    bool
	firstErr  error

	f       *os.File // .part 文件句柄（并发 WriteAt）
	target  atomic.Int64
	current atomic.Int64
	wg      sync.WaitGroup
}

// minChunkSize 单块最小字节数（避免块过碎带来过多请求开销）
const minChunkSize = int64(1024 * 1024) // 1 MiB

// errArchiveEOF 归档分块下载连接被服务器提前中断（EOF）的哨兵错误。
// downloadChunk 将 EOF / ErrUnexpectedEOF 包装为该哨兵，供上层 runChunkDownload 识别后
// 依据「自动降低线程数规避 EOF」开关决定自动降级重试或直接报错提示手动调低线程数。
var errArchiveEOF = errors.New("归档下载连接被中断(EOF)")

// isEOFNetworkError 判断底层网络错误是否为连接被提前关闭（EOF / unexpected EOF）。
func isEOFNetworkError(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// newArchiveChunkDownloader 构造分块下载器
func newArchiveChunkDownloader(g *archiveDownloader, downloadURL string, total int64) *archiveChunkDownloader {
	ctx, cancel := context.WithCancel(context.Background())

	// 每块大小：默认 minChunkSize；当线程数多时按 total/threads 分块，避免块过大导致
	// 单块下载时间过长（暂停时不能及时响应）。块数最少 1。
	chunkSize := minChunkSize
	threads := g.m.GetSettings().ArchiveThreads
	if threads < 1 {
		threads = 1
	}
	if total > 0 && total/int64(threads) > chunkSize {
		chunkSize = total / int64(threads)
	}
	count := (total + chunkSize - 1) / chunkSize
	if count < 1 {
		count = 1
	}

	return &archiveChunkDownloader{
		g:       g,
		url:     downloadURL,
		total:   total,
		chunk:   chunkSize,
		count:   count,
		part:    g.partPath,
		client:  g.downloadClient(),
		ctx:     ctx,
		cancel:  cancel,
		doneBits: make([]uint64, (count+63)/64),
	}
}

// run 以 threads 个 worker 并发下载，全部完成后校验并落盘
func (d *archiveChunkDownloader) run(threads int) error {
	defer d.cancel()

	f, err := os.OpenFile(d.part, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	d.f = f
	defer func() {
		if d.f != nil {
			_ = d.f.Close()
		}
	}()

	// 断点续传：.part 当前大小即「连续完成前缀」字节数（上次结束/暂停时被截断到前缀）
	existing := fiSize(d.part)
	if existing >= d.total {
		log.Printf("%s [archive-engine] 任务 %s .part 已下载完整（%d 字节），直接校验", dlLogTag, d.g.task.ID, existing)
		return d.finalize()
	}
	startIdx := existing / d.chunk
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx > d.count {
		startIdx = d.count
	}
	d.mu.Lock()
	d.nextIdx = startIdx
	d.prefix = startIdx
	d.doneBytes = startIdx * d.chunk
	d.mu.Unlock()
	_ = f.Truncate(startIdx * d.chunk) // 截断高位残留，保证续传点一致

	log.Printf("%s [archive-engine] 任务 %s 分块并发下载：total=%.2f MiB 块=%d 块大小=%.2f MiB 线程=%d 续传@%.2f MiB",
		dlLogTag, d.g.task.ID, float64(d.total)/1048576, d.count, float64(d.chunk)/1048576,
		threads, float64(startIdx*d.chunk)/1048576)

	d.setTarget(threads)
	d.wg.Wait()

	d.mu.Lock()
	failed := d.failed
	firstErr := d.firstErr
	prefix := d.prefix
	d.mu.Unlock()

	// 未完整完成：截断到「连续完成前缀」，保证 .part 大小即续传点。
	// 注意不能在 markDone 中逐块截断——并发 worker 可能已写入更高偏移的数据，截断会破坏已下载块。
	if prefix < d.count {
		d.mu.Lock()
		_ = d.f.Truncate(prefix * d.chunk)
		d.mu.Unlock()
	}

	if failed {
		// 暂停/取消导致的停止不算真正失败
		if errors.Is(firstErr, errTaskStopped) || errors.Is(firstErr, context.Canceled) || d.g.stopped() {
			return errTaskStopped
		}
		return firstErr
	}

	prefBytes := prefix * d.chunk
	if prefBytes < d.total {
		return fmt.Errorf("分块下载未完成：已下载 %.2f MiB / %.2f MiB（可重试续传）",
			float64(prefBytes)/1048576, float64(d.total)/1048576)
	}

	return d.finalize()
}

// finalize 校验 zip 并 rename 为正式文件
func (d *archiveChunkDownloader) finalize() error {
	f := d.f
	if f == nil {
		// 续传完整路径：只读打开校验（无写句柄，不做 Sync）
		var err error
		f, err = os.Open(d.part)
		if err != nil {
			return err
		}
		defer f.Close()
	} else {
		if err := f.Sync(); err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		d.f = nil
	}

	if !isValidZip(d.part) {
		return fmt.Errorf("下载完成但 zip 校验失败（文件损坏，可重试）")
	}
	if err := os.Rename(d.part, d.g.zipPath); err != nil {
		return err
	}
	d.g.recordBytes(fiSize(d.g.zipPath))
	log.Printf("%s [archive-engine] 任务 %s zip 分块下载完成（%.2f MiB）",
		dlLogTag, d.g.task.ID, float64(fiSize(d.g.zipPath))/1048576)
	return nil
}

// setTarget 动态调整目标 worker（线程）数：
//   - 增大：立即补齐差额的 worker
//   - 减小：目标降低后，超员的 worker 在领取下一个块前自动退出
func (d *archiveChunkDownloader) setTarget(n int) {
	if n < 0 {
		n = 0
	}
	old := d.target.Swap(int64(n))
	if int64(n) > old {
		diff := int(n) - int(old)
		for i := 0; i < diff; i++ {
			d.wg.Add(1)
			go d.worker()
		}
	}
	// n < old：现有 worker 会在循环顶部检测 current > target 后退出
}

// stop 立即中断所有进行中的 Range 请求（暂停/取消时调用）
func (d *archiveChunkDownloader) stop() {
	d.cancel()
}

// worker 单个下载线程：循环领取块并下载，直到块耗尽或停止
func (d *archiveChunkDownloader) worker() {
	d.current.Add(1)
	defer d.wg.Done()
	defer d.current.Add(-1)

	for {
		// 线程数被调小（超员）或已停止 → 退出
		if d.current.Load() > d.target.Load() || d.ctx.Err() != nil {
			return
		}
		idx := d.takeChunk()
		if idx < 0 {
			return
		}
		if err := d.downloadChunk(idx); err != nil {
			d.setFailed(err)
			return
		}
	}
}

// takeChunk 领取下一个待下载块索引；无块可领或已失败返回 -1
func (d *archiveChunkDownloader) takeChunk() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.failed || d.nextIdx >= d.count {
		return -1
	}
	idx := d.nextIdx
	d.nextIdx++
	return idx
}

// downloadChunk 通过 Range 请求下载单个块并写入 .part 对应偏移
func (d *archiveChunkDownloader) downloadChunk(idx int64) error {
	start := idx * d.chunk
	end := start + d.chunk
	if end > d.total {
		end = d.total
	}

	req, err := http.NewRequestWithContext(d.ctx, "GET", d.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", ehReaderUserAgent)
	req.Header.Set("Referer", d.g.referer)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end-1))

	resp, err := d.client.Do(req)
	if err != nil {
		if d.ctx.Err() != nil || d.g.stopped() {
			return errTaskStopped
		}
		if isEOFNetworkError(err) {
			return fmt.Errorf("下载分块 %d 失败(EOF 连接中断): %w", idx, errArchiveEOF)
		}
		return fmt.Errorf("下载分块 %d 失败: %v", idx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		// 配额/限流锁定检测
		if msg, locked := d.lockMessage(resp); locked {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("分块 %d 返回状态码 %d（未获 206）", idx, resp.StatusCode)
	}

	buf := make([]byte, 256*1024)
	off := start
	readChunk := 0
	for {
		readChunk++
		if readChunk%4 == 0 && d.g.stopped() {
			return errTaskStopped
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := d.f.WriteAt(buf[:n], off); werr != nil {
				return werr
			}
			off += int64(n)
			d.mu.Lock()
			d.doneBytes += int64(n)
			d.mu.Unlock()
			if d.g.limiter != nil {
				d.g.limiter.wait(int64(n))
			}
			// 周期性进度落盘（每约 8 MiB）
			if (off-start)%(8*1024*1024) < 256*1024 {
				d.mu.Lock()
				done := d.doneBytes
				d.mu.Unlock()
				d.g.recordBytes(done)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			if d.ctx.Err() != nil || d.g.stopped() {
				return errTaskStopped
			}
			if isEOFNetworkError(rerr) {
				return fmt.Errorf("下载分块 %d 读取中断(EOF): %w", idx, errArchiveEOF)
			}
			return rerr
		}
	}

	d.markDone(idx)
	return nil
}

// markDone 标记块完成：置位、推进连续完成前缀、更新进度。
// 注意：不在此处截断 .part——并发 worker 可能已写入更高偏移的数据，逐块截断会破坏已下载块；
// 截断统一在 run() 结束、确认未完整完成时进行，以保证断点续传点正确。
func (d *archiveChunkDownloader) markDone(idx int64) {
	d.mu.Lock()
	d.doneBits[idx/64] |= 1 << (uint(idx) % 64)
	for d.prefix < d.count && d.doneBits[d.prefix/64]&(1<<(uint(d.prefix)%64)) != 0 {
		d.prefix++
	}
	done := d.doneBytes
	d.mu.Unlock()
	d.g.recordBytes(done)
}

// setFailed 记录首个错误并终止后续取块
func (d *archiveChunkDownloader) setFailed(err error) {
	d.mu.Lock()
	if !d.failed {
		d.failed = true
		d.firstErr = err
	}
	d.mu.Unlock()
}

// lockMessage 检测响应是否为配额/限流锁定（分类器与 downloadZip 一致）
func (d *archiveChunkDownloader) lockMessage(resp *http.Response) (string, bool) {
	body := ""
	if data, err := io.ReadAll(io.LimitReader(resp.Body, 512)); err == nil {
		body = strings.ToLower(string(data))
	}
	reason := classifyArchiveLockBody(body)
	if reason == archiveLockNone {
		return "", false
	}
	d.g.mu.Lock()
	d.g.lockFailed = true
	d.g.lockReason = reason
	d.g.mu.Unlock()
	return archiveLockErrorMessage(resp.StatusCode, reason), true
}

// ─────────────────────────────────────────────────────────────
// 探测
// ─────────────────────────────────────────────────────────────

// probeArchiveDownload 探测归档下载链接：是否支持 Range、文件总大小。
// 采用「GET + 小 Range（bytes=0-1023）」实测（对齐 JHentai 多 Isolate Range 分片可行）：
//   - 之前用单字节 Range（bytes=0-0）对 H@H 下载页直链返回 404 而误判「不支持分块」→ 单线程
//   - 改为 1KB 小 Range 后，H@H 直链返回 206 即启用分块多线程；仍 404 则容错回退单线程
//   - 206 由 Content-Range 同时取得总大小，一次请求完成探测
//
// 返回 (total, rangeOK, err)。
func (g *archiveDownloader) probeArchiveDownload(downloadURL string) (int64, bool, error) {
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return 0, false, err
	}
	req.Header.Set("User-Agent", ehReaderUserAgent)
	req.Header.Set("Referer", g.referer)
	req.Header.Set("Range", "bytes=0-1023")

	resp, err := g.downloadClient().Do(req)
	if err != nil {
		return 0, false, fmt.Errorf("探测归档下载失败: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	switch resp.StatusCode {
	case http.StatusPartialContent:
		total := parseContentRangeTotal(resp.Header.Get("Content-Range"))
		if total <= 0 {
			// 206 但无法从 Content-Range 解析总大小（异常服务器）→ 视为不支持分块，走单线程
			log.Printf("%s [archive-engine] 任务 %s 探测：206 但无总大小，走单线程下载", dlWarnTag, g.task.ID)
			return 0, false, nil
		}
		log.Printf("%s [archive-engine] 任务 %s 探测：支持 Range，总大小=%.2f MiB",
			dlLogTag, g.task.ID, float64(total)/1048576)
		return total, true, nil

	case http.StatusOK:
		// 服务器忽略 Range 返回整个文件 → 不支持分块，走单线程
		log.Printf("%s [archive-engine] 任务 %s 探测：服务器忽略 Range（200），走单线程下载", dlWarnTag, g.task.ID)
		return resp.ContentLength, false, nil

	case http.StatusRequestedRangeNotSatisfiable:
		// 416：.part 已下载完整 → 交给单线程逻辑校验 rename
		log.Printf("%s [archive-engine] 任务 %s 探测返回 416，视为文件已下载完整", dlWarnTag, g.task.ID)
		return 0, false, nil

	case http.StatusNotFound:
		// H@H 下载页直链等节点可能拒绝 Range 探测请求（404）：
		// 视为不支持分块，回退单线程下载（downloadZip 在无 .part 时不带 Range 请求）。
		body := ""
		if data, err := io.ReadAll(io.LimitReader(resp.Body, 512)); err == nil {
			body = strings.ToLower(string(data))
		}
		if err := g.probeLockError(http.StatusNotFound, body); err != nil {
			return 0, false, err
		}
		log.Printf("%s [archive-engine] 任务 %s 探测返回 404（节点拒绝 Range 探测），走单线程下载", dlWarnTag, g.task.ID)
		return 0, false, nil

	default:
		body := ""
		if data, err := io.ReadAll(io.LimitReader(resp.Body, 512)); err == nil {
			body = strings.ToLower(string(data))
		}
		if err := g.probeLockError(resp.StatusCode, body); err != nil {
			return 0, false, err
		}
		return 0, false, fmt.Errorf("探测归档下载失败 HTTP %d", resp.StatusCode)
	}
}

// probeLockError 若探测响应体命中配额/限流提示，则置锁定标记并返回对应错误；否则返回 nil。
func (g *archiveDownloader) probeLockError(status int, body string) error {
	reason := classifyArchiveLockBody(body)
	if reason == archiveLockNone {
		return nil
	}
	g.mu.Lock()
	g.lockFailed = true
	g.lockReason = reason
	g.mu.Unlock()
	return errors.New(archiveLockErrorMessage(status, reason))
}

// parseContentRangeTotal 从 "bytes 0-0/12345" 或 "bytes */12345" 解析总大小
func parseContentRangeTotal(s string) int64 {
	idx := strings.LastIndexByte(s, '/')
	if idx < 0 {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s[idx+1:]), 10, 64)
	if err != nil {
		return 0
	}
	return n
}
