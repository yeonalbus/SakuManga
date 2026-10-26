package services

import (
	"errors"
	"strings"
	"testing"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round45：降级范围收敛回归测试
//
// 规则（珱垣拍板）：
//   - 普通下载（UpdateForComicID 为空）：归档失败一律报错可重试，绝不降级为画廊逐图；
//   - 更新任务（UpdateForComicID 非空）：保留降级能力，仍由 autoUpdateFallbackToGallery 开关决定。
// ─────────────────────────────────────────────────────────────

// newArchiveTaskForFallback 构造归档模式任务（可指定是否更新任务与初始状态）
func newArchiveTaskForFallback(t *testing.T, mgr *DownloadManager, id, updateFor string, status models.DownloadTaskStatus) *models.DownloadTask {
	t.Helper()
	task := &models.DownloadTask{
		ID:               id,
		GID:              "g-" + id,
		Token:            "t",
		Title:            "title-" + id,
		UserID:           1,
		Mode:             models.DownloadModeArchive,
		ArchiveType:      models.ArchiveTypeOriginal,
		Status:           status,
		UpdateForComicID: updateFor,
	}
	if err := mgr.db.Create(task).Error; err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	return task
}

// setAutoUpdateFallback 写入 autoUpdateFallbackToGallery 开关（走真实设置单例）
func setAutoUpdateFallback(t *testing.T, mgr *DownloadManager, on bool) {
	t.Helper()
	s := mgr.GetSettings()
	s.AutoUpdateFallbackToGallery = on
	if err := mgr.db.Save(s).Error; err != nil {
		t.Fatalf("保存下载设置失败: %v", err)
	}
}

// reloadTask 从 DB 读回任务
func reloadTask(t *testing.T, mgr *DownloadManager, id string) models.DownloadTask {
	t.Helper()
	var got models.DownloadTask
	if err := mgr.db.First(&got, "id = ?", id).Error; err != nil {
		t.Fatalf("读取任务 %s 失败: %v", id, err)
	}
	return got
}

// TestFallbackRefusedForNormalDownload 普通下载调用 fallbackArchiveToGallery 必须被拒绝：
// mode 保持 archive、状态不被改回 queued、不触发重新入队。
func TestFallbackRefusedForNormalDownload(t *testing.T) {
	mgr := newTestDownloadManager(t)
	setAutoUpdateFallback(t, mgr, true) // 即便开关开启，普通下载也不许降级
	task := newArchiveTaskForFallback(t, mgr, "normal-fallback", "", models.DownloadError)

	mgr.fallbackArchiveToGallery(task, "测试：普通下载不应降级")

	got := reloadTask(t, mgr, task.ID)
	if got.Mode != models.DownloadModeArchive {
		t.Fatalf("普通下载不应降级，mode 期望 archive，得到 %s", got.Mode)
	}
	if got.Status != models.DownloadError {
		t.Fatalf("普通下载不应被重新排队，status 期望 error，得到 %s", got.Status)
	}
	if got.ArchiveType != models.ArchiveTypeOriginal {
		t.Fatalf("普通下载的 archiveType 不应被清空，得到 %q", got.ArchiveType)
	}
}

// TestHandleHathdlOnlyNormalDownloadFails 普通下载遇「结构性不可直链」：置 error（文案含可重试），不降级。
func TestHandleHathdlOnlyNormalDownloadFails(t *testing.T) {
	mgr := newTestDownloadManager(t)
	setAutoUpdateFallback(t, mgr, true) // 开关开启也不影响普通下载
	task := newArchiveTaskForFallback(t, mgr, "normal-hath", "", models.DownloadDownloading)
	g := &archiveDownloader{m: mgr, task: task}

	g.handleHathdlOnly(errors.New("测试：H@H 下载页无 #db > p > a 直链"))

	got := reloadTask(t, mgr, task.ID)
	if got.Mode != models.DownloadModeArchive {
		t.Fatalf("普通下载不应降级，mode 期望 archive，得到 %s", got.Mode)
	}
	if got.Status != models.DownloadError {
		t.Fatalf("普通下载失败应置 error，得到 %s", got.Status)
	}
	if !strings.Contains(got.Error, "可重试") {
		t.Fatalf("错误文案应提示可重试，得到 %q", got.Error)
	}
}

// TestHandleHathdlOnlyUpdateSwitchOffFails 更新任务 + 开关关闭：保持 error，不降级。
func TestHandleHathdlOnlyUpdateSwitchOffFails(t *testing.T) {
	mgr := newTestDownloadManager(t)
	setAutoUpdateFallback(t, mgr, false)
	task := newArchiveTaskForFallback(t, mgr, "update-off", "comic-1", models.DownloadDownloading)
	g := &archiveDownloader{m: mgr, task: task}

	g.handleHathdlOnly(errors.New("测试：H@H 下载页无直链"))

	got := reloadTask(t, mgr, task.ID)
	if got.Mode != models.DownloadModeArchive {
		t.Fatalf("开关关闭时更新任务不应降级，mode 期望 archive，得到 %s", got.Mode)
	}
	if got.Status != models.DownloadError {
		t.Fatalf("开关关闭时更新任务应置 error，得到 %s", got.Status)
	}
}

// TestHandleHathdlOnlyUpdateSwitchOnFallsBack 更新任务 + 开关开启：保留降级（mode=gallery 且重新排队）。
func TestHandleHathdlOnlyUpdateSwitchOnFallsBack(t *testing.T) {
	mgr := newTestDownloadManager(t)
	setAutoUpdateFallback(t, mgr, true)
	task := newArchiveTaskForFallback(t, mgr, "update-on", "comic-2", models.DownloadDownloading)
	g := &archiveDownloader{m: mgr, task: task}

	g.handleHathdlOnly(errors.New("测试：H@H 下载页无直链"))

	got := reloadTask(t, mgr, task.ID)
	if got.Mode != models.DownloadModeGallery {
		t.Fatalf("开关开启时更新任务应降级为画廊，得到 mode=%s", got.Mode)
	}
	if got.Status != models.DownloadQueued {
		t.Fatalf("降级后应重新排队（queued），得到 status=%s", got.Status)
	}
	if got.ArchiveType != "" {
		t.Fatalf("降级后 archiveType 应清空，得到 %q", got.ArchiveType)
	}
}
