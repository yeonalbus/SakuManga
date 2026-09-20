package services

import (
	"testing"
	"time"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ─────────────────────────────────────────────────────────────
// Round44：忽略清单成员视图（宽松口径 + 同组标注）+ 成员快照 / 新增感知 / 回填
// ─────────────────────────────────────────────────────────────

func newIgnoreMemberTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(&models.OfflineComic{}, &models.IgnoredIdentifier{}, &models.ExtraScanPath{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

func TestListIgnoresWithMembersLooseScope(t *testing.T) {
	db := newIgnoreMemberTestDB(t)

	// 同一核心名 + 同画师，4 本：第01話 两本（同卷 → 构成重复组）、第02/03話 各一本
	comics := []models.OfflineComic{
		mkComic("m1", "[CIRCLE] テスト作品 第01話", []string{"artist:tester", "language:japanese"}, 20),
		mkComic("m2", "[CIRCLE] テスト作品 第01話 (English)", []string{"artist:tester", "language:english"}, 21),
		mkComic("m3", "[CIRCLE] テスト作品 第02話", []string{"artist:tester"}, 22),
		mkComic("m4", "[CIRCLE] テスト作品 第03話", []string{"artist:tester"}, 23),
	}
	for i := range comics {
		comics[i].Source = models.SourceOffline
		comics[i].LocalPath = "/nonexistent/" + comics[i].ID
		if err := db.Create(&comics[i]).Error; err != nil {
			t.Fatalf("创建漫画失败: %v", err)
		}
	}

	if _, err := CreateIgnore(db, "title", "テスト作品", "tester", "", "", ""); err != nil {
		t.Fatalf("创建忽略失败: %v", err)
	}

	list, err := ListIgnoresWithMembers(db)
	if err != nil {
		t.Fatalf("读取忽略清单失败: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("应有 1 条忽略，得到 %d", len(list))
	}
	it := list[0]
	// 宽松口径：4 本全部列出（含不成组的散本）
	if it.MatchedCount != 4 {
		t.Errorf("宽松口径应覆盖 4 本，得到 %d", it.MatchedCount)
	}
	if it.GroupCount != 1 || it.GroupedCount != 2 {
		t.Errorf("应识别 1 个重复组 / 2 本同组，得到 groupCount=%d groupedCount=%d", it.GroupCount, it.GroupedCount)
	}
	if it.NewCount != 0 {
		t.Errorf("刚创建忽略时不应有新增，得到 %d", it.NewCount)
	}
	grouped := map[string]bool{}
	for _, m := range it.Members {
		grouped[m.Comic.ID] = m.Grouped
	}
	if !grouped["m1"] || !grouped["m2"] {
		t.Error("第01話 的两本应标记为同组（会构成重复）")
	}
	if grouped["m3"] || grouped["m4"] {
		t.Error("第02/03話 各一本不应标记为同组")
	}

	// ── 新增感知：忽略之后新入库同卷本子 → 该成员标记 isNew ──
	newComic := mkComic("m5", "[CIRCLE] テスト作品 第01話 (Chinese)", []string{"artist:tester", "language:chinese"}, 20)
	newComic.Source = models.SourceOffline
	newComic.LocalPath = "/nonexistent/m5"
	newComic.AddedAt = time.Now() // 忽略之后入库
	if err := db.Create(&newComic).Error; err != nil {
		t.Fatalf("创建新增漫画失败: %v", err)
	}
	list, _ = ListIgnoresWithMembers(db)
	it = list[0]
	if it.MatchedCount != 5 || it.NewCount != 1 {
		t.Fatalf("应覆盖 5 本且 1 本为新增，得到 matched=%d new=%d", it.MatchedCount, it.NewCount)
	}
	newFlagged := false
	for _, m := range it.Members {
		if m.Comic.ID == "m5" && m.IsNew {
			newFlagged = true
		}
	}
	if !newFlagged {
		t.Error("新入库的 m5 应带 isNew 标记")
	}

	// ── 确认新增：快照刷新为当前集合 → 不再有新增 ──
	if _, err := AckIgnoreSnapshot(db, it.ID); err != nil {
		t.Fatalf("确认新增失败: %v", err)
	}
	list, _ = ListIgnoresWithMembers(db)
	if list[0].NewCount != 0 {
		t.Errorf("确认新增后不应再有新增，得到 %d", list[0].NewCount)
	}

	// ── 回填：快照被清空（存量数据形态）→ BackfillIgnoreSnapshots 按当前匹配集合补齐 ──
	if err := db.Model(&models.IgnoredIdentifier{}).Where("id = ?", it.ID).
		Update("seen_comic_ids", "").Error; err != nil {
		t.Fatalf("清空快照失败: %v", err)
	}
	if n := BackfillIgnoreSnapshots(db); n != 1 {
		t.Errorf("应回填 1 条，得到 %d", n)
	}
	list, _ = ListIgnoresWithMembers(db)
	if list[0].NewCount != 0 {
		t.Errorf("回填后不应有新增（快照＝当前集合），得到 %d", list[0].NewCount)
	}
}

func TestCreateIgnoreSnapshotWritten(t *testing.T) {
	db := newIgnoreMemberTestDB(t)
	c := mkComic("s1", "[CIRCLE] スナップショット作品", []string{"artist:snap"}, 10)
	c.Source = models.SourceOffline
	c.LocalPath = "/nonexistent/s1"
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("创建漫画失败: %v", err)
	}
	rec, err := CreateIgnore(db, "title", "スナップショット作品", "snap", "", "", "")
	if err != nil {
		t.Fatalf("创建忽略失败: %v", err)
	}
	seen := parseSeenComicIDs(rec.SeenComicIDs)
	if !seen["s1"] {
		t.Errorf("创建忽略时应写入成员快照，得到 %q", rec.SeenComicIDs)
	}
	// gid / comic 型不写快照
	if r2, err := CreateIgnore(db, "gid", "", "", "12345", "", ""); err != nil || r2.SeenComicIDs != "" {
		t.Errorf("gid 型不应写成员快照（err=%v）", err)
	}
}
