package services

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
)

// ─────────────────────────────────────────────────────────────
// 画廊下载引擎改造（5.4）测试
// 覆盖：接入全局线程配额池（多画廊总线程 ≤ 10 + 结束 releaseAll）、
//       停止能力（本地停止标记 + 共享 context 中断进行中图片请求，.part 保留续传）。
// ─────────────────────────────────────────────────────────────

// newGalleryImageServer 返回画廊测试图片服务器（可选每请求延迟）
func newGalleryImageServer(data []byte, delay time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if delay > 0 {
			time.Sleep(delay)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
}

// useTestScannerDB 让 scanDirectory/saveComic 使用测试库并迁移所需表。
// downloadAll 成功路径会调用 ScanAndSaveDirectory（依赖全局 database.DB），
// 测试环境 database.DB 为 nil 会导致 panic，因此注入测试库。
func useTestScannerDB(t *testing.T, mgr *DownloadManager) {
	t.Helper()
	if err := mgr.db.AutoMigrate(&models.OfflineComic{}); err != nil {
		t.Fatalf("迁移 OfflineComic 失败: %v", err)
	}
	old := database.DB
	database.DB = mgr.db
	t.Cleanup(func() { database.DB = old })
}

// newTestGalleryDownloader 构造画廊下载器（client 指向测试服务器，任务已入库）
func newTestGalleryDownloader(t *testing.T, mgr *DownloadManager, taskID string, srv *httptest.Server, dir string) *galleryDownloader {
	t.Helper()
	task := &models.DownloadTask{
		ID:     taskID,
		Status: models.DownloadQueued,
		GID:    "1",
		Token:  "t",
		Title:  "test",
		UserID: 1,
	}
	if err := mgr.db.Create(task).Error; err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	g := &galleryDownloader{
		m:         mgr,
		task:      task,
		client:    srv.Client(),
		destDir:   dir,
		referer:   srv.URL + "/",
		setting:   mgr.GetSettings(),
		startedAt: time.Now(),
	}
	mgr.registerGallery(g)
	t.Cleanup(func() { mgr.unregisterGallery(taskID) })
	g.initContext()
	return g
}

// TestGalleryEngineQuotaPool 验证画廊下载接入全局线程配额池：
//   - 下载期间持有 min(ConcurrentImageDownloads, 全局余量) 线程（单画廊满额 10）
//   - 下载结束（含全部成功路径）releaseAll → active 归 0
//   - 全部图片正确落盘、任务置 completed
func TestGalleryEngineQuotaPool(t *testing.T) {
	mgr := newTestDownloadManager(t)
	useTestScannerDB(t, mgr)
	img := bytes.Repeat([]byte{0xAB, 0xCD}, 256*1024) // 512 KiB / 张
	srv := newGalleryImageServer(img, 0)
	defer srv.Close()

	dir := t.TempDir()
	g := newTestGalleryDownloader(t, mgr, "gallery-quota-1", srv, dir)

	n := 30
	urls := make([]string, n)
	for i := 0; i < n; i++ {
		urls[i] = fmt.Sprintf("%s/img/%d.jpg", srv.URL, i)
	}

	done := make(chan struct{})
	go func() {
		g.downloadAll(urls, nil)
		close(done)
	}()

	// 采样下载期间活跃线程峰值（acquirePartial 一次性持有整个下载期间，应稳定 = 满额 10）
	maxActive := 0
	poll := true
	for poll {
		select {
		case <-done:
			poll = false
		case <-time.After(2 * time.Millisecond):
		}
		if a := mgr.archivePool.currentActive(); a > maxActive {
			maxActive = a
		}
	}
	<-done

	if maxActive != 10 {
		t.Errorf("下载期间活跃线程峰值=%d，期望 10（单画廊满额 ConcurrentImageDownloads）", maxActive)
	}
	if got := mgr.archivePool.currentActive(); got != 0 {
		t.Errorf("下载结束后 active=%d，期望 0（releaseAll 应生效）", got)
	}

	var latest models.DownloadTask
	if err := mgr.db.First(&latest, "id = ?", g.task.ID).Error; err != nil {
		t.Fatalf("读取任务失败: %v", err)
	}
	if latest.Status != models.DownloadCompleted {
		t.Errorf("任务状态=%s，期望 completed", latest.Status)
	}
	if latest.DoneFiles != n {
		t.Errorf("DoneFiles=%d，期望 %d", latest.DoneFiles, n)
	}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("%03d.jpg", i+1)
		if fi, err := os.Stat(filepath.Join(dir, name)); err != nil || fi.Size() == 0 {
			t.Errorf("文件 %s 未正确落盘: %v", name, err)
		}
	}
}

// TestGalleryEngineStopInterrupts 验证画廊下载停止能力：
//   - stopGalleryDownload 置位本地停止标记 + 取消共享 context，中断进行中图片请求
//   - downloadAll 及时返回、任务状态不被覆盖（保持 queued）
//   - 线程配额释放（active 归 0）
func TestGalleryEngineStopInterrupts(t *testing.T) {
	mgr := newTestDownloadManager(t)
	useTestScannerDB(t, mgr)
	img := bytes.Repeat([]byte{0xAB, 0xCD}, 256*1024) // 512 KiB / 张，配合慢速响应制造进行中窗口
	srv := newGalleryImageServer(img, 40*time.Millisecond)
	defer srv.Close()

	dir := t.TempDir()
	g := newTestGalleryDownloader(t, mgr, "gallery-stop-1", srv, dir)

	n := 40
	urls := make([]string, n)
	for i := 0; i < n; i++ {
		urls[i] = fmt.Sprintf("%s/img/%d.jpg", srv.URL, i)
	}

	done := make(chan struct{})
	go func() {
		g.downloadAll(urls, nil)
		close(done)
	}()

	// 等待额度获取（downloadAll 已进入下载阶段）
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if mgr.archivePool.currentActive() > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if mgr.archivePool.currentActive() == 0 {
		t.Fatal("downloadAll 未获取到线程额度")
	}

	// 停止：应中断进行中图片请求并唤醒额度池
	taskID := g.task.ID
	mgr.stopGalleryDownload(taskID)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("停止后 downloadAll 未及时返回")
	}

	// 本地停止标记生效（无需回查 DB）
	if !g.stopped() {
		t.Errorf("stopDownload 后 stopped() 应为 true")
	}

	// 状态保持 queued（未被 completed/error 覆盖）
	var latest models.DownloadTask
	if err := mgr.db.First(&latest, "id = ?", taskID).Error; err != nil {
		t.Fatalf("读取任务失败: %v", err)
	}
	if latest.Status != models.DownloadQueued {
		t.Errorf("停止后任务状态=%s，期望保持 queued", latest.Status)
	}

	// 线程配额已释放
	if got := mgr.archivePool.currentActive(); got != 0 {
		t.Errorf("停止后 active=%d，期望 0", got)
	}
}

// TestGalleryTwoConcurrentWithinPool 验证多画廊并行总线程 ≤ 全局上限（修复路径一 N×10 超限）：
// 两个画廊各请求 ConcurrentImageDownloads(10)，同时下载时池内活跃线程不得超过 defaultMaxArchiveThreads(10)。
func TestGalleryTwoConcurrentWithinPool(t *testing.T) {
	mgr := newTestDownloadManager(t)
	useTestScannerDB(t, mgr)
	img := bytes.Repeat([]byte{0xAB, 0xCD}, 256*1024)
	srv := newGalleryImageServer(img, 5*time.Millisecond)
	defer srv.Close()

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	g1 := newTestGalleryDownloader(t, mgr, "gallery-two-1", srv, dir1)
	g2 := newTestGalleryDownloader(t, mgr, "gallery-two-2", srv, dir2)

	n := 40
	urls := make([]string, n)
	for i := 0; i < n; i++ {
		urls[i] = fmt.Sprintf("%s/img/%d.jpg", srv.URL, i)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); g1.downloadAll(urls, nil) }()
	go func() { defer wg.Done(); g2.downloadAll(urls, nil) }()

	// 采样活跃线程峰值：不得超过全局上限 10
	maxActive := 0
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	poll := true
	for poll {
		select {
		case <-done:
			poll = false
		case <-time.After(2 * time.Millisecond):
		}
		if a := mgr.archivePool.currentActive(); a > maxActive {
			maxActive = a
		}
	}
	<-done

	if maxActive > defaultMaxArchiveThreads {
		t.Errorf("两画廊并行活跃线程峰值=%d，超过全局上限 %d", maxActive, defaultMaxArchiveThreads)
	}
	if got := mgr.archivePool.currentActive(); got != 0 {
		t.Errorf("全部结束后 active=%d，期望 0", got)
	}
}
