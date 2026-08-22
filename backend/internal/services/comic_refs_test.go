package services

import (
	"strings"
	"testing"
	"time"

	"SakuHentai/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Round20-Bug1/Bug4：漫画删除/替换后的引用清理与迁移（历史/书架/离线阅读清单）

func newRefsTestDB(t *testing.T) *gorm.DB {
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
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(
		&models.HistoryRecord{},
		&models.Bookshelf{},
		&models.ReadingList{},
		&models.OfflineComic{},
	); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

func TestCleanupComicReferencesMigrate(t *testing.T) {
	db := newRefsTestDB(t)

	// 历史（同用户同 gid 两条，指向新旧 id）+ 书架 + 离线阅读清单
	userID := uint(7)
	now := time.Now()
	db.Create(&models.HistoryRecord{UserID: userID, ComicID: "old-md5", GID: "12345", Source: models.SourceOffline, LastReadAt: now})
	db.Create(&models.HistoryRecord{UserID: userID, ComicID: "new-md5", GID: "12345", Source: models.SourceOffline, LastReadAt: now})
	db.Create(&models.Bookshelf{ID: "s1", UserID: userID, Name: "A", ComicIDs: `["old-md5","other"]`, Count: 2})
	db.Create(&models.ReadingList{UserID: userID, Source: string(models.SourceOffline), Items: `[{"id":"old-md5","title":"x"},{"id":"other","title":"y"}]`})

	CleanupComicReferences(db, "old-md5", "new-md5")

	// 1. 历史：旧 id 全部迁移为新 id
	var hist []models.HistoryRecord
	db.Where("user_id = ?", userID).Find(&hist)
	if len(hist) != 2 {
		t.Fatalf("历史应保留 2 条（迁移而非删除），得到 %d", len(hist))
	}
	for _, r := range hist {
		if r.ComicID != "new-md5" {
			t.Errorf("历史 comic_id 应迁移为 new-md5，得到 %s", r.ComicID)
		}
	}

	// 2. 书架：old-md5 → new-md5（不重复）
	var shelf models.Bookshelf
	db.First(&shelf, "id = ?", "s1")
	ids := parseComicIDsJSON(shelf.ComicIDs)
	if len(ids) != 2 || ids[0] != "new-md5" || ids[1] != "other" {
		t.Errorf("书架 comicIds 迁移错误: %v", ids)
	}

	// 3. 离线阅读清单：id 字段迁移
	var rl models.ReadingList
	db.Where("user_id = ?", userID).First(&rl)
	if !strings.Contains(rl.Items, "new-md5") || strings.Contains(rl.Items, "old-md5") {
		t.Errorf("阅读清单迁移错误: %s", rl.Items)
	}
}

func TestCleanupComicReferencesDelete(t *testing.T) {
	db := newRefsTestDB(t)

	userID := uint(8)
	db.Create(&models.HistoryRecord{UserID: userID, ComicID: "gone-id", Source: models.SourceOffline, LastReadAt: time.Now()})
	// 在线来源的历史不受影响（comic_id 即 gid，不随本地路径变化）
	db.Create(&models.HistoryRecord{UserID: userID, ComicID: "4045732", Source: models.SourceOnline, LastReadAt: time.Now()})
	db.Create(&models.Bookshelf{ID: "s2", UserID: userID, Name: "B", ComicIDs: `["gone-id","keep"]`, Count: 2})
	db.Create(&models.ReadingList{UserID: userID, Source: string(models.SourceOffline), Items: `[{"id":"gone-id"}]`})

	CleanupComicReferences(db, "gone-id", "")

	var hist []models.HistoryRecord
	db.Find(&hist)
	if len(hist) != 1 || hist[0].ComicID != "4045732" {
		t.Fatalf("孤儿离线历史应被删除、在线历史应保留: %+v", hist)
	}

	var shelf models.Bookshelf
	db.First(&shelf, "id = ?", "s2")
	ids := parseComicIDsJSON(shelf.ComicIDs)
	if len(ids) != 1 || ids[0] != "keep" {
		t.Errorf("书架应剔除 gone-id: %v", ids)
	}

	var rl models.ReadingList
	db.Where("user_id = ?", userID).First(&rl)
	if rl.Items != "[]" {
		t.Errorf("阅读清单应剔除 gone-id: %s", rl.Items)
	}
}

func TestFindReplacementByGID(t *testing.T) {
	db := newRefsTestDB(t)
	db.Create(&models.OfflineComic{ID: "a", GID: "999", LocalPath: "/x/a"})
	db.Create(&models.OfflineComic{ID: "b", GID: "999", LocalPath: "/x/b"})

	if got := FindReplacementByGID(db, "999", "a"); got != "b" {
		t.Errorf("应找到替换记录 b，得到 %q", got)
	}
	if got := FindReplacementByGID(db, "999", "b"); got != "a" {
		t.Errorf("应找到替换记录 a，得到 %q", got)
	}
	if got := FindReplacementByGID(db, "not-exist", "a"); got != "" {
		t.Errorf("不存在的 gid 应返回空，得到 %q", got)
	}
}
