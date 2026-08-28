package services

import (
	"testing"

	"SakuHentai/internal/models"

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
	idx := &IgnoreIndex{titleKeys: map[string]bool{}, gids: map[string]bool{}}
	idx.titleKeys[titleIgnoreKey("もう一つの世界", "abc")] = true

	// 增量：忽略的簇直接跳过
	incr := detectTitleClusters(comics, idx, false)
	if len(incr) != 0 {
		t.Fatalf("增量查重应跳过已忽略簇，得到 %d 簇", len(incr))
	}
	// 全量：仍列出，带 Ignored 标记
	full := detectTitleClusters(comics, idx, true)
	if len(full) != 1 {
		t.Fatalf("全量核对应列出已忽略簇，得到 %d 簇", len(full))
	}
	if !full[0].Ignored {
		t.Error("全量核对时命中忽略的簇应带 Ignored=true")
	}
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
