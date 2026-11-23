package services

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round53 阶段二：D14 两阶段原子删除 + 关联表清理
// ─────────────────────────────────────────────────────────────

func setupDeleteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newRefsTestDB(t) // 已迁移 OfflineComic/HistoryRecord/Bookshelf/ReadingList
	if err := db.AutoMigrate(&models.ComicRating{}, &models.ComicBookmark{}, &models.ComicChapter{}); err != nil {
		t.Fatalf("迁移关联表失败: %v", err)
	}
	withoutXpCloud(t)
	return db
}

// seedDeleteComic 建目录（1.jpg）与记录，返回记录 ID 与目录
func seedDeleteComic(t *testing.T, db *gorm.DB, parent, name string, fn func(*models.OfflineComic)) (string, string) {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "1.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatalf("写图片失败: %v", err)
	}
	id := generateID(dir)
	c := &models.OfflineComic{ID: id, Title: name, LocalPath: dir, PageCount: 1}
	if fn != nil {
		fn(c)
	}
	if err := db.Create(c).Error; err != nil {
		t.Fatalf("创建记录失败: %v", err)
	}
	return id, dir
}

func TestDeleteStagesFileToTrashThenRemoves(t *testing.T) {
	db := setupDeleteDB(t)
	parent := t.TempDir()
	id, dir := seedDeleteComic(t, db, parent, "comic", nil)

	if err := DeleteOfflineComic(db, id, true); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	var count int64
	db.Model(&models.OfflineComic{}).Where("id = ?", id).Count(&count)
	if count != 0 {
		t.Errorf("记录应已删除")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("原目录应已删除")
	}
	// 回收目录应被清理（.saku_trash 目录本身可保留，内部无残留）
	entries, _ := os.ReadDir(filepath.Join(parent, trashDirName))
	if len(entries) != 0 {
		t.Errorf("回收目录应已清空，实际残留 %d 项", len(entries))
	}
}

// TestDeleteRenameFailureKeepsRecordAndFile 第一阶段失败（回收目录不可建）→ DB 未动。
func TestDeleteRenameFailureKeepsRecordAndFile(t *testing.T) {
	db := setupDeleteDB(t)
	parent := t.TempDir()
	id, dir := seedDeleteComic(t, db, parent, "comic", nil)
	// 用同名普通文件占住回收目录位置，使 MkdirAll 失败
	if err := os.WriteFile(filepath.Join(parent, trashDirName), []byte("x"), 0o644); err != nil {
		t.Fatalf("占位失败: %v", err)
	}

	err := DeleteOfflineComic(db, id, true)
	if err == nil {
		t.Fatalf("第一阶段失败时应返回错误")
	}
	var count int64
	db.Model(&models.OfflineComic{}).Where("id = ?", id).Count(&count)
	if count != 1 {
		t.Errorf("第一阶段失败时记录应保留")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "1.jpg")); statErr != nil {
		t.Errorf("第一阶段失败时文件应保留: %v", statErr)
	}
}

// TestDeleteCommitFailureRestoresFile DB 事务失败 → 文件 Rename 回原路径，记录保留。
func TestDeleteCommitFailureRestoresFile(t *testing.T) {
	db := setupDeleteDB(t)
	parent := t.TempDir()
	id, dir := seedDeleteComic(t, db, parent, "comic", nil)

	// 注入删除回调失败（仅 offline_comics）
	if err := db.Callback().Delete().Before("gorm:delete").Register("test_fail_delete", func(tx *gorm.DB) {
		if tx.Statement.Table == "offline_comics" {
			_ = tx.AddError(errors.New("injected delete failure"))
		}
	}); err != nil {
		t.Fatalf("注册回调失败: %v", err)
	}

	if err := DeleteOfflineComic(db, id, true); err == nil {
		t.Fatalf("事务失败时应返回错误")
	}

	var count int64
	db.Model(&models.OfflineComic{}).Where("id = ?", id).Count(&count)
	if count != 1 {
		t.Errorf("事务失败时记录应保留")
	}
	if _, err := os.Stat(filepath.Join(dir, "1.jpg")); err != nil {
		t.Errorf("事务失败时文件应已恢复原位: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(parent, trashDirName))
	if len(entries) != 0 {
		t.Errorf("回滚后回收目录应无残留，实际 %d 项", len(entries))
	}
}

func TestDeleteWithoutFileSkipsTrash(t *testing.T) {
	db := setupDeleteDB(t)
	parent := t.TempDir()
	id, dir := seedDeleteComic(t, db, parent, "comic", nil)

	if err := DeleteOfflineComic(db, id, false); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	var count int64
	db.Model(&models.OfflineComic{}).Where("id = ?", id).Count(&count)
	if count != 0 {
		t.Errorf("记录应已删除")
	}
	if _, err := os.Stat(filepath.Join(dir, "1.jpg")); err != nil {
		t.Errorf("deleteFile=false 时文件应保留: %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, trashDirName)); !os.IsNotExist(err) {
		t.Errorf("deleteFile=false 时不应创建回收目录")
	}
}

// TestDeleteMigratesAuxiliaryAndRefs 同 gid 有替代记录：评分/书架/权重迁移，书签章节删除。
func TestDeleteMigratesAuxiliaryAndRefs(t *testing.T) {
	db := setupDeleteDB(t)
	parent := t.TempDir()
	oldID, _ := seedDeleteComic(t, db, parent, "old", func(c *models.OfflineComic) {
		c.GID = "100"
	})
	// 替代记录（同 gid）
	newDir := filepath.Join(parent, "new")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	newID := generateID(newDir)
	if err := db.Create(&models.OfflineComic{ID: newID, GID: "100", Title: "new", LocalPath: newDir, PageCount: 1}).Error; err != nil {
		t.Fatalf("创建替代记录失败: %v", err)
	}

	// 关联数据
	db.Create(&models.ComicRating{UserID: 1, ComicID: oldID, Score: 7})
	db.Create(&models.ComicBookmark{ComicID: oldID, PageIndex: 0})
	db.Create(&models.ComicChapter{ComicID: oldID, Level: 1, Title: "c", PageIndex: 0})
	db.Create(&models.HistoryRecord{UserID: 1, ComicID: oldID, GID: "100", Source: models.SourceOffline, LastReadAt: time.Now()})
	db.Create(&models.Bookshelf{
		ID: "s1", UserID: 1, Name: "S",
		ComicIDs:     `["` + oldID + `","other"]`,
		SortKeys:     `{"` + oldID + `":5000,"other":0}`,
		CoverComicID: oldID,
		Count:        2,
	})

	// 封面/页缓存（相对 cwd）
	if err := os.MkdirAll(coverCacheDir, 0o755); err != nil {
		t.Fatalf("创建封面缓存目录失败: %v", err)
	}
	coverFile := filepath.Join(coverCacheDir, oldID+".jpg")
	if err := os.WriteFile(coverFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("写封面缓存失败: %v", err)
	}
	pageDir := filepath.Join(pageCacheDir, oldID)
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		t.Fatalf("创建页缓存目录失败: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(coverCacheDir)
		_ = os.RemoveAll(pageCacheDir)
	})

	if err := DeleteOfflineComic(db, oldID, true); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	// 评分迁移到新 id
	var oldRatings, newRatings int64
	db.Model(&models.ComicRating{}).Where("comic_id = ?", oldID).Count(&oldRatings)
	db.Model(&models.ComicRating{}).Where("user_id = ? AND comic_id = ?", 1, newID).Count(&newRatings)
	if oldRatings != 0 || newRatings != 1 {
		t.Errorf("评分应迁移到新 id（old=%d new=%d）", oldRatings, newRatings)
	}
	// 书签/章节删除
	var bookmarks, chapters int64
	db.Model(&models.ComicBookmark{}).Where("comic_id = ?", oldID).Count(&bookmarks)
	db.Model(&models.ComicChapter{}).Where("comic_id = ?", oldID).Count(&chapters)
	if bookmarks != 0 || chapters != 0 {
		t.Errorf("书签/章节应删除（bookmarks=%d chapters=%d）", bookmarks, chapters)
	}
	// 书架 comicIds / sortKeys / cover 迁移
	var shelf models.Bookshelf
	if err := db.First(&shelf, "id = ?", "s1").Error; err != nil {
		t.Fatalf("读取书架失败: %v", err)
	}
	if shelf.ComicIDs != `["`+newID+`","other"]` {
		t.Errorf("书架 comicIds 应迁移，实际 %s", shelf.ComicIDs)
	}
	if shelf.CoverComicID != newID {
		t.Errorf("书架封面应迁移到新 id，实际 %q", shelf.CoverComicID)
	}
	var keys map[string]float64
	if err := json.Unmarshal([]byte(shelf.SortKeys), &keys); err != nil {
		t.Fatalf("解析 sortKeys 失败: %v（raw=%s）", err, shelf.SortKeys)
	}
	if _, ok := keys[oldID]; ok {
		t.Errorf("sortKeys 应删除旧 id，实际 %v", keys)
	}
	if w, ok := keys[newID]; !ok || w != 5000 {
		t.Errorf("sortKeys 应迁移旧权重到新 id，实际 %v", keys)
	}
	// 封面缓存删除
	if _, err := os.Stat(coverFile); !os.IsNotExist(err) {
		t.Errorf("封面缓存应删除")
	}
	// 页缓存目录删除
	if _, err := os.Stat(pageDir); !os.IsNotExist(err) {
		t.Errorf("页缓存目录应删除")
	}
}

// TestDeleteRemovesAuxiliaryWithoutReplacement 无替代记录：评分/书签/章节/权重全部清除。
func TestDeleteRemovesAuxiliaryWithoutReplacement(t *testing.T) {
	db := setupDeleteDB(t)
	parent := t.TempDir()
	id, _ := seedDeleteComic(t, db, parent, "solo", nil)

	db.Create(&models.ComicRating{UserID: 1, ComicID: id, Score: 7})
	db.Create(&models.Bookshelf{ID: "s1", UserID: 1, Name: "S", ComicIDs: `["` + id + `"]`, SortKeys: `{"` + id + `":5000}`, Count: 1})

	if err := DeleteOfflineComic(db, id, true); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	var ratings int64
	db.Model(&models.ComicRating{}).Where("comic_id = ?", id).Count(&ratings)
	if ratings != 0 {
		t.Errorf("无替代记录时评分应删除，实际 %d", ratings)
	}
	var shelf models.Bookshelf
	if err := db.First(&shelf, "id = ?", "s1").Error; err != nil {
		t.Fatal(err)
	}
	if shelf.ComicIDs != "[]" || shelf.SortKeys != "{}" || shelf.Count != 0 {
		t.Errorf("书架引用应清空，实际 ids=%s keys=%s count=%d", shelf.ComicIDs, shelf.SortKeys, shelf.Count)
	}
}

// TestDeleteBatchRollsBackOnStageFailure 批量删除：第二条文件阶段失败 → 第一条文件恢复、DB 未删。
func TestDeleteBatchRollsBackOnStageFailure(t *testing.T) {
	db := setupDeleteDB(t)
	parent := t.TempDir()
	id1, dir1 := seedDeleteComic(t, db, parent, "ok", func(c *models.OfflineComic) { c.ScanPathID = "path-x" })

	badParent := t.TempDir()
	if err := os.WriteFile(filepath.Join(badParent, trashDirName), []byte("x"), 0o644); err != nil {
		t.Fatalf("占位失败: %v", err)
	}
	// 前置自检：存在的目标 + 回收目录位置被文件占用 → 必须失败
	probeDir := filepath.Join(badParent, "probe")
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		t.Fatalf("创建 probe 失败: %v", err)
	}
	if _, err := stageToTrash(probeDir, "probe"); err == nil {
		t.Fatalf("占位文件应导致 stageToTrash 失败（回收目录位置被文件占用）")
	}
	id2, dir2 := seedDeleteComic(t, db, badParent, "bad", func(c *models.OfflineComic) { c.ScanPathID = "path-x" })

	if _, err := DeleteOfflineComicsByScanPath(db, "path-x"); err == nil {
		t.Fatalf("批量删除应失败")
	}

	var count int64
	db.Model(&models.OfflineComic{}).Where("id IN ?", []string{id1, id2}).Count(&count)
	if count != 2 {
		t.Errorf("批量失败时两条记录都应保留，实际 %d", count)
	}
	if _, err := os.Stat(filepath.Join(dir1, "1.jpg")); err != nil {
		t.Errorf("已移动的第一条文件应恢复: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir2, "1.jpg")); err != nil {
		t.Errorf("第二条文件应保持原位: %v", err)
	}
}

// TestFinalizeUpdateMigratesReferences 更新删旧版走统一删除链：引用迁移到新版而非清空。
func TestFinalizeUpdateMigratesReferences(t *testing.T) {
	mgr := newTestDownloadManager(t)
	if err := mgr.db.AutoMigrate(&models.OfflineComic{}, &models.HistoryRecord{}, &models.Bookshelf{},
		&models.ComicRating{}, &models.ComicBookmark{}, &models.ComicChapter{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	withoutXpCloud(t)

	parent := t.TempDir()
	oldID, oldDir := seedDeleteComic(t, mgr.db, parent, "old", func(c *models.OfflineComic) { c.GID = "100" })
	newDir := filepath.Join(parent, "new")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	newID := generateID(newDir)
	if err := mgr.db.Create(&models.OfflineComic{ID: newID, GID: "100", Title: "new", LocalPath: newDir, PageCount: 1}).Error; err != nil {
		t.Fatalf("创建新版失败: %v", err)
	}
	mgr.db.Create(&models.HistoryRecord{UserID: 1, ComicID: oldID, GID: "100", Source: models.SourceOffline, LastReadAt: time.Now()})
	mgr.db.Create(&models.ComicRating{UserID: 1, ComicID: oldID, Score: 9})

	task := &models.DownloadTask{ID: "t1", GID: "100", UpdateForComicID: oldID, Status: models.DownloadCompleted, ForceDeleteOriginal: true}
	mgr.finalizeUpdate(task)

	var count int64
	mgr.db.Model(&models.OfflineComic{}).Where("id = ?", oldID).Count(&count)
	if count != 0 {
		t.Errorf("旧版记录应删除")
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("旧版目录应删除")
	}
	mgr.db.Model(&models.HistoryRecord{}).Where("comic_id = ?", newID).Count(&count)
	if count != 1 {
		t.Errorf("历史应迁移到新版，实际 %d", count)
	}
	mgr.db.Model(&models.ComicRating{}).Where("user_id = ? AND comic_id = ?", 1, newID).Count(&count)
	if count != 1 {
		t.Errorf("评分应迁移到新版，实际 %d", count)
	}
}

func TestRecoverPendingTrash(t *testing.T) {
	parent := t.TempDir()
	local := filepath.Join(parent, "comic")
	trash := filepath.Join(parent, trashDirName, "c-1")
	if err := os.MkdirAll(trash, 0o755); err != nil {
		t.Fatalf("创建回收目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(trash, "1.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatalf("写回收文件失败: %v", err)
	}

	oldFile := pendingTrashFile
	pendingTrashFile = filepath.Join(t.TempDir(), "pending.json")
	t.Cleanup(func() { pendingTrashFile = oldFile })
	recordPendingTrash([]pendingTrashEntry{{TrashPath: trash, LocalPath: local}})

	if n := RecoverPendingTrash(); n != 1 {
		t.Fatalf("应恢复 1 条，实际 %d", n)
	}
	if _, err := os.Stat(filepath.Join(local, "1.jpg")); err != nil {
		t.Errorf("文件应恢复到原路径: %v", err)
	}
	if _, err := os.Stat(pendingTrashFile); !os.IsNotExist(err) {
		t.Errorf("全部恢复后 pending 清单应删除")
	}
}
