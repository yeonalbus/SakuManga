package services

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"SakuHentai/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// 归档下载并发控制测试
// 覆盖：archiveThreadPool 配额池、archiveChunkDownloader 分块并发下载、
//       断点续传、探测 Range、单线程回退。
// ─────────────────────────────────────────────────────────────

// makeTestZip 生成接近 size 字节的真实 zip 数据（用于校验下载结果）。
// 内容使用伪随机字节（难压缩），保证 zip 压缩后仍接近 size，
// 从而让分块下载/断点续传测试拥有真实的多块意义。
func makeTestZip(t *testing.T, size int) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fw, err := zw.Create("test.txt")
	if err != nil {
		t.Fatalf("创建 zip 条目失败: %v", err)
	}
	payload := make([]byte, size)
	rng := rand.New(rand.NewSource(42))
	if _, err := rng.Read(payload); err != nil {
		t.Fatalf("生成随机内容失败: %v", err)
	}
	if _, err := fw.Write(payload); err != nil {
		t.Fatalf("写入 zip 内容失败: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
	return buf.Bytes()
}

// newRangeServer 返回支持/不支持 Range 的测试文件服务器
func newRangeServer(data []byte, supportRange bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		if !supportRange {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
		rng := r.Header.Get("Range")
		if rng == "" {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
		var start, end int64
		if _, err := fmt.Sscanf(rng, "bytes=%d-%d", &start, &end); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if end <= 0 || end >= int64(len(data)) {
			end = int64(len(data)) - 1
		}
		if start >= int64(len(data)) {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(data)))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data[start : end+1])
	}))
}

// newTestDownloadManager 构造带内存 sqlite 的 DownloadManager（仅用于下载引擎测试）
func newTestDownloadManager(t *testing.T) *DownloadManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	// 内存库需固定为单连接：glebarez/sqlite 的 :memory: 是「每连接独立」，
	// 若走连接池，迁移建的表现在其他连接上不可见（no such table）。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.DownloadSetting{}, &models.DownloadTask{}); err != nil {
		t.Fatalf("迁移模型失败: %v", err)
	}
	return NewDownloadManager(db, nil)
}

// newTestArchiveDownloader 构造归档下载器（client 指向测试服务器）
func newTestArchiveDownloader(t *testing.T, mgr *DownloadManager, srvURL, dir string) *archiveDownloader {
	t.Helper()
	return &archiveDownloader{
		m:         mgr,
		client:    &http.Client{},
		referer:   srvURL + "/",
		partPath:  filepath.Join(dir, "test.part"),
		zipPath:   filepath.Join(dir, "test.zip"),
		task:      &models.DownloadTask{ID: "test-task"},
		startedAt: time.Now(),
	}
}

// ─────────────────────────────────────────────────────────────
// archiveThreadPool 配额池
// ─────────────────────────────────────────────────────────────

func TestArchiveThreadPoolAcquireRelease(t *testing.T) {
	p := newArchiveThreadPool(10)

	if got := p.acquire("a", 4, nil); got != 4 {
		t.Fatalf("a 应获取 4 个线程，得到 %d", got)
	}
	if got := p.acquire("b", 4, nil); got != 4 {
		t.Fatalf("b 应获取 4 个线程，得到 %d", got)
	}

	// 第三个任务需要 4 个（4+4+4=12 > 10）→ 必须排队等待
	done := make(chan int, 1)
	go func() { done <- p.acquire("c", 4, nil) }()
	select {
	case <-done:
		t.Fatal("c 不应立即获取到配额（全局余量不足）")
	case <-time.After(100 * time.Millisecond):
	}

	// 释放 a 后 c 应被唤醒并拿到 4 个线程
	p.releaseAll("a")
	select {
	case got := <-done:
		if got != 4 {
			t.Fatalf("c 应获取 4 个线程，得到 %d", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("释放后 c 未被唤醒")
	}

	// 全部释放后应无持有
	p.releaseAll("b")
	p.releaseAll("c")
	if p.acquired("a") != 0 || p.acquired("b") != 0 || p.acquired("c") != 0 {
		t.Fatal("释放后不应仍有线程持有")
	}
}

func TestArchiveThreadPoolAdjust(t *testing.T) {
	p := newArchiveThreadPool(10)
	p.acquire("a", 6, nil)

	// 调小：立即生效
	if got := p.adjust("a", 3); got != 3 {
		t.Fatalf("调小后应持有 3，得到 %d", got)
	}
	// 调大：受全局余量限制（active=3, max=10 → 可到 8）
	if got := p.adjust("a", 8); got != 8 {
		t.Fatalf("调大后应持有 8，得到 %d", got)
	}
	// 调大超过余量：尽力而为
	if got := p.adjust("a", 10); got != 10 {
		t.Fatalf("调大到上限应持有 10，得到 %d", got)
	}
	// 再调大超过全局上限：只能维持 10
	if got := p.adjust("a", 12); got != 10 {
		t.Fatalf("调大超上限应保持 10，得到 %d", got)
	}
	p.releaseAll("a")
	if p.acquired("a") != 0 {
		t.Fatal("释放后应无持有")
	}
}

func TestArchiveThreadPoolStopCallback(t *testing.T) {
	p := newArchiveThreadPool(10)
	p.acquire("a", 10, nil)

	stopped := false
	done := make(chan int, 1)
	go func() { done <- p.acquire("b", 5, func() bool { return stopped }) }()

	select {
	case <-done:
		t.Fatal("b 不应在 a 未释放时获取到配额")
	case <-time.After(100 * time.Millisecond):
	}

	// 置位停止标记并唤醒 → b 应放弃等待返回 0
	stopped = true
	p.wakeAll()
	select {
	case got := <-done:
		if got != 0 {
			t.Fatalf("停止回调应返回 0，得到 %d", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("唤醒后 b 未返回")
	}
}

// ─────────────────────────────────────────────────────────────
// archiveChunkDownloader 分块并发下载
// ─────────────────────────────────────────────────────────────

func TestArchiveChunkDownloaderMultiThread(t *testing.T) {
	size := 8 * 1024 * 1024 // 8 MiB
	data := makeTestZip(t, size)
	srv := newRangeServer(data, true)
	defer srv.Close()

	mgr := newTestDownloadManager(t)
	g := newTestArchiveDownloader(t, mgr, srv.URL, t.TempDir())

	// 探测：应支持 Range 且总大小正确
	total, rangeOK, err := g.probeArchiveDownload(srv.URL + "/file.zip")
	if err != nil {
		t.Fatalf("探测失败: %v", err)
	}
	if !rangeOK {
		t.Fatal("期望服务器支持 Range")
	}
	if total != int64(len(data)) {
		t.Fatalf("探测 total=%d，期望 %d", total, len(data))
	}

	// 5 线程分块并发下载
	if err := g.runChunkDownload(srv.URL+"/file.zip", total, 5); err != nil {
		t.Fatalf("分块并发下载失败: %v", err)
	}

	got, err := os.ReadFile(g.zipPath)
	if err != nil {
		t.Fatalf("读取 zip 失败: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("下载内容与源不一致：got=%d 字节，want=%d 字节", len(got), len(data))
	}
}

func TestArchiveChunkDownloaderResume(t *testing.T) {
	size := 8 * 1024 * 1024 // 8 MiB
	data := makeTestZip(t, size)
	srv := newRangeServer(data, true)
	defer srv.Close()

	mgr := newTestDownloadManager(t)
	dir := t.TempDir()
	g := newTestArchiveDownloader(t, mgr, srv.URL, dir)

	// 预先写入前 3 MiB（连续前缀），模拟暂停后的断点
	const prefixBytes = 3 * 1024 * 1024
	if err := os.WriteFile(g.partPath, data[:prefixBytes], 0o644); err != nil {
		t.Fatalf("预写 .part 失败: %v", err)
	}

	if err := g.runChunkDownload(srv.URL+"/file.zip", int64(len(data)), 5); err != nil {
		t.Fatalf("断点续传失败: %v", err)
	}

	got, err := os.ReadFile(g.zipPath)
	if err != nil {
		t.Fatalf("读取 zip 失败: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("续传结果与源不一致：got=%d 字节，want=%d 字节", len(got), len(data))
	}
}

// TestArchiveChunkDownloaderEOFBitmapResume 验证 EOF/暂停后「伴生位图」断点续传：
// 预写非前缀高位块（块 1、3）并持久化位图，重启下载应复用已完成块、不重复请求，
// 且完成后 .bits 被清理。这是修复 .part 被截断为 0B 无法续传问题的回归测试。
func TestArchiveChunkDownloaderEOFBitmapResume(t *testing.T) {
	size := 8 * 1024 * 1024 // 8 MiB
	data := makeTestZip(t, size)
	total := int64(len(data))

	// 跟踪每个 Range 请求的 start（检测已完成块是否被重复请求）
	var mu sync.Mutex
	reqStarts := make(map[int64]int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rng := r.Header.Get("Range")
		if rng == "" {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", total))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
		var start, end int64
		if _, err := fmt.Sscanf(rng, "bytes=%d-%d", &start, &end); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if end <= 0 || end >= total {
			end = total - 1
		}
		if start >= total {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", total))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		mu.Lock()
		reqStarts[start]++
		mu.Unlock()
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data[start : end+1])
	}))
	defer srv.Close()

	mgr := newTestDownloadManager(t)
	g := newTestArchiveDownloader(t, mgr, srv.URL, t.TempDir())

	// 以与生产一致的分块参数取 chunk/count（块大小/块数由设置线程数决定）
	d := newArchiveChunkDownloader(g, srv.URL+"/file.zip", total)
	chunk, count := d.chunk, d.count
	d.cancel()
	if count < 4 {
		t.Fatalf("测试数据块数不足（count=%d），无法构造高位块场景", count)
	}

	// 预写块 1、3（跳过前缀块 0 与块 2），模拟 EOF/暂停后保留的高位完成块
	done := []int64{1, 3}
	f, err := os.OpenFile(g.partPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("打开 .part 失败: %v", err)
	}
	for _, idx := range done {
		lo := idx * chunk
		hi := lo + chunk
		if hi > total {
			hi = total
		}
		if _, werr := f.WriteAt(data[lo:hi], lo); werr != nil {
			_ = f.Close()
			t.Fatalf("预写块 %d 失败: %v", idx, werr)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("关闭 .part 失败: %v", err)
	}
	// 持久化位图标记块 1、3 完成
	bits := make([]uint64, (count+63)/64)
	for _, idx := range done {
		bits[idx/64] |= 1 << (uint(idx) % 64)
	}
	if err := writeArchiveBitmap(g.partPath, chunk, count, bits); err != nil {
		t.Fatalf("写位图失败: %v", err)
	}

	// 执行分块并发下载：应按位图续传，跳过已完成块 1、3
	if err := g.runChunkDownload(srv.URL+"/file.zip", total, 5); err != nil {
		t.Fatalf("位图断点续传失败: %v", err)
	}

	// 1) 最终文件与源一致
	got, err := os.ReadFile(g.zipPath)
	if err != nil {
		t.Fatalf("读取 zip 失败: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("续传结果与源不一致：got=%d 字节，want=%d 字节", len(got), len(data))
	}

	// 2) 已完成块 1、3 不应被重新请求（位图复用生效）
	mu.Lock()
	defer mu.Unlock()
	for _, idx := range done {
		start := idx * chunk
		if n := reqStarts[start]; n != 0 {
			t.Fatalf("块 %d（start=%d）应被位图复用跳过，但被重新请求 %d 次", idx, start, n)
		}
	}

	// 3) 下载完成后伴生位图应被清理
	if _, err := os.Stat(archiveBitmapPath(g.partPath)); err == nil {
		t.Fatal("下载完成后伴生位图 .bits 应被清理")
	}
}

func TestArchiveChunkDownloaderSingleThreadFallback(t *testing.T) {
	size := 8 * 1024 * 1024 // 8 MiB
	data := makeTestZip(t, size)
	srv := newRangeServer(data, false) // 服务器忽略 Range（返回 200）
	defer srv.Close()

	mgr := newTestDownloadManager(t)
	g := newTestArchiveDownloader(t, mgr, srv.URL, t.TempDir())

	// 探测：不支持 Range → rangeOK=false
	total, rangeOK, err := g.probeArchiveDownload(srv.URL + "/file.zip")
	if err != nil {
		t.Fatalf("探测失败: %v", err)
	}
	if rangeOK {
		t.Fatal("服务器忽略 Range，探测应返回 rangeOK=false")
	}
	if total != int64(len(data)) {
		t.Fatalf("探测 total=%d，期望 %d", total, len(data))
	}

	// 入口应回退到单线程 downloadZip
	if err := g.downloadArchiveFile(srv.URL + "/file.zip"); err != nil {
		t.Fatalf("单线程回退下载失败: %v", err)
	}

	got, err := os.ReadFile(g.zipPath)
	if err != nil {
		t.Fatalf("读取 zip 失败: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("单线程回退结果与源不一致：got=%d 字节，want=%d 字节", len(got), len(data))
	}
}

// TestProbeArchiveDownload404Fallback 模拟 H@H 节点：带 Range 的请求返回 404。
// 探测应回退为不支持 Range（rangeOK=false），入口走单线程下载仍可成功。
func TestProbeArchiveDownload404Fallback(t *testing.T) {
	size := 8 * 1024 * 1024 // 8 MiB
	data := makeTestZip(t, size)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	mgr := newTestDownloadManager(t)
	g := newTestArchiveDownloader(t, mgr, srv.URL, t.TempDir())

	total, rangeOK, err := g.probeArchiveDownload(srv.URL + "/file.zip")
	if err != nil {
		t.Fatalf("探测 404 应回退而非报错: %v", err)
	}
	if rangeOK {
		t.Fatal("404 探测应返回 rangeOK=false")
	}
	if total != 0 {
		t.Fatalf("404 探测 total=%d，期望 0", total)
	}

	// 入口应回退到单线程 downloadZip（无 Range 请求），下载成功
	if err := g.downloadArchiveFile(srv.URL + "/file.zip"); err != nil {
		t.Fatalf("404 回退单线程下载失败: %v", err)
	}
	got, err := os.ReadFile(g.zipPath)
	if err != nil {
		t.Fatalf("读取 zip 失败: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("下载内容与源不一致：got=%d 字节，want=%d 字节", len(got), len(data))
	}
}

// TestIsHathStreamDownloadURL 验证 H@H「下载页直链」流式链接判定
func TestIsHathStreamDownloadURL(t *testing.T) {
	cases := map[string]bool{
		"https://encvgvvzml.hath.network/archive/4099258/abc/itbb/2?start=1": true,
		"https://encvgvvzml.hath.network/archive/4099258/abc/itbb/2":         false,
		"https://encvgvvzml.hath.network/archive/4099258/abc/itbb/2?x=1":     false,
		"https://e-hentai.org/archiver_download/1/2/3?start=1":               false,
		"https://node.hath.network/archive/1/2/3/4?start=2":                  true,
	}
	for in, want := range cases {
		if got := isHathStreamDownloadURL(in); got != want {
			t.Errorf("isHathStreamDownloadURL(%q)=%v，期望 %v", in, got, want)
		}
	}
}

// 验证 parseContentRangeTotal 解析
func TestParseContentRangeTotal(t *testing.T) {
	cases := map[string]int64{
		"bytes 0-0/12345":     12345,
		"bytes 0-1023/8388608": 8388608,
		"bytes */8388608":      8388608,
		"garbage":              0,
		"bytes 0-0/":           0,
	}
	for in, want := range cases {
		if got := parseContentRangeTotal(in); got != want {
			t.Errorf("parseContentRangeTotal(%q)=%d，期望 %d", in, got, want)
		}
	}
}

// TestClassifyArchiveLockBody 覆盖 JHentai _check410Or404Reason 的精确锁定文案分类：
//   - "This archive session has been used from too many different locations" → IP/会话封锁
//   - "IP quota exhausted" → IP/会话封锁
//   - "You have clocked too many downloaded bytes on this gallery" → IP/会话封锁
//   - "Expired or invalid session" → 会话过期（仅暂停）
func TestClassifyArchiveLockBody(t *testing.T) {
	cases := []struct {
		name string
		body string
		want archiveLockReason
	}{
		{"多个不同 IP 使用（原文）", "This archive session has been used from too many different locations.", archiveLockIPSession},
		{"多个不同 IP 使用（小写）", "this archive session has been used from too many different locations", archiveLockIPSession},
		{"IP 配额耗尽", "IP quota exhausted, wait until tomorrow.", archiveLockIPSession},
		{"本画廊下载字节超限", "You have clocked too many downloaded bytes on this gallery.", archiveLockIPSession},
		{"Session 过期/失效", "Expired or invalid session.", archiveLockSessionExpired},
		{"Session 过期（小写）", "expired or invalid session", archiveLockSessionExpired},
		{"通用配额提示", "quota exceeded, please try again later", archiveLockQuota},
		{"通用限流提示", "rate limit exceeded", archiveLockQuota},
		{"通用封锁提示", "sadpanda too many requests temporarily banned", archiveLockQuota},
		{"正常响应不锁定", "you have unlocked this gallery's archive", archiveLockNone},
		{"空响应不锁定", "", archiveLockNone},
	}
	for _, c := range cases {
		if got := classifyArchiveLockBody(c.body); got != c.want {
			t.Errorf("%s: classifyArchiveLockBody(%q)=%v，期望 %v", c.name, c.body, got, c.want)
		}
	}
}

// TestCancelArchiveSession 验证取消服务端归档 Session：POST invalidate_sessions=1
func TestCancelArchiveSession(t *testing.T) {
	var mu sync.Mutex
	var gotMethod, gotPath, gotBody, gotGID, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody = string(b)
		gotGID = r.URL.Query().Get("gid")
		gotToken = r.URL.Query().Get("token")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>archiver page</html>"))
	}))
	defer srv.Close()

	client := srv.Client()
	if err := cancelArchiveSession(client, srv.URL, "4099258", "abc123"); err != nil {
		t.Fatalf("cancelArchiveSession 失败: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != http.MethodPost {
		t.Errorf("请求方法=%s，期望 POST", gotMethod)
	}
	if gotPath != "/archiver.php" {
		t.Errorf("请求路径=%s，期望 /archiver.php", gotPath)
	}
	if gotGID != "4099258" || gotToken != "abc123" {
		t.Errorf("查询参数 gid=%q token=%q，期望 gid=4099258 token=abc123", gotGID, gotToken)
	}
	if !strings.Contains(gotBody, "invalidate_sessions=1") {
		t.Errorf("请求体=%q，应包含 invalidate_sessions=1", gotBody)
	}
}

// TestCancelArchiveSessionRetry 验证首次非 2xx/3xx 时自动重试成功
func TestCancelArchiveSessionRetry(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := srv.Client()
	if err := cancelArchiveSession(client, srv.URL, "1", "t"); err != nil {
		t.Fatalf("cancelArchiveSession 重试后仍失败: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls < 2 {
		t.Errorf("期望至少重试一次（实际 calls=%d）", calls)
	}
}

// ─────────────────────────────────────────────────────────────
// 全局额度池升级（5.2 归档并发窗口 + 每归档配额 + 画廊尽力获取）测试
// ─────────────────────────────────────────────────────────────

// TestArchiveEffConcurrency 验证实际允许的归档并发数上限
func TestArchiveEffConcurrency(t *testing.T) {
	cases := []struct {
		threads, maxConc, want int
	}{
		{10, 0, 1},
		{10, 1, 1},
		{10, 3, 3},
		{10, 6, 6},
		{10, 10, 10},
		{10, 15, 10}, // 上限不能超过线程数
		{6, 10, 6},
		{0, 2, 0}, // 边界：maxConc>=threads → threads
	}
	for _, c := range cases {
		if got := archiveEffConcurrency(c.threads, c.maxConc); got != c.want {
			t.Errorf("archiveEffConcurrency(%d,%d)=%d，期望 %d", c.threads, c.maxConc, got, c.want)
		}
	}
}

// TestArchiveQuota 验证 5.2 每归档额度分配规则
func TestArchiveQuota(t *testing.T) {
	// 单归档拿满全部线程
	if got := archiveQuota(10, 1, 0); got != 10 {
		t.Errorf("archiveQuota(10,1,0)=%d，期望 10", got)
	}
	// maxConc >= threads → 每归档 1 线程
	if got := archiveQuota(10, 10, 5); got != 1 {
		t.Errorf("archiveQuota(10,10,5)=%d，期望 1", got)
	}
	// threads=10, maxConc=6 → 前 4 个 2 线程、后 2 个 1 线程
	want := []int{2, 2, 2, 2, 1, 1}
	for idx, w := range want {
		if got := archiveQuota(10, 6, idx); got != w {
			t.Errorf("archiveQuota(10,6,%d)=%d，期望 %d", idx, got, w)
		}
	}
	// threads=10, maxConc=3 → 4,3,3
	want2 := []int{4, 3, 3}
	for idx, w := range want2 {
		if got := archiveQuota(10, 3, idx); got != w {
			t.Errorf("archiveQuota(10,3,%d)=%d，期望 %d", idx, got, w)
		}
	}
	// 边界：threads=0 → maxConc>=threads 分支返回 1（兜底，避免 0 线程死锁）
	if got := archiveQuota(0, 2, 0); got != 1 {
		t.Errorf("archiveQuota(0,2,0)=%d，期望 1", got)
	}
}

// TestArchiveSlotSingleArchiveFullThreads 验证 MaxArchiveConcurrency=1 时：
// 单归档拿满全部线程，第二个归档在并发窗口假死阻塞，释放后唤醒并拿满。
func TestArchiveSlotSingleArchiveFullThreads(t *testing.T) {
	p := newArchiveThreadPool(10)

	if got := p.archiveSlot("a", 10, 1, nil); got != 10 {
		t.Fatalf("单归档期望拿满 10 线程，实际 %d", got)
	}

	done := make(chan int, 1)
	go func() {
		done <- p.archiveSlot("b", 10, 1, nil)
	}()
	select {
	case g := <-done:
		t.Fatalf("b 不应立即获得额度（并发窗口 1/1 已满，g=%d）", g)
	case <-time.After(80 * time.Millisecond):
	}

	p.releaseArchive("a")
	select {
	case g := <-done:
		if g != 10 {
			t.Errorf("b 在 a 释放后应获得 10 线程，实际 %d", g)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("b 在 a 释放后未获得额度")
	}

	if active := p.currentActive(); active != 10 {
		t.Errorf("释放后 active 应为 10（b 持有），实际 %d", active)
	}
	p.releaseArchive("b")
	if active := p.currentActive(); active != 0 {
		t.Errorf("全部释放后 active 应为 0，实际 %d", active)
	}
}

// TestArchiveSlotQuotaDistribution 验证并发窗口内每归档额度的 5.2 分配
// 与第 7 个归档在并发窗口（eff=6）阻塞、释放后按末位配额获得 1 线程。
func TestArchiveSlotQuotaDistribution(t *testing.T) {
	p := newArchiveThreadPool(10)
	want := []int{2, 2, 2, 2, 1, 1}
	var got []int
	for i := 0; i < 6; i++ {
		got = append(got, p.archiveSlot(fmt.Sprintf("a%d", i), 10, 6, nil))
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("归档 %d 期望 %d 线程，实际 %d", i, w, got[i])
		}
	}

	// 第 7 个归档阻塞在并发窗口（eff=6）
	done := make(chan int, 1)
	go func() {
		done <- p.archiveSlot("a6", 10, 6, nil)
	}()
	select {
	case g := <-done:
		t.Fatalf("a6 不应立即获得额度（并发窗口 6/6 已满，g=%d）", g)
	case <-time.After(80 * time.Millisecond):
	}

	p.releaseArchive("a0")
	select {
	case g := <-done:
		if g != 1 {
			t.Errorf("a6 在 a0 释放后应获得 1 线程（末位配额），实际 %d", g)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a6 在释放后未获得额度")
	}
}

// TestArchiveSlotStopCallback 验证归档任务在并发窗口阻塞期间被停止时
// 立即放弃（返回 0），不占用窗口与线程。
func TestArchiveSlotStopCallback(t *testing.T) {
	p := newArchiveThreadPool(10)
	if got := p.archiveSlot("a", 10, 1, nil); got != 10 {
		t.Fatalf("a 期望 10 线程，实际 %d", got)
	}

	var mu sync.Mutex
	stopped := false
	stop := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return stopped
	}

	done := make(chan int, 1)
	go func() {
		done <- p.archiveSlot("b", 10, 1, stop)
	}()
	select {
	case g := <-done:
		t.Fatalf("b 不应立即获得额度（g=%d）", g)
	case <-time.After(80 * time.Millisecond):
	}

	mu.Lock()
	stopped = true
	mu.Unlock()
	p.wakeAll()
	select {
	case g := <-done:
		if g != 0 {
			t.Errorf("b 停止后应返回 0，实际 %d", g)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("b 未在停止后退出")
	}

	// a 不受影响，仍持有 10 线程
	if got := p.acquired("a"); got != 10 {
		t.Errorf("a 仍应持有 10 线程，实际 %d", got)
	}
}

// TestAcquirePartialBestEffort 验证画廊任务尽力获取：余量足够取满、
// 余量不足取余量、余量 0 时假死阻塞、释放后唤醒。
func TestAcquirePartialBestEffort(t *testing.T) {
	p := newArchiveThreadPool(10)

	if got := p.acquirePartial("g1", 5, nil); got != 5 {
		t.Fatalf("g1 期望 5 线程，实际 %d", got)
	}
	if got := p.acquirePartial("g1", 5, nil); got != 5 {
		t.Fatalf("g1 第二次期望 5 线程，实际 %d", got)
	}

	// 余量 0，g2 应假死阻塞
	done := make(chan int, 1)
	go func() {
		done <- p.acquirePartial("g2", 3, nil)
	}()
	select {
	case g := <-done:
		t.Fatalf("g2 不应立即获得额度（余量 0，g=%d）", g)
	case <-time.After(80 * time.Millisecond):
	}

	p.release("g1", 7)
	select {
	case g := <-done:
		if g != 3 {
			t.Errorf("g2 在释放后应获得 3 线程，实际 %d", g)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("g2 在释放后未获得额度")
	}

	if active := p.currentActive(); active != 6 {
		t.Errorf("active 应为 6（g1 3 + g2 3），实际 %d", active)
	}
}

// TestAcquirePartialStop 验证画廊任务在余量 0 阻塞期间被停止时立即放弃。
func TestAcquirePartialStop(t *testing.T) {
	p := newArchiveThreadPool(10)
	if got := p.acquirePartial("g1", 10, nil); got != 10 {
		t.Fatalf("g1 期望 10 线程，实际 %d", got)
	}

	var mu sync.Mutex
	stopped := false
	stop := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return stopped
	}

	done := make(chan int, 1)
	go func() {
		done <- p.acquirePartial("g2", 2, stop)
	}()
	select {
	case g := <-done:
		t.Fatalf("g2 不应立即获得额度（g=%d）", g)
	case <-time.After(80 * time.Millisecond):
	}

	mu.Lock()
	stopped = true
	mu.Unlock()
	p.wakeAll()
	select {
	case g := <-done:
		if g != 0 {
			t.Errorf("g2 停止后应返回 0，实际 %d", g)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("g2 未在停止后退出")
	}
}

// TestHathStreamURLProbeChunkPath 验证现象 A 关键修复：
// 带 start 参数的 H@H「下载页直链」不再短路为单线程，而是参与 Range 探测分块多线程下载
// （对齐 JHentai 同一套 start=1 直链 + 多 Isolate Range 分片，archive_download_service.dart）。
//
// 修复前：isHathStreamDownloadURL 判定为真 → 直接单线程 downloadZip（全程无 Range 请求）。
// 修复后：一律 probeArchiveDownload（GET + bytes=0-1023）→ 206 则分块并发（Range 请求 >= 2）。
func TestHathStreamURLProbeChunkPath(t *testing.T) {
	size := 8 * 1024 * 1024 // 8 MiB（>= minChunkSize*2，满足分块条件）
	data := makeTestZip(t, size)

	var mu sync.Mutex
	var rangeReqs, plainReqs int

	// 支持 Range 的测试服务器：带 Range 请求返回 206，无 Range 返回 200 完整数据；
	// 统计两类请求数以区分「分块路径」与「单线程短路路径」。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rng := r.Header.Get("Range")
		if rng == "" {
			mu.Lock()
			plainReqs++
			mu.Unlock()
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
		mu.Lock()
		rangeReqs++
		mu.Unlock()
		var start, end int64
		if _, err := fmt.Sscanf(rng, "bytes=%d-%d", &start, &end); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if end <= 0 || end >= int64(len(data)) {
			end = int64(len(data)) - 1
		}
		if start >= int64(len(data)) {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(data)))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data[start : end+1])
	}))
	defer srv.Close()

	// 构造 H@H 下载页直链（host 含 hath.network + start=1，isHathStreamDownloadURL 判定为真），
	// 通过自定义 Transport 的 DialContext 将请求重定向到 httptest 测试服务器（免真实 DNS/网络）。
	srvAddr := strings.TrimPrefix(srv.URL, "http://")
	customTransport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, srvAddr)
		},
	}

	mgr := newTestDownloadManager(t)
	g := newTestArchiveDownloader(t, mgr, srv.URL, t.TempDir())
	g.client = &http.Client{Transport: customTransport}

	downloadURL := "http://eh-archive-1234.hath.network/archive/file.zip?start=1"
	if !isHathStreamDownloadURL(downloadURL) {
		t.Fatal("测试 URL 应被判定为 H@H 直链（带 start 参数）")
	}

	if err := g.downloadArchiveFile(downloadURL); err != nil {
		t.Fatalf("带 start 直链下载失败: %v", err)
	}

	got, err := os.ReadFile(g.zipPath)
	if err != nil {
		t.Fatalf("读取 zip 失败: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("下载内容与源不一致：got=%d 字节，want=%d 字节", len(got), len(data))
	}

	mu.Lock()
	r, p := rangeReqs, plainReqs
	mu.Unlock()
	if r < 2 {
		t.Fatalf("带 start 直链应参与 Range 探测分块（探测 1 次 + 至少 1 个分块请求），实际 Range 请求=%d", r)
	}
	if p != 0 {
		t.Fatalf("带 start 直链不应走无 Range 的单线程短路下载，实际无 Range 请求=%d", p)
	}
}

// TestArchiveEngineAcquireSlot 验证步骤 4：acquire 提前到解锁前 + 每归档额度按 5.2 规则。
// 引擎 acquireSlot 读取设置调用 archiveSlot（MaxArchiveConcurrency=2、ArchiveThreads=10
// → 每个归档分配 5 线程），releaseSlot 幂等释放；并发窗口满时第 3 个归档阻塞，
// 空位出现（releaseSlot）后被唤醒并进入新窗口拿满配额。
func TestArchiveEngineAcquireSlot(t *testing.T) {
	mgr := newTestDownloadManager(t)

	setting := mgr.GetSettings()
	setting.ArchiveThreads = 10
	setting.MaxArchiveConcurrency = 2
	setting.ControlArchiveConcurrency = true
	if _, err := mgr.SaveSettings(setting); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}

	mk := func(id string) *archiveDownloader {
		return &archiveDownloader{m: mgr, task: &models.DownloadTask{ID: id}}
	}
	g1 := mk("arch-1")
	g2 := mk("arch-2")
	g3 := mk("arch-3")

	// 前两个归档各占 5 线程（并发窗口 2/2，active=10/10 满）
	if !g1.acquireSlot() || g1.threads != 5 {
		t.Fatalf("arch-1 应获 5 线程，实际 ok=%v threads=%d", g1.threads > 0, g1.threads)
	}
	if !g2.acquireSlot() || g2.threads != 5 {
		t.Fatalf("arch-2 应获 5 线程，实际 ok=%v threads=%d", g2.threads > 0, g2.threads)
	}
	if got := mgr.archivePool.currentActive(); got != 10 {
		t.Fatalf("两个归档应占满 10 线程，实际 active=%d", got)
	}

	// 并发窗口已满：arch-3 阻塞假死（不耗 GP）
	done := make(chan bool, 1)
	go func() { done <- g3.acquireSlot() }()
	select {
	case <-done:
		t.Fatal("arch-3 不应立即获得额度（并发窗口已满）")
	case <-time.After(80 * time.Millisecond):
	}

	// 释放 arch-1 → 空位唤醒 arch-3（新窗口 idx=0 → 5 线程）
	g1.releaseSlot()
	select {
	case ok := <-done:
		if !ok || g3.threads != 5 {
			t.Fatalf("arch-3 应获 5 线程，实际 ok=%v threads=%d", ok, g3.threads)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("arch-1 释放后 arch-3 未被唤醒")
	}

	// releaseSlot 幂等：重复释放无副作用；全部释放后 active=0
	g1.releaseSlot()
	g2.releaseSlot()
	g3.releaseSlot()
	if got := mgr.archivePool.currentActive(); got != 0 {
		t.Fatalf("全部释放后 active 应为 0，实际 %d", got)
	}
}
