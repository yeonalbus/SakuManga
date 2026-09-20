package services

import (
	"testing"
	"time"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round26 O3 端到端：maintainDedupWithProgress 完整链路
// （DB → 规则 1/2/3/4 → O3 名称聚类 → 忽略联动 → 簇输出）
// 验证：簇接线正确、忽略增量跳过/全量标记、确定性重复不受忽略影响。
// ─────────────────────────────────────────────────────────────

func TestMaintainDedupClustersE2E(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(&models.OfflineComic{}, &models.ExtraScanPath{}, &models.IgnoredIdentifier{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	// ── 数据构造 ──
	// A 组：语言版疑似重复（同名同画师，页数差 ≤5，gid 各不相同）
	comics := []models.OfflineComic{
		mkComic("a1", "[サークル] サンプルタイトル", []string{"artist:てすと", "language:japanese"}, 30),
		mkComic("a2", "[サークル] サンプルタイトル (English)", []string{"artist:てすと", "language:english"}, 31),
		mkComic("a3", "[サークル] サンプルタイトル 汉化版", []string{"artist:てすと", "language:chinese"}, 32),
		// B：续集（卷号不同 → 隔离）
		mkComic("b1", "[サークル] サンプルタイトル Vol.2", []string{"artist:てすと"}, 28),
		// C：撞名（画师不同 → 不聚）
		mkComic("c1", "サンプルタイトル", []string{"artist:someoneelse"}, 30),
		// D：同 GID 确定性重复（规则 1，不受忽略影响）
		mkComic("d1", "[D] とある本", []string{"artist:d"}, 20),
		mkComic("d2", "[D] とある本 copy", []string{"artist:d"}, 20),
	}
	for i := range comics {
		comics[i].GID = string(rune('g' + i))
		comics[i].LocalPath = "/nonexistent/" + comics[i].ID // 目录不存在 → 规则 4 签名自动跳过
	}
	comics[5].GID = "999" // d1
	comics[6].GID = "999" // d2 同 GID → 规则 1 建议删除其一
	for i := range comics {
		if err := db.Create(&comics[i]).Error; err != nil {
			t.Fatalf("创建漫画失败: %v", err)
		}
	}
	// 忽略 A 组（title 型：核心名 + 画师）
	if _, err := CreateIgnore(db, "title", "サンプルタイトル", "てすと", "", "", ""); err != nil {
		t.Fatalf("创建忽略失败: %v", err)
	}

	// ── 增量（forceFull=false）：被忽略的 A 组簇跳过 ──
	incr, err := MaintainDedupWithProgress(db, nil, nil, false)
	if err != nil {
		t.Fatalf("增量维护查重失败: %v", err)
	}
	if len(incr.Clusters) != 0 {
		t.Errorf("增量查重应跳过已忽略簇，得到 %d 簇", len(incr.Clusters))
	}
	// 确定性重复（同 GID）不受忽略影响
	if len(incr.Items) == 0 {
		t.Fatal("同 GID 确定性重复应产出建议项")
	}
	removeCount := 0
	for _, it := range incr.Items {
		if !it.Keep {
			removeCount++
			if it.Rule != "gid" {
				t.Errorf("同 GID 建议删除项的 rule 应为 gid，得到 %q", it.Rule)
			}
		}
	}
	if removeCount != 1 {
		t.Errorf("同 GID 应建议删除 1 项，得到 %d", removeCount)
	}

	// ── 全量（forceFull=true）：Round44 起不再"永久列出"已忽略项 ──
	// 语义＝"直到有变化为止"：成员全在快照内 → 增量与全量都不列出。
	full, err := MaintainDedupWithProgress(db, nil, nil, true)
	if err != nil {
		t.Fatalf("全量维护查重失败: %v", err)
	}
	for _, cl := range full.Clusters {
		if cl.TitleKey == "サンプルタイトル" && cl.Artist == "てすと" {
			t.Error("忽略且无新增时，全量核对同样不应列出该簇")
		}
		if cl.TitleKey == "サンプルタイトル vol.2" {
			t.Errorf("续集不应聚簇，得到 %q", cl.TitleKey)
		}
	}

	// ── Round44 新增感知：忽略之后新入库同指纹本子 → 该簇重新冒头 ──
	newComic := mkComic("a4", "[サークル] サンプルタイトル (Korean)", []string{"artist:てすと", "language:korean"}, 31)
	newComic.GID = "g-new"
	newComic.LocalPath = "/nonexistent/a4"
	newComic.AddedAt = time.Now() // 忽略之后入库
	if err := db.Create(&newComic).Error; err != nil {
		t.Fatalf("创建新增漫画失败: %v", err)
	}
	after, err := MaintainDedupWithProgress(db, nil, nil, false)
	if err != nil {
		t.Fatalf("新增后维护查重失败: %v", err)
	}
	foundNew := false
	for _, cl := range after.Clusters {
		if cl.TitleKey == "サンプルタイトル" && cl.Artist == "てすと" {
			foundNew = true
			if !cl.Ignored {
				t.Error("出现快照外的成员时该簇应带 Ignored=true（此前已忽略）")
			}
			if cl.IgnoredNewCount != 1 {
				t.Errorf("IgnoredNewCount 应为 1，得到 %d", cl.IgnoredNewCount)
			}
			if cl.IgnoreID == "" {
				t.Error("应带 IgnoreID（供前端「确认新增」调用）")
			}
		}
	}
	if !foundNew {
		t.Error("忽略项出现新入库成员时应重新列出该簇")
	}

	// ── 确认新增（刷新快照）→ 该簇回到静默 ──
	var ig models.IgnoredIdentifier
	if err := db.Where("type = ?", "title").First(&ig).Error; err != nil {
		t.Fatalf("读取忽略条目失败: %v", err)
	}
	if _, err := AckIgnoreSnapshot(db, ig.ID); err != nil {
		t.Fatalf("确认新增失败: %v", err)
	}
	acked, err := MaintainDedupWithProgress(db, nil, nil, false)
	if err != nil {
		t.Fatalf("确认新增后维护查重失败: %v", err)
	}
	for _, cl := range acked.Clusters {
		if cl.TitleKey == "サンプルタイトル" && cl.Artist == "てすと" {
			t.Errorf("确认新增后该簇应回到静默，不再列出（得到 %d 成员）", len(cl.Members))
		}
	}
}

// TestDetectClustersComicIgnore 成员级忽略（type=comic）：被忽略的漫画不参与聚类，
// 组内剩余成员 ≥2 时组保留并收缩，<2 时组消失。
func TestDetectClustersComicIgnore(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(&models.OfflineComic{}, &models.IgnoredIdentifier{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	comics := []models.OfflineComic{
		mkComic("m1", "[サークル] ワールド", []string{"artist:abc"}, 30),
		mkComic("m2", "[サークル] ワールド (English)", []string{"artist:abc"}, 31),
		mkComic("m3", "[サークル] ワールド 汉化版", []string{"artist:abc"}, 32),
	}
	idx := LoadIgnoreIndex(db)

	// 基线：3 成员 1 簇
	base := detectTitleClusters(comics, idx, false)
	if len(base) != 1 || len(base[0].Members) != 3 {
		t.Fatalf("基线应为 3 成员 1 簇，得到 %d 簇", len(base))
	}

	// 忽略 m2（成员级）→ 组收缩为 2 成员
	if _, err := CreateIgnore(db, "comic", "", "", "", "m2", ""); err != nil {
		t.Fatalf("创建成员级忽略失败: %v", err)
	}
	idx2 := LoadIgnoreIndex(db)
	after := detectTitleClusters(comics, idx2, false)
	if len(after) != 1 || len(after[0].Members) != 2 {
		t.Fatalf("忽略 m2 后应为 2 成员 1 簇，得到 %d 簇", len(after))
	}
	for _, m := range after[0].Members {
		if m.Comic.ID == "m2" {
			t.Error("被忽略成员 m2 不应出现在簇中")
		}
	}

	// 再忽略 m1 → 组内仅剩 1 成员 → 组消失
	if _, err := CreateIgnore(db, "comic", "", "", "", "m1", ""); err != nil {
		t.Fatalf("创建成员级忽略失败: %v", err)
	}
	idx3 := LoadIgnoreIndex(db)
	final := detectTitleClusters(comics, idx3, false)
	if len(final) != 0 {
		t.Fatalf("组内剩余 1 成员不应成簇，得到 %d 簇", len(final))
	}
}

// TestMaintainDedupParentIgnoreE2E 规则 3（父画廊）gid 型忽略端到端：
// 3b 本地父子关系发现时，被忽略的父画廊（旧版）增量不提示「旧版可删除」；全量仍提示。
func TestMaintainDedupParentIgnoreE2E(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(&models.OfflineComic{}, &models.ExtraScanPath{}, &models.IgnoredIdentifier{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	// 父画廊 P（旧版，gid=100）被子画廊 C（新版，parentGID=100）取代
	p := mkComic("p1", "[たいらー] たんプリまんが", []string{"artist:たいらー"}, 30)
	p.GID = "100"
	p.LocalPath = "/nonexistent/p1"
	c := mkComic("c1", "[たいらー] たんプリまんが (English)", []string{"artist:たいらー"}, 31)
	c.GID = "101"
	c.ParentGID = "100"
	c.LocalPath = "/nonexistent/c1"
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("创建父画廊失败: %v", err)
	}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("创建子画廊失败: %v", err)
	}

	// 基线：未忽略时，规则 3b 应建议删除旧版 P（rule=parent）
	base, err := MaintainDedupWithProgress(db, nil, nil, false)
	if err != nil {
		t.Fatalf("基线维护查重失败: %v", err)
	}
	foundParent := false
	for _, it := range base.Items {
		if !it.Keep && it.Rule == "parent" && it.Comic.ID == "p1" {
			foundParent = true
		}
	}
	if !foundParent {
		t.Fatal("基线：父画廊 P 应被规则 3 建议删除（rule=parent）")
	}

	// 忽略父画廊 gid=100
	if _, err := CreateIgnore(db, "gid", "", "", "100", "", ""); err != nil {
		t.Fatalf("创建 gid 忽略失败: %v", err)
	}

	// 增量：被忽略的父画廊不再提示
	incr, err := MaintainDedupWithProgress(db, nil, nil, false)
	if err != nil {
		t.Fatalf("增量维护查重失败: %v", err)
	}
	for _, it := range incr.Items {
		if it.Comic.ID == "p1" && !it.Keep {
			t.Errorf("增量查重不应再提示被忽略的父画廊 P（rule=%q）", it.Rule)
		}
	}

	// 全量：仍提示（不豁免）
	full, err := MaintainDedupWithProgress(db, nil, nil, true)
	if err != nil {
		t.Fatalf("全量维护查重失败: %v", err)
	}
	foundFull := false
	for _, it := range full.Items {
		if it.Comic.ID == "p1" && !it.Keep && it.Rule == "parent" {
			foundFull = true
		}
	}
	if !foundFull {
		t.Error("全量核对应仍提示被忽略的父画廊（不豁免）")
	}
}
