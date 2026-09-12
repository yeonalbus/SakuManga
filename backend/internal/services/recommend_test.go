package services

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round32 阶段二：本地偏好推荐单测
//
// 覆盖：参数夹取、打分口径（θ 融合 / γ 长度归一 / IDF 抑制 / 命中理由）、
// 温度采样偏向高权重、探索率生效、多样性限流、无权重降级。
// ─────────────────────────────────────────────────────────────

func TestNormalizeRecommendOptions(t *testing.T) {
	// 缺省/非法 → 回退文档默认值
	def := NormalizeRecommendOptions(RecommendOptions{Theta: -1, Explore: 9, Temperature: 0, Gamma: -3})
	if def.Theta != RecoDefaultTheta || def.Explore != RecoDefaultExplore ||
		def.Temperature != RecoDefaultTemperature || def.Gamma != RecoDefaultGamma {
		t.Fatalf("非法参数未回退默认：%+v", def)
	}
	// 合法值原样保留（含边界 0 / 1）
	kept := NormalizeRecommendOptions(RecommendOptions{Theta: 1, Explore: 0, Temperature: RecoMaxTemperature, Gamma: 1})
	if kept.Theta != 1 || kept.Explore != 0 || kept.Temperature != RecoMaxTemperature || kept.Gamma != 1 {
		t.Fatalf("合法参数被改写：%+v", kept)
	}
}

func TestScoreComicFusion(t *testing.T) {
	table := &XpWeightTable{
		Lib:    map[string]float64{"female:a": 1.0, "female:b": 0.5},
		Read:   map[string]float64{"female:a": 0.2},
		IDF:    map[string]float64{"female:a": 1.0, "female:b": 1.0},
		LibMax: 1.0, ReadMax: 0.2,
	}
	tags := []string{"female:a", "female:b"}

	// θ=0（纯库藏）+ γ=0（不归一）：1.0 + 0.5
	score, matched := ScoreComic(tags, table, RecommendOptions{Theta: 0, Gamma: 0})
	if math.Abs(score-1.5) > 1e-9 {
		t.Fatalf("纯库藏得分 = %v，期望 1.5", score)
	}
	if len(matched) != 2 || matched[0] != "female:a" {
		t.Fatalf("命中理由应按贡献降序：%v", matched)
	}

	// θ=1（纯阅读）：只有 female:a 有阅读权重 → 0.2
	score, _ = ScoreComic(tags, table, RecommendOptions{Theta: 1, Gamma: 0})
	if math.Abs(score-0.2) > 1e-9 {
		t.Fatalf("纯阅读得分 = %v，期望 0.2", score)
	}

	// γ=0.5：长度归一 1.5/√2
	score, _ = ScoreComic(tags, table, RecommendOptions{Theta: 0, Gamma: 0.5})
	if math.Abs(score-1.5/math.Sqrt(2)) > 1e-9 {
		t.Fatalf("长度归一得分 = %v，期望 %v", score, 1.5/math.Sqrt(2))
	}

	// θ=0.6 融合：(0.4×1.0 + 0.6×0.2) + (0.4×0.5 + 0.6×0) = 0.52 + 0.20 = 0.72
	score, _ = ScoreComic(tags, table, RecommendOptions{Theta: 0.6, Gamma: 0})
	if math.Abs(score-0.72) > 1e-9 {
		t.Fatalf("融合得分 = %v，期望 0.72", score)
	}

	// 无有效 tag → 0 分且无命中理由
	if s, m := ScoreComic([]string{"female:unknown"}, table, RecommendOptions{Theta: 0.6, Gamma: 0.5}); s != 0 || m != nil {
		t.Fatalf("未收录 tag 应得 0 分，实得 %v / %v", s, m)
	}
}

func TestScoreComicIDFSuppression(t *testing.T) {
	// 泛化 tag（IDF 低）与冷门 tag（IDF 高）同库藏权重下的得分差异
	table := &XpWeightTable{
		Lib: map[string]float64{"female:common": 1.0, "female:rare": 1.0},
		IDF: map[string]float64{"female:common": math.Log(1 + 100.0/101.0), "female:rare": math.Log(1 + 100.0/2.0)},
	}
	common, _ := ScoreComic([]string{"female:common"}, table, RecommendOptions{Theta: 0, Gamma: 0})
	rare, _ := ScoreComic([]string{"female:rare"}, table, RecommendOptions{Theta: 0, Gamma: 0})
	if !(rare > common) {
		t.Fatalf("IDF 抑制失效：泛化 %v 应低于冷门 %v", common, rare)
	}
}

func TestRecommendPrefersHighWeight(t *testing.T) {
	db := newXpTestDB(t)
	xp := NewXpCloudService(db)
	reco := NewRecommendService(xp)

	user := models.User{Username: "u1"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	// 9 本含热门 tag + 各自唯一 tag；1 本只有冷门 tag
	total := 10
	for i := 0; i < total; i++ {
		tags := fmt.Sprintf(`["female:rare%d"]`, i)
		if i < total-1 {
			tags = fmt.Sprintf(`["female:hot","female:rare%d"]`, i)
		}
		if err := db.Create(&models.OfflineComic{
			ID: fmt.Sprintf("c%d", i), LocalPath: fmt.Sprintf("p%d", i), OnlineTags: tags,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := xp.RebuildAll(); err != nil {
		t.Fatal(err)
	}

	var candidates []models.OfflineComic
	if err := db.Find(&candidates).Error; err != nil {
		t.Fatal(err)
	}

	// 反复抽 1 本：高权重候选（含 female:hot）应占绝对多数
	opts := NormalizeRecommendOptions(RecommendOptions{Theta: 0, Explore: 0, Temperature: 0.5, Gamma: 0})
	hotPicked := 0
	rounds := 60
	for i := 0; i < rounds; i++ {
		out, err := reco.Recommend(user.ID, candidates, 1, opts)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 1 {
			t.Fatalf("抽取数量不符：%d", len(out))
		}
		for _, tag := range out[0].MatchedTags {
			if tag == "female:hot" {
				hotPicked++
				break
			}
		}
	}
	if hotPicked < rounds*3/4 {
		t.Fatalf("推荐未偏向高权重：%d/%d 命中热门 tag", hotPicked, rounds)
	}

	// 命中理由必须来自该本自身 tag
	if out, err := reco.Recommend(user.ID, candidates, 3, opts); err == nil {
		for _, item := range out {
			own := map[string]bool{}
			for _, tag := range XpEffectiveTags(&item.Comic) {
				own[tag] = true
			}
			for _, tag := range item.MatchedTags {
				if !own[tag] {
					t.Fatalf("命中理由包含本子不含的 tag：%v not in %v", tag, own)
				}
			}
		}
	}
}

func TestRecommendExploreFlattensDistribution(t *testing.T) {
	db := newXpTestDB(t)
	xp := NewXpCloudService(db)
	reco := NewRecommendService(xp)

	user := models.User{Username: "u1"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	// 高权重组：10 本含广泛出现的 female:hot；低权重组：10 本各含只出现一次的唯一 tag
	// （注意稀释公式：**广泛出现的 tag** 才拿到高权重，单本堆 tag 并不会抬高权重）
	for i := 0; i < 20; i++ {
		tags := fmt.Sprintf(`["female:rare%d"]`, i)
		if i < 10 {
			tags = fmt.Sprintf(`["female:hot","female:rare%d"]`, i)
		}
		if err := db.Create(&models.OfflineComic{
			ID: fmt.Sprintf("c%d", i), LocalPath: fmt.Sprintf("p%d", i), OnlineTags: tags,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := xp.RebuildAll(); err != nil {
		t.Fatal(err)
	}
	var candidates []models.OfflineComic
	if err := db.Find(&candidates).Error; err != nil {
		t.Fatal(err)
	}

	// 统计「抽中高权重组」的比例：探索率越高越接近均匀（50%）
	hotRate := func(explore float64) float64 {
		opts := NormalizeRecommendOptions(RecommendOptions{Theta: 0, Explore: explore, Temperature: 0.5, Gamma: 0})
		hits := 0
		rounds := 100
		for i := 0; i < rounds; i++ {
			out, err := reco.Recommend(user.ID, candidates, 1, opts)
			if err != nil {
				t.Fatal(err)
			}
			for _, tag := range out[0].MatchedTags {
				if tag == "female:hot" {
					hits++
					break
				}
			}
		}
		return float64(hits) / float64(rounds)
	}

	noExplore := hotRate(0)
	withExplore := hotRate(RecoMaxExplore)
	if noExplore < 0.7 {
		t.Fatalf("ε=0 时应明显偏向高权重组，实得 %.2f", noExplore)
	}
	if !(withExplore < noExplore) {
		t.Fatalf("探索率未生效：ε=0 命中 %.2f，ε=%.1f 命中 %.2f", noExplore, RecoMaxExplore, withExplore)
	}
}

func TestRecommendDiversityLimits(t *testing.T) {
	db := newXpTestDB(t)
	xp := NewXpCloudService(db)
	reco := NewRecommendService(xp)

	user := models.User{Username: "u1"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	// 6 本同 parody（高权重：广泛出现）+ 6 本各不相同（低权重但有替代空间）。
	// 限流是软约束：只有当存在替代候选时才生效，因此必须给出可替代的池子。
	for i := 0; i < 12; i++ {
		tags := fmt.Sprintf(`["parody:saga %d","female:tag%d"]`, i, i)
		if i < 6 {
			tags = fmt.Sprintf(`["parody:same saga","female:tag%d"]`, i)
		}
		if err := db.Create(&models.OfflineComic{
			ID: fmt.Sprintf("c%d", i), LocalPath: fmt.Sprintf("p%d", i), OnlineTags: tags,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := xp.RebuildAll(); err != nil {
		t.Fatal(err)
	}
	var candidates []models.OfflineComic
	if err := db.Find(&candidates).Error; err != nil {
		t.Fatal(err)
	}

	opts := NormalizeRecommendOptions(RecommendOptions{Theta: 0, Explore: 0, Temperature: 0.5, Gamma: 0})
	out, err := reco.Recommend(user.ID, candidates, 6, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 6 {
		t.Fatalf("抽取数量 = %d，期望 6", len(out))
	}
	same := 0
	ids := map[string]bool{}
	for _, item := range out {
		if ids[item.Comic.ID] {
			t.Fatalf("同一本被重复抽出：%s", item.Comic.ID)
		}
		ids[item.Comic.ID] = true
		for _, tag := range XpEffectiveTags(&item.Comic) {
			if tag == "parody:same saga" {
				same++
				break
			}
		}
	}
	if same > RecoDiversityMaxSameKey {
		t.Fatalf("多样性限流失效：同一 parody 抽中 %d 本（上限 %d）", same, RecoDiversityMaxSameKey)
	}
}

func TestRecommendFallsBackWhenNoWeights(t *testing.T) {
	db := newXpTestDB(t)
	xp := NewXpCloudService(db)
	reco := NewRecommendService(xp)

	user := models.User{Username: "u1"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	// 本子无有效 tag → 统计表无行 → 权重表为空
	if err := db.Create(&models.OfflineComic{
		ID: "c1", LocalPath: "p1", OnlineTags: `["language:chinese"]`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := xp.RebuildAll(); err != nil {
		t.Fatal(err)
	}
	var candidates []models.OfflineComic
	if err := db.Find(&candidates).Error; err != nil {
		t.Fatal(err)
	}

	_, err := reco.Recommend(user.ID, candidates, 3, RecommendOptions{})
	if !errors.Is(err, ErrRecommendNoWeights) {
		t.Fatalf("无权重时应返回 ErrRecommendNoWeights，实得 %v", err)
	}

	// 空候选集：返回空结果，不报错
	if out, err := reco.Recommend(user.ID, nil, 3, RecommendOptions{}); err != nil || out != nil {
		t.Fatalf("空候选应返回空结果，实得 %v / %v", out, err)
	}
}
