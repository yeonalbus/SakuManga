package services

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"math/rand"
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

// newTestDownloadManager 构造带内存 sqlite 的 DownloadManager（仅用于归档引擎测试）
func newTestDownloadManager(t *testing.T) *DownloadManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
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

var _ = io.Discard // 占位：确保 io 导入被使用（供未来扩展）
