package services

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"SakuHentai/internal/models"

	"github.com/PuerkitoBio/goquery"
)

// FetchGalleryList 发起网络请求并解析画廊列表
func (s *EHService) FetchGalleryList(account *models.AccountSetting, params SearchParams, setting *models.EHSetting) (*OnlineComicResult, error) {
	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}

	baseURL := GetBaseURL(account, setting)

	reqURL, _ := url.Parse(baseURL)
	q := reqURL.Query()

	// 1. 搜索关键词
	// ⚠️ 实时性说明：E 站首页时差（约 2h 缓存）的真正原因是缺少 sk 会话 Cookie，
	// 而非缺少 f_search 参数。请确保账号已配置 sk（见 eh_auth.go BuildClient）。
	keyword := params.Keyword
	// E-Hentai tag 语法自动修正（TagMaintain 设置可关，默认开启）：
	// 将 group:da hootch 之类自由输入修正为 group:"da hootch$" 标准语法，
	// 避免 E 站把多词 tag 拆成独立 token 导致无结果（见 fsearch_normalize.go）。
	if keyword != "" && FSearchAutoCorrectEnabled() {
		keyword = NormalizeFSearch(keyword)
	}
	if keyword != "" {
		q.Set("f_search", keyword)
	}

	// 1.5 语言筛选：以 language:xxx token 并入 f_search（与禁用语言过滤互斥）
	// E 站本身没有独立的语言参数，语言过滤通过 f_search 的 language: 命名空间实现。
	if params.Language != "" && params.Language != "All" && !params.DisableLangFilter {
		langToken := "language:" + strings.ToLower(params.Language)
		if existing := q.Get("f_search"); existing != "" {
			q.Set("f_search", existing+" "+langToken)
		} else {
			q.Set("f_search", langToken)
		}
	}

	// 2. 游标与日期跳转优先级逻辑
	if params.Next != "" {
		// 向下滑动：向下游标
		q.Set("next", params.Next)
	} else if params.Prev != "" {
		// 向上滑动：向上游标
		q.Set("prev", params.Prev)
	} else if params.Seek != "" {
		// 按日期跳转：填入 seek 参数 (YYYY-MM-DD)
		q.Set("seek", params.Seek)
	} else if params.Page > 1 {
		// 兜底兼容传统页码
		q.Set("p", strconv.Itoa(params.Page-1))
	}

	// 3. 分类掩码
	fCats := CalculateFCats(params.ActiveCategories)
	if fCats > 0 {
		q.Set("f_cats", strconv.Itoa(fCats))
	}

	// 4. E-Hentai 高级筛选 (advsearch=1 开启后，f_* 参数才会生效)
	q.Set("advsearch", "1")
	if params.OnlyRemoved {
		q.Set("f_sh", "on") // 仅搜索移除了的画廊
	}
	if params.OnlyTorrents {
		q.Set("f_sto", "on") // 只显示有种子的画廊
	}
	if params.MinRating != "" {
		q.Set("f_srdd", params.MinRating) // x星
	}
	if params.DisableLangFilter {
		q.Set("f_sfl", "on") // 禁用语言过滤
	}
	if params.DisableUploaderFilter {
		q.Set("f_sfu", "on") // 禁用上传者过滤
	}
	if params.DisableTagFilter {
		q.Set("f_sft", "on") // 禁用 Tag 过滤
	}

	// 🟢 追加随机时间戳参数，强制 E 站绕过服务器端缓存，确保首页始终返回最新列表
	q.Set("t", strconv.FormatInt(time.Now().UnixNano(), 10))

	reqURL.RawQuery = q.Encode()

	// 发起请求
	req, _ := http.NewRequest("GET", reqURL.String(), nil)
	req.Close = true

	// 补全伪装浏览器请求头
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	// 🟢 禁用缓存：强制 E 站返回最新列表，避免命中服务器端缓存导致首页更新不及时
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
		return nil, fmt.Errorf("请求 E 站失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("E 站响应状态异常: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析 HTML 失败: %v", err)
	}

	var comics []OnlineComicDTO

	// 解析画廊卡片
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

		pageCount := 0
		itemText := s.Text()
		if matches := pageCountRegex.FindStringSubmatch(itemText); len(matches) > 1 {
			pageCount, _ = strconv.Atoi(matches[1])
		}

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

	// 问题2：优先从底部翻页器提取总页数（首页无 "N results" 文案时旧逻辑恒为 1）
	totalPages := parseTotalPages(doc)

	// 🟢 提取 Next 游标 (节点 ID 为 #dnext)
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

	// 🟢 提取 Prev 游标 (节点 ID 为 #dprev)
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
		HasMore:     nextCursor != "", // 如果 Next 游标不为空，则认为还有下一页
	}, nil
}

// FetchPopularList 维持原样...
func (s *EHService) FetchPopularList(account *models.AccountSetting, ehSetting *models.EHSetting) ([]OnlineComicDTO, error) {
	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}

	baseURL := GetBaseURL(account, ehSetting)

	reqURL := baseURL + "popular"

	req, _ := http.NewRequest("GET", reqURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求热门列表失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("E 站热门响应状态异常: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析 HTML 失败: %v", err)
	}

	var comics []OnlineComicDTO

	doc.Find("table.itg tr, div.gl1t, div.gl2t, div.gl1e").Each(func(i int, s *goquery.Selection) {
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

		comics = append(comics, OnlineComicDTO{
			ID:           gid,
			Token:        token,
			Title:        title,
			CoverURL:     proxiedCoverURL,
			Source:       "online",
			Category:     category,
			Rating:       rating,
			IsDownloaded: false,
		})
	})

	return comics, nil
}