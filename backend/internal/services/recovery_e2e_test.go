package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round54 阶段5：恢复功能端到端集成测试（不含真实 E 站下载）
//
// 链路：扫描入库 → 磁盘目录被删 → 全量对账出候选 → 启动恢复建任务
//       → 模拟下载完成（ingest 新记录）→ finalizeUpdate 完成钩子
//       → 验证：旧记录删除、候选/阵亡清空、历史引用迁移到新记录。
//
// 真实下载（E 站账号/代理/GP）由珱垣在 UI 手动验证，不在自动化范围内。
// ─────────────────────────────────────────────────────────────

func newRecoveryE2EDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	// 完整迁移：对账/恢复/完成钩子（DeleteOfflineComic 引用迁移）涉及的全部表
	if err := db.AutoMigrate(
		&models.OfflineComic{}, &models.ExtraScanPath{},
		&models.RecoveryCandidate{}, &models.FallenComic{},
		&models.DownloadSetting{}, &models.DownloadTask{},
		&models.HistoryRecord{}, &models.Bookshelf{}, &models.ReadingList{},
		&models.ComicRating{}, &models.ComicBookmark{}, &models.ComicChapter{},
	); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

func TestRecoveryEndToEnd(t *testing.T) {
	withoutXpCloud(t)
	db := newRecoveryE2EDB(t)
	mgr := NewDownloadManager(db, nil)

	root := t.TempDir()
	galleryDir := filepath.Join(root, "gallery1")
	mustWriteFile(t, filepath.Join(galleryDir, "1.jpg"), "x")

	// 1. 扫描入库后的库内形态
	if err := db.Create(&models.ExtraScanPath{ID: "p1", Path: root, Name: "测试路径"}).Error; err != nil {
		t.Fatalf("创建路径失败: %v", err)
	}
	if err := db.Create(&models.OfflineComic{
		ID: "comic1", GID: "999", Token: "t999", Title: "测试本子",
		LocalPath: galleryDir, ScanPathID: "p1", PageCount: 1, FileSize: 1,
	}).Error; err != nil {
		t.Fatalf("创建离线记录失败: %v", err)
	}
	// 历史引用（完成后应迁移到新记录）
	if err := db.Create(&models.HistoryRecord{
		UserID: 1, ComicID: "comic1", Source: models.SourceOffline, GID: "999",
		LastReadAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("创建历史记录失败: %v", err)
	}

	// 2. 模拟磁盘被炸：删除整个本子目录（根路径仍在 → PathMissing=false）
	if err := os.RemoveAll(galleryDir); err != nil {
		t.Fatalf("删除目录失败: %v", err)
	}

	// 3. 全量扫描对账 → 产出恢复候选
	recon, err := ReconcileScanPathAgainstDisk(db, "p1", root, nil)
	if err != nil {
		t.Fatalf("对账失败: %v", err)
	}
	if recon.PathMissing {
		t.Fatalf("根路径存在时不应标记 PathMissing")
	}
	if recon.Missing != 1 || recon.Candidates != 1 {
		t.Fatalf("对账应产出 1 个候选，得到 %+v", recon)
	}
	var cand models.RecoveryCandidate
	if err := db.First(&cand, "g_id = ?", "999").Error; err != nil {
		t.Fatalf("候选应写入: %v", err)
	}
	if cand.Token != "t999" || cand.ComicID != "comic1" {
		t.Fatalf("候选快照异常: %+v", cand)
	}

	// 4. 启动恢复 → 创建 group=recovery 更新任务
	res, err := mgr.StartRecovery(RecoveryStartParams{Scheme: RecoverySchemeGallery, UserID: 1})
	if err != nil {
		t.Fatalf("启动恢复失败: %v", err)
	}
	if res.Created != 1 || res.Failed != 0 {
		t.Fatalf("应创建 1 个任务，得到 %+v", res)
	}
	var task models.DownloadTask
	if err := db.First(&task, "\"group\" = ?", RecoveryGroup).Error; err != nil {
		t.Fatalf("恢复任务应存在: %v", err)
	}
	if task.UpdateForComicID != "comic1" || !task.ForceDeleteOriginal {
		t.Fatalf("恢复任务字段异常: %+v", task)
	}

	// 恢复中：该路径应被锁定（扫描守卫）
	locked, err := IsRecoveryLockedPath(db, "p1")
	if err != nil || !locked {
		t.Fatalf("恢复中应锁定路径 p1，得到 locked=%v err=%v", locked, err)
	}

	// 5. 模拟下载完成（ingest 产物：新记录 + 落地文件）
	restoredDir := filepath.Join(root, "restored")
	mustWriteFile(t, filepath.Join(restoredDir, "1.jpg"), "x")
	if err := db.Create(&models.OfflineComic{
		ID: "new-comic", GID: "999", Token: "t999", Title: "测试本子",
		LocalPath: restoredDir, PageCount: 1, FileSize: 1,
	}).Error; err != nil {
		t.Fatalf("创建恢复后记录失败: %v", err)
	}
	task.Status = models.DownloadCompleted
	if err := db.Save(&task).Error; err != nil {
		t.Fatalf("更新任务状态失败: %v", err)
	}

	// 6. 完成钩子
	mgr.finalizeUpdate(&task)

	// 7. 断言
	var oldCount int64
	db.Model(&models.OfflineComic{}).Where("id = ?", "comic1").Count(&oldCount)
	if oldCount != 0 {
		t.Fatalf("旧记录应被删除，实际剩 %d", oldCount)
	}
	var newCount int64
	db.Model(&models.OfflineComic{}).Where("id = ?", "new-comic").Count(&newCount)
	if newCount != 1 {
		t.Fatalf("新记录应存在，实际 %d", newCount)
	}
	var candCount, fallenCount int64
	db.Model(&models.RecoveryCandidate{}).Count(&candCount)
	db.Model(&models.FallenComic{}).Count(&fallenCount)
	if candCount != 0 || fallenCount != 0 {
		t.Fatalf("恢复完成应清空候选/阵亡（候选=%d 阵亡=%d）", candCount, fallenCount)
	}
	var hist models.HistoryRecord
	if err := db.First(&hist, "comic_id = ?", "new-comic").Error; err != nil {
		t.Fatalf("历史引用应迁移到新记录: %v", err)
	}
	if hist.GID != "999" {
		t.Fatalf("迁移后历史 gid 应保留，得到 %q", hist.GID)
	}
	// 恢复完成后该路径解除锁定
	locked, err = IsRecoveryLockedPath(db, "p1")
	if err != nil || locked {
		t.Fatalf("恢复完成后路径应解除锁定，得到 locked=%v err=%v", locked, err)
	}
}
