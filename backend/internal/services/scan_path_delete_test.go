package services

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round53：删除「额外的画廊扫描路径」必须连带清理该路径下已入库的漫画
//
// 背景：DeleteScanPath 原先只删 extra_scan_paths 一行，offline_comics 中
// ScanPathID 指向该路径的记录全部残留（离线库照常展示、来源标签退化成「下载」、
// 且不再命中「关闭离线维护」集合而被重新纳入联网更新检测）。
//
// 覆盖：批量删除范围隔离 / 引用清理与同 gid 迁移 / 空 id 防误删全库 / nil db。
// ─────────────────────────────────────────────────────────────

// withoutXpCloud 屏蔽 XP 全局服务，避免测试触发后台全量重建 goroutine。
func withoutXpCloud(t *testing.T) {
	t.Helper()
	old := GlobalXpCloud
	GlobalXpCloud = nil
	t.Cleanup(func() { GlobalXpCloud = old })
}

func TestDeleteOfflineComicsByScanPathRemovesRecords(t *testing.T) {
	withoutXpCloud(t)
	db := newRefsTestDB(t)

	const (
		pathA = "path-a"
		pathB = "path-b"
	)
	now := time.Now()
	comics := []models.OfflineComic{
		{ID: "a1", GID: "100", Title: "a1", LocalPath: "/x/a1", ScanPathID: pathA},
		{ID: "a2", GID: "200", Title: "a2", LocalPath: "/x/a2", ScanPathID: pathA},
		{ID: "b1", GID: "300", Title: "b1", LocalPath: "/x/b1", ScanPathID: pathB},
		{ID: "dl", GID: "400", Title: "dl", LocalPath: "/x/dl"},          // 下载导入（ScanPathID 空）
		{ID: "a1-dl", GID: "100", Title: "a1-dl", LocalPath: "/x/a1-dl"}, // 同 gid 100 的下载导入 → 引用迁移目标
	}
	for i := range comics {
		if err := db.Create(&comics[i]).Error; err != nil {
			t.Fatalf("创建漫画 %s 失败: %v", comics[i].ID, err)
		}
	}

	userID := uint(9)
	// 历史：a1 有同 gid 的替代记录 → 迁移；a2 无替代 → 删除
	db.Create(&models.HistoryRecord{UserID: userID, ComicID: "a1", GID: "100", Source: models.SourceOffline, LastReadAt: now})
	db.Create(&models.HistoryRecord{UserID: userID, ComicID: "a2", GID: "200", Source: models.SourceOffline, LastReadAt: now})
	db.Create(&models.Bookshelf{ID: "s1", UserID: userID, Name: "S", ComicIDs: `["a1","a2","b1"]`, Count: 3, CoverComicID: "a2"})
	db.Create(&models.ReadingList{UserID: userID, Source: string(models.SourceOffline), Items: `[{"id":"a1"},{"id":"a2"},{"id":"b1"}]`})

	deleted, err := DeleteOfflineComicsByScanPath(db, pathA)
	if err != nil {
		t.Fatalf("DeleteOfflineComicsByScanPath 失败: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("应删除 2 本，实际 %d", deleted)
	}

	// 1. 只删 pathA 下的记录
	for _, id := range []string{"b1", "dl", "a1-dl"} {
		var c models.OfflineComic
		if err := db.First(&c, "id = ?", id).Error; err != nil {
			t.Errorf("记录 %s 不应被删除: %v", id, err)
		}
	}
	for _, id := range []string{"a1", "a2"} {
		var c models.OfflineComic
		if err := db.First(&c, "id = ?", id).Error; err == nil {
			t.Errorf("记录 %s 应被删除，实际仍存在", id)
		}
	}

	// 2. 历史：a1 → a1-dl（同 gid 迁移），a2 删除
	var hist []models.HistoryRecord
	db.Where("user_id = ?", userID).Find(&hist)
	if len(hist) != 1 {
		t.Fatalf("历史应只保留 1 条（迁移），得到 %d 条: %+v", len(hist), hist)
	}
	if hist[0].ComicID != "a1-dl" {
		t.Errorf("历史应迁移到 a1-dl，得到 %s", hist[0].ComicID)
	}

	// 3. 书架：a1→a1-dl、a2 剔除、b1 保留，封面 a2 因无替代被清空
	var shelf models.Bookshelf
	db.First(&shelf, "id = ?", "s1")
	shelfIDs := parseComicIDsJSON(shelf.ComicIDs)
	if len(shelfIDs) != 2 || shelfIDs[0] != "a1-dl" || shelfIDs[1] != "b1" {
		t.Errorf("书架 comicIds 应为 [a1-dl b1]，得到 %v", shelfIDs)
	}
	if shelf.Count != 2 {
		t.Errorf("书架 Count 应同步为 2，得到 %d", shelf.Count)
	}
	if shelf.CoverComicID != "" {
		t.Errorf("封面引用 a2 已删除且无替代 → 应清空，得到 %q", shelf.CoverComicID)
	}

	// 4. 阅读清单：a1→a1-dl、a2 剔除、b1 保留
	var list models.ReadingList
	db.First(&list, "user_id = ?", userID)
	items := []map[string]interface{}{}
	if err := json.Unmarshal([]byte(list.Items), &items); err != nil {
		t.Fatalf("解析阅读清单失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("阅读清单应保留 2 项，得到 %d: %s", len(items), list.Items)
	}
	if items[0]["id"] != "a1-dl" || items[1]["id"] != "b1" {
		t.Errorf("阅读清单应为 [a1-dl b1]，得到 %s", list.Items)
	}

	// 5. 重复调用幂等
	if n, err := DeleteOfflineComicsByScanPath(db, pathA); err != nil || n != 0 {
		t.Errorf("二次调用应返回 (0,nil)，得到 (%d,%v)", n, err)
	}
}

func TestDeleteOfflineComicsByScanPathGuards(t *testing.T) {
	withoutXpCloud(t)
	db := newRefsTestDB(t)
	if err := db.Create(&models.OfflineComic{ID: "keep", LocalPath: "/x/keep", Title: "keep"}).Error; err != nil {
		t.Fatalf("创建漫画失败: %v", err)
	}

	// 空/空白 id 绝不能触发「全库删除」
	for _, id := range []string{"", "   "} {
		n, err := DeleteOfflineComicsByScanPath(db, id)
		if err != nil {
			t.Fatalf("id=%q 不应报错: %v", id, err)
		}
		if n != 0 {
			t.Errorf("id=%q 应返回 0，得到 %d", id, n)
		}
	}
	// nil db 安全
	if n, err := DeleteOfflineComicsByScanPath(nil, "path-a"); err != nil || n != 0 {
		t.Errorf("nil db 应返回 (0,nil)，得到 (%d,%v)", n, err)
	}

	var count int64
	db.Model(&models.OfflineComic{}).Count(&count)
	if count != 1 {
		t.Fatalf("守卫路径不得删除任何记录，剩余 %d 条", count)
	}
}

// TestDeleteScanPathWithComicsRejectsWhileScanning 校验扫描进行中拒绝删除：
// 否则扫描 goroutine 会继续以即将删除的路径 ID 落库，留下新的孤儿记录。
func TestDeleteScanPathWithComicsRejectsWhileScanning(t *testing.T) {
	withoutXpCloud(t)
	db := newRefsTestDB(t)
	if err := db.AutoMigrate(&models.ExtraScanPath{}); err != nil {
		t.Fatalf("迁移 ExtraScanPath 失败: %v", err)
	}
	if err := db.Create(&models.ExtraScanPath{ID: "path-busy", Path: "Z:/busy"}).Error; err != nil {
		t.Fatalf("创建路径失败: %v", err)
	}
	if err := db.Create(&models.OfflineComic{
		ID: "busy1", Title: "busy1", LocalPath: "Z:/busy/1", ScanPathID: "path-busy",
	}).Error; err != nil {
		t.Fatalf("创建漫画失败: %v", err)
	}

	// 直接占位一个「扫描中」进度（同包内可访问 running）
	mgr := GetScanManager()
	const id = "path-busy"
	mgr.mu.Lock()
	mgr.running[id] = newScanProgress(id, "full")
	mgr.mu.Unlock()
	t.Cleanup(func() {
		mgr.mu.Lock()
		delete(mgr.running, id)
		mgr.mu.Unlock()
	})

	if _, err := DeleteScanPathWithComics(db, id, false); !errors.Is(err, ErrScanPathInProgress) {
		t.Fatalf("扫描中应返回 ErrScanPathInProgress，得到 %v", err)
	}
	// 拒绝时不得有任何删除
	var count int64
	db.Model(&models.OfflineComic{}).Where("id = ?", "busy1").Count(&count)
	if count != 1 {
		t.Fatalf("拒绝删除时漫画记录不应被清理")
	}
	db.Model(&models.ExtraScanPath{}).Where("id = ?", id).Count(&count)
	if count != 1 {
		t.Fatalf("拒绝删除时路径记录不应被删除")
	}

	// 扫描结束后可正常删除
	mgr.mu.Lock()
	mgr.running[id].finish(nil)
	mgr.mu.Unlock()

	removed, err := DeleteScanPathWithComics(db, id, false)
	if err != nil {
		t.Fatalf("扫描结束后删除失败: %v", err)
	}
	if removed != 1 {
		t.Errorf("应清理 1 本，得到 %d", removed)
	}
	db.Model(&models.OfflineComic{}).Count(&count)
	if count != 0 {
		t.Errorf("漫画记录应被清理，剩余 %d 条", count)
	}
	db.Model(&models.ExtraScanPath{}).Count(&count)
	if count != 0 {
		t.Errorf("路径记录应被删除，剩余 %d 条", count)
	}
}

// TestScanManagerIsScanning 校验扫描态判定（未启动 / 未完成 / 已完成）。
func TestScanManagerIsScanning(t *testing.T) {
	mgr := GetScanManager()
	const id = "test-path-is-scanning"
	mgr.mu.Lock()
	delete(mgr.running, id)
	mgr.mu.Unlock()
	t.Cleanup(func() {
		mgr.mu.Lock()
		delete(mgr.running, id)
		mgr.mu.Unlock()
	})

	if mgr.IsScanning(id) {
		t.Fatalf("从未启动扫描时应返回 false")
	}
	p := newScanProgress(id, "full")
	mgr.mu.Lock()
	mgr.running[id] = p
	mgr.mu.Unlock()
	if !mgr.IsScanning(id) {
		t.Fatalf("扫描进行中应返回 true")
	}
	p.finish(nil)
	if mgr.IsScanning(id) {
		t.Fatalf("扫描结束后应返回 false")
	}
}
