package services

import (
	"log"
	"math/rand"
	"strings"

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

// normalizeTag 归一化原始 tag（namespace:key）：小写 + 下划线→空格 + 去首尾空格。
// 与前端 src/utils/tagFilter.ts 的 normalizeTag 语义一致，用于负向 tag 精确匹配。
func normalizeTag(raw string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(raw, "_", " ")))
}

// matchesRandomExclude 判断在线候选是否命中任一负向排除规则（与前端 matchExcludes 语义一致）：
//   - excludeTags：对原始 tag（namespace:key）做归一化精确匹配（如 "female:yuri"）
//   - excludeKeywords：对 标题/标签/上传者 做不区分大小写的子串匹配（如 "3d"）
func matchesRandomExclude(c OnlineComicDTO, excludeTags, excludeKeywords []string) bool {
	for _, raw := range excludeTags {
		norm := normalizeTag(raw)
		if norm == "" {
			continue
		}
		for _, tag := range c.Tags {
			if normalizeTag(tag) == norm {
				return true
			}
		}
	}

	if len(excludeKeywords) > 0 {
		title := strings.ToLower(c.Title)
		uploader := strings.ToLower(c.Uploader)
		tagText := strings.ToLower(strings.Join(c.Tags, " "))
		for _, raw := range excludeKeywords {
			kw := strings.ToLower(strings.TrimSpace(raw))
			if kw == "" {
				continue
			}
			if strings.Contains(title, kw) || strings.Contains(uploader, kw) || strings.Contains(tagText, kw) {
				return true
			}
		}
	}

	return false
}

// FetchRandomGalleryList 全卡池随机：多页采样近似真实随机。
//
// 策略：E 站没有直接的随机接口，这里采用「随机翻页 + 多页采样」：
//  1. 先请求第 1 页（携带关键词/分类筛选）获取总页数 totalPages；
//  2. 采样若干互不重复的随机页（每页最多 25 本；下限 minSamplingPages 页，
//     上限 maxSamplingPages 页，确保候选池足够大）；
//  3. 跨页按 gid 去重后合并成池，Fisher-Yates 洗牌后取前 count 本返回。
//
// Round3-任务6（可选增强）：接受负向排除规则 excludeTags / excludeKeywords，
// 在「采样池构建时」直接丢弃命中项（而非前端抽到后再丢弃），
// 候选池因负向丢弃而不足时继续随机补采样页（受 maxSamplingPages 与 totalPages 约束）。
//
// 修复（问题2/问题3）：旧实现只翻 1 个随机页，因此最多只能抽 25 本，
// 且当 totalPages 解析失败回退 1 时永远落在首页前 25 本，随机性极差；
// 现改为即使 count=1 也至少采样 minSamplingPages 页，让单次抽卡从约 100 条
// 候选池中随机，更接近"全卡池随机"。
func (s *EHService) FetchRandomGalleryList(account *models.AccountSetting, params SearchParams, setting *models.EHSetting, count int, excludeTags, excludeKeywords []string) ([]OnlineComicDTO, error) {
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

	// 3. 逐页抓取并跨页去重（同 gid 只保留一份）；负向命中的候选直接丢弃，不入池
	pool := make([]OnlineComicDTO, 0, count*2)
	seenGID := make(map[string]bool, len(first.Comics))
	addPool := func(comics []OnlineComicDTO) {
		for _, c := range comics {
			if c.ID == "" || seenGID[c.ID] {
				continue
			}
			if matchesRandomExclude(c, excludeTags, excludeKeywords) {
				continue
			}
			seenGID[c.ID] = true
			pool = append(pool, c)
		}
	}

	fetchPage := func(p int) {
		if p == 1 {
			addPool(first.Comics)
			return
		}
		randomParams := params
		randomParams.Next = ""
		randomParams.Prev = ""
		randomParams.Seek = ""
		randomParams.Page = p
		result, err := s.FetchGalleryList(account, randomParams, setting)
		if err != nil {
			log.Printf("[EH-DEBUG] 随机采样第 %d 页失败（跳过）: %v", p, err)
			return
		}
		addPool(result.Comics)
	}

	// 4. 第一页已抓过，直接复用其内容，避免重复请求
	for _, p := range pages {
		if p == 1 {
			fetchPage(1)
			break
		}
	}

	for _, p := range pages {
		if p == 1 {
			continue
		}
		fetchPage(p)
	}

	// 5. 负向丢弃补位：候选池因负向过滤而不足时，继续随机补采样页
	// （受 maxSamplingPages 与 totalPages 约束：最多采样 maxSamplingPages 页）
	for len(pool) < count && len(picked) < maxSamplingPages && len(picked) < totalPages {
		var nextPage int
		found := false
		for attempt := 0; attempt < totalPages*3+5; attempt++ {
			p := rand.Intn(totalPages) + 1
			if !picked[p] {
				nextPage = p
				found = true
				break
			}
		}
		if !found {
			break
		}
		picked[nextPage] = true
		fetchPage(nextPage)
	}

	if len(pool) == 0 {
		return []OnlineComicDTO{}, nil
	}

	// 6. Fisher-Yates 洗牌后截取前 count 本
	rand.Shuffle(len(pool), func(i, j int) {
		pool[i], pool[j] = pool[j], pool[i]
	})
	if len(pool) > count {
		pool = pool[:count]
	}

	return pool, nil
}
