package services

import (
	"strings"
	"testing"
	"time"

	"SakuManga/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round26 O3：疑似重复（名称级）清洗层 / 判定层单测
// ─────────────────────────────────────────────────────────────

// mkComic 构造测试漫画（OnlineTags 传 tag 数组）
func mkComic(id, title string, tags []string, pageCount int) models.OfflineComic {
	return models.OfflineComic{
		ID:                id,
		Title:             title,
		OnlineTags:        MarshalTagSlice(tags),
		PageCount:         pageCount,
		OriginalPageCount: pageCount,
	}
}

// ── 清洗层 ──

func TestFingerprintTitleBasic(t *testing.T) {
	cases := []struct {
		title  string
		artist string
		core   string
		volume string
	}{
		{"[milky] サンプル (C94) [Digital]", "milky", "サンプル", ""},
		{"(C94) [サークル] サンプルタイトル", "circle", "サンプルタイトル", ""},
		{"サンプル 第2話", "", "サンプル", "2"},
		{"サンプル 第3巻", "", "サンプル", "3"},
		{"サンプル vol.3", "", "サンプル", "3"},
		{"サンプル Part 2", "", "サンプル", "2"},
		{"サンプル 後編", "", "サンプル", "後編"},
		{"サンプル 上巻", "", "サンプル", "上巻"},
		{"サンプル 総集編", "", "サンプル", "合集"},
		{"[TOMO] 深夜のレポート (reupload)", "TOMO", "深夜のレポート", ""},
		{"[萌你妹汉化组] サンプル (中文)", "", "サンプル", ""},
		{"サンプル (English)", "", "サンプル", ""},
		{"サンプル DL版", "", "サンプル", ""},
		{"[CIRCLE] とある本 (Fate)", "", "とある本 (fate)", ""},
	}
	for _, c := range cases {
		fp := fingerprintTitle(c.title, c.artist)
		if fp.Core != c.core {
			t.Errorf("标题 %q：核心名 = %q，期望 %q", c.title, fp.Core, c.core)
		}
		if fp.Volume != c.volume {
			t.Errorf("标题 %q：卷号 = %q，期望 %q", c.title, fp.Volume, c.volume)
		}
	}
}

func TestFingerprintTitleRoman(t *testing.T) {
	// 罗马数字独立 token 转阿拉伯
	fp := fingerprintTitle("サンプル II", "")
	if fp.Core != "サンプル 2" {
		t.Errorf("罗马数字 II 应转为 2，得到 %q", fp.Core)
	}
	// 内嵌罗马数字的作品名（独立 token 规则下不误转数字，保持原样）
	fp2 := fingerprintTitle("Fate Grand Order III", "")
	if fp2.Core == "" {
		t.Error("内嵌罗马数字不应导致核心名为空")
	}
}

func TestFingerprintTitleCaseWidth(t *testing.T) {
	// 全角/大小写归一
	fp := fingerprintTitle("［ＣＩＲＣＬＥ］ Ｓａｍｐｌｅ (Ｃ９４)", "")
	if fp.Core != "sample" {
		t.Errorf("全角/大小写归一失败，核心名 = %q", fp.Core)
	}
}

func TestFingerprintTitleNoFalseStrip(t *testing.T) {
	// Round26 审查修复：英文版本标记必须括号包裹才剥离，防误伤作品名内嵌单词
	cases := []struct {
		title string
		core  string
	}{
		{"[CIRCLE] Digital Devil Story", "digital devil story"},     // digital 是作品名一部分，不剥
		{"[CIRCLE] Chinese School Girl", "chinese school girl"},     // chinese 是作品名一部分，不剥
		{"[CIRCLE] English Breakfast Club", "english breakfast club"}, // english 是作品名一部分，不剥
		{"[CIRCLE] Digital Devil Story (English)", "digital devil story"}, // 括号包裹的 (English) 剥离
		{"[CIRCLE] Game (Remaster)", "game"},                        // 括号包裹的 (Remaster) 剥离
		{"[CIRCLE] Game Remaster", "game remaster"},                 // 无括号 Remaster 保留
	}
	for _, c := range cases {
		fp := fingerprintTitle(c.title, "")
		if fp.Core != c.core {
			t.Errorf("标题 %q：核心名 = %q，期望 %q", c.title, fp.Core, c.core)
		}
	}
}

func TestCreateIgnoreValidation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(&models.IgnoredIdentifier{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	// 非法类型
	if _, err := CreateIgnore(db, "bogus", "k", "", "", "", ""); err == nil {
		t.Error("非法 type 应报错")
	}
	// title 型缺核心名
	if _, err := CreateIgnore(db, "title", "  ", "", "", "", ""); err == nil {
		t.Error("title 型缺核心名应报错")
	}
	// gid 型缺 gid
	if _, err := CreateIgnore(db, "gid", "", "", "", "", ""); err == nil {
		t.Error("gid 型缺 gid 应报错")
	}
	// 幂等：同 key 返回同一条
	a, err := CreateIgnore(db, "title", "テスト", "abc", "", "", "")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	b, err := CreateIgnore(db, "title", "テスト", "abc", "", "", "")
	if err != nil {
		t.Fatalf("重复创建失败: %v", err)
	}
	if a.ID != b.ID {
		t.Errorf("幂等应返回同一条，得到 %s vs %s", a.ID, b.ID)
	}
	// 恢复：不存在的 id 容错
	if err := RestoreIgnore(db, "not-exist"); err != ErrIgnoreNotFound {
		t.Errorf("恢复不存在的条目应返回 ErrIgnoreNotFound，得到 %v", err)
	}
	if err := RestoreIgnore(db, a.ID); err != nil {
		t.Errorf("恢复失败: %v", err)
	}
}

// ── 判定层 ──

func TestDetectClustersMultiLanguage(t *testing.T) {
	// 多语言版本（标题保留原名 + 语言后缀，E 站常见形态）→ 聚组成功；
	// 跨语言「翻译标题」（如日文片假名 vs 英文意译）字符层面无关，名称级查重不覆盖（宁漏不误）。
	comics := []models.OfflineComic{
		mkComic("a1", "[サークル] サンプルタイトル", []string{"artist:てすと", "language:japanese"}, 32),
		mkComic("a2", "[サークル] サンプルタイトル (English)", []string{"artist:てすと", "language:english"}, 34),
		mkComic("a3", "[サークル] サンプルタイトル 汉化版", []string{"artist:てすと", "language:chinese"}, 33),
	}
	clusters := detectTitleClusters(comics, nil, false)
	if len(clusters) != 1 {
		t.Fatalf("多语言版本应聚成 1 簇，得到 %d 簇", len(clusters))
	}
	cl := clusters[0]
	if len(cl.Members) != 3 {
		t.Errorf("簇成员应为 3，得到 %d", len(cl.Members))
	}
	if cl.Confidence != "high" {
		t.Errorf("页数差 ≤5 应为 high 置信，得到 %s", cl.Confidence)
	}
	if cl.Artist != "てすと" {
		t.Errorf("画师应从 artist:xxx 提取，得到 %q", cl.Artist)
	}
	if cl.Members[1].Lang != "english" {
		t.Errorf("成员语言应从 language:xxx 提取，得到 %q", cl.Members[1].Lang)
	}
}

func TestDetectClustersTranslatedTitleNotClustered(t *testing.T) {
	// 跨语言翻译标题（字符层面无关）→ 不聚组（保守，避免误报）
	comics := []models.OfflineComic{
		mkComic("g1", "[サークル] サンプルタイトル", []string{"artist:てすと"}, 32),
		mkComic("g2", "[Circle] Sample Title (English)", []string{"artist:てすと"}, 34),
	}
	clusters := detectTitleClusters(comics, nil, false)
	if len(clusters) != 0 {
		t.Fatalf("翻译标题不应聚组，得到 %d 簇", len(clusters))
	}
}

func TestDetectClustersVolumeIsolation(t *testing.T) {
	// 续集/分卷：卷号不同 → 天然隔离，不判重复
	comics := []models.OfflineComic{
		mkComic("b1", "[TOMO] シリーズ Vol.1", []string{"artist:TOMO"}, 20),
		mkComic("b2", "[TOMO] シリーズ Vol.2", []string{"artist:TOMO"}, 21),
		mkComic("b3", "[TOMO] シリーズ 第3話", []string{"artist:TOMO"}, 22),
	}
	clusters := detectTitleClusters(comics, nil, false)
	if len(clusters) != 0 {
		t.Fatalf("不同卷号不应判为重复，得到 %d 簇", len(clusters))
	}
}

func TestDetectClustersRemaster(t *testing.T) {
	// 重绘版：页数差大 → medium 置信，仍列出
	comics := []models.OfflineComic{
		mkComic("c1", "[MILK] ロリポップ", []string{"artist:MILK"}, 42),
		mkComic("c2", "[MILK] ロリポップ (新装版)", []string{"artist:MILK"}, 60),
	}
	clusters := detectTitleClusters(comics, nil, false)
	if len(clusters) != 1 {
		t.Fatalf("重绘版应聚成 1 簇，得到 %d 簇", len(clusters))
	}
	if clusters[0].Confidence != "medium" {
		t.Errorf("页数差 >5 应为 medium 置信，得到 %s", clusters[0].Confidence)
	}
}

func TestDetectClustersNameClash(t *testing.T) {
	// 撞名：核心名相同但画师不同 → 不聚组（防误伤）
	comics := []models.OfflineComic{
		mkComic("d1", "COMIC LO", []string{"artist:aaa"}, 100),
		mkComic("d2", "COMIC LO", []string{"artist:bbb"}, 100),
	}
	clusters := detectTitleClusters(comics, nil, false)
	if len(clusters) != 0 {
		t.Fatalf("撞名不同画师不应聚组，得到 %d 簇", len(clusters))
	}
}

func TestDetectClustersIgnore(t *testing.T) {
	comics := []models.OfflineComic{
		mkComic("e1", "[CIRCLE] もう一つの世界", []string{"artist:abc"}, 30),
		mkComic("e2", "[CIRCLE] もう一つの世界 (English)", []string{"artist:abc"}, 31),
	}
	// Round44：忽略条目带「忽略当时的成员快照」
	idx := newTestIgnoreIndex("もう一つの世界", "abc", []string{"e1", "e2"})

	// 成员全在快照内 → 增量与全量都不列出（语义＝"直到有变化为止"）
	if got := detectTitleClusters(comics, idx, false); len(got) != 0 {
		t.Fatalf("忽略且无新增时不应列出（增量），得到 %d 簇", len(got))
	}
	if got := detectTitleClusters(comics, idx, true); len(got) != 0 {
		t.Fatalf("忽略且无新增时不应列出（全量，Round44 已撤销「永久列出」），得到 %d 簇", len(got))
	}

	// 出现快照外的成员（忽略之后新入库）→ 照常列出并标注新增
	newComic := mkComic("e3", "[CIRCLE] もう一つの世界 (Chinese)", []string{"artist:abc"}, 30)
	newComic.AddedAt = time.Now() // 忽略之后入库
	comics = append(comics, newComic)
	got := detectTitleClusters(comics, idx, true)
	if len(got) != 1 {
		t.Fatalf("忽略项出现新入库成员时应列出该簇，得到 %d 簇", len(got))
	}
	if !got[0].Ignored {
		t.Error("应带 Ignored=true（此前已忽略）")
	}
	if got[0].IgnoredNewCount != 1 {
		t.Errorf("IgnoredNewCount 应为 1，得到 %d", got[0].IgnoredNewCount)
	}
	if got[0].IgnoreID != "test-ignore" {
		t.Errorf("IgnoreID 应为 test-ignore，得到 %q", got[0].IgnoreID)
	}
}

// newTestIgnoreIndex 构造带 title 型忽略（含成员快照）的内存索引
func newTestIgnoreIndex(titleKey, artist string, seen []string) *IgnoreIndex {
	idx := &IgnoreIndex{
		titleEntries:     map[string]*TitleIgnoreEntry{},
		titleEntriesNorm: map[string]*TitleIgnoreEntry{},
		gids:             map[string]bool{},
		comicIDs:         map[string]bool{},
	}
	e := &TitleIgnoreEntry{
		ID:        "test-ignore",
		TitleKey:  titleKey,
		Artist:    artist,
		CreatedAt: time.Now().Add(-time.Hour).UnixMilli(), // 新增感知的时间基线（1 小时前）
		seen:      map[string]bool{},
	}
	for _, id := range seen {
		e.seen[id] = true
	}
	idx.titleEntries[titleIgnoreKey(titleKey, artist)] = e
	idx.titleEntriesNorm[ignoreTitleKeyOf(titleKey, artist)] = e
	return idx
}

func TestDetectClustersNoArtistTag(t *testing.T) {
	// 无画师 tag：仅核心名 + 页数聚类（reason 提示无画师辅助）
	comics := []models.OfflineComic{
		mkComic("f1", "とあるシリーズ", nil, 20),
		mkComic("f2", "とあるシリーズ (reupload)", nil, 20),
	}
	clusters := detectTitleClusters(comics, nil, false)
	if len(clusters) != 1 {
		t.Fatalf("无画师 tag 的相同核心名应聚成 1 簇，得到 %d 簇", len(clusters))
	}
	if clusters[0].Artist != "" {
		t.Errorf("无画师 tag 时 Artist 应为空，得到 %q", clusters[0].Artist)
	}
}

// ─────────────────────────────────────────────────────────────
// Round42：方括号版本标记 / 空括号残留 / 跨字段池 / 作者硬否决
// 依据：plans/round42-dedup-calibration-set.md（人工确认结论）
// ─────────────────────────────────────────────────────────────

// mkComicFull 构造带日文原名（TitleJpn）的测试漫画
func mkComicFull(id, title, titleJpn string, tags []string, pageCount int) models.OfflineComic {
	return models.OfflineComic{
		ID:                id,
		Title:             title,
		TitleJpn:          titleJpn,
		OnlineTags:        MarshalTagSlice(tags),
		PageCount:         pageCount,
		OriginalPageCount: pageCount,
	}
}

func TestFingerprintTitleBracketVersionMarks(t *testing.T) {
	// Round42：方括号形式的版本/语言/汉化标记必须剥离（此前只认圆括号），且剥离后不留空括号
	cases := []struct {
		title string
		core  string
	}{
		{"[CIRCLE] サンプル [無修正]", "サンプル"},
		{"[CIRCLE] サンプル [中国翻訳]", "サンプル"},
		{"[CIRCLE] サンプル [Chinese]", "サンプル"},
		{"[CIRCLE] サンプル [Digital]", "サンプル"},
		{"[CIRCLE] サンプル [空気系☆漢化]", "サンプル"},
		{"[CIRCLE] サンプル [廉价汉化组]", "サンプル"},
		{"[CIRCLE] サンプル [C99]", "サンプル"},
		{"[CIRCLE] サンプル (C99)", "サンプル"},
		{"[CIRCLE] サンプル [江之下流个人AI润色]", "サンプル"},
		// 空括号残留（根因 R3）：[DL版] 剥成 "[ ]" 后必须清空，不留痕
		{"[CIRCLE] サンプル [DL版]", "サンプル"},
		// 安全：作品名括注必须保留（黑名单式，不剥作品名）
		{"[CIRCLE] とある本 (Fate/Grand Order)", "とある本 (fate/grand order)"},
		{"[CIRCLE] とある本 (ブルーアーカイブ)", "とある本 (ブルーアーカイブ)"},
	}
	for _, c := range cases {
		fp := fingerprintTitle(c.title, "")
		if fp.Core != c.core {
			t.Errorf("标题 %q：核心名 = %q，期望 %q", c.title, fp.Core, c.core)
		}
		if strings.ContainsAny(fp.Core, "[]") {
			t.Errorf("标题 %q：核心名仍残留方括号 %q", c.title, fp.Core)
		}
	}
}

func TestFingerprintTitleUserReportedPair(t *testing.T) {
	// 用户报告样本：同一作品的 [中国翻訳] [DL版] 版 vs [中国翻訳] 版（不同汉化组上传），
	// 剥净版本标记后核心名必须一致（此前因 "[DL版]" 剥成 "[ ]" 残留而不同键）
	a := "[夢ねこ屋 (むーにゃん)] 極東絢爛賭博島ドリームアイランド3 アルトリア&頼光編 (Fate/Grand Order) [中国翻訳] [DL版]"
	b := "[夢ねこ屋 (むーにゃん)] 極東絢爛賭博島ドリームアイランド3 アルトリア&頼光編 (Fate/Grand Order) [中国翻訳]"
	fa := fingerprintTitle(a, "muunyan")
	fb := fingerprintTitle(b, "muunyan")
	if fa.Core != fb.Core {
		t.Errorf("用户报告样本核心名应一致：\n  A=%q\n  B=%q", fa.Core, fb.Core)
	}
	if fa.Volume != fb.Volume {
		t.Errorf("卷号应一致：%q vs %q", fa.Volume, fb.Volume)
	}
}

func TestDetectClustersUserReportedCase(t *testing.T) {
	// 用户报告样本（端到端）：两本 title 是不同汉化组的罗马音，title_jpn 是同一日文原名
	//（仅差 [DL版]）→ 双字段池应聚成 1 簇
	comics := []models.OfflineComic{
		mkComicFull("u1",
			"[Yumenekoya (Muunyan)] Kyokutou Kenran Tobakutou Dream Island 3 - Artoria & Raikou Hen (Fate/Grand Order) [Chinese] [空気系☆漢化] [Digital]",
			"[夢ねこ屋 (むーにゃん)] 極東絢爛賭博島ドリームアイランド3 アルトリア&頼光編 (Fate/Grand Order) [中国翻訳] [DL版]",
			[]string{"artist:muunyan", "language:chinese", "language:translated"}, 112),
		mkComicFull("u2",
			"[Yumenekoya (Muunyan)] Kyokutou Kenran Tobakutou Dream Island 3 - Artoria & Raikou Hen (Fate/Grand Order) [Chinese] [黎欧出资汉化]",
			"[夢ねこ屋 (むーにゃん)] 極東絢爛賭博島ドリームアイランド3 アルトリア&頼光編 (Fate/Grand Order) [中国翻訳]",
			[]string{"artist:muunyan", "language:chinese", "language:translated"}, 112),
	}
	clusters := detectTitleClusters(comics, nil, false)
	if len(clusters) != 1 {
		t.Fatalf("用户报告样本应聚成 1 簇，得到 %d 簇", len(clusters))
	}
	if len(clusters[0].Members) != 2 {
		t.Errorf("簇成员应为 2，得到 %d", len(clusters[0].Members))
	}
	if clusters[0].Confidence != "high" {
		t.Errorf("页数相同 + 画师相同应为 high 置信，得到 %s", clusters[0].Confidence)
	}
	if clusters[0].Artist != "muunyan" {
		t.Errorf("Artist 应为 muunyan，得到 %q", clusters[0].Artist)
	}
}

func TestDetectClustersCrossFieldPool(t *testing.T) {
	// 跨字段池：一本标题是罗马音（title_jpn 有日文原名），另一本标题本身就是日文原名 → 应聚组
	comics := []models.OfflineComic{
		mkComicFull("x1",
			"[Yumenekoya] Kyokutou Kenran Tobakutou Dream Island - Miyamoto Musashi Hen [Chinese] [Digital]",
			"[夢ねこ屋 (むーにゃん)] 極東絢爛賭博島ドリームアイランドー宮本武蔵編 (Fate/Grand Order) [中国翻訳] [無修正] [DL版]",
			[]string{"artist:muunyan"}, 35),
		mkComicFull("x2",
			"[夢ねこ屋 (むーにゃん)] 極東絢爛賭博島ドリームアイランドー宮本武蔵編 (Fate/Grand Order) [中国翻訳] [DL版]",
			"",
			[]string{"artist:muunyan"}, 35),
	}
	if clusters := detectTitleClusters(comics, nil, false); len(clusters) != 1 {
		t.Fatalf("跨字段同作品应聚成 1 簇，得到 %d 簇", len(clusters))
	}
}

func TestDetectClustersAuthorVeto(t *testing.T) {
	// 作者硬否决（D10）：同名同页数但 artist 不同 → 不聚组。
	// 场景取自标定集：`[thirty8ght] サツキ` vs `[りおん] サツキ` 是不同作者的同名作品，
	// 用户明确要求「必须避免」这类误报。
	comics := []models.OfflineComic{
		mkComicFull("v1", "[thirty8ght] サツキ (ブルーアーカイブ)", "", []string{"artist:38 | thirty8ght"}, 22),
		mkComicFull("v2", "[りおん] サツキ (ブルーアーカイブ)", "", []string{"artist:rion"}, 22),
	}
	if clusters := detectTitleClusters(comics, nil, false); len(clusters) != 0 {
		t.Fatalf("不同作者的同名作品不应聚组，得到 %d 簇", len(clusters))
	}
	// 反向对照：只把 artist 改成相同 → 应聚组（证明否决来自作者，而非标题差异）
	comics[1].OnlineTags = MarshalTagSlice([]string{"artist:38 | thirty8ght"})
	if clusters := detectTitleClusters(comics, nil, false); len(clusters) != 1 {
		t.Fatalf("同作者同名应聚组，得到 %d 簇", len(clusters))
	}
}

func TestDetectClustersNoArtistDowngrade(t *testing.T) {
	// 作者未知（无 artist tag）：不否决（部分版权本有意不带 artist tag）→ 仍聚组，
	// 但置信度降一档并标注「作者未知」，供人工优先复核
	comics := []models.OfflineComic{
		mkComic("w1", "[CIRCLE] とある本 [中国翻訳]", nil, 30),
		mkComic("w2", "[CIRCLE] とある本 [無修正] [中国翻訳]", nil, 31),
	}
	clusters := detectTitleClusters(comics, nil, false)
	if len(clusters) != 1 {
		t.Fatalf("作者未知时不应否决，应聚成 1 簇，得到 %d 簇", len(clusters))
	}
	if clusters[0].Confidence != "medium" {
		t.Errorf("作者未知应降一档为 medium，得到 %s", clusters[0].Confidence)
	}
	if !strings.Contains(clusters[0].Reason, "作者未知") {
		t.Errorf("reason 应标注「作者未知」，得到 %q", clusters[0].Reason)
	}
}
