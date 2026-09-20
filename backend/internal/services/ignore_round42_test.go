package services

import (
	"testing"
	"time"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round42 D8：忽略表 key 迁移（兼容存量旧 key）
//
// 背景：Round42 前写入的 title_key 由旧清洗算法产出（版本标记未剥离，如 `… [中国翻訳]`）。
// 算法升级后新产出的 TitleKey 已剥净 → 必须做兼容匹配，否则用户已忽略的组会全部「复活」。
// ─────────────────────────────────────────────────────────────

func newIgnoreTestDB(t *testing.T) *gorm.DB {
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
	if err := db.AutoMigrate(&models.IgnoredIdentifier{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

func TestIgnoreIndexLegacyKeyCompat(t *testing.T) {
	db := newIgnoreTestDB(t)
	// 模拟「Round42 前」写入的忽略记录：key 里含未剥离的版本标记
	legacyKey := "女神ックス (超次元ゲイム ネプテューヌ) [中国翻訳]"
	if _, err := CreateIgnore(db, "title", legacyKey, "nyamota", "", "", ""); err != nil {
		t.Fatalf("写入存量忽略记录失败: %v", err)
	}
	idx := LoadIgnoreIndex(db)

	// 新算法产出的 TitleKey（已剥净 [中国翻訳]）→ 必须命中（否则该组会复活）
	if !idx.IsTitleIgnored("女神ックス (超次元ゲイム ネプテューヌ)", "nyamota") {
		t.Error("存量旧 key 应能与新算法 key 匹配（D8 兼容迁移）")
	}
	// 精确匹配仍需生效（旧调用方原样传 key 的场景）
	if !idx.IsTitleIgnored(legacyKey, "nyamota") {
		t.Error("精确匹配不应被破坏")
	}
	// 画师不同 → 不命中（防撞名误伤）
	if idx.IsTitleIgnored("女神ックス (超次元ゲイム ネプテューヌ)", "someone-else") {
		t.Error("画师不同时不应命中")
	}
	// 无关标题 → 不命中
	if idx.IsTitleIgnored("まったく別の作品", "nyamota") {
		t.Error("无关标题不应命中")
	}
}

func TestIgnoreIndexLegacyKeyRound42RealCases(t *testing.T) {
	db := newIgnoreTestDB(t)
	// 用真实库里的存量忽略记录形态（含汉化组 / 语言标记等）
	cases := []struct {
		legacyKey string
		artist    string
		newKey    string // Round42 新算法产出的 key
	}{
		{"月夜鴉『退魔ノ隷刻~跡ノ祭~』 【机翻+个人润色】", "masa", "月夜鴉『退魔ノ隷刻~跡ノ祭~』"},
		{"つるぺたみおしゃのぷにボディ純愛なかだしハメセックスcg集 (ホロライブ)", "dikk0", "つるぺたみおしゃのぷにボディ純愛なかだしハメセックスcg集 (ホロライブ)"},
		{"velina (zenless zone zero)", "onigirikao", "velina (zenless zone zero)"},
	}
	for _, c := range cases {
		if _, err := CreateIgnore(db, "title", c.legacyKey, c.artist, "", "", ""); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}
	idx := LoadIgnoreIndex(db)
	for _, c := range cases {
		if !idx.IsTitleIgnored(c.newKey, c.artist) {
			t.Errorf("存量 key %q（artist=%s）应与新 key %q 匹配", c.legacyKey, c.artist, c.newKey)
		}
	}
}

func TestDetectClustersIgnoreCompatLegacyKey(t *testing.T) {
	// 端到端：存量旧 key 的忽略记录，应能抑制新算法聚出的簇。
	// Round44：忽略＝"直到有变化为止"——无新增成员时增量与全量都静默；出现新成员才列出并标记。
	db := newIgnoreTestDB(t)
	if _, err := CreateIgnore(db, "title", "もう一つの世界 [中国翻訳]", "abc", "", "", ""); err != nil {
		t.Fatalf("写入忽略失败: %v", err)
	}
	idx := LoadIgnoreIndex(db)
	lc1 := mkComic("lc1", "[CIRCLE] もう一つの世界 [中国翻訳]", []string{"artist:abc"}, 30)
	lc2 := mkComic("lc2", "[CIRCLE] もう一つの世界 [無修正] [中国翻訳]", []string{"artist:abc"}, 31)
	comics := []models.OfflineComic{lc1, lc2}
	// 增量：命中忽略（含兼容匹配）→ 跳过
	if got := detectTitleClusters(comics, idx, false); len(got) != 0 {
		t.Errorf("兼容匹配应让增量查重跳过已忽略簇，得到 %d 簇", len(got))
	}
	// 全量：无新增成员 → 同样静默（Round44 撤销「永久列出」）
	if got := detectTitleClusters(comics, idx, true); len(got) != 0 {
		t.Errorf("无新增成员时全量核对也不应列出，得到 %d 簇", len(got))
	}

	// 兼容匹配 + 出现快照外的成员（且是忽略之后入库）→ 列出并标记新增
	lc2.AddedAt = time.Now()
	comics2 := []models.OfflineComic{lc1, lc2}
	idx2 := LoadIgnoreIndex(db)
	for _, e := range idx2.titleEntries {
		e.seen["lc1"] = true // 只把 lc1 视为"忽略时已知"
	}
	got := detectTitleClusters(comics2, idx2, true)
	if len(got) != 1 || !got[0].Ignored || got[0].IgnoredNewCount != 1 {
		t.Errorf("兼容匹配下出现新成员应列出并标记 NewCount=1，得到 %+v", got)
	}
}
