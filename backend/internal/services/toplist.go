package services

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"SakuHentai/internal/models"

	"github.com/PuerkitoBio/goquery"
)

// ─── 排行榜类型（E 站 toplist.php 的 tl 参数）───
const (
	ToplistAllTime   = "11" // Galleries All-Time
	ToplistPastYear  = "12" // Galleries Past Year
	ToplistPastMonth = "13" // Galleries Past Month
	ToplistYesterday = "15" // Galleries Yesterday（默认）
)

const (
	ToplistMaxPage  = 200 // 排行榜固定 1~200 页
	ToplistPageSize = 50  // 每页 50 个画廊
)

// ToplistType 排行榜类型描述
type ToplistType struct {
	TL    string // toplist.php?tl= 参数
	Name  string // 前端路由/标识用短名
	Label string // 显示标签
}

// ToplistTypes 支持的全部排行榜类型（E 站顺序：All-Time / Past Year / Past Month / Yesterday）
var ToplistTypes = []ToplistType{
	{TL: ToplistAllTime, Name: "alltime", Label: "Galleries All-Time"},
	{TL: ToplistPastYear, Name: "pastyear", Label: "Galleries Past Year"},
	{TL: ToplistPastMonth, Name: "pastmonth", Label: "Galleries Past Month"},
	{TL: ToplistYesterday, Name: "yesterday", Label: "Galleries Yesterday"},
}

// IsValidToplistTL 校验 tl 是否在支持列表内
func IsValidToplistTL(tl string) bool {
	for _, t := range ToplistTypes {
		if t.TL == tl {
			return true
		}
	}
	return false
}

// ToplistTypeLabel 根据 tl 返回显示标签（未知返回空串）
func ToplistTypeLabel(tl string) string {
	for _, t := range ToplistTypes {
		if t.TL == tl {
			return t.Label
		}
	}
	return ""
}

// RankedOnlineComicDTO 带排名与热度的在线画廊
type RankedOnlineComicDTO struct {
	OnlineComicDTO
	Rank  int `json:"rank"`
	Score int `json:"score"`
}

// toplistPageCache 按 (tl, page) 缓存当日榜单页（E 站排行榜每日更新一次）
type toplistPageCache struct {
	expiredAt time.Time
	comics    []RankedOnlineComicDTO
}

type ToplistService struct {
	ehService *EHService
	mu        sync.RWMutex
	pageCache map[string]toplistPageCache
}

func NewToplistService(ehService *EHService) *ToplistService {
	return &ToplistService{
		ehService: ehService,
		pageCache: make(map[string]toplistPageCache),
	}
}

// toplistCacheKey 生成 (tl, page) 缓存键
func toplistCacheKey(tl string, page int) string {
	return tl + ":" + strconv.Itoa(page)
}

// nextToplistExpiry 返回缓存过期时间：次日 0 点（E 站排行榜每日更新一次）
func nextToplistExpiry() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
}

// GetToplist 读取指定类型 + 页码的榜单；优先内存缓存，未命中时实时抓取
func (s *ToplistService) GetToplist(account *models.AccountSetting, tl string, page int) ([]RankedOnlineComicDTO, error) {
	key := toplistCacheKey(tl, page)

	s.mu.RLock()
	entry, ok := s.pageCache[key]
	s.mu.RUnlock()
	if ok && time.Now().Before(entry.expiredAt) {
		return entry.comics, nil
	}

	// 缓存未命中或已过期 → 实时抓取
	comics, err := s.fetchToplistPage(account, tl, page)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.pageCache[key] = toplistPageCache{expiredAt: nextToplistExpiry(), comics: comics}
	s.mu.Unlock()

	log.Printf("[Toplist] 已刷新缓存 tl=%s p=%d，共 %d 条画廊", tl, page, len(comics))
	return comics, nil
}

// fetchToplistPage 向 E 站抓取指定 tl + page 的排行榜（每页 50 条）
func (s *ToplistService) fetchToplistPage(account *models.AccountSetting, tl string, page int) ([]RankedOnlineComicDTO, error) {
	if account == nil || account.IPBMemberID == "" {
		return nil, fmt.Errorf("账号未绑定或未登录")
	}

	client, err := s.ehService.BuildClient(account)
	if err != nil {
		return nil, fmt.Errorf("构建 Client 失败: %w", err)
	}

	reqURL := fmt.Sprintf("https://e-hentai.org/toplist.php?tl=%s&p=%d", tl, page)

	req, _ := http.NewRequest("GET", reqURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("网络请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP 响应异常状态码: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析 HTML 失败: %w", err)
	}

	list := make([]RankedOnlineComicDTO, 0, ToplistPageSize)
	rankCounter := 1

	// 解析整页 50 条榜单数据（每条含封面/标题/分类/评分/链接）
	doc.Find("table.itg tr, div.gl1t, div.gl2t, table.glc tr").Each(func(i int, sel *goquery.Selection) {
		if rankCounter > ToplistPageSize {
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
			proxiedCoverURL = "/api/v1/comics/cover-proxy?url=" + url.QueryEscape(rawCoverURL)
		}

		category := strings.TrimSpace(sel.Find(".cs, .cn").First().Text())
		rating := 0.0
		if style, ok := sel.Find(".ir").Attr("style"); ok {
			rating = parseRatingFromStyle(style)
		}

		// 全局排名 = (page-1)*50 + 页内序号；热度分数沿用模拟值（基于真实全局排名）
		globalRank := (page-1)*ToplistPageSize + rankCounter
		score := 100000 - globalRank*1250

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
			Rank:  globalRank,
			Score: score,
		})

		rankCounter++
	})

	if len(list) == 0 {
		return nil, fmt.Errorf("未从 toplist.php 页面解析到任何画廊")
	}

	return list, nil
}
