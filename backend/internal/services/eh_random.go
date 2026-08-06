package services

import (
	"log"
	"math/rand"

	"SakuHentai/internal/models"
)

// 在线随机采样页数控制（全卡池随机近似策略）
const (
	// 每页画廊数（E 站列表页固定）
	perPage = 25

	// 全卡池随机：最少采样页数。
	// 即使 count=1 也至少采样 4 页（约 100 条候选池），避免"只从首页/单页 25 条里随机"的偏差。
	minSamplingPages = 4

	// 全卡池随机：最多采样页数（约 300 条候选池）。
	// 继续增加页数会显著拉高 E 站请求量，收益递减，故设上限。
	maxSamplingPages = 12
)

// FetchRandomGalleryList 全卡池随机：多页采样近似真实随机。
//
// 策略：E 站没有直接的随机接口，这里采用「随机翻页 + 多页采样」：
//  1. 先请求第 1 页（携带关键词/分类筛选）获取总页数 totalPages；
//  2. 采样若干互不重复的随机页（每页最多 25 本；下限 minSamplingPages 页，
//     上限 maxSamplingPages 页，确保候选池足够大）；
//  3. 跨页按 gid 去重后合并成池，Fisher-Yates 洗牌后取前 count 本返回。
//
// 修复（问题2/问题3）：旧实现只翻 1 个随机页，因此最多只能抽 25 本，
// 且当 totalPages 解析失败回退 1 时永远落在首页前 25 本，随机性极差；
// 现改为即使 count=1 也至少采样 minSamplingPages 页，让单次抽卡从约 100 条
// 候选池中随机，更接近"全卡池随机"。
func (s *EHService) FetchRandomGalleryList(account *models.AccountSetting, params SearchParams, setting *models.EHSetting, count int) ([]OnlineComicDTO, error) {
	if count < 1 {
		count = 1
	}

	// 1. 请求第一页，获取总页数（翻页器解析，见 parseTotalPages）
	first, err := s.FetchGalleryList(account, params, setting)
	if err != nil {
		return nil, err
	}

	totalPages := first.TotalPages
	if totalPages < 1 {
		// 抓不到总页数时退化为仅首页
		totalPages = 1
	}

	// 2. 计算需要采样的页数（受总页数约束），并随机挑选互不重复的页码
	//
	// 全卡池随机增强：count 再小也至少采样 minSamplingPages 页（约 100 条候选池），
	// 最多 maxSamplingPages 页（约 300 条候选池），避免"只从首页/单页 25 条里随机"的偏差。
	neededPages := (count + perPage - 1) / perPage
	if neededPages < minSamplingPages {
		neededPages = minSamplingPages
	}
	if neededPages > maxSamplingPages {
		neededPages = maxSamplingPages
	}
	if neededPages > totalPages {
		neededPages = totalPages
	}
	if neededPages < 1 {
		neededPages = 1
	}

	picked := make(map[int]bool, neededPages)
	pages := make([]int, 0, neededPages)
	for attempt := 0; len(pages) < neededPages && attempt < totalPages*3+5; attempt++ {
		p := rand.Intn(totalPages) + 1
		if picked[p] {
			continue
		}
		picked[p] = true
		pages = append(pages, p)
	}
	if len(pages) == 0 {
		pages = []int{1}
	}

	// 3. 逐页抓取并跨页去重（同 gid 只保留一份）
	pool := make([]OnlineComicDTO, 0, count*2)
	seenGID := make(map[string]bool, len(first.Comics))
	addPool := func(comics []OnlineComicDTO) {
		for _, c := range comics {
			if c.ID == "" || seenGID[c.ID] {
				continue
			}
			seenGID[c.ID] = true
			pool = append(pool, c)
		}
	}

	// 4. 第一页已抓过，直接复用其内容，避免重复请求
	hasPageOne := false
	for _, p := range pages {
		if p == 1 {
			hasPageOne = true
			break
		}
	}
	if hasPageOne {
		addPool(first.Comics)
	}

	for _, p := range pages {
		if p == 1 {
			continue
		}
		randomParams := params
		randomParams.Next = ""
		randomParams.Prev = ""
		randomParams.Seek = ""
		randomParams.Page = p
		result, err := s.FetchGalleryList(account, randomParams, setting)
		if err != nil {
			log.Printf("[EH-DEBUG] 随机采样第 %d 页失败（跳过）: %v", p, err)
			continue
		}
		addPool(result.Comics)
	}

	if len(pool) == 0 {
		return []OnlineComicDTO{}, nil
	}

	// 5. Fisher-Yates 洗牌后截取前 count 本
	rand.Shuffle(len(pool), func(i, j int) {
		pool[i], pool[j] = pool[j], pool[i]
	})
	if len(pool) > count {
		pool = pool[:count]
	}

	return pool, nil
}
