package services

import "testing"

// 搜索联想 Suggest 回归测试（修复3a）：
//   1) 裸词查询不得因「namespace:key 子串匹配」误命中命名空间本身含查询词的标签
//      （如 "la" 命中所有 language 系列标签的旧实现，导致高热度乱联想霸榜）；
//   2) 前缀命中应优先于子串命中排序；
//   3) 多词未完成短语（huge penis）应命中对应 tag；
//   4) 显式命名空间（female:sto）按 ns:key 前缀匹配。
//
// 注意：female:glasses 的 key "glasses" 本身包含子串 "la"（g-la-sses），
// 属合法的 key 子串命中，不能被断言排除；真正要排除的是「仅命名空间含查询词」的标签。
func TestSuggestPrefixPriorityNoNamespaceNoise(t *testing.T) {
	e := &TagEngine{
		tags:       make(map[string]*TagItem),
		EnableCN:   true,
		EnableSort: true,
		tagList: []*TagItem{
			{Namespace: "female", Key: "stockings", Name: "长筒袜", Count: 1000},
			{Namespace: "female", Key: "glasses", Name: "眼镜", Count: 900},
			{Namespace: "female", Key: "lactation", Name: "泌乳", Count: 700},
			{Namespace: "male", Key: "huge penis", Name: "巨根", Count: 800},
			{Namespace: "language", Key: "japanese", Name: "日本語", Count: 600},
		},
	}

	// 1) 裸词 "la"：不得命中 female:stockings（key/name/ns 均不含 "la"）
	//    更关键：不得命中 language:japanese —— 旧实现 matchNS=Contains("language:japanese","la")
	//    因命名空间 "language" 含 "la" 而误命中；新实现裸词只匹配 key/中文名。
	res := e.Suggest("la", 8)
	for _, it := range res {
		if it.Namespace == "language" && it.Key == "japanese" {
			t.Fatalf("裸词 la 不应命中 language:japanese（命名空间子串误匹配）")
		}
		if it.Namespace == "female" && it.Key == "stockings" {
			t.Fatalf("裸词 la 不应命中 female:stockings（key/name/ns 均不含 la）")
		}
	}
	foundLactation := false
	for _, it := range res {
		if it.Namespace == "female" && it.Key == "lactation" {
			foundLactation = true
		}
	}
	if !foundLactation {
		t.Fatalf("裸词 la 应命中 female:lactation（前缀匹配），实际=%v", res)
	}

	// 2) 前缀命中优先：la 的首个结果应为前缀命中的 lactation，
	//    而非 count 更高(900)但仅为子串命中的 female:glasses
	if len(res) < 2 {
		t.Fatalf("la 应有至少 2 个结果，实际=%d", len(res))
	}
	if res[0].Key != "lactation" {
		t.Fatalf("la 首个结果应为前缀命中的 lactation，实际=%s", res[0].Key)
	}

	// 3) 多词未完成短语 "huge penis"：应命中 male:huge penis
	res = e.Suggest("huge penis", 8)
	foundHuge := false
	for _, it := range res {
		if it.Namespace == "male" && it.Key == "huge penis" {
			foundHuge = true
		}
	}
	if !foundHuge {
		t.Fatalf("huge penis 应命中 male:huge penis，实际=%v", res)
	}

	// 4) 显式命名空间 "female:sto"：应命中 female:stockings（ns:key 前缀匹配）
	res = e.Suggest("female:sto", 8)
	foundStockings := false
	for _, it := range res {
		if it.Namespace == "female" && it.Key == "stockings" {
			foundStockings = true
		}
	}
	if !foundStockings {
		t.Fatalf("female:sto 应命中 female:stockings，实际=%v", res)
	}
}

// 需求1-热度协同：输入 "penis" 应通过「完整单词命中」找到 male:huge penis，
// 且在热度协同排序下 huge penis(36080, 完整词命中) 排在 penis enlargement(21051, 前缀) 之前。
func TestSuggestHeatCoopFullWordMatch(t *testing.T) {
	e := &TagEngine{
		tags:       make(map[string]*TagItem),
		EnableCN:   true,
		EnableSort: true,
		tagList: []*TagItem{
			{Namespace: "male", Key: "huge penis", Name: "巨根", Count: 36080},
			{Namespace: "male", Key: "penis enlargement", Name: "阴茎增大", Count: 21051},
			{Namespace: "female", Key: "penis ring", Name: "阴茎环", Count: 15000},
		},
	}

	// 1) 裸词 "penis"：huge penis（完整单词命中）应出现且排在 penis enlargement（前缀）之前，
	//    与需求1给出的预期一致（huge penis 36080 第一、penis enlargement 21051 第二）。
	res := e.Suggest("penis", 8)
	if len(res) < 2 {
		t.Fatalf("penis 应至少有 2 个结果，实际=%v", res)
	}
	if res[0].Namespace != "male" || res[0].Key != "huge penis" {
		t.Fatalf("penis 首个结果应为 male:huge penis（完整词命中+高热度），实际=%v", res[0])
	}
	if res[1].Key != "penis enlargement" {
		t.Fatalf("penis 第二个结果应为 penis enlargement，实际=%v", res[1])
	}

	// 2) 跨词前缀 "peni"：应命中 male:huge penis（完整单词 "penis" 以 "peni" 开头）
	res = e.Suggest("peni", 8)
	foundHuge := false
	for _, it := range res {
		if it.Namespace == "male" && it.Key == "huge penis" {
			foundHuge = true
		}
	}
	if !foundHuge {
		t.Fatalf("跨词前缀 peni 应命中 male:huge penis，实际=%v", res)
	}
}
