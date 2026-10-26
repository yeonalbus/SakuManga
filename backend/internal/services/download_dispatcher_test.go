package services

import (
	"fmt"
	"testing"
	"time"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round45：统一调度器单测
//
// 覆盖：
//   - Enqueue 无上限（1000 条不丢）
//   - 串行模式：同一时刻仅 1 个任务获牌
//   - 归档并发窗口：MaxArchiveConcurrency=1 时第 2 个归档拿不到额度，释放后可拿
//   - 优先级门控（并行模式）：低优先级任务等更高优先级结束
//   - 暂停：从待调度队列移除
//   - dispatch：发牌后任务离开队列，执行结束释放预留（runningTotal/active 归零）
// ─────────────────────────────────────────────────────────────

func newSchedTask(t *testing.T, mgr *DownloadManager, id string, mode models.DownloadMode, priority int) *models.DownloadTask {
	t.Helper()
	task := &models.DownloadTask{
		ID:        id,
		GID:       "g-" + id,
		Token:     "t",
		Title:     id,
		UserID:    1,
		Mode:      mode,
		Status:    models.DownloadQueued,
		Priority:  priority,
		CreatedAt: time.Now(),
	}
	if mode == models.DownloadModeArchive {
		task.ArchiveType = models.ArchiveTypeOriginal
	}
	if err := mgr.db.Create(task).Error; err != nil {
		t.Fatalf("创建任务 %s 失败: %v", id, err)
	}
	return task
}

// setSchedSetting 修改下载设置（调度相关字段）
func setSchedSetting(t *testing.T, mgr *DownloadManager, mutate func(*models.DownloadSetting)) {
	t.Helper()
	s := mgr.GetSettings()
	mutate(s)
	if err := mgr.db.Save(s).Error; err != nil {
		t.Fatalf("保存下载设置失败: %v", err)
	}
}

// TestEnqueueThousandTasksNoLoss 1000 个任务全部进入待调度队列（旧实现超过 256 即丢弃）。
func TestEnqueueThousandTasksNoLoss(t *testing.T) {
	mgr := newTestDownloadManager(t)
	for i := 0; i < 1000; i++ {
		task := newSchedTask(t, mgr, fmt.Sprintf("t%04d", i), models.DownloadModeArchive, 0)
		mgr.Enqueue(task.ID)
	}
	if got := mgr.pending.Len(); got != 1000 {
		t.Fatalf("待调度队列应有 1000 个任务，得到 %d", got)
	}
}

// TestReserveSerialMode 串行模式：已运行 1 个任务时其他任务拿不到槽位。
func TestReserveSerialMode(t *testing.T) {
	mgr := newTestDownloadManager(t)
	setSchedSetting(t, mgr, func(s *models.DownloadSetting) {
		s.DownloadAllGalleriesSamePriority = false
	})
	t1 := newSchedTask(t, mgr, "serial-1", models.DownloadModeGallery, 0)
	t2 := newSchedTask(t, mgr, "serial-2", models.DownloadModeGallery, 0)

	res1, ok := mgr.tryReserve(t1)
	if !ok {
		t.Fatalf("第一个任务应获牌")
	}
	if _, ok := mgr.tryReserve(t2); ok {
		t.Fatalf("串行模式下第二个任务不应获牌")
	}
	mgr.releaseReservation(res1)
	if _, ok := mgr.tryReserve(t2); !ok {
		t.Fatalf("释放后第二个任务应获牌")
	}
}

// TestReserveArchiveWindow 归档并发窗口：MaxArchiveConcurrency=1 时第 2 个归档拿不到额度。
func TestReserveArchiveWindow(t *testing.T) {
	mgr := newTestDownloadManager(t)
	setSchedSetting(t, mgr, func(s *models.DownloadSetting) {
		s.ArchiveThreads = 10
		s.MaxArchiveConcurrency = 1
		s.ControlArchiveConcurrency = true
	})
	t1 := newSchedTask(t, mgr, "arch-w1", models.DownloadModeArchive, 0)
	t2 := newSchedTask(t, mgr, "arch-w2", models.DownloadModeArchive, 0)

	res1, ok := mgr.tryReserve(t1)
	if !ok || res1.threads != 10 {
		t.Fatalf("首个归档应拿满 10 线程，得到 ok=%v", ok)
	}
	if _, ok := mgr.tryReserve(t2); ok {
		t.Fatalf("归档并发窗口已满，第二个归档不应获牌")
	}
	mgr.releaseReservation(res1)
	if mgr.archivePool.currentActive() != 0 {
		t.Fatalf("释放后全局活跃线程应为 0，得到 %d", mgr.archivePool.currentActive())
	}
	res2, ok := mgr.tryReserve(t2)
	if !ok {
		t.Fatalf("窗口空出后第二个归档应获牌")
	}
	mgr.releaseReservation(res2)
}

// TestReservePriorityGate 并行模式：低优先级任务需等更高优先级任务结束。
func TestReservePriorityGate(t *testing.T) {
	mgr := newTestDownloadManager(t)
	setSchedSetting(t, mgr, func(s *models.DownloadSetting) {
		s.DownloadAllGalleriesSamePriority = true
		s.ConcurrentImageDownloads = 5 // 预留 5 线程，留出余量给高优先级任务
	})
	low := newSchedTask(t, mgr, "pri-low", models.DownloadModeGallery, 0)
	high := newSchedTask(t, mgr, "pri-high", models.DownloadModeGallery, 5)

	resLow, ok := mgr.tryReserve(low)
	if !ok {
		t.Fatalf("低优先级任务在无运行任务时应获牌")
	}
	// 高优先级可并行
	resHigh, ok := mgr.tryReserve(high)
	if !ok {
		t.Fatalf("并行模式下高优先级任务应获牌")
	}
	mgr.releaseReservation(resLow)
	mgr.releaseReservation(resHigh)

	// 运行中仅剩高优先级：低优先级再尝试应被拒
	if _, ok := mgr.tryReserve(high); !ok {
		t.Fatalf("高优先级任务应获牌")
	}
	if _, ok := mgr.tryReserve(low); ok {
		t.Fatalf("低优先级任务应等待更高优先级任务结束")
	}
}

// TestPauseRemovesFromPending 暂停后任务应从待调度队列移除。
func TestPauseRemovesFromPending(t *testing.T) {
	mgr := newTestDownloadManager(t)
	task := newSchedTask(t, mgr, "pause-1", models.DownloadModeArchive, 0)
	mgr.Enqueue(task.ID)
	if mgr.pending.Len() != 1 {
		t.Fatalf("入队后应有 1 个任务")
	}
	if _, err := mgr.PauseTask(task.ID); err != nil {
		t.Fatalf("暂停失败: %v", err)
	}
	if got := mgr.pending.Len(); got != 0 {
		t.Fatalf("暂停后待调度队列应为空，得到 %d", got)
	}
}

// TestDispatchLeavesQueueAndReleases dispatch 发牌后任务离开队列；
// 执行协程失败（未绑定账号）后释放预留，计数归零。
func TestDispatchLeavesQueueAndReleases(t *testing.T) {
	mgr := newTestDownloadManager(t)
	setSchedSetting(t, mgr, func(s *models.DownloadSetting) {
		s.ArchiveThreads = 10
		s.MaxArchiveConcurrency = 1
	})
	task := newSchedTask(t, mgr, "dispatch-1", models.DownloadModeArchive, 0)
	mgr.Enqueue(task.ID)

	mgr.dispatch()

	if mgr.pending.Contains(task.ID) {
		t.Fatalf("发牌后任务不应仍在待调度队列")
	}

	// 等执行协程结束（未绑定账号 → 快速 fail）并释放预留
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mgr.schedMu.Lock()
		total := mgr.runningTotal
		mgr.schedMu.Unlock()
		if total == 0 && mgr.archivePool.currentActive() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mgr.schedMu.Lock()
	total := mgr.runningTotal
	_, stillReserved := mgr.reserved[task.ID]
	mgr.schedMu.Unlock()
	if total != 0 {
		t.Fatalf("执行结束后 runningTotal 应归零，得到 %d", total)
	}
	if stillReserved {
		t.Fatalf("执行结束后应释放预留记录")
	}
	if got := mgr.archivePool.currentActive(); got != 0 {
		t.Fatalf("执行结束后全局线程应释放，得到 %d", got)
	}
}
