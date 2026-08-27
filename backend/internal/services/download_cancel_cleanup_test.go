package services

import (
	"os"
	"path/filepath"
	"testing"

	"SakuHentai/internal/models"
)

// ─────────────────────────────────────────────────────────────
// BUG1：解压失败任务「取消」需删除损坏压缩包与半解压残留目录
//
// 线上现象：归档下载完成但 zip 内部数据损坏（目录头完整），解压失败后任务报 error；
// 此时 zip 保留。用户点「取消」后坏 zip 仍残留，下次同画廊下载时 run() 第 3 步
// isValidZip（仅验目录头）误判「完整」，跳过下载直接解压坏文件，反复解压失败。
// 修复：CancelTask 对「archive + error + Error 以『解压失败』开头」的任务，
// 删除 zipPath 与 extractPath 下的半解压残留目录；其余错误不删（避免误删好 zip 浪费 GP）。
// ─────────────────────────────────────────────────────────────

// makeCorruptArchiveTask 构造解压失败（error）归档任务，并预置坏 zip 与半解压残留目录。
func makeCorruptArchiveTask(t *testing.T, mgr *DownloadManager) (*models.DownloadTask, string, string) {
	t.Helper()
	archiveDir := t.TempDir()
	extractDir := t.TempDir()
	task := &models.DownloadTask{
		ID:          "cancel-cleanup-1",
		Status:      models.DownloadError,
		Error:       "解压失败: 解压 \"031_14.jpg\" 失败: zip: not a valid zip file",
		Mode:        models.DownloadModeArchive,
		GID:         "4148459",
		Token:       "tok",
		Title:       "test title",
		UserID:      1,
		ArchivePath: archiveDir,
		ExtractPath: extractDir,
	}
	if err := mgr.db.Create(task).Error; err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	dirName := "archive - 4148459 - test title"
	zipPath := filepath.Join(archiveDir, dirName+".zip")
	if err := os.WriteFile(zipPath, []byte("not a real zip"), 0o644); err != nil {
		t.Fatalf("预置坏 zip 失败: %v", err)
	}
	extract := filepath.Join(extractDir, dirName)
	if err := os.MkdirAll(extract, 0o755); err != nil {
		t.Fatalf("预置残留目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(extract, "001.jpg"), []byte("partial"), 0o644); err != nil {
		t.Fatalf("预置残留文件失败: %v", err)
	}
	return task, zipPath, extract
}

// TestCancelTaskRemovesCorruptArchive 解压失败任务取消：坏 zip 与半解压残留目录必须被删除。
func TestCancelTaskRemovesCorruptArchive(t *testing.T) {
	mgr := newTestDownloadManager(t)
	task, zipPath, extractDir := makeCorruptArchiveTask(t, mgr)

	if _, err := mgr.CancelTask(task.ID); err != nil {
		t.Fatalf("取消任务失败: %v", err)
	}

	if _, err := os.Stat(zipPath); !os.IsNotExist(err) {
		t.Errorf("取消后损坏 zip 仍存在: %s", zipPath)
	}
	if _, err := os.Stat(extractDir); !os.IsNotExist(err) {
		t.Errorf("取消后半解压残留目录仍存在: %s", extractDir)
	}

	var after models.DownloadTask
	if err := mgr.db.First(&after, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("回读任务失败: %v", err)
	}
	if after.Status != models.DownloadCancelled {
		t.Errorf("任务状态应为 cancelled，得到 %s", after.Status)
	}
}

// TestCancelTaskKeepsZipForNonExtractError 非解压失败错误（如 HTTP 错误）取消时不得删除 zip——
// zip 可能是完好数据，误删会浪费 GP/配额。
func TestCancelTaskKeepsZipForNonExtractError(t *testing.T) {
	mgr := newTestDownloadManager(t)
	archiveDir := t.TempDir()
	task := &models.DownloadTask{
		ID:          "cancel-keep-1",
		Status:      models.DownloadError,
		Error:       "HTTP 404（下载 H@H zip 失败）",
		Mode:        models.DownloadModeArchive,
		GID:         "999",
		Token:       "tok",
		Title:       "keep",
		UserID:      1,
		ArchivePath: archiveDir,
		ExtractPath: t.TempDir(),
	}
	if err := mgr.db.Create(task).Error; err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	zipPath := filepath.Join(archiveDir, "archive - 999 - keep.zip")
	if err := os.WriteFile(zipPath, []byte("good zip data"), 0o644); err != nil {
		t.Fatalf("预置 zip 失败: %v", err)
	}

	if _, err := mgr.CancelTask(task.ID); err != nil {
		t.Fatalf("取消任务失败: %v", err)
	}
	if _, err := os.Stat(zipPath); err != nil {
		t.Errorf("非解压失败取消不应删除 zip: %v", err)
	}
}

// TestCancelTaskDownloadingKeepsPart 下载中取消：.part 断点续传文件应保留（供下次任务续传）。
func TestCancelTaskDownloadingKeepsPart(t *testing.T) {
	mgr := newTestDownloadManager(t)
	archiveDir := t.TempDir()
	task := &models.DownloadTask{
		ID:          "cancel-part-1",
		Status:      models.DownloadDownloading,
		Mode:        models.DownloadModeArchive,
		GID:         "888",
		Token:       "tok",
		Title:       "part keep",
		UserID:      1,
		ArchivePath: archiveDir,
		ExtractPath: t.TempDir(),
	}
	if err := mgr.db.Create(task).Error; err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	partPath := filepath.Join(archiveDir, "archive - 888 - part keep.zip.part")
	if err := os.WriteFile(partPath, []byte("partial download"), 0o644); err != nil {
		t.Fatalf("预置 .part 失败: %v", err)
	}

	if _, err := mgr.CancelTask(task.ID); err != nil {
		t.Fatalf("取消任务失败: %v", err)
	}
	if _, err := os.Stat(partPath); err != nil {
		t.Errorf("下载中取消不应删除 .part: %v", err)
	}
}
