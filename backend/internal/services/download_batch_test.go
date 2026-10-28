package services

import (
	"errors"
	"testing"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round47：下载列表批量操作 + 状态统计
// ─────────────────────────────────────────────────────────────

func createBatchTask(t *testing.T, mgr *DownloadManager, id string, userID uint, status models.DownloadTaskStatus, mode models.DownloadMode) *models.DownloadTask {
	t.Helper()
	task := &models.DownloadTask{
		ID:     id,
		UserID: userID,
		GID:    id,
		Token:  "t",
		Title:  "task-" + id,
		Status: status,
		Mode:   mode,
	}
	if err := mgr.db.Create(task).Error; err != nil {
		t.Fatalf("创建任务 %s 失败: %v", id, err)
	}
	return task
}

func taskStatus(t *testing.T, mgr *DownloadManager, id string) models.DownloadTaskStatus {
	t.Helper()
	var task models.DownloadTask
	if err := mgr.db.First(&task, "id = ?", id).Error; err != nil {
		t.Fatalf("读取任务 %s 失败: %v", id, err)
	}
	return task.Status
}

func TestTaskStatsScopesAndCounts(t *testing.T) {
	mgr := newTestDownloadManager(t)

	createBatchTask(t, mgr, "u1-queued-g", 1, models.DownloadQueued, models.DownloadModeGallery)
	createBatchTask(t, mgr, "u1-queued-a", 1, models.DownloadQueued, models.DownloadModeArchive)
	createBatchTask(t, mgr, "u1-paused-g", 1, models.DownloadPaused, models.DownloadModeGallery)
	createBatchTask(t, mgr, "u1-completed-g", 1, models.DownloadCompleted, models.DownloadModeGallery)
	createBatchTask(t, mgr, "u1-error-g", 1, models.DownloadError, models.DownloadModeGallery)
	createBatchTask(t, mgr, "u2-queued-g", 2, models.DownloadQueued, models.DownloadModeGallery)
	createBatchTask(t, mgr, "u2-lock-a", 2, models.DownloadErrorLock, models.DownloadModeArchive)

	// 普通用户视角：只统计自己的任务
	stats, err := mgr.TaskStats("", 1)
	if err != nil {
		t.Fatalf("TaskStats 失败: %v", err)
	}
	if stats.Total != 5 || stats.Active != 4 {
		t.Fatalf("用户 1 总览应为 total=5 active=4，得到 total=%d active=%d", stats.Total, stats.Active)
	}
	if stats.Counts[string(models.DownloadQueued)] != 2 ||
		stats.Counts[string(models.DownloadPaused)] != 1 ||
		stats.Counts[string(models.DownloadCompleted)] != 1 ||
		stats.Counts[string(models.DownloadError)] != 1 {
		t.Fatalf("用户 1 状态计数不正确: %+v", stats.Counts)
	}

	// 模式过滤
	gallery, err := mgr.TaskStats("gallery", 1)
	if err != nil {
		t.Fatalf("TaskStats(gallery) 失败: %v", err)
	}
	if gallery.Total != 4 || gallery.Active != 3 ||
		gallery.Counts[string(models.DownloadQueued)] != 1 ||
		gallery.Counts[string(models.DownloadPaused)] != 1 {
		t.Fatalf("画廊模式统计不正确: total=%d active=%d counts=%+v", gallery.Total, gallery.Active, gallery.Counts)
	}

	// 管理员视角：全部用户
	all, err := mgr.TaskStats("all", 0)
	if err != nil {
		t.Fatalf("TaskStats(all) 失败: %v", err)
	}
	if all.Total != 7 || all.Active != 6 ||
		all.Counts[string(models.DownloadQueued)] != 3 ||
		all.Counts[string(models.DownloadErrorLock)] != 1 {
		t.Fatalf("管理员统计不正确: total=%d active=%d counts=%+v", all.Total, all.Active, all.Counts)
	}

	// 7 个状态 key 必须始终存在（前端页签直接取值）
	for _, status := range []models.DownloadTaskStatus{
		models.DownloadQueued, models.DownloadDownloading, models.DownloadPaused,
		models.DownloadCompleted, models.DownloadError, models.DownloadErrorLock, models.DownloadCancelled,
	} {
		if _, ok := all.Counts[string(status)]; !ok {
			t.Fatalf("统计结果缺少状态 %s", status)
		}
	}
}

func TestBatchActionPauseResumeAndUserIsolation(t *testing.T) {
	mgr := newTestDownloadManager(t)

	createBatchTask(t, mgr, "u1-q1", 1, models.DownloadQueued, models.DownloadModeGallery)
	createBatchTask(t, mgr, "u1-q2", 1, models.DownloadQueued, models.DownloadModeGallery)
	createBatchTask(t, mgr, "u1-q3", 1, models.DownloadQueued, models.DownloadModeArchive)
	createBatchTask(t, mgr, "u2-q1", 2, models.DownloadQueued, models.DownloadModeGallery)

	res, err := mgr.BatchAction(BatchActionParams{Action: "pause", Status: "queued", Mode: "all", UserID: 1})
	if err != nil {
		t.Fatalf("批量暂停失败: %v", err)
	}
	if res.Affected != 3 || res.Failed != 0 || res.Skipped != 0 {
		t.Fatalf("批量暂停结果不正确: %+v", res)
	}
	if taskStatus(t, mgr, "u1-q1") != models.DownloadPaused || taskStatus(t, mgr, "u1-q2") != models.DownloadPaused {
		t.Fatal("用户 1 的排队任务应全部暂停")
	}
	if taskStatus(t, mgr, "u2-q1") != models.DownloadQueued {
		t.Fatal("普通用户批量暂停不应影响其他用户的任务")
	}

	res, err = mgr.BatchAction(BatchActionParams{Action: "resume", Status: "paused", Mode: "archive", UserID: 1})
	if err != nil {
		t.Fatalf("批量恢复失败: %v", err)
	}
	if res.Affected != 1 {
		t.Fatalf("归档模式应只恢复 1 个任务，得到 %+v", res)
	}
	if taskStatus(t, mgr, "u1-q3") != models.DownloadQueued {
		t.Fatal("归档暂停任务应恢复为排队")
	}
	if taskStatus(t, mgr, "u1-q1") != models.DownloadPaused {
		t.Fatal("画廊任务不应被归档模式的批量恢复处理")
	}
}

func TestBatchActionRetryCancelAndSkip(t *testing.T) {
	mgr := newTestDownloadManager(t)

	createBatchTask(t, mgr, "err1", 1, models.DownloadError, models.DownloadModeGallery)
	createBatchTask(t, mgr, "err2", 1, models.DownloadError, models.DownloadModeArchive)
	createBatchTask(t, mgr, "lock1", 1, models.DownloadErrorLock, models.DownloadModeGallery)
	createBatchTask(t, mgr, "lock2", 1, models.DownloadErrorLock, models.DownloadModeGallery)

	res, err := mgr.BatchAction(BatchActionParams{Action: "retry", Status: "error", Mode: "all", UserID: 1})
	if err != nil {
		t.Fatalf("批量重试失败: %v", err)
	}
	if res.Affected != 2 {
		t.Fatalf("应重试 2 个出错任务，得到 %+v", res)
	}
	if taskStatus(t, mgr, "err1") != models.DownloadQueued || taskStatus(t, mgr, "err2") != models.DownloadQueued {
		t.Fatal("出错任务应重试为排队")
	}

	res, err = mgr.BatchAction(BatchActionParams{Action: "cancel", Status: "error_lock", Mode: "all", UserID: 1})
	if err != nil {
		t.Fatalf("批量取消失败: %v", err)
	}
	if res.Affected != 2 {
		t.Fatalf("应取消 2 个锁定任务，得到 %+v", res)
	}
	if taskStatus(t, mgr, "lock1") != models.DownloadCancelled || taskStatus(t, mgr, "lock2") != models.DownloadCancelled {
		t.Fatal("锁定任务应取消")
	}

	// 已取消任务不会再被同一批操作处理（用于验证来源状态复查）
	res, err = mgr.BatchAction(BatchActionParams{Action: "cancel", Status: "error_lock", Mode: "all", UserID: 1})
	if err != nil {
		t.Fatalf("重复批量取消失败: %v", err)
	}
	if res.Affected != 0 || res.Skipped != 0 || res.Failed != 0 {
		t.Fatalf("重复批量取消应无任务可处理，得到 %+v", res)
	}
}

func TestBatchActionModeFilter(t *testing.T) {
	mgr := newTestDownloadManager(t)
	createBatchTask(t, mgr, "g1", 1, models.DownloadQueued, models.DownloadModeGallery)
	createBatchTask(t, mgr, "a1", 1, models.DownloadQueued, models.DownloadModeArchive)

	res, err := mgr.BatchAction(BatchActionParams{Action: "pause", Status: "queued", Mode: "gallery", UserID: 1})
	if err != nil {
		t.Fatalf("批量暂停失败: %v", err)
	}
	if res.Affected != 1 {
		t.Fatalf("画廊模式应只暂停 1 个任务，得到 %+v", res)
	}
	if taskStatus(t, mgr, "g1") != models.DownloadPaused {
		t.Fatal("画廊任务应暂停")
	}
	if taskStatus(t, mgr, "a1") != models.DownloadQueued {
		t.Fatal("归档任务不应被画廊模式处理")
	}
}

func TestBatchActionRejectsInvalidTarget(t *testing.T) {
	mgr := newTestDownloadManager(t)

	cases := []BatchActionParams{
		{Action: "pause", Status: "error", UserID: 1},
		{Action: "unknown", Status: "queued", UserID: 1},
		{Action: "pause", Status: "queued", Mode: "video", UserID: 1},
	}
	for _, p := range cases {
		if _, err := mgr.BatchAction(p); !errors.Is(err, ErrInvalidBatchAction) {
			t.Fatalf("参数 %+v 应返回 ErrInvalidBatchAction，得到 %v", p, err)
		}
	}
}

func TestBatchActionUnlockWithoutEHService(t *testing.T) {
	mgr := newTestDownloadManager(t)
	createBatchTask(t, mgr, "lock1", 1, models.DownloadErrorLock, models.DownloadModeGallery)
	createBatchTask(t, mgr, "lock2", 1, models.DownloadErrorLock, models.DownloadModeArchive)

	res, err := mgr.BatchAction(BatchActionParams{Action: "unlock", Status: "error_lock", Mode: "all", UserID: 1})
	if err != nil {
		t.Fatalf("批量解锁不应 panic/返回系统错误: %v", err)
	}
	if res.Affected != 0 || res.Failed != 2 {
		t.Fatalf("未初始化 E 站服务时解锁应全部失败，得到 %+v", res)
	}
	if len(res.Errors) == 0 {
		t.Fatal("解锁失败应返回可读错误")
	}
	if taskStatus(t, mgr, "lock1") != models.DownloadErrorLock || taskStatus(t, mgr, "lock2") != models.DownloadErrorLock {
		t.Fatal("解锁失败时任务状态不应改变")
	}
}
