package services

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"SakuHentai/internal/models"

	"github.com/PuerkitoBio/goquery"
)

// FetchWatchedList 抓取 E 站订阅列表 (/watched)
func (s *EHService) FetchWatchedList(account *models.AccountSetting, params SearchParams,setting *models.EHSetting) (*OnlineComicResult, error) {
	// 1. 订阅（Watched）页面强依赖 E 站 Cookie，必须先校验账号登录状态
	if account == nil || account.IPBMemberID == "" || account.IPBPassHash == "" {
		return nil, errors.New("未登录 E 站账号或 Cookie 无效，无法获取订阅列表")
	}

	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}

	// 2. 指向 /watched 端点
	// Round15-Bug2：订阅（Watched）是 E 站表站功能，EX 站 /watched 无内容（返回首页/空），
	// 固定指向 e-hentai.org（ex 账号 Cookie 访问表站 /watched 仍能读取订阅列表）。
	reqURL, _ := url.Parse("https://e-hentai.org/")
	q := reqURL.Query()

	// 3. 拼接搜索关键词
	// ⚠️ 实时性说明：E 站订阅页时差（约 2h 缓存）的真正原因是缺少 sk 会话 Cookie，
	// 而非缺少 f_search 参数。请确保账号已配置 sk（见 eh_auth.go BuildClient）。
	if params.Keyword != "" {
		q.Set("f_search", params.Keyword)
	}

	// 4. 游标与日期跳转逻辑 (与首页一致)
	if params.Next != "" {
		q.Set("next", params.Next)
	} else if params.Prev != "" {
		q.Set("prev", params.Prev)
	} else if params.Seek != "" {
		q.Set("seek", params.Seek)
	} else if params.Page > 1 {
		q.Set("p", strconv.Itoa(params.Page-1))
	}

	// 5. 分类掩码计算
	fCats := CalculateFCats(params.ActiveCategories)
	if fCats > 0 {
		q.Set("f_cats", strconv.Itoa(fCats))
	}

	// 🟢 追加随机时间戳参数，强制 E 站绕过服务器端缓存，确保订阅列表始终返回最新数据
	q.Set("t", strconv.FormatInt(time.Now().UnixNano(), 10))

	reqURL.RawQuery = q.Encode()

	// 6. 发起 HTTP 请求
	req, _ := http.NewRequest("GET", reqURL.String(), nil)
	req.Close = true

	// 补全 HTTP 伪装头信息
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	// 🟢 禁用缓存：强制 E 站返回最新订阅列表，避免命中服务器端缓存
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	resp, err := client.Do(req)
	if err != nil && strings.Contains(err.Error(), "EOF") {
		reqRetry, _ := http.NewRequest("GET", reqURL.String(), nil)
		reqRetry.Close = true
		reqRetry.Header = req.Header
		resp, err = client.Do(reqRetry)
	}
	if err != nil {
		return nil, fmt.Errorf("请求 E 站订阅页失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("E 站订阅页响应状态异常: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析订阅页 HTML 失败: %v", err)
	}

	var comics []OnlineComicDTO

	// 7. 解析画廊卡片 (完全复用画廊卡片的选择器)
	doc.Find("table.itg tr, div.gl1t, div.gl2t, div.gl1e, div.gl2e").Each(func(i int, s *goquery.Selection) {
		linkNode := s.Find("a[href*='/g/']").First()
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

		title := s.Find(".glink, .gltitle").First().Text()
		if title == "" {
			return
		}

		rawCoverURL := extractCoverURL(s)
		proxiedCoverURL := ""
		if rawCoverURL != "" {
			proxiedCoverURL = "/api/v1/comics/cover-proxy?url=" + url.QueryEscape(rawCoverURL)
		}

		category := strings.TrimSpace(s.Find(".cs, .cn").First().Text())

		rating := 0.0
		if style, ok := s.Find(".ir").Attr("style"); ok {
			rating = parseRatingFromStyle(style)
		}

		var tags []string
		s.Find("div.gt, div.gtl, div.gtw, div.gtd, div[title*=':']").Each(func(_ int, tagNode *goquery.Selection) {
			tagStr := ""
			if t, ok := tagNode.Attr("title"); ok && strings.Contains(t, ":") {
				tagStr = t
			} else if id, ok := tagNode.Attr("id"); ok && strings.HasPrefix(id, "ta_") {
				tagStr = strings.TrimPrefix(id, "ta_")
				tagStr = strings.ReplaceAll(tagStr, "_", " ")
			} else {
				tagStr = strings.TrimSpace(tagNode.Text())
			}

			if tagStr != "" {
				tags = append(tags, strings.ToLower(tagStr))
			}
		})

		itemText := s.Text()
		// Round11-Bug3：页数改走叶子节点整串匹配 —— 整行文本会把上传者名字数字与页数拼接
		// （如上传者 "Nid135" + "29 pages" → "Nid13529 pages" 误取 13529）
		pageCount := extractListPageCount(s)

		updatedAt := ""
		if match := dateRegex.FindString(itemText); match != "" {
			updatedAt = match
		}

		uploader := ""
		s.Find("a[href*='/uploader/'], .gl5m a, .gl3e a").EachWithBreak(func(_ int, uNode *goquery.Selection) bool {
			txt := strings.TrimSpace(uNode.Text())
			if txt != "" {
				uploader = txt
				return false
			}
			return true
		})

		comics = append(comics, OnlineComicDTO{
			ID:           gid,
			Token:        token,
			Title:        title,
			CoverURL:     proxiedCoverURL,
			Source:       "online",
			Category:     category,
			Rating:       rating,
			Tags:         tags,
			PageCount:    pageCount,
			UpdatedAt:    updatedAt,
			Uploader:     uploader,
			IsDownloaded: false,
		})
	})

	totalPages := parseTotalPages(doc)

	// 8. 提取 Next 游标 (#dnext)
	nextCursor := ""
	if dnext := doc.Find("#dnext"); dnext.Length() > 0 {
		if href, ok := dnext.Attr("href"); ok {
			if u, err := url.Parse(href); err == nil {
				nextCursor = u.Query().Get("next")
				if nextCursor == "" {
					nextCursor = u.Query().Get("from")
				}
			}
		}
	}

	// 9. 提取 Prev 游标 (#dprev)
	prevCursor := ""
	if dprev := doc.Find("#dprev"); dprev.Length() > 0 {
		if href, ok := dprev.Attr("href"); ok {
			if u, err := url.Parse(href); err == nil {
				prevCursor = u.Query().Get("prev")
				if prevCursor == "" {
					prevCursor = u.Query().Get("from")
				}
			}
		}
	}

	return &OnlineComicResult{
		Comics:      comics,
		TotalPages:  totalPages,
		CurrentPage: params.Page,
		Next:        nextCursor,
		Prev:        prevCursor,
		HasMore:     nextCursor != "",
	}, nil
}