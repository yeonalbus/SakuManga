package services

import (
	"math/rand"

	"SakuHentai/internal/models"
)

// FetchRandomGalleryList 抓取一个随机页，并从当页结果中随机采样 count 本画廊。
//
// 策略：E 站没有直接的随机接口，这里采用「随机翻页」近似随机：
//  1. 先请求第 1 页（携带关键词/分类筛选）获取总页数 totalPages；
//  2. 随机翻到 [1, totalPages] 中的某一页；
//  3. 对该页画廊列表做 Fisher-Yates 洗牌，取前 count 本返回。
func (s *EHService) FetchRandomGalleryList(account *models.AccountSetting, params SearchParams, setting *models.EHSetting, count int) ([]OnlineComicDTO, error) {
	if count < 1 {
		count = 1
	}

	// 1. 请求第一页，获取总页数（用于划定随机翻页范围）
	first, err := s.FetchGalleryList(account, params, setting)
	if err != nil {
		return nil, err
	}

	totalPages := first.TotalPages
	if totalPages < 1 {
		// 抓不到总页数时退化为仅首页
		totalPages = 1
	}

	// 2. 随机选择一页（清空游标参数，只保留关键词与分类筛选）
	randomParams := params
	randomParams.Next = ""
	randomParams.Prev = ""
	randomParams.Seek = ""
	randomParams.Page = rand.Intn(totalPages) + 1

	result, err := s.FetchGalleryList(account, randomParams, setting)
	if err != nil {
		return nil, err
	}

	pool := result.Comics
	if len(pool) == 0 {
		return []OnlineComicDTO{}, nil
	}

	// 3. Fisher-Yates 洗牌后截取前 count 本
	rand.Shuffle(len(pool), func(i, j int) {
		pool[i], pool[j] = pool[j], pool[i]
	})
	if len(pool) > count {
		pool = pool[:count]
	}

	return pool, nil
}
