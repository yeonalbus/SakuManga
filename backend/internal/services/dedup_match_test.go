package services

import (
	"strings"
	"testing"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round42 Tier 2（近似层）单测
// 阈值/门槛依据 plans/round42-dedup-calibration-set.md 的 P/N 样本标定：
//   - 正文（去括注）相似度 ≥ dedupBodySimMin 才进入近似判定（括注差异是元数据、正文差异是内容）
//   - 作者硬否决（D10）在 Tier 2 同样生效；作者未知时不否决但降档并标注
// ─────────────────────────────────────────────────────────────

func TestApproxClustersBracketNoteDifference(t *testing.T) {
	// 括注增删（元数据差异：原作名 `(Elsword)` / 汉化组标注有无）→ 正文完全相同 → 应聚组。
	// 场景取自标定集 P5：`… C<3#EVE Esencia H>` vs `… C<3#EVE Esencia H> (Elsword) [不咕鸟汉化组]`
	comics := []models.OfflineComic{
		mkComicFull("ap1", "[lestart] ELSWORD Series C<3#EVE Esencia H> [Chinese]", "",
			[]string{"artist:lestart", "language:chinese"}, 10),
		mkComicFull("ap2", "[lestart] ELSWORD Series C<3#EVE Esencia H> (Elsword) [Chinese] [不咕鸟汉化组]", "",
			[]string{"artist:lestart", "language:chinese"}, 12),
	}
	clusters := detectApproxClusters(comics, nil, nil, false)
	if len(clusters) != 1 {
		t.Fatalf("括注差异（正文相同）应聚成 1 簇，得到 %d 簇", len(clusters))
	}
	if len(clusters[0].Members) != 2 {
		t.Errorf("簇成员应为 2，得到 %d", len(clusters[0].Members))
	}
}

func TestApproxClustersContentSequelNotClustered(t *testing.T) {
	// 正文差异（续篇/加笔，如 `… 甘露寺蜜璃` vs `… 甘露寺蜜璃 妊娠中`）→ 不聚组。
	// 场景取自标定集 N2（用户确认「不重复」）
	comics := []models.OfflineComic{
		mkComicFull("sq1", "[エロマズン (まー九郎)] 催眠温泉 甘露寺蜜璃 (鬼滅の刃) [中国翻訳] [DL版]", "",
			[]string{"artist:ma-kurou", "language:chinese"}, 52),
		mkComicFull("sq2", "[エロマズン (まー九郎)] 催眠温泉 甘露寺蜜璃 妊娠中 (鬼滅の刃) [中国翻訳] [DL版]", "",
			[]string{"artist:ma-kurou", "language:chinese"}, 51),
	}
	if clusters := detectApproxClusters(comics, nil, nil, false); len(clusters) != 0 {
		t.Fatalf("续篇/加笔（正文不同）不应聚组，得到 %d 簇", len(clusters))
	}
}

func TestApproxClustersVolumeNumbersNotClustered(t *testing.T) {
	// 裸数字卷号（`… saimin 3 …` vs `… saimin 4 …`，volume 提取抓不到）→ 不聚组。
	// 场景取自标定集 N2：同画师同系列不同卷，正文相似度 0.79 但未达正文门槛
	comics := []models.OfflineComic{
		mkComicFull("vn1", "[Puu no Puupuupuu (Puuzaki Puuna)] Okasare saimin 3 danshi hitori shika inai", "",
			[]string{"artist:puu no puupuupuu"}, 63),
		mkComicFull("vn2", "[Puu no Puupuupuu (Puuzaki Puuna)] Okasare saimin 4 danshi hitori shika inai", "",
			[]string{"artist:puu no puupuupuu"}, 67),
	}
	if clusters := detectApproxClusters(comics, nil, nil, false); len(clusters) != 0 {
		t.Fatalf("不同卷（数字不同）不应聚组，得到 %d 簇", len(clusters))
	}
}

func TestApproxClustersAuthorVeto(t *testing.T) {
	// 作者硬否决（D10）在 Tier 2 同样生效：同名同正文但 artist 不同 → 不聚组
	comics := []models.OfflineComic{
		mkComicFull("av1", "[thirty8ght] サツキ (ブルーアーカイブ)", "", []string{"artist:38 | thirty8ght"}, 22),
		mkComicFull("av2", "[りおん] サツキ (ブルーアーカイブ)", "", []string{"artist:rion"}, 6),
	}
	if clusters := detectApproxClusters(comics, nil, nil, false); len(clusters) != 0 {
		t.Fatalf("不同作者不应聚组（Tier 2 亦须遵守 D10），得到 %d 簇", len(clusters))
	}
}

func TestApproxClustersUnknownAuthorDowngrade(t *testing.T) {
	// 作者未知（无 artist tag）：不否决 → 聚组，但置信度降档并标注「作者未知」
	comics := []models.OfflineComic{
		mkComic("aw1", "[Fanbox] Sample Work", nil, 30),
		mkComic("aw2", "[Fanbox] Sample Work [Chinese]", nil, 31),
	}
	clusters := detectApproxClusters(comics, nil, nil, false)
	if len(clusters) != 1 {
		t.Fatalf("作者未知时不否决，应聚成 1 簇，得到 %d 簇", len(clusters))
	}
	if clusters[0].Confidence != "medium" {
		t.Errorf("作者未知应降档为 medium，得到 %s", clusters[0].Confidence)
	}
	if !strings.Contains(clusters[0].Reason, "作者未知") {
		t.Errorf("reason 应标注「作者未知」，得到 %q", clusters[0].Reason)
	}
}

func TestApproxClustersSkipsExcluded(t *testing.T) {
	// exclude（Tier 1 已聚入簇的条目）不重复参与 Tier 2，避免同一条目出现在两个簇里
	comics := []models.OfflineComic{
		mkComicFull("ex1", "[lestart] ELSWORD Series C<3#EVE Esencia H> [Chinese]", "", []string{"artist:lestart"}, 10),
		mkComicFull("ex2", "[lestart] ELSWORD Series C<3#EVE Esencia H> (Elsword) [Chinese]", "", []string{"artist:lestart"}, 12),
	}
	if clusters := detectApproxClusters(comics, map[string]bool{"ex1": true, "ex2": true}, nil, false); len(clusters) != 0 {
		t.Fatalf("已被 Tier 1 聚入的条目不应再进 Tier 2，得到 %d 簇", len(clusters))
	}
}

func TestBodyOfAndParenHelpers(t *testing.T) {
	// helper 语义：bodyOf 去括注；parenSet 取括注 token
	if got := bodyOf("sample work (elsword) [chinese]"); got != "sample work" {
		t.Errorf("bodyOf = %q，期望 \"sample work\"", got)
	}
	ps := parenSet("sample work (elsword)")
	if !ps["elsword"] {
		t.Errorf("parenSet 应含 elsword，得到 %v", ps)
	}
	if got := bodyOf("催眠温泉 甘露寺蜜璃 妊娠中 (鬼滅の刃)"); got == bodyOf("催眠温泉 甘露寺蜜璃 (鬼滅の刃)") {
		t.Error("正文不同（妊娠中）时 bodyOf 结果不应相同")
	}
}
