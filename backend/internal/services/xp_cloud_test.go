package services

import (
	"math"
	"strings"
	"testing"
	"time"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ─────────────────────────────────────────────────────────────
// Round32 阶段一：XP 词云统计服务单测
//
// 覆盖：tag 归一/分组口径、有效 tag 合并、信号融合、差分幂等、
// 增量 ⇄ 全量重建一致性（关键回归防线）、权重表 IDF。
// ─────────────────────────────────────────────────────────────

func newXpTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	// 内存库需固定为单连接：glebarez/sqlite 的 :memory: 是「每连接独立」，
	// 多连接会看到不同的空库（建表对其他连接不可见）。
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.OfflineComic{},
		&models.HistoryRecord{},
		&models.ComicRating{},
		&models.XpComicStat{},
		&models.XpTagStat{},
		&models.XpMeta{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}

// xpSnapshot 读取聚合表快照（key = namespace:key）
func xpSnapshot(t *testing.T, db *gorm.DB, userID uint) map[string][3]float64 {
	t.Helper()
	var rows []models.XpTagStat
	if err := db.Where("user_id = ?", userID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	out := map[string][3]float64{}
	for _, r := range rows {
		out[r.Namespace+":"+r.TagKey] = [3]float64{r.LibWeight, r.ReadWeight, float64(r.ComicCount)}
	}
	return out
}

// assertSnapshotEqual 断言两份快照一致（浮点容差）
func assertSnapshotEqual(t *testing.T, label string, want, got map[string][3]float64) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s：聚合行数不一致 want=%d got=%d\nwant=%v\ngot=%v", label, len(want), len(got), want, got)
	}
	const tol = 1e-6
	for k, w := range want {
		g, ok := got[k]
		if !ok {
			t.Fatalf("%s：缺失 tag %s", label, k)
		}
		for i := 0; i < 3; i++ {
			if math.Abs(w[i]-g[i]) > tol {
				t.Fatalf("%s：tag %s 第 %d 项不一致 want=%v got=%v", label, k, i, w[i], g[i])
			}
		}
	}
}

func TestSplitXpTagAndGroup(t *testing.T) {
	cases := []struct {
		raw     string
		wantNS  string
		wantKey string
		group   string
	}{
		{"female:big_breasts", "female", "big breasts", "core"},
		{"MALE:Anal", "male", "anal", "core"},
		{"character:arisu tachibana", "character", "arisu tachibana", "ip"},
		{"parody:touhou_project", "parody", "touhou project", "ip"},
		{"artist:some artist", "artist", "some artist", "artist"},
		{"group:circle", "group", "circle", "artist"},
		{"language:chinese", "language", "chinese", "skip"},
		{"reclass:something", "reclass", "something", "skip"},
		{"location:outdoors", "location", "outdoors", "misc"},
		{"裸词", "other", "裸词", "core"}, // 无冒号 → other（other 属核心 XP 组）
		{"", "", "", "skip"},
		{"female:", "", "", "skip"},
	}
	for _, c := range cases {
		ns, key := SplitXpTag(c.raw)
		if ns != c.wantNS || key != c.wantKey {
			t.Fatalf("SplitXpTag(%q) = (%q,%q)，期望 (%q,%q)", c.raw, ns, key, c.wantNS, c.wantKey)
		}
		if c.wantNS == "" {
			continue
		}
		if got := XpGroupOf(ns); got != c.group {
			t.Fatalf("XpGroupOf(%q) = %q，期望 %q", ns, got, c.group)
		}
	}
}

func TestXpEffectiveTagsMergeAndFallback(t *testing.T) {
	// 1. 双轨三态：(online ∪ add) − remove，并剔除 language/reclass
	comic := &models.OfflineComic{
		OnlineTags:        `["female:ahegao","male:anal","language:chinese","artist:aaa"]`,
		OfflineAddTags:    `["female:nakadashi"]`,
		OfflineRemoveTags: `["male:anal"]`,
		Tags:              `["female:should_be_ignored"]`,
	}
	got := XpEffectiveTags(comic)
	want := map[string]bool{"female:ahegao": true, "female:nakadashi": true, "artist:aaa": true}
	if len(got) != len(want) {
		t.Fatalf("有效 tag = %v，期望 %d 项", got, len(want))
	}
	for _, tag := range got {
		if !want[tag] {
			t.Fatalf("有效 tag 出现意外项：%s（全部 %v）", tag, got)
		}
	}

	// 2. 三态全空 → 回退旧 Tags 字段（兼容旧数据）
	legacy := &models.OfflineComic{Tags: `["female:yuri","reclass:x","language:english"]`}
	legacyTags := XpEffectiveTags(legacy)
	if len(legacyTags) != 1 || legacyTags[0] != "female:yuri" {
		t.Fatalf("旧数据回退口径不符：%v", legacyTags)
	}
}

func TestXpReadSignalFusion(t *testing.T) {
	db := newXpTestDB(t)
	svc := NewXpCloudService(db)

	user := models.User{Username: "u1"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	// 三本：纯库藏 / 有阅读次数 / 全信号齐备
	comics := []models.OfflineComic{
		{ID: "c1", Title: "纯库藏", OnlineTags: `["female:ahegao"]`, LocalPath: "p1"},
		{ID: "c2", Title: "读过 9 次", OnlineTags: `["female:yuri"]`, ReadCount: 9, LocalPath: "p2"},
		{ID: "c3", Title: "全信号", OnlineTags: `["female:stockings"]`, ReadCount: 4, LocalPath: "p3"},
	}
	for i := range comics {
		if err := db.Create(&comics[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	// c3：历史 + 满分评分（评分上限 10）
	if err := db.Create(&models.HistoryRecord{
		UserID: user.ID, ComicID: "c3", Source: models.SourceOffline, LastReadAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ComicRating{UserID: user.ID, ComicID: "c3", Score: 10}).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.RebuildAll(); err != nil {
		t.Fatal(err)
	}

	var s1, s2, s3 models.XpComicStat
	if err := db.Where("user_id = ? AND comic_id = ?", user.ID, "c1").First(&s1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("user_id = ? AND comic_id = ?", user.ID, "c2").First(&s2).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("user_id = ? AND comic_id = ?", user.ID, "c3").First(&s3).Error; err != nil {
		t.Fatal(err)
	}

	if s1.ReadWeight != 0 {
		t.Fatalf("纯库藏本阅读信号应为 0，实得 %v", s1.ReadWeight)
	}
	wantC2 := 0.60 * math.Log1p(9)
	if math.Abs(s2.ReadWeight-wantC2) > 1e-6 {
		t.Fatalf("c2 阅读信号 = %v，期望 %v", s2.ReadWeight, wantC2)
	}
	// c3：0.6·log1p(4) + 0.25·exp(0) + 0.15·1（历史刚发生、评分满分）
	wantC3 := 0.60*math.Log1p(4) + 0.25 + 0.15
	if math.Abs(s3.ReadWeight-wantC3) > 0.01 {
		t.Fatalf("c3 阅读信号 = %v，期望 ≈%v", s3.ReadWeight, wantC3)
	}

	// 稀释：每本只有 1 个 tag，聚合值应等于该本信号本身
	snap := xpSnapshot(t, db, user.ID)
	if math.Abs(snap["female:yuri"][1]-wantC2) > 1e-6 {
		t.Fatalf("聚合表 female:yuri 阅读权重 = %v，期望 %v", snap["female:yuri"][1], wantC2)
	}
}

func TestXpDilutionAcrossManyTags(t *testing.T) {
	db := newXpTestDB(t)
	svc := NewXpCloudService(db)

	user := models.User{Username: "u1"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	// 一本含 4 个 tag：库藏侧每个 tag 应得 1/4（稀释，防泛化标签淹没核心 tag）
	comic := models.OfflineComic{
		ID: "c1", LocalPath: "p1",
		OnlineTags: `["female:ahegao","female:big breasts","male:anal","parody:x"]`,
	}
	if err := db.Create(&comic).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.RebuildAll(); err != nil {
		t.Fatal(err)
	}

	snap := xpSnapshot(t, db, user.ID)
	for tag, row := range snap {
		if math.Abs(row[0]-0.25) > 1e-9 {
			t.Fatalf("tag %s 库藏权重 = %v，期望 0.25（1/4 稀释）", tag, row[0])
		}
		if row[2] != 1 {
			t.Fatalf("tag %s 本数 = %v，期望 1", tag, row[2])
		}
	}
}

func TestXpIncrementalMatchesRebuild(t *testing.T) {
	db := newXpTestDB(t)
	svc := NewXpCloudService(db)

	user := models.User{Username: "u1"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	seed := []models.OfflineComic{
		{ID: "c1", LocalPath: "p1", OnlineTags: `["female:ahegao","parody:x"]`, ReadCount: 3},
		{ID: "c2", LocalPath: "p2", OnlineTags: `["female:yuri","artist:aaa"]`},
		{ID: "c3", LocalPath: "p3", OnlineTags: `["male:anal","female:ahegao"]`, ReadCount: 1},
	}
	for i := range seed {
		if err := db.Create(&seed[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.RebuildAll(); err != nil {
		t.Fatal(err)
	}

	// ── 场景 1：新增一本 → 增量
	if err := db.Create(&models.OfflineComic{
		ID: "c4", LocalPath: "p4", OnlineTags: `["female:stockings","parody:x"]`, ReadCount: 5,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.RecomposeComic("c4", user.ID); err != nil {
		t.Fatal(err)
	}
	incremental := xpSnapshot(t, db, user.ID)

	// 幂等：同本重复调用不应改变聚合值
	if err := svc.RecomposeComic("c4", user.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.RecomposeComic("c4", user.ID); err != nil {
		t.Fatal(err)
	}
	assertSnapshotEqual(t, "重复增量幂等", incremental, xpSnapshot(t, db, user.ID))

	// 与全量重建比对
	if err := svc.RebuildAll(); err != nil {
		t.Fatal(err)
	}
	assertSnapshotEqual(t, "新增后 增量⇄重建", incremental, xpSnapshot(t, db, user.ID))

	// ── 场景 2：修改 tag（合并口径变化）→ 增量
	if err := db.Model(&models.OfflineComic{}).Where("id = ?", "c1").
		Updates(map[string]interface{}{
			"online_tags":         `["female:ahegao","parody:x","female:glasses"]`,
			"offline_remove_tags": `["parody:x"]`,
			"read_count":          7,
		}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.RecomposeComic("c1", user.ID); err != nil {
		t.Fatal(err)
	}
	incremental2 := xpSnapshot(t, db, user.ID)
	if _, ok := incremental2["parody:x"]; !ok {
		t.Fatalf("parody:x 仍被 c4 持有，不应消失：%v", incremental2)
	}
	if err := svc.RebuildAll(); err != nil {
		t.Fatal(err)
	}
	assertSnapshotEqual(t, "改 tag 后 增量⇄重建", incremental2, xpSnapshot(t, db, user.ID))

	// ── 场景 3：删除一本（漫画记录已不存在）→ 增量只做减法
	if err := db.Where("id = ?", "c2").Delete(&models.OfflineComic{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.RecomposeComic("c2", user.ID); err != nil {
		t.Fatal(err)
	}
	incremental3 := xpSnapshot(t, db, user.ID)
	if _, ok := incremental3["female:yuri"]; ok {
		t.Fatalf("c2 是 female:yuri 唯一持有者，删除后该 tag 行应被清理：%v", incremental3)
	}
	if _, ok := incremental3["artist:aaa"]; ok {
		t.Fatalf("c2 是 artist:aaa 唯一持有者，删除后该 tag 行应被清理：%v", incremental3)
	}
	// 共享 tag 只剩 c3 的贡献（c3 有 2 个有效 tag → 稀释后库藏权重 1/2）
	if row, ok := incremental3["male:anal"]; !ok || row[2] != 1 || math.Abs(row[0]-0.5) > 1e-9 {
		t.Fatalf("male:anal 应仅剩 c3 贡献（1 本、权重 0.5），实得 %v", row)
	}
	if err := svc.RebuildAll(); err != nil {
		t.Fatal(err)
	}
	assertSnapshotEqual(t, "删除后 增量⇄重建", incremental3, xpSnapshot(t, db, user.ID))
}

func TestXpReadSideIsolatedPerUser(t *testing.T) {
	db := newXpTestDB(t)
	svc := NewXpCloudService(db)

	u1 := models.User{Username: "u1"}
	u2 := models.User{Username: "u2"}
	if err := db.Create(&u1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&u2).Error; err != nil {
		t.Fatal(err)
	}
	// 阅读次数是全局字段（离线库共享），历史/评分按用户隔离
	if err := db.Create(&models.OfflineComic{
		ID: "c1", LocalPath: "p1", OnlineTags: `["female:ahegao"]`, ReadCount: 5,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.HistoryRecord{
		UserID: u2.ID, ComicID: "c1", Source: models.SourceOffline, LastReadAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.RebuildAll(); err != nil {
		t.Fatal(err)
	}

	var s1, s2 models.XpComicStat
	if err := db.Where("user_id = ? AND comic_id = ?", u1.ID, "c1").First(&s1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("user_id = ? AND comic_id = ?", u2.ID, "c1").First(&s2).Error; err != nil {
		t.Fatal(err)
	}
	// u2 有历史近期性加分，阅读权重必须高于 u1
	if s2.ReadWeight <= s1.ReadWeight {
		t.Fatalf("阅读侧未按用户隔离：u1=%v u2=%v", s1.ReadWeight, s2.ReadWeight)
	}
	if math.Abs(s2.ReadWeight-s1.ReadWeight-0.25) > 0.01 {
		t.Fatalf("历史近期性增益应 ≈0.25，实得 %v", s2.ReadWeight-s1.ReadWeight)
	}
}

func TestXpWeightTableIDF(t *testing.T) {
	db := newXpTestDB(t)
	svc := NewXpCloudService(db)

	user := models.User{Username: "u1"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	// 泛化 tag 出现在全部 3 本；冷门 tag 只出现在 1 本
	for i, tags := range []string{
		`["female:big breasts","female:rare a"]`,
		`["female:big breasts","female:rare b"]`,
		`["female:big breasts","female:rare c"]`,
	} {
		if err := db.Create(&models.OfflineComic{
			ID: "c" + string(rune('1'+i)), LocalPath: "p" + string(rune('1'+i)), OnlineTags: tags,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.RebuildAll(); err != nil {
		t.Fatal(err)
	}

	table, err := svc.WeightTable(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if table.Tagged != 3 {
		t.Fatalf("参与统计本数 = %d，期望 3", table.Tagged)
	}
	common := table.IDF["female:big breasts"]
	rare := table.IDF["female:rare a"]
	if !(rare > common) {
		t.Fatalf("泛化抑制失效：泛化 tag IDF=%v 应低于冷门 tag IDF=%v", common, rare)
	}
	// 库藏权重：泛化 tag 累加 3 本（每本 1/2）= 1.5 → 满量程 1.0；
	// 冷门 tag 仅 1 本（1/2）→ 归一后 0.5/1.5 = 1/3
	if math.Abs(table.Lib["female:big breasts"]-1.0) > 1e-9 {
		t.Fatalf("泛化 tag 库藏归一权重应满量程 1.0，实得 %v", table.Lib["female:big breasts"])
	}
	if math.Abs(table.Lib["female:rare a"]-1.0/3.0) > 1e-9 {
		t.Fatalf("冷门 tag 库藏归一权重应 1/3，实得 %v", table.Lib["female:rare a"])
	}
}

func TestXpQueryGroupsAndMeta(t *testing.T) {
	db := newXpTestDB(t)
	svc := NewXpCloudService(db)

	user := models.User{Username: "u1"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.OfflineComic{
		ID: "c1", LocalPath: "p1", ReadCount: 2,
		OnlineTags: `["female:ahegao","character:arisu tachibana","location:outdoors","artist:aaa","language:chinese"]`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.RebuildAll(); err != nil {
		t.Fatal(err)
	}

	core, err := svc.Query(user.ID, "core", "library", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(core.Tags) != 1 || core.Tags[0].Key != "ahegao" {
		t.Fatalf("core 分组词条不符：%+v", core.Tags)
	}
	if core.Tags[0].Name == "" {
		t.Fatal("词条缺少展示名（翻译回退失败）")
	}

	ip, err := svc.Query(user.ID, "ip", "library", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(ip.Tags) != 1 || ip.Tags[0].Namespace != "character" {
		t.Fatalf("ip 分组词条不符：%+v", ip.Tags)
	}

	misc, err := svc.Query(user.ID, "misc", "library", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(misc.Tags) != 1 || misc.Tags[0].Namespace != "location" {
		t.Fatalf("misc 分组应只含未知命名空间（location），实得 %+v", misc.Tags)
	}

	// 画师/社团不进词云，进 artists 列表
	if len(misc.Artists) != 1 || misc.Artists[0].Namespace != "artist" {
		t.Fatalf("artists 列表不符：%+v", misc.Artists)
	}
	for _, tag := range core.Tags {
		if tag.Namespace == "artist" || tag.Namespace == "group" {
			t.Fatalf("画师不应出现在词云词条中：%+v", tag)
		}
	}

	// 元信息
	if misc.Meta.TotalComics != 1 || misc.Meta.TaggedComics != 1 {
		t.Fatalf("元信息不符：%+v", misc.Meta)
	}
	if misc.Meta.ReadSignalComics != 1 {
		t.Fatalf("阅读信号覆盖本数应为 1，实得 %d", misc.Meta.ReadSignalComics)
	}

	// 阅读视图：按阅读权重排序且不含零信号词条
	reading, err := svc.Query(user.ID, "core", "reading", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(reading.Tags) != 1 || reading.Tags[0].ReadWeight <= 0 {
		t.Fatalf("阅读视图不符：%+v", reading.Tags)
	}
}

func TestCleanTagDisplayName(t *testing.T) {
	cases := []struct{ in, want string }{
		// EH 翻译词典真实数据：图标 markdown + 中文名
		{
			`![长筒袜图标](https://raw.githubusercontent.com/wiki/EhTagTranslation/Database/database-icon/stockings.webp)长筒袜`,
			"长筒袜",
		},
		{`![](https://x/y.png)Fate/Grand Order`, "Fate/Grand Order"},
		{`![大船](https://x/y.png)`, "大船"}, // 纯图标 → 退回 alt
		{`![alt]`, "alt"},               // 残片（无 url）→ 退回 alt
		{`巨乳`, "巨乳"},                   // 普通文本原样
		{``, ""},
	}
	for _, c := range cases {
		if got := CleanTagDisplayName(c.in); got != c.want {
			t.Fatalf("CleanTagDisplayName(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestXpQueryCleansIconMarkdown(t *testing.T) {
	db := newXpTestDB(t)
	svc := NewXpCloudService(db)

	user := models.User{Username: "u1"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	// 词条名必须不含 markdown 图标语法（否则前端词云会画出整段 URL）
	if err := db.Create(&models.OfflineComic{
		ID: "c1", LocalPath: "p1", OnlineTags: `["female:stockings","artist:aaa"]`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.RebuildAll(); err != nil {
		t.Fatal(err)
	}
	result, err := svc.Query(user.ID, "core", "library", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tags) == 0 {
		t.Fatal("无词条")
	}
	for _, tag := range result.Tags {
		if strings.Contains(tag.Name, "![") || strings.Contains(tag.Name, "http") {
			t.Fatalf("词条展示名未清洗：%q", tag.Name)
		}
		if strings.TrimSpace(tag.Name) == "" {
			t.Fatalf("词条展示名为空：%+v", tag)
		}
	}
}

func TestXpDirtyFlagTriggersRebuild(t *testing.T) {
	db := newXpTestDB(t)
	svc := NewXpCloudService(db)

	user := models.User{Username: "u1"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.OfflineComic{ID: "c1", LocalPath: "p1", OnlineTags: `["female:ahegao"]`}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.RebuildAll(); err != nil {
		t.Fatal(err)
	}

	// 绕过增量直接改库（模拟增量链路漏接/失败），置脏后查询应自动重建
	if err := db.Create(&models.OfflineComic{ID: "c2", LocalPath: "p2", OnlineTags: `["female:yuri"]`}).Error; err != nil {
		t.Fatal(err)
	}
	svc.markDirty()

	result, err := svc.Query(user.ID, "core", "library", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tags) != 2 {
		t.Fatalf("脏标记未触发重建：词条数 = %d，期望 2（%+v）", len(result.Tags), result.Tags)
	}
	var meta models.XpMeta
	if err := db.First(&meta, 1).Error; err != nil {
		t.Fatal(err)
	}
	if meta.Dirty {
		t.Fatal("重建后脏标记应被清除")
	}
}
