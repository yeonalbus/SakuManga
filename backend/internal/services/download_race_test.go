package services

import (
	"testing"
	"time"

	"SakuHentai/internal/models"
)

// ─────────────────────────────────────────────────────────────
// 暂停→快速恢复「线程不足」回归测试
//
// 线上现象：下载中暂停后立即恢复，新 worker 报
//   [sched] 任务 ... 获取槽位（runningTotal=2 runningArchive=2）
//   [archive-thread] 任务 ... 线程不足（active=10/10，需要 10），排队等待…
// 根因：暂停→恢复间隔过短，旧 worker 尚未释放线程配额与调度槽位，
//       新 worker 启动后因线程池满而永久排队。
// 修复：
//   Fix1（download.go wait*Stopped）：暂停/取消/抢占后等待旧引擎完全退出
//         （先释放线程再注销），确保返回时线程已归零，恢复无需排队。
//   Fix2（download.go runTask taskRunning）：同一任务 ID 同一时刻仅允许一个
//         worker 处理，重复 worker 重新入队等待，杜绝并发处理同一任务。
// ─────────────────────────────────────────────────────────────

// TestPauseTaskWaitsEngineExitAndReleasesThreads 复现「暂停时旧引擎仍持有全部线程配额
// （active=10/10）且尚未退出」场景：PauseTask 必须先等待引擎完全退出（释放线程并注销）
// 再返回，否则随后的恢复会因线程未释放而排队等待（"线程不足"）。
func TestPauseTaskWaitsEngineExitAndReleasesThreads(t *testing.T) {
	mgr := newTestDownloadManager(t)
	task := &models.DownloadTask{
		ID:     "pause-wait-1",
		Status: models.DownloadDownloading,
		GID:    "1",
		Token:  "t",
		Title:  "test",
		UserID: 1,
	}
	if err := mgr.db.Create(task).Error; err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}

	g := &archiveDownloader{m: mgr, task: task}
	g.stopFlag.Store(false)

	// 模拟引擎已获取全部线程配额（active=10/10，与线上日志一致）
	if got := mgr.archivePool.acquire(task.ID, defaultMaxArchiveThreads, nil); got != defaultMaxArchiveThreads {
		t.Fatalf("应获取 %d 个线程，得到 %d", defaultMaxArchiveThreads, got)
	}
	// 注册引擎：PauseTask 通过 activeArchives 找到它并中断
	mgr.registerArchive(g)

	// 模拟引擎运行：阻塞在下载循环中直到被暂停。
	// defer 按 LIFO 执行：先释放线程（releaseSlot 语义）→ 再注销（runArchiveEngine 的 defer）→ 最后关闭 done。
	done := make(chan struct{})
	go func() {
		defer close(done)                         // 最后执行：确认线程与注销均已完成
		defer mgr.unregisterArchive(task.ID)      // 其次执行
		defer mgr.archivePool.releaseAll(task.ID) // 最先执行：先释放线程再注销
		for !g.stopped() {
			time.Sleep(5 * time.Millisecond)
		}
	}()

	start := time.Now()
	if _, err := mgr.PauseTask(task.ID); err != nil {
		t.Fatalf("暂停失败: %v", err)
	}

	// PauseTask 返回后引擎应已完全退出（waitArchiveStopped 保证）
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("PauseTask 返回后引擎仍未退出")
	}

	// 线程配额已释放：恢复后的新 worker 无需排队等线程
	if got := mgr.archivePool.currentActive(); got != 0 {
		t.Errorf("暂停后线程未释放：active=%d，期望 0", got)
	}

	// 引擎已注销：后续暂停/取消可正常中断新引擎
	mgr.mu.Lock()
	_, registered := mgr.activeArchives[task.ID]
	mgr.mu.Unlock()
	if registered {
		t.Error("暂停后引擎仍注册在 activeArchives 中")
	}

	// 暂停应快速返回（引擎被 stopDownload 中断立即退出，不应等待超时兜底）
	if elapsed := time.Since(start); elapsed >= engineStopWaitTimeout {
		t.Errorf("暂停耗时 %v，不应等到超时 %v", elapsed, engineStopWaitTimeout)
	}
}

// TestRunTaskMutualExclusionReenqueuesDuplicate 验证每任务互斥（Fix2）：
// 同一任务 ID 已被某 worker 处理（旧 worker 尚未退出）时，新 worker 取出同一任务
// 应检测到互斥并快速重新入队，而不是并发处理导致线程/槽位重复占用。
func TestRunTaskMutualExclusionReenqueuesDuplicate(t *testing.T) {
	mgr := newTestDownloadManager(t)
	task := &models.DownloadTask{
		ID:     "mutex-1",
		Status: models.DownloadQueued,
		GID:    "1",
		Token:  "t",
		Title:  "test",
		UserID: 1,
	}
	if err := mgr.db.Create(task).Error; err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}

	// 模拟旧 worker 已占用该任务（暂停→快速恢复时旧 worker 尚未退出）
	mgr.taskRunMu.Lock()
	mgr.taskRunning[task.ID] = true
	mgr.taskRunMu.Unlock()

	// 新 worker 取出同一任务：应检测互斥并快速重新入队（不获取槽位/不分派引擎）
	start := time.Now()
	mgr.runTask(task.ID)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("互斥分支应快速返回，实际耗时 %v", elapsed)
	}

	// 旧 worker 仍持有标记（未被新 worker 覆盖/清除）
	mgr.taskRunMu.Lock()
	running := mgr.taskRunning[task.ID]
	mgr.taskRunMu.Unlock()
	if !running {
		t.Error("旧 worker 的任务标记应保持 true")
	}

	// 任务应被重新入队等待旧 worker 退出
	select {
	case id := <-mgr.queue:
		if id != task.ID {
			t.Errorf("队列应收到任务 %s，收到 %s", task.ID, id)
		}
	default:
		t.Error("重复 worker 应把任务重新入队")
	}
}

// TestWaitEngineExitedNonBlockingWhenIdle 验证等待辅助在引擎未运行时立即返回：
// 暂停/取消/抢占一个「非下载中」任务（activeArchives/activeGalleries 为空）时，
// waitArchiveStopped/waitGalleryStopped 不应阻塞（不触发超时等待）。
func TestWaitEngineExitedNonBlockingWhenIdle(t *testing.T) {
	mgr := newTestDownloadManager(t)

	start := time.Now()
	mgr.waitArchiveStopped("nonexistent")
	mgr.waitGalleryStopped("nonexistent")
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("无运行引擎时等待辅助不应阻塞，实际耗时 %v", elapsed)
	}
}
