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
// Round51：忽略清单成员视图（按组成员）+ 破坏性清理（旧语义条目）
// ─────────────────────────────────────────────────────────────

func newIgnoreMemberTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	// 内存库必须固定为单连接：glebarez/sqlite 的 :memory: 是「每连接独立库」，
	// 若走连接池，迁移/写入的表现在其他连接上不可见（同包其他 db 辅助一致的处理）。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&models.OfflineComic{}, &models.IgnoredIdentifier{}, &models.ExtraScanPath{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

// TestListIgnoresWithMembersGroupScope Round51：清单成员 = 忽略集合中的现存成员（按组，不再按名字）
func TestListIgnoresWithMembersGroupScope(t *testing.T) {
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

	// 忽略「第01話 组」：Round51 按组成员，传该簇成员 {m1, m2}
	if _, err := CreateIgnore(db, "title", "テスト作品", "tester", "", "", []string{"m1", "m2"}); err != nil {
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
	// 只展示集合成员（2 本），不再按名字把散本也拉进来
	if it.MatchedCount != 2 {
		t.Errorf("应覆盖集合内 2 本，得到 %d", it.MatchedCount)
	}
	if it.GroupCount != 1 || it.GroupedCount != 2 {
		t.Errorf("应识别 1 个重复组 / 2 本同组，得到 groupCount=%d groupedCount=%d", it.GroupCount, it.GroupedCount)
	}
	grouped := map[string]bool{}
	for _, m := range it.Members {
		grouped[m.Comic.ID] = m.Grouped
	}
	if !grouped["m1"] || !grouped["m2"] {
		t.Error("第01話 的两本应标记为同组（会构成重复）")
	}

	// ── 新增感知：忽略之后新入库同组本子 m5 → 维护缓存里该组显示新增 1 本 ──
	newComic := mkComic("m5", "[CIRCLE] テスト作品 第01話 (Chinese)", []string{"artist:tester", "language:chinese"}, 20)
	newComic.Source = models.SourceOffline
	newComic.LocalPath = "/nonexistent/m5"
	newComic.AddedAt = time.Now().Add(time.Second) // 明确晚于忽略创建时间
	if err := db.Create(&newComic).Error; err != nil {
		t.Fatalf("创建新增漫画失败: %v", err)
	}
	var all []models.OfflineComic
	if err := db.Where("source = ?", models.SourceOffline).Find(&all).Error; err != nil {
		t.Fatalf("读取漫画失败: %v", err)
	}
	clusters := detectTitleClusters(all, LoadIgnoreIndex(db))
	StoreMaintainDedupResult(&DedupResult{Clusters: clusters}, false)
	list, _ = ListIgnoresWithMembers(db)
	it = list[0]
	if it.NewCount != 1 {
		t.Fatalf("新入库的 m5 应计入新增 1 本，得到 %d", it.NewCount)
	}

	// ── 确认新增：把 m5 并入集合 → 该组回到静默 ──
	if _, err := AckIgnoreSnapshot(db, it.ID, []string{"m5"}); err != nil {
		t.Fatalf("确认新增失败: %v", err)
	}
	clusters = detectTitleClusters(all, LoadIgnoreIndex(db))
	StoreMaintainDedupResult(&DedupResult{Clusters: clusters}, false)
	list, _ = ListIgnoresWithMembers(db)
	if list[0].NewCount != 0 {
		t.Errorf("确认新增后不应再有新增，得到 %d", list[0].NewCount)
	}
}

// TestCreateIgnoreMembersWritten Round51：创建时写入成员集合；无成员 / 非 title 类型校验
func TestCreateIgnoreMembersWritten(t *testing.T) {
	db := newIgnoreMemberTestDB(t)

	if _, err := CreateIgnore(db, "title", "作品", "snap", "", "", nil); err == nil {
		t.Error("title 型未提供成员时应报错")
	}
	if _, err := CreateIgnore(db, "comic", "", "", "s1", "", nil); err == nil {
		t.Error("comic 型已移除，应报错")
	}

	rec, err := CreateIgnore(db, "title", "スナップショット作品", "snap", "", "", []string{"s1", "s2", "s1"})
	if err != nil {
		t.Fatalf("创建忽略失败: %v", err)
	}
	seen := parseSeenComicIDs(rec.SeenComicIDs)
	if len(seen) != 2 || !seen["s1"] || !seen["s2"] {
		t.Errorf("成员集合应去重为 {s1,s2}，得到 %q", rec.SeenComicIDs)
	}
	if rec.SnapshotVersion != ignoreSnapshotVersion {
		t.Errorf("新条目快照版本应为 %d，得到 %d", ignoreSnapshotVersion, rec.SnapshotVersion)
	}

	// gid 型不写成员集合
	if r2, err := CreateIgnore(db, "gid", "", "", "12345", "", nil); err != nil || r2.SeenComicIDs != "" {
		t.Errorf("gid 型不应写成员集合（err=%v）", err)
	}
}

// TestPurgeLegacyIgnores 破坏性更新：启动时清空旧语义条目（version<2），保留新条目（幂等）
func TestPurgeLegacyIgnores(t *testing.T) {
	db := newIgnoreMemberTestDB(t)
	old := models.IgnoredIdentifier{
		ID: "ignore-old", Type: "title", TitleKey: "旧作品", Artist: "a",
		SeenComicIDs: `["x1"]`, CreatedAt: time.Now(), SnapshotVersion: 0,
	}
	if err := db.Create(&old).Error; err != nil {
		t.Fatalf("写入旧条目失败: %v", err)
	}
	fresh, err := CreateIgnore(db, "title", "新作品", "b", "", "", []string{"y1"})
	if err != nil {
		t.Fatalf("创建新条目失败: %v", err)
	}

	if n := PurgeLegacyIgnores(db); n != 1 {
		t.Errorf("应清理 1 条旧条目，得到 %d", n)
	}
	if err := db.First(&models.IgnoredIdentifier{}, "id = ?", "ignore-old").Error; err == nil {
		t.Error("旧条目应已被删除")
	}
	if err := db.First(&models.IgnoredIdentifier{}, "id = ?", fresh.ID).Error; err != nil {
		t.Error("新条目不应被清理")
	}
	// 幂等：再清一次为 0
	if n := PurgeLegacyIgnores(db); n != 0 {
		t.Errorf("重复清理应为 0，得到 %d", n)
	}
}
