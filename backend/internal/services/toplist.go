package services

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"SakuHentai/internal/models"

	"github.com/PuerkitoBio/goquery"
)

type RankedOnlineComicDTO struct {
	OnlineComicDTO
	Rank  int `json:"rank"`
	Score int `json:"score"`
}

type ToplistService struct {
	ehService *EHService
	mu        sync.RWMutex
	cache     []RankedOnlineComicDTO
}

func NewToplistService(ehService *EHService) *ToplistService {
	return &ToplistService{
		ehService: ehService,
		cache:     make([]RankedOnlineComicDTO, 0),
	}
}

// StartScheduler 启动定时任务：启动时刷榜并带重试，之后每天 0 点自动刷新
func (s *ToplistService) StartScheduler(account *models.AccountSetting) {
	go func() {
		// 1. 启动时执行重试刷新 (最多重试 3 次)
		s.refreshWithRetry(account, 3)

		// 2. 每天凌晨 0 点定时刷新
		for {
			now := time.Now()
			nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
			duration := time.Until(nextMidnight)

			log.Printf("[Toplist] 距离下一次凌晨 0 点刷新还有: %v", duration)
			time.Sleep(duration)

			log.Println("[Toplist] 触发每日凌晨 0 点排行榜自动刷新...")
			s.refreshWithRetry(account, 5)
		}
	}()
}

// refreshWithRetry 增加重试机制，避免单次失败后等 24 小时
func (s *ToplistService) refreshWithRetry(account *models.AccountSetting, maxRetries int) {
	for i := 1; i <= maxRetries; i++ {
		err := s.RefreshToplist(account)
		if err == nil {
			return // 成功即退出
		}
		log.Printf("[Toplist] 第 %d/%d 次刷新榜单失败: %v", i, maxRetries, err)
		if i < maxRetries {
			time.Sleep(10 * time.Second) // 间隔 10 秒重试
		}
	}
}

// GetCachedToplist 读取内存数据；若缓存为空，触发按需同步拉取 (懒加载兜底)
func (s *ToplistService) GetCachedToplist(account *models.AccountSetting) []RankedOnlineComicDTO {
	s.mu.RLock()
	if len(s.cache) > 0 {
		defer s.mu.RUnlock()
		return s.cache
	}
	s.mu.RUnlock()

	// 缓存为空，触发按需同步拉取兜底
	log.Println("[Toplist] 内存缓存为空，触发按需同步拉取兜底...")
	_ = s.RefreshToplist(account)

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache
}

// RefreshToplist 向 E 站抓取最新排行榜
func (s *ToplistService) RefreshToplist(account *models.AccountSetting) error {
	if account == nil || account.IPBMemberID == "" {
		return fmt.Errorf("账号未绑定或未登录")
	}

	client, err := s.ehService.BuildClient(account)
	if err != nil {
		return fmt.Errorf("构建 Client 失败: %w", err)
	}

	// 排行榜 (tl=15 每日画廊)
	reqURL := "https://e-hentai.org/toplist.php?tl=15"

	req, _ := http.NewRequest("GET", reqURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("网络请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP 响应异常状态码: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return fmt.Errorf("解析 HTML 失败: %w", err)
	}

	list := make([]RankedOnlineComicDTO, 0, 25)
	rankCounter := 1

	// 解析 E 站排行榜前 25 条数据
	doc.Find("table.itg tr, div.gl1t, div.gl2t, table.glc tr").Each(func(i int, sel *goquery.Selection) {
		if rankCounter > 25 {
			return
		}

		linkNode := sel.Find("a[href*='/g/']").First()
		href, exists := linkNode.Attr("href")
		if !exists {
			return
		}

		parts := strings.Split(strings.Trim(href, "/"), "/")
		if len(parts) < 2 {
			return
		}
		gid := parts[len(parts)-2]
		token := parts[len(parts)-1]

		title := sel.Find(".glink, .gltitle").First().Text()
		if title == "" {
			return
		}

		rawCoverURL := extractCoverURL(sel)
		proxiedCoverURL := ""
		if rawCoverURL != "" {
			proxiedCoverURL = "http://localhost:8081/api/v1/comics/cover-proxy?url=" + url.QueryEscape(rawCoverURL)
		}

		category := strings.TrimSpace(sel.Find(".cs, .cn").First().Text())
		rating := 0.0
		if style, ok := sel.Find(".ir").Attr("style"); ok {
			rating = parseRatingFromStyle(style)
		}

		score := 100000 - rankCounter*1250

		list = append(list, RankedOnlineComicDTO{
			OnlineComicDTO: OnlineComicDTO{
				ID:           gid,
				Token:        token,
				Title:        title,
				CoverURL:     proxiedCoverURL,
				Source:       "online",
				Category:     category,
				Rating:       rating,
				IsDownloaded: false,
			},
			Rank:  rankCounter,
			Score: score,
		})

		rankCounter++
	})

	if len(list) == 0 {
		return fmt.Errorf("未从 toplist.php 页面解析到任何画廊")
	}

	s.mu.Lock()
	s.cache = list
	s.mu.Unlock()

	log.Printf("[Toplist] 成功更新全站热度榜单！共写入 %d 条画廊", len(list))
	return nil
}