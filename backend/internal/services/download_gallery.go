package services

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"SakuHentai/internal/models"
)

// ─────────────────────────────────────────────────────────────
// 画廊下载引擎（计划第 2 步）
//
// 流程：gdata 拉取每页原图 URL → 按「同时下载图片数量」并发逐图下载
//       → 断点续传（目标存在跳过 / .part + Range 续传）→ 写 metadata + ComicInfo.xml
//
// 落地目录：extractPath / {gid} - {清理后的本子名}/
// ─────────────────────────────────────────────────────────────

// invalidFilenameChars 文件名字符清理（Windows 非法字符 + 控制字符）
var invalidFilenameChars = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]`)

// galleryDownloader 单次画廊下载的执行上下文
type galleryDownloader struct {
	m         *DownloadManager
	task      *models.DownloadTask
	account   *models.AccountSetting
	setting   *models.DownloadSetting
	ehSetting *models.EHSetting
	client    *http.Client

	destDir string // 落地目录（extractPath/gid - 本子名）
	referer string // 图片下载 Referer（图床校验用）
	limiter *rateLimiter

	mu         sync.Mutex // 保护 task 进度字段与并发计数
	startedAt  time.Time  // 开始时间（速度计算基准）
	lockFailed bool       // 是否检测到配额/限流（决定 error_lock 还是 error）
}

// runGalleryEngine 画廊下载引擎入口（由任务管理器分派）
func (m *DownloadManager) runGalleryEngine(task *models.DownloadTask) {
	g := &galleryDownloader{m: m, task: task}
	g.run()
}

// run 执行完整画廊下载流程
func (g *galleryDownloader) run() {
	log.Printf("%s [gallery-engine] 任务 %s 开始画廊下载 gid=%s title=%q", dlLogTag, g.task.ID, g.task.GID, g.task.Title)

	// 1. 账号 / 设置 / 客户端
	var account models.AccountSetting
	if err := g.m.db.First(&account, 1).Error; err != nil || account.IPBMemberID == "" {
		g.fail("未绑定 E 站账号凭证，无法下载")
		return
	}
	g.account = &account
	g.setting = g.m.GetSettings()
	g.ehSetting = loadEHSetting(g.m.db)

	client, err := g.m.ehService.BuildClient(&account)
	if err != nil {
		g.fail("构建下载客户端失败: " + err.Error())
		return
	}
	g.client = client
	g.referer = GetBaseURL(&account, g.ehSetting)
	g.limiter = newRateLimiter(g.setting.SpeedLimitImages, g.setting.SpeedLimitInterval)

	limitDesc := "不限速"
	if g.limiter != nil {
		limitDesc = fmt.Sprintf("%d MiB/%s", g.setting.SpeedLimitImages, g.setting.SpeedLimitInterval)
	}
	log.Printf("%s [gallery-engine] 任务 %s 客户端就绪 referer=%s 并发图片=%d 限速=%s",
		dlLogTag, g.task.ID, g.referer, g.setting.ConcurrentImageDownloads, limitDesc)

	// 2. 落地目录（gid - 本子名）
	dirName := fmt.Sprintf("%s - %s", g.task.GID, cleanFolderName(g.task.Title))
	g.destDir = filepath.Join(g.task.ExtractPath, dirName)
	if err := os.MkdirAll(g.destDir, 0o755); err != nil {
		g.fail("创建落地目录失败: " + err.Error())
		return
	}
	log.Printf("%s [gallery-engine] 任务 %s 落地目录 %q", dlLogTag, g.task.ID, g.destDir)

	// 3. 页面 URL 列表（gdata 主方案 + 逐页兜底）
	pages, err := g.m.ehService.FetchOnlinePageUrls(g.account, g.task.GID, g.task.Token, g.ehSetting)
	if err != nil {
		g.fail("获取画廊页面列表失败: " + err.Error())
		return
	}
	if len(pages.URLs) == 0 {
		g.fail("获取到的页面列表为空")
		return
	}

	// 4. 详情元数据（失败仅告警，图片仍可下载，metadata 写入基础字段）
	var detail *GalleryDetailResult
	if d, err := g.m.ehService.FetchGalleryDetail(g.account, g.task.GID, g.task.Token, g.ehSetting); err == nil {
		detail = d
		log.Printf("%s [gallery-engine] 任务 %s 详情抓取成功 title=%q pageCount=%d tags=%d parentGID=%s",
			dlLogTag, g.task.ID, d.Title, d.PageCount, len(d.Tags), d.ParentGID)
	} else {
		log.Printf("%s [gallery-engine] 任务 %s 详情抓取失败（将写入基础 metadata）: %v", dlWarnTag, g.task.ID, err)
	}

	// 5. 更新任务总量
	g.mu.Lock()
	g.task.TotalFiles = len(pages.URLs)
	g.task.Error = ""
	g.mu.Unlock()

	// 6. 并发下载
	g.startedAt = time.Now()
	g.downloadAll(pages.URLs, detail)
}

// downloadAll 按并发度下载全部页面，汇总结果后收尾
func (g *galleryDownloader) downloadAll(urls []string, detail *GalleryDetailResult) {
	concurrency := g.setting.ConcurrentImageDownloads
	if concurrency <= 0 {
		concurrency = 1
	}

	// 文件名位数：<100 用 3 位，<1000 用 4 位，否则 5 位
	width := 3
	if len(urls) >= 1000 {
		width = 4
	}
	if len(urls) >= 10000 {
		width = 5
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var failMu sync.Mutex
	failed := make([]int, 0)

	for i, u := range urls {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, imageURL string) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := g.downloadOne(idx, imageURL, width); err != nil {
				failMu.Lock()
				failed = append(failed, idx+1)
				failMu.Unlock()
				log.Printf("%s [gallery-engine] 任务 %s 第 %d 页下载失败: %v", dlWarnTag, g.task.ID, idx+1, err)
			}
		}(i, u)
	}
	wg.Wait()

	// 任务在下载过程中被取消/暂停：中止收尾，避免用 error/completed 覆盖用户状态
	if g.stopped() {
		log.Printf("%s [gallery-engine] 任务 %s 已被取消/暂停，中止收尾（保留用户状态）", dlWarnTag, g.task.ID)
		return
	}

	success := len(urls) - len(failed)
	log.Printf("%s [gallery-engine] 任务 %s 下载结束：成功 %d/%d 张，失败 %d 张，总字节 %d",
		dlLogTag, g.task.ID, success, len(urls), len(failed), g.task.DoneBytes)

	g.mu.Lock()
	lockFailed := g.lockFailed
	g.mu.Unlock()

	if len(failed) > 0 {
		msg := fmt.Sprintf("有 %d 张图片下载失败（已完成 %d 张已断点保存，可重试跳过）", len(failed), success)
		if lockFailed {
			g.failLocked(msg + "；疑似 E 站配额/限流，请在 E 站处理配额后点击解锁重试")
		} else {
			g.fail(msg)
		}
		return
	}

	// 全部成功 → 写 metadata + ComicInfo.xml → completed
	if err := g.writeMetadata(detail); err != nil {
		log.Printf("%s [gallery-engine] 任务 %s 写入 metadata/ComicInfo.xml 失败（不影响已下载图片）: %v", dlWarnTag, g.task.ID, err)
	}

	// 扫描入库：让下载完成后的文件夹出现在离线书架
	if count, err := ScanAndSaveDirectory(g.destDir, false); err == nil {
		log.Printf("%s [gallery-engine] 任务 %s 下载目录已扫描入库 %d 个", dlLogTag, g.task.ID, count)
	} else {
		log.Printf("%s [gallery-engine] 任务 %s 下载目录扫描入库失败: %v", dlWarnTag, g.task.ID, err)
	}

	g.mu.Lock()
	g.task.Status = models.DownloadCompleted
	g.task.Error = ""
	g.task.UpdatedAt = time.Now()
	if err := g.m.db.Save(g.task).Error; err != nil {
		log.Printf("%s [gallery-engine] 任务 %s 完成状态保存失败: %v", dlErrTag, g.task.ID, err)
	}
	g.mu.Unlock()

	log.Printf("%s [gallery-engine] 任务 %s 画廊下载完成：共 %d 张 %.2f MiB，落地 %q",
		dlLogTag, g.task.ID, g.task.TotalFiles, float64(g.task.DoneBytes)/1024/1024, g.destDir)
}

// downloadOne 下载单页图片（断点续传 + 瞬态错误指数退避重试）
func (g *galleryDownloader) downloadOne(idx int, imageURL string, width int) error {
	ext := imageExt(imageURL)
	filename := fmt.Sprintf("%0*d%s", width, idx+1, ext)
	destPath := filepath.Join(g.destDir, filename)
	partPath := destPath + ".part"

	// 断点续传 1：目标文件已存在且非空 → 跳过
	if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
		g.recordDone(fi.Size())
		log.Printf("%s [gallery-engine] 任务 %s 第 %d 页已存在，跳过（断点续传）: %s", dlLogTag, g.task.ID, idx+1, filename)
		return nil
	}

	// 瞬态错误（网络错误 / 429 / 5xx / 非限流 403）指数退避重试；
	// 配额/限流锁定（error_lock）不重试，避免在配额不足时继续消耗资源。
	const maxRetries = 3
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if g.stopped() {
			return errTaskStopped
		}
		if attempt > 0 {
			if g.locked() {
				return lastErr
			}
			delay := 2 * time.Second * time.Duration(1<<(attempt-1))
			log.Printf("%s [gallery-engine] 任务 %s 第 %d 页下载失败（第 %d/%d 次），%.0fs 后退避重试: %v",
				dlWarnTag, g.task.ID, idx+1, attempt, maxRetries, delay.Seconds(), lastErr)
			time.Sleep(delay)
		}
		lastErr = g.downloadOneOnce(idx, imageURL, width, destPath, partPath, filename)
		if lastErr == nil {
			return nil
		}
		if g.locked() {
			return lastErr
		}
		if g.stopped() {
			return errTaskStopped
		}
	}
	return lastErr
}

// locked 是否已被判定为配额/限流锁定
func (g *galleryDownloader) locked() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.lockFailed
}

// stopped 任务是否已被取消/暂停（回查 DB，引擎内存状态不会更新）
func (g *galleryDownloader) stopped() bool {
	return g.m.taskStopped(g.task.ID)
}

// downloadOneOnce 单次下载单页图片（断点续传：目标存在跳过 / .part + Range 续传）
func (g *galleryDownloader) downloadOneOnce(idx int, imageURL string, width int, destPath, partPath, filename string) error {
	// 断点续传 2：.part 已存在则从偏移继续（Range 请求）
	var startOffset int64
	if fi, err := os.Stat(partPath); err == nil {
		startOffset = fi.Size()
	}

	req, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", ehReaderUserAgent)
	req.Header.Set("Referer", g.referer)
	if startOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
		log.Printf("%s [gallery-engine] 任务 %s 第 %d 页从 %d 字节续传", dlLogTag, g.task.ID, idx+1, startOffset)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// 服务器忽略 Range：从头下载，清掉旧 .part
		if startOffset > 0 {
			_ = os.Remove(partPath)
			startOffset = 0
		}
	case http.StatusPartialContent:
		// 206：正常续传
	case http.StatusRequestedRangeNotSatisfiable:
		// 416：.part 已是完整文件 → 直接 rename 落盘
		if err := os.Rename(partPath, destPath); err != nil {
			return err
		}
		if fi, err := os.Stat(destPath); err == nil {
			g.recordDone(fi.Size())
		}
		return nil
	default:
		return g.classifyHTTPError(resp)
	}

	f, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()

	buf := make([]byte, 64*1024)
	written := int64(0)
	readChunk := 0
	for {
		readChunk++
		if readChunk%8 == 0 && g.stopped() {
			return errTaskStopped
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			g.limiter.wait(int64(n))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}

	// 落盘：sync + close（Windows 需先关闭才能 rename）→ rename 为目标文件
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	closed = true

	if err := os.Rename(partPath, destPath); err != nil {
		return err
	}

	var size int64
	if fi, err := os.Stat(destPath); err == nil {
		size = fi.Size()
	}
	g.recordDone(size)
	log.Printf("%s [gallery-engine] 任务 %s 第 %d 页下载完成 %s (%.1f KiB)", dlLogTag, g.task.ID, idx+1, filename, float64(size)/1024)
	return nil
}

// classifyHTTPError 根据响应状态码与页面内容判断是普通错误还是配额/限流（error_lock）
func (g *galleryDownloader) classifyHTTPError(resp *http.Response) error {
	body := ""
	if resp.Body != nil {
		if data, err := io.ReadAll(io.LimitReader(resp.Body, 512)); err == nil {
			body = string(data)
		}
	}
	text := strings.ToLower(body)

	lockHints := []string{"banned", "quota", "rate limit", "exceeded", "sadpanda", "too many", "temporarily", "panda"}
	for _, h := range lockHints {
		if strings.Contains(text, h) {
			g.mu.Lock()
			g.lockFailed = true
			g.mu.Unlock()
			log.Printf("%s [gallery-engine] 任务 %s 检测到疑似配额/限流响应(HTTP %d)，将走 error_lock: 片段=%q",
				dlWarnTag, g.task.ID, resp.StatusCode, truncateForLog(body, 120))
			return fmt.Errorf("HTTP %d（疑似配额/限流，需解锁后重试）", resp.StatusCode)
		}
	}
	log.Printf("%s [gallery-engine] 任务 %s HTTP %d 下载失败: 片段=%q", dlWarnTag, g.task.ID, resp.StatusCode, truncateForLog(body, 120))
	return fmt.Errorf("HTTP %d", resp.StatusCode)
}

// recordDone 记录单张图片完成进度（内存更新 + 每 5 张写库一次 + 速度）
func (g *galleryDownloader) recordDone(size int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.task.DoneFiles++
	g.task.DoneBytes += size
	if g.task.DoneFiles%5 == 0 {
		g.persistLocked()
	}
}

// persistLocked 将进度与实时速度写库（调用方需持有 g.mu）
// 只更新进度字段、不改状态列，并限定 status ∈ (queued, downloading)，
// 避免在用户取消/暂停后仍用内存里的 downloading 状态覆盖。
func (g *galleryDownloader) persistLocked() {
	elapsed := time.Since(g.startedAt).Seconds()
	if elapsed > 0 {
		g.task.Speed = float64(g.task.DoneBytes) / elapsed
	}
	g.task.UpdatedAt = time.Now()
	if err := g.m.db.Model(&models.DownloadTask{}).
		Where("id = ? AND status IN ?", g.task.ID, []string{
			string(models.DownloadQueued), string(models.DownloadDownloading),
		}).
		Updates(map[string]interface{}{
			"done_files": g.task.DoneFiles,
			"done_bytes": g.task.DoneBytes,
			"speed":      g.task.Speed,
			"updated_at": g.task.UpdatedAt,
		}).Error; err != nil {
		log.Printf("%s 任务 %s 进度保存失败: %v", dlErrTag, g.task.ID, err)
	}
}

// fail 将任务置为 error（若任务已被取消/暂停则不再写终态）
func (g *galleryDownloader) fail(msg string) {
	if g.stopped() {
		log.Printf("%s [gallery-engine] 任务 %s 已被取消/暂停，跳过失败收尾", dlWarnTag, g.task.ID)
		return
	}
	g.m.failTask(g.task, msg)
}

// failLocked 将任务置为 error_lock（配额/限流）
func (g *galleryDownloader) failLocked(msg string) {
	if g.stopped() {
		log.Printf("%s [gallery-engine] 任务 %s 已被取消/暂停，跳过锁定收尾", dlWarnTag, g.task.ID)
		return
	}
	log.Printf("%s [gallery-engine] 任务 %s 因配额/限流进入锁定: %s", dlErrTag, g.task.ID, msg)
	g.mu.Lock()
	g.task.Status = models.DownloadErrorLock
	g.task.Error = msg
	g.task.UpdatedAt = time.Now()
	if err := g.m.db.Save(g.task).Error; err != nil {
		log.Printf("%s 任务 %s 保存 error_lock 状态失败: %v", dlErrTag, g.task.ID, err)
	}
	g.mu.Unlock()
}

// writeMetadata 写入 metadata / ametadata（JSON）+ ComicInfo.xml 到落地目录。
// 完整元数据字段（标题/作者/分类/标签/评分等）参考 JHentai EHGalleryComicInfo 映射，
// 由 buildFullComicInfo 统一构建；detail 可为 nil（回退任务字段最小集）。
func (g *galleryDownloader) writeMetadata(detail *GalleryDetailResult) error {
	galleryURL := ""
	if detail != nil {
		galleryURL = GetGalleryURL(g.account, g.ehSetting, detail.ID, detail.Token)
	} else if g.task.GID != "" && g.task.Token != "" {
		galleryURL = GetGalleryURL(g.account, g.ehSetting, g.task.GID, g.task.Token)
	}

	xmlMeta, meta := buildFullComicInfo(g.task, detail, galleryURL)

	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(g.destDir, "metadata"), metaData, 0o644); err != nil {
		return err
	}
	// JHentai 约定：ametadata 为内部专有备份（双写一份，兼容第三方工具/扫描器）
	if err := os.WriteFile(filepath.Join(g.destDir, "ametadata"), metaData, 0o644); err != nil {
		return err
	}

	// ComicInfo.xml
	xmlData, err := xml.MarshalIndent(xmlMeta, "", "  ")
	if err != nil {
		return err
	}
	xmlFull := append([]byte(xml.Header), xmlData...)
	if err := os.WriteFile(filepath.Join(g.destDir, "ComicInfo.xml"), xmlFull, 0o644); err != nil {
		return err
	}

	log.Printf("%s [gallery-engine] 任务 %s 已写入 metadata + ametadata + ComicInfo.xml（title=%q category=%q tags=%d）",
		dlLogTag, g.task.ID, xmlMeta.Title, meta.Category, len(meta.Tags))
	return nil
}

// ─────────────────────────────────────────────────────────────
// 工具函数
// ─────────────────────────────────────────────────────────────

// cleanFolderName 清理文件夹名非法字符、首尾/连续空格，超长截断
func cleanFolderName(name string) string {
	name = strings.TrimSpace(name)
	name = invalidFilenameChars.ReplaceAllString(name, "_")
	name = strings.Join(strings.Fields(name), " ")
	runes := []rune(name)
	if len(runes) > 120 {
		runes = runes[:120]
		name = string(runes)
	}
	if name == "" {
		name = "untitled"
	}
	return name
}

// imageExt 从原图 URL 提取图片扩展名（去掉 query/fragment），非图片扩展名默认 .jpg
func imageExt(rawURL string) string {
	u := rawURL
	if idx := strings.IndexByte(u, '?'); idx >= 0 {
		u = u[:idx]
	}
	if idx := strings.IndexByte(u, '#'); idx >= 0 {
		u = u[:idx]
	}
	ext := strings.ToLower(filepath.Ext(u))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".avif":
		return ext
	default:
		return ".jpg"
	}
}

// rateLimiter 简单窗口限速：每 interval 最多下载 limit 字节
type rateLimiter struct {
	mu          sync.Mutex
	limit       int64
	interval    time.Duration
	windowStart time.Time
	used        int64
}

// newRateLimiter 构造限速器；limitPerInterval<=0 时返回 nil（不限速）
// 语义：speedLimitImages 单位 MiB/interval（30/50/99 MiB 配 1s/2s/5s）
func newRateLimiter(limitPerInterval int, intervalStr string) *rateLimiter {
	if limitPerInterval <= 0 {
		return nil
	}
	interval := 1 * time.Second
	switch intervalStr {
	case "2s":
		interval = 2 * time.Second
	case "5s":
		interval = 5 * time.Second
	}
	return &rateLimiter{
		limit:       int64(limitPerInterval) * 1024 * 1024,
		interval:    interval,
		windowStart: time.Now(),
	}
}

// wait 累计 n 字节，超出窗口限制则睡眠至窗口结束
func (r *rateLimiter) wait(n int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if now.Sub(r.windowStart) >= r.interval {
		r.windowStart = now
		r.used = 0
	}
	r.used += n
	if r.used > r.limit {
		wait := r.interval - now.Sub(r.windowStart)
		if wait > 0 {
			time.Sleep(wait)
		}
		r.windowStart = time.Now()
		r.used = n
	}
}
