package services

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"SakuHentai/internal/models"
)

// ─────────────────────────────────────────────────────────────
// 优先级 + 抢占式调度（计划书 5.5）测试
//
// 覆盖：
//   - preemptLowerPriority 仅抢占优先级更低的正在运行任务（置回 queued，进度保留）
//   - 高优先级任务不受低优先级抢占触发影响
//   - SetTaskPriority 提升优先级 → 触发抢占（含 completed/cancelled 终态校验）
//   - 集成链路：低优先级画廊引擎真实下载中被高优先级任务抢占 → 引擎中断 + 线程释放 + 状态回 queued
// ─────────────────────────────────────────────────────────────

// TestPreemptLowerPriorityStopsLowPriority 验证抢占式调度核心：
// 正在运行的低优先级任务被 preemptLowerPriority(高) 置回 queued（进度保留，可断点续传），
// 更高优先级任务不受影响。
func TestPreemptLowerPriorityStopsLowPriority(t *testing.T) {
	mgr := newTestDownloadManager(t)

	low := &models.DownloadTask{
		ID:         "low-task",
		Status:     models.DownloadDownloading,
		GID:        "1",
		Token:      "t",
		Title:      "low",
		UserID:     1,
		Priority:   0,
		DoneFiles:  5,
		TotalFiles: 100,
	}
	if err := mgr.db.Create(low).Error; err != nil {
		t.Fatalf("创建低优先级任务失败: %v", err)
	}

	high := &models.DownloadTask{
		ID:       "high-task",
		Status:   models.DownloadDownloading,
		GID:      "2",
		Token:    "t",
		Title:    "high",
		UserID:   1,
		Priority: 20,
	}
	if err := mgr.db.Create(high).Error; err != nil {
		t.Fatalf("创建高优先级任务失败: %v", err)
	}

	// 模拟调度器记录两个任务正在运行（真实系统中由 acquireSlot 成功时写入）
	mgr.schedMu.Lock()
	mgr.runningTasks[low.ID] = low.Priority
	mgr.runningTasks[high.ID] = high.Priority
	mgr.schedMu.Unlock()

	// 新入队的中优先级任务触发抢占：仅 low 应被抢占
	mgr.preemptLowerPriority(10)

	var lowGot models.DownloadTask
	if err := mgr.db.First(&lowGot, "id = ?", low.ID).Error; err != nil {
		t.Fatalf("读取 low 任务失败: %v", err)
	}
	if lowGot.Status != models.DownloadQueued {
		t.Fatalf("低优先级任务应被置回 queued，得到 %s", lowGot.Status)
	}
	if lowGot.DoneFiles != 5 {
		t.Fatalf("被抢占任务应保留进度（断点续传），得到 %d", lowGot.DoneFiles)
	}

	var highGot models.DownloadTask
	if err := mgr.db.First(&highGot, "id = ?", high.ID).Error; err != nil {
		t.Fatalf("读取 high 任务失败: %v", err)
	}
	if highGot.Status != models.DownloadDownloading {
		t.Fatalf("优先级 20 > 10 的任务不应被抢占，得到 %s", highGot.Status)
	}
}

// TestPreemptSkipsNonDownloading 验证抢占仅针对 downloading 状态：
// 等待槽位/排队中的任务（queued）无需打断，直接由调度竞争槽位。
func TestPreemptSkipsNonDownloading(t *testing.T) {
	mgr := newTestDownloadManager(t)

	queued := &models.DownloadTask{
		ID:       "queued-task",
		Status:   models.DownloadQueued,
		UserID:   1,
		Priority: 0,
	}
	if err := mgr.db.Create(queued).Error; err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	mgr.schedMu.Lock()
	mgr.runningTasks[queued.ID] = queued.Priority
	mgr.schedMu.Unlock()

	// 触发抢占：queued 任务不在下载，不应被 preemptOne 打断（状态保持不变）
	mgr.preemptLowerPriority(10)

	var got models.DownloadTask
	if err := mgr.db.First(&got, "id = ?", queued.ID).Error; err != nil {
		t.Fatalf("读取任务失败: %v", err)
	}
	if got.Status != models.DownloadQueued {
		t.Fatalf("queued 任务不应被抢占改状态，得到 %s", got.Status)
	}
}

// TestSetTaskPriorityTriggersPreempt 验证 SetTaskPriority 提升优先级触发抢占：
// queued 任务提升后，正在运行的低优先级任务被置回 queued。
func TestSetTaskPriorityTriggersPreempt(t *testing.T) {
	mgr := newTestDownloadManager(t)

	low := &models.DownloadTask{
		ID:       "low-task",
		Status:   models.DownloadDownloading,
		UserID:   1,
		Priority: 0,
	}
	if err := mgr.db.Create(low).Error; err != nil {
		t.Fatalf("创建低优先级任务失败: %v", err)
	}
	mgr.schedMu.Lock()
	mgr.runningTasks[low.ID] = low.Priority
	mgr.schedMu.Unlock()

	up := &models.DownloadTask{
		ID:       "up-task",
		Status:   models.DownloadQueued,
		UserID:   1,
		Priority: 0,
	}
	if err := mgr.db.Create(up).Error; err != nil {
		t.Fatalf("创建提升任务失败: %v", err)
	}

	updated, err := mgr.SetTaskPriority(up.ID, 10)
	if err != nil {
		t.Fatalf("SetTaskPriority 失败: %v", err)
	}
	if updated.Priority != 10 {
		t.Fatalf("优先级应更新为 10，得到 %d", updated.Priority)
	}

	var lowGot models.DownloadTask
	if err := mgr.db.First(&lowGot, "id = ?", low.ID).Error; err != nil {
		t.Fatalf("读取 low 任务失败: %v", err)
	}
	if lowGot.Status != models.DownloadQueued {
		t.Fatalf("提升优先级应抢占低优先级任务，low 状态应为 queued，得到 %s", lowGot.Status)
	}
}

// TestSetTaskPriorityValidation 验证终态任务不允许修改优先级。
func TestSetTaskPriorityValidation(t *testing.T) {
	mgr := newTestDownloadManager(t)

	done := &models.DownloadTask{
		ID:       "done-task",
		Status:   models.DownloadCompleted,
		UserID:   1,
		Priority: 0,
	}
	if err := mgr.db.Create(done).Error; err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if _, err := mgr.SetTaskPriority(done.ID, 10); err == nil {
		t.Fatalf("completed 任务不应允许修改优先级")
	}

	cancelled := &models.DownloadTask{
		ID:       "cancelled-task",
		Status:   models.DownloadCancelled,
		UserID:   1,
		Priority: 0,
	}
	if err := mgr.db.Create(cancelled).Error; err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if _, err := mgr.SetTaskPriority(cancelled.ID, 10); err == nil {
		t.Fatalf("cancelled 任务不应允许修改优先级")
	}
}

// TestPreemptInterruptsRunningGallery 集成验证抢占完整链路：
// 低优先级画廊引擎真实下载中，高优先级任务 SetTaskPriority 提升触发抢占 →
// 引擎被中断（downloadAll 返回）、任务置回 queued（进度保留）、全局线程额度释放。
func TestPreemptInterruptsRunningGallery(t *testing.T) {
	mgr := newTestDownloadManager(t)
	useTestScannerDB(t, mgr)
	img := bytes.Repeat([]byte{0xAB, 0xCD}, 256*1024) // 512 KiB / 张，慢速响应制造进行中窗口
	srv := newGalleryImageServer(img, 40*time.Millisecond)
	defer srv.Close()

	dir := t.TempDir()

	// 低优先级任务：引擎运行中（downloading）
	low := &models.DownloadTask{
		ID:         "preempt-low",
		Status:     models.DownloadDownloading,
		GID:        "1",
		Token:      "t",
		Title:      "low",
		UserID:     1,
		Priority:   0,
		TotalFiles: 40,
	}
	if err := mgr.db.Create(low).Error; err != nil {
		t.Fatalf("创建低优先级任务失败: %v", err)
	}

	// 高优先级任务：排队中
	high := &models.DownloadTask{
		ID:       "preempt-high",
		Status:   models.DownloadQueued,
		GID:      "2",
		Token:    "t",
		Title:    "high",
		UserID:   1,
		Priority: 0,
	}
	if err := mgr.db.Create(high).Error; err != nil {
		t.Fatalf("创建高优先级任务失败: %v", err)
	}

	// 构造低优先级画廊引擎（复用 newTestGalleryDownloader 结构，但任务已是 downloading）
	g := &galleryDownloader{
		m:         mgr,
		task:      low,
		client:    srv.Client(),
		destDir:   dir,
		referer:   srv.URL + "/",
		setting:   mgr.GetSettings(),
		startedAt: time.Now(),
	}
	mgr.registerGallery(g)
	defer mgr.unregisterGallery(low.ID)
	g.initContext()

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

	// 模拟调度器记录低优先级任务正在运行（真实系统由 acquireSlot 写入）
	mgr.schedMu.Lock()
	mgr.runningTasks[low.ID] = low.Priority
	mgr.schedMu.Unlock()

	// 高优先级任务提升 → 触发抢占，中断低优先级引擎
	if _, err := mgr.SetTaskPriority(high.ID, 10); err != nil {
		t.Fatalf("SetTaskPriority 失败: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("抢占后低优先级 downloadAll 未及时返回")
	}

	// 低优先级任务被置回 queued（未被 completed/error 覆盖）
	var latest models.DownloadTask
	if err := mgr.db.First(&latest, "id = ?", low.ID).Error; err != nil {
		t.Fatalf("读取低优先级任务失败: %v", err)
	}
	if latest.Status != models.DownloadQueued {
		t.Errorf("抢占后任务状态=%s，期望 queued", latest.Status)
	}

	// 高优先级任务优先级已更新
	var highGot models.DownloadTask
	if err := mgr.db.First(&highGot, "id = ?", high.ID).Error; err != nil {
		t.Fatalf("读取高优先级任务失败: %v", err)
	}
	if highGot.Priority != 10 {
		t.Errorf("高优先级任务优先级=%d，期望 10", highGot.Priority)
	}

	// 全局线程额度已释放
	if got := mgr.archivePool.currentActive(); got != 0 {
		t.Errorf("抢占后 active=%d，期望 0", got)
	}
}
