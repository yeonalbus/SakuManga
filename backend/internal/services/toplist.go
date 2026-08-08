package services

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"SakuHentai/internal/models"

	"github.com/PuerkitoBio/goquery"
)

// toplistPageFullRegex 匹配独立 "N pages" 文案（整串匹配，
// 避免与 posted 日期拼接如 "08:05394 pages" 造成贪婪误判）
var toplistPageFullRegex = regexp.MustCompile(`(?i)^\s*(\d+)\s*pages?\s*$`)

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

// fetchToplistPage 向 E 站抓取指定 tl + page 的排行榜（每页 50 条）。
//
// 注意：E 站 toplist.php 的 p 参数为 0 基页码（p=0 → 第 1 页 #1-#50，
// p=1 → 第 2 页 #51-#100），因此把前端 1 基页码 page 转为 p=page-1。
func (s *ToplistService) fetchToplistPage(account *models.AccountSetting, tl string, page int) ([]RankedOnlineComicDTO, error) {
	if account == nil || account.IPBMemberID == "" {
		return nil, fmt.Errorf("账号未绑定或未登录")
	}
	if page < 1 {
		page = 1
	}

	client, err := s.ehService.BuildClient(account)
	if err != nil {
		return nil, fmt.Errorf("构建 Client 失败: %w", err)
	}

	// E 站排行榜分页 0 基：p=page-1
	reqURL := fmt.Sprintf("https://e-hentai.org/toplist.php?tl=%s&p=%d", tl, page-1)

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

	return parseToplistDoc(doc)
}

// parseToplistDoc 从排行榜 HTML 文档解析整页 50 条画廊数据。
// 与 fetchToplistPage 分离，便于直接以保存的样本 HTML 做单元测试。
func parseToplistDoc(doc *goquery.Document) ([]RankedOnlineComicDTO, error) {
	list := make([]RankedOnlineComicDTO, 0, ToplistPageSize)

	// 真实行结构（2026-08 实测样本）：table.itg.gltc > tr，首行为 th 表头；
	// 数据行共 5 列 td：
	//   0) <p>#N</p><p>views</p>
	//   1) td.gl1c.glcat > .cn 分类
	//   2) td.gl2c：封面(glthumb>img) + posted + .ir 评分雪碧 + "N pages"
	//   3) td.gl3c.glname > a[href*='/g/']：.glink 标题 + .gt[title] 标签
	//   4) td.gl4c.glhide：a[href*='/uploader/'] 上传者 + "N pages"
	doc.Find("table.itg.gltc tr").Each(func(_ int, sel *goquery.Selection) {
		if len(list) >= ToplistPageSize {
			return
		}
		if sel.Find("th").Length() > 0 {
			return // 跳过表头行
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

		title := strings.TrimSpace(sel.Find(".glink").First().Text())
		if title == "" {
			return
		}

		rawCoverURL := extractCoverURL(sel)
		proxiedCoverURL := ""
		if rawCoverURL != "" {
			proxiedCoverURL = "/api/v1/comics/cover-proxy?url=" + url.QueryEscape(rawCoverURL)
		}

		category := strings.TrimSpace(sel.Find(".gl1c .cn, .gl1c .cs").First().Text())
		rating := 0.0
		if style, ok := sel.Find(".ir").First().Attr("style"); ok {
			rating = parseToplistRatingFromStyle(style)
		}

		var tags []string
		sel.Find(".gl3c .gt").Each(func(_ int, t *goquery.Selection) {
			if tag, ok := t.Attr("title"); ok && strings.TrimSpace(tag) != "" {
				tags = append(tags, strings.TrimSpace(tag))
			}
		})

		list = append(list, RankedOnlineComicDTO{
			OnlineComicDTO: OnlineComicDTO{
				ID:           gid,
				Token:        token,
				Title:        title,
				CoverURL:     proxiedCoverURL,
				Source:       "online",
				Category:     category,
				Rating:       rating,
				Tags:         tags,
				PageCount:    parseToplistPageCount(sel),
				Uploader:     strings.TrimSpace(sel.Find(".gl4c a[href*='/uploader/']").First().Text()),
				IsDownloaded: false,
			},
			// 真实全局排名与浏览量（替代旧版 (page-1)*50+i 与模拟热度分）
			Rank:  parseToplistRank(sel),
			Score: parseToplistViews(sel),
		})
	})

	if len(list) == 0 {
		return nil, fmt.Errorf("未从 toplist.php 页面解析到任何画廊")
	}

	return list, nil
}

// parseToplistRank 从行首 td 提取全局排名（如 "<p>#51</p>" → 51）
func parseToplistRank(sel *goquery.Selection) int {
	first := sel.ChildrenFiltered("td").First()
	txt := strings.TrimSpace(first.Find("p").First().Text())
	txt = strings.TrimPrefix(txt, "#")
	if n, err := strconv.Atoi(txt); err == nil {
		return n
	}
	return 0
}

// parseToplistViews 从行首 td 提取浏览量（如 "32,801,399" → 32801399）
func parseToplistViews(sel *goquery.Selection) int {
	first := sel.ChildrenFiltered("td").First()
	txt := strings.ReplaceAll(strings.TrimSpace(first.Find("p").Eq(1).Text()), ",", "")
	if n, err := strconv.Atoi(txt); err == nil {
		return n
	}
	return 0
}

// parseToplistPageCount 从行内独立 "N pages" 文案提取页数。
// 逐个 div 整串匹配，避免日期("08:05")与页数拼接成 "08:05394" 被贪婪误判。
func parseToplistPageCount(sel *goquery.Selection) int {
	var result int
	sel.Find(".gl2c div, .gl4c div").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		m := toplistPageFullRegex.FindStringSubmatch(s.Text())
		if len(m) > 1 {
			result, _ = strconv.Atoi(m[1])
			return false
		}
		return true
	})
	return result
}
