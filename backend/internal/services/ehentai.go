package services

import (
	"regexp"
	"fmt"
	"io"
	"log"       // 👈 用于控制台打出排查日志
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"SakuHentai/internal/models"

	"github.com/PuerkitoBio/goquery"
)

// 用于正则匹配 CSS style 里的 url(...)
var urlRegex = regexp.MustCompile(`url\(['"]?(.*?)['"]?\)`)
var resultCountRegex = regexp.MustCompile(`(?i)(?:Found\s+(?:about\s+)?|of\s+)([\d,]+)\s+results`)
var (
	pageCountRegex = regexp.MustCompile(`(\d+)\s*(?:pages|P|页)`)
	dateRegex      = regexp.MustCompile(`\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}`)
)


// 判断是否为有效的封面图片地址（过滤透明占位图）
func isValidCoverURL(u string) bool {
	if u == "" {
		return false
	}
	if strings.Contains(u, "clear.ad.png") || strings.Contains(u, "blank.gif") {
		return false
	}
	return true
}

// 深度提取封面图逻辑
func extractCoverURL(s *goquery.Selection) string {
	var rawURL string

	// 1. 优先从节点内部带有 style="background:...url(...)" 的 div 或 img 中提取
	s.Find("div[style*='url'], img[style*='url']").EachWithBreak(func(i int, sel *goquery.Selection) bool {
		style, _ := sel.Attr("style")
		matches := urlRegex.FindStringSubmatch(style)
		if len(matches) > 1 && isValidCoverURL(matches[1]) {
			rawURL = matches[1]
			return false // 找到了，停止遍历
		}
		return true
	})

	if rawURL != "" {
		return rawURL
	}

	// 2. 若无 CSS 背景图，检查 <img> 的 data-src 或 src 属性
	s.Find("img").EachWithBreak(func(i int, sel *goquery.Selection) bool {
		if dataSrc, ok := sel.Attr("data-src"); ok && isValidCoverURL(dataSrc) {
			rawURL = dataSrc
			return false
		}
		if src, ok := sel.Attr("src"); ok && isValidCoverURL(src) {
			rawURL = src
			return false
		}
		return true
	})

	return rawURL
}

// EHService E站核心服务
type EHService struct{}

func NewEHService() *EHService {
	return &EHService{}
}

// ----------------------------------------------------------------------------
// 1. 数据结构定义
// ----------------------------------------------------------------------------

// SearchParams 前端发来的搜索请求参数
type SearchParams struct {
	Keyword          string   `form:"keyword"`
	Page             int      `form:"page"` // 1-based，前端第 1 页对应 E 站 p=0
	Next             string   `form:"next"` // 支持传递 GID 游标 (可选)
	ActiveCategories []string `form:"categories"`
}

// OnlineComicResult 抓取结果与分页信息
type OnlineComicResult struct {
	Comics      []OnlineComicDTO `json:"comics"`
	TotalPages  int              `json:"totalPages"`
	CurrentPage int              `json:"currentPage"`
}

type OnlineComicDTO struct {
	ID           string   `json:"id"`
	Token        string   `json:"token,omitempty"`
	Title        string   `json:"title"`
	CoverURL     string   `json:"coverUrl"`
	Source       string   `json:"source"`
	Category     string   `json:"category,omitempty"`
	Rating       float64  `json:"rating,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	PageCount    int      `json:"pageCount,omitempty"`
	UpdatedAt    string   `json:"updatedAt"`
	Uploader     string   `json:"uploader,omitempty"`
	IsFavorite   bool     `json:"isFavorite"`         // 👈 补上这行
	FavIndex     *int     `json:"favIndex,omitempty"` // 👈 补上这行
	IsDownloaded bool     `json:"isDownloaded"`
	ClickCount   int      `json:"clickCount,omitempty"`
}

// CategoryBitmaskMap 分类掩码映射表
var categoryBitmaskMap = map[string]int{
	"Misc":       1,
	"Doujinshi":  2,
	"Manga":      4,
	"Artist CG":  8,
	"Game CG":    16,
	"Image Set":  32,
	"Cosplay":    64,
	"Asian Porn": 128,
	"Non-H":      256,
	"Western":    512,
}

// ----------------------------------------------------------------------------
// 2. 账号与 Cookie 认证服务
// ----------------------------------------------------------------------------

// TryFetchIgneous 尝试自动请求 ExHentai 并提取下发的 igneous
func (s *EHService) TryFetchIgneous(setting *models.AccountSetting) string {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return ""
	}

	u, _ := url.Parse("https://exhentai.org")
	cookies := []*http.Cookie{
		{Name: "ipb_member_id", Value: setting.IPBMemberID, Path: "/", Domain: u.Host},
		{Name: "ipb_pass_hash", Value: setting.IPBPassHash, Path: "/", Domain: u.Host},
	}
	jar.SetCookies(u, cookies)

	client := &http.Client{
		Jar:     jar,
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", "https://exhentai.org/", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	for _, c := range jar.Cookies(u) {
		if c.Name == "igneous" && c.Value != "" && c.Value != "mystery" && c.Value != "deleted" {
			return c.Value
		}
	}

	return ""
}

// BuildClient 根据凭证构造 HTTP Client
func (s *EHService) BuildClient(setting *models.AccountSetting) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	domains := []string{"https://e-hentai.org", "https://exhentai.org"}
	for _, domainStr := range domains {
		u, _ := url.Parse(domainStr)
		cookies := []*http.Cookie{
			{Name: "ipb_member_id", Value: setting.IPBMemberID, Path: "/", Domain: u.Host},
			{Name: "ipb_pass_hash", Value: setting.IPBPassHash, Path: "/", Domain: u.Host},
		}
		if setting.Igneous != "" {
			cookies = append(cookies, &http.Cookie{Name: "igneous", Value: setting.Igneous, Path: "/", Domain: u.Host})
		}
		jar.SetCookies(u, cookies)
	}

	client := &http.Client{
		Jar:     jar,
		Timeout: 10 * time.Second,
	}

	return client, nil
}

// VerifyAccount 校验凭证并在必要时自动刷新/抓取 igneous
func (s *EHService) VerifyAccount(setting *models.AccountSetting) (isEx bool, err error) {
	// 1. 未填 igneous 时自动尝试抓取
	if setting.Igneous == "" {
		fetched := s.TryFetchIgneous(setting)
		if fetched != "" {
			setting.Igneous = fetched
		}
	}

	// 2. 尝试验证 ExHentai (里站)
	if setting.Igneous != "" {
		client, err := s.BuildClient(setting)
		if err == nil {
			req, _ := http.NewRequest("GET", "https://exhentai.org/", nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == 200 {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				bodyStr := string(body)
				if !strings.Contains(bodyStr, "sadpanda") && !strings.Contains(bodyStr, "Your IP address has been temporarily banned") {
					return true, nil
				}
			}
		}
	}

	// 3. 抓取/更新 igneous 重新校验里站
	fetched := s.TryFetchIgneous(setting)
	if fetched != "" && fetched != setting.Igneous {
		setting.Igneous = fetched
		client, _ := s.BuildClient(setting)
		req, _ := http.NewRequest("GET", "https://exhentai.org/", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == 200 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if !strings.Contains(string(body), "sadpanda") {
				return true, nil
			}
		}
	}

	// 4. 降级验证 E-Hentai (表站)
	client, err := s.BuildClient(setting)
	if err != nil {
		return false, err
	}

	req, _ := http.NewRequest("GET", "https://e-hentai.org/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("网络连接失败，请检查代理配置: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if strings.Contains(bodyStr, "act=Logout") || strings.Contains(bodyStr, setting.IPBMemberID) {
		return false, nil
	}

	return false, fmt.Errorf("Cookie 凭证无效或已过期")
}

// ----------------------------------------------------------------------------
// 3. 在线画廊列表抓取与 HTML 解析
// ----------------------------------------------------------------------------

// CalculateFCats 计算 E 站反向分类掩码
func CalculateFCats(activeCategories []string) int {
	if len(activeCategories) == 0 {
		return 0
	}
	selectedMask := 0
	for _, cat := range activeCategories {
		if bit, ok := categoryBitmaskMap[cat]; ok {
			selectedMask |= bit
		}
	}
	return 1023 ^ selectedMask
}

// FetchGalleryList 发起网络请求并解析画廊列表
func (s *EHService) FetchGalleryList(account *models.AccountSetting, params SearchParams) (*OnlineComicResult, error) {
	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}

	baseURL := "https://e-hentai.org/"
	if account.IsEx {
		baseURL = "https://exhentai.org/"
	}

	reqURL, _ := url.Parse(baseURL)
	q := reqURL.Query()

	// 1. 搜索关键词
	if params.Keyword != "" {
		q.Set("f_search", params.Keyword)
	}

	// 2. 翻页逻辑兼容：优先处理游标 next，其次处理页码 p
	if params.Next != "" {
		q.Set("next", params.Next)
	} else {
		ehPage := params.Page - 1
		if ehPage < 0 {
			ehPage = 0
		}
		q.Set("p", strconv.Itoa(ehPage))
	}

	// 3. 分类掩码
	fCats := CalculateFCats(params.ActiveCategories)
	if fCats > 0 {
		q.Set("f_cats", strconv.Itoa(fCats))
	}

	reqURL.RawQuery = q.Encode()

	// 发起请求
	req, _ := http.NewRequest("GET", reqURL.String(), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
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

		// 1. 封面图
		rawCoverURL := extractCoverURL(s)
		proxiedCoverURL := ""
		if rawCoverURL != "" {
			proxiedCoverURL = "http://localhost:8080/api/v1/comics/cover-proxy?url=" + url.QueryEscape(rawCoverURL)
		}

		// 2. 提取分类
		category := strings.TrimSpace(s.Find(".cs, .cn").First().Text())

		// 3. 提取评分
		rating := 0.0
		if style, ok := s.Find(".ir").Attr("style"); ok {
			rating = parseRatingFromStyle(style)
		}

		// 4. 🟢 核心：提取标签列表 (格式化为 namespace:key)
		var tags []string
		s.Find("div.gt, div.gtl, div.gtw, div.gtd, div[title*=':']").Each(func(_ int, tagNode *goquery.Selection) {
			tagStr := ""
			// 优先读取 title 属性 (如 title="female:big breasts")
			if t, ok := tagNode.Attr("title"); ok && strings.Contains(t, ":") {
				tagStr = t
			} else if id, ok := tagNode.Attr("id"); ok && strings.HasPrefix(id, "ta_") {
				// 兜底读取 id 属性 (如 id="ta_female:big_breasts")
				tagStr = strings.TrimPrefix(id, "ta_")
				tagStr = strings.ReplaceAll(tagStr, "_", " ")
			} else {
				// 再次兜底直接读取文本
				tagStr = strings.TrimSpace(tagNode.Text())
			}

			if tagStr != "" {
				tags = append(tags, strings.ToLower(tagStr))
			}
		})

		// 5. 🟢 提取总页数 PageCount
		pageCount := 0
		itemText := s.Text()
		if matches := pageCountRegex.FindStringSubmatch(itemText); len(matches) > 1 {
			pageCount, _ = strconv.Atoi(matches[1])
		}

		// 6. 🟢 提取发布时间 UpdatedAt
		updatedAt := ""
		if match := dateRegex.FindString(itemText); match != "" {
			updatedAt = match
		}

		// 7. 🟢 提取上传者 Uploader
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
			Tags:         tags,       // 格式如 ["female:big breasts", "artist:okuma"]
			PageCount:    pageCount,  // 页数
			UpdatedAt:    updatedAt,  // 时间
			Uploader:     uploader,   // 上传者
			IsDownloaded: false,
		})
	})

	// 4. 计算总页数
	totalPages := parseTotalPagesByCount(doc)

	return &OnlineComicResult{
		Comics:      comics,
		TotalPages:  totalPages,
		CurrentPage: params.Page,
	}, nil
}

func parseRatingFromStyle(style string) float64 {
	if strings.Contains(style, "0px 0px") || strings.Contains(style, "0px -1") {
		return 5.0
	}
	if strings.Contains(style, "-16px") {
		return 4.0
	}
	if strings.Contains(style, "-32px") {
		return 3.0
	}
	if strings.Contains(style, "-48px") {
		return 2.0
	}
	if strings.Contains(style, "-64px") {
		return 1.0
	}
	return 0.0
}

func parseTotalPagesByCount(doc *goquery.Document) int {
	// 获取页面包含关键词的节点文本
	var targetText string
	doc.Find("p, div, span").EachWithBreak(func(i int, s *goquery.Selection) bool {
		txt := s.Text()
		if strings.Contains(txt, "results") || strings.Contains(txt, "Found") {
			targetText += " " + txt
		}
		return true
	})

	// 正则匹配提取数字
	matches := resultCountRegex.FindStringSubmatch(targetText)
	if len(matches) > 1 {
		// 清理千分位逗号："2,064,279" -> "2064279"
		cleanNum := strings.ReplaceAll(matches[1], ",", "")
		if totalCount, err := strconv.Atoi(cleanNum); err == nil && totalCount > 0 {
			// 每页 25 条，向上取整计算总页数
			totalPages := (totalCount + 24) / 25
			log.Printf("[EH-DEBUG] 成功抓取到总结果数: %d 条 | 计算得出总页数: %d 页", totalCount, totalPages)
			return totalPages
		}
	}

	log.Printf("[EH-DEBUG] 未提取到总结果数文案，回退默认第 1 页")
	return 1
}

// ----------------------------------------------------------------------------
// 画廊详情页数据结构
// ----------------------------------------------------------------------------

type GalleryDetailResult struct {
	ID           string           `json:"id"`
	Token        string           `json:"token"`
	Title        string           `json:"title"`        // 主标题 (日文/原名)
	SubTitle     string           `json:"subTitle"`     // 副标题 (英文/译名)
	CoverURL     string           `json:"coverUrl"`
	Source       string           `json:"source"`       // "online"
	Category     string           `json:"category"`
	Uploader     string           `json:"uploader"`
	Rating       float64          `json:"rating"`
	PageCount    int              `json:"pageCount"`
	UpdatedAt    string           `json:"updatedAt"`
	Tags         []string         `json:"tags"`         // 格式: ["female:big breasts", "artist:okuma"]
	PreviewPages []PreviewPageDTO `json:"previewPages"` // 预览切片列表
	Comments     []CommentDTO     `json:"comments"`     // 社区评论列表
	IsFavorite   bool             `json:"isFavorite"` // 🟢 新增
	FavIndex     *int             `json:"favIndex"`   // 🟢 新增
}

type PreviewPageDTO struct {
	PageIndex int    `json:"pageIndex"` // 第几页 (1-based)
	ImageURL  string `json:"url"`       // 代理后的缩略图地址
}

type CommentDTO struct {
	ID      int64  `json:"id"`
	User    string `json:"user"`
	Date    string `json:"date"`
	Content string `json:"content"`
}

// extractPreviewImage 深度提取单张预览缩略图（兼容 gdtl 大图、gdtm 精灵图及各类变体）
func extractPreviewImage(s *goquery.Selection) string {
	// 1. 优先尝试从子节点 <img> 的 src / data-src 提取
	var imgURL string
	s.Find("img").EachWithBreak(func(_ int, img *goquery.Selection) bool {
		if dataSrc, ok := img.Attr("data-src"); ok && isValidCoverURL(dataSrc) {
			imgURL = dataSrc
			return false
		}
		if src, ok := img.Attr("src"); ok && isValidCoverURL(src) {
			imgURL = src
			return false
		}
		return true
	})
	if imgURL != "" {
		return imgURL
	}

	// 2. 尝试从 style 属性提取 background: url(...)
	var checkStyle = func(sel *goquery.Selection) string {
		if style, ok := sel.Attr("style"); ok {
			matches := urlRegex.FindStringSubmatch(style)
			if len(matches) > 1 && isValidCoverURL(matches[1]) {
				return matches[1]
			}
		}
		return ""
	}

	// 依次检查：节点自身 -> 父级节点 -> 所有子孙节点
	if u := checkStyle(s); u != "" {
		return u
	}
	if u := checkStyle(s.Parent()); u != "" {
		return u
	}
	s.Find("*[style*='url']").EachWithBreak(func(_ int, child *goquery.Selection) bool {
		if u := checkStyle(child); u != "" {
			imgURL = u
			return false
		}
		return true
	})

	return imgURL
}

// FetchGalleryDetail 请求并解析画廊详情页（全量无重复预览切片 + 社区评论）
func (s *EHService) FetchGalleryDetail(account *models.AccountSetting, gid, token string) (*GalleryDetailResult, error) {
	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}

	baseURL := "https://e-hentai.org"
	if account.IsEx {
		baseURL = "https://exhentai.org"
	}

	// 1. 初始化空切片，提防 null 导致 Vue 报错
	tags := make([]string, 0)
	previewPages := make([]PreviewPageDTO, 0)
	comments := make([]CommentDTO, 0)

	// 2. 请求画廊详情页
	detailURL := fmt.Sprintf("%s/g/%s/%s/?p=0&inline_set=ts_l", baseURL, gid, token)
	req, _ := http.NewRequest("GET", detailURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return nil, fmt.Errorf("获取画廊详情失败")
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("解析 HTML 失败")
	}

	// 解析标题
	title := strings.TrimSpace(doc.Find("#gj").Text())
	if title == "" {
		title = strings.TrimSpace(doc.Find("#gn").Text())
	}
	subTitle := strings.TrimSpace(doc.Find("#gn").Text())

	// 解析封面
	rawCover := extractCoverURL(doc.Find("#gd1"))
	proxiedCover := ""
	if rawCover != "" {
		proxiedCover = "http://localhost:8080/api/v1/comics/cover-proxy?url=" + url.QueryEscape(rawCover)
	}

	uploader := strings.TrimSpace(doc.Find("#gdn").Text())
	category := strings.TrimSpace(doc.Find("#gdc").Text())
	updatedAt := ""
	pageCount := 0

	doc.Find("#gdd tr").Each(func(i int, tr *goquery.Selection) {
		label := strings.TrimSpace(tr.Find("td.gdt1").Text())
		value := strings.TrimSpace(tr.Find("td.gdt2").Text())
		if strings.HasPrefix(label, "Posted:") {
			updatedAt = value
		} else if strings.HasPrefix(label, "Length:") {
			if matches := pageCountRegex.FindStringSubmatch(value); len(matches) > 1 {
				pageCount, _ = strconv.Atoi(matches[1])
			}
		}
	})

	rating := 0.0
	if ratingStr := strings.TrimSpace(doc.Find("#rating_label").Text()); ratingStr != "" {
		parts := strings.Split(ratingStr, ":")
		if len(parts) > 1 {
			rating, _ = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		}
	}

	// 解析标签
	doc.Find("#taglist tr").Each(func(i int, tr *goquery.Selection) {
		ns := strings.TrimSuffix(strings.TrimSpace(tr.Find("td.tc").Text()), ":")
		if ns == "" {
			ns = "other"
		}
		tr.Find("div[id^='ta_'], div.gt, div.gtl").Each(func(_ int, tagNode *goquery.Selection) {
			rawText := strings.TrimSpace(tagNode.Text())
			if rawText != "" {
				cleanKey := strings.ReplaceAll(rawText, "_", " ")
				tags = append(tags, fmt.Sprintf("%s:%s", ns, cleanKey))
			}
		})
	})

	// 🟢 3. 容错解析社区评论（全局搜寻 .c1 节点）
	doc.Find(".c1").Each(func(i int, s *goquery.Selection) {
		user := strings.TrimSpace(s.Find(".c3 a").First().Text())
		if user == "" {
			user = "匿名用户"
		}
		date := strings.TrimSpace(s.Find(".c3").Text())
		if dateMatches := dateRegex.FindString(date); dateMatches != "" {
			date = dateMatches
		}
		content := strings.TrimSpace(s.Find(".c6").Text())
		if content != "" {
			comments = append(comments, CommentDTO{
				ID:      int64(i + 1),
				User:    user,
				Date:    date,
				Content: content,
			})
		}
	})

	// 🟢 4. 核心：解析预览切片 (直接对准 #gdt 内部所有阅读链接)
	pageIdx := 1
	extractPreviewsFromDoc := func(d *goquery.Document) {
		previewLinks := d.Find("#gdt a[href*='/s/']")
		if previewLinks.Length() == 0 {
			previewLinks = d.Find("#gdt > div")
		}

		previewLinks.Each(func(_ int, s *goquery.Selection) {
			rawURL := extractPreviewImage(s)
			if rawURL != "" {
				proxiedURL := "http://localhost:8080/api/v1/comics/cover-proxy?url=" + url.QueryEscape(rawURL)
				previewPages = append(previewPages, PreviewPageDTO{
					PageIndex: pageIdx,
					ImageURL:  proxiedURL,
				})
				pageIdx++
			}
		})
	}

	// 提取第 0 页预览图
	extractPreviewsFromDoc(doc)

	// 🟢 5. 解析预览图最大页数，自动抓取后续预览页
	maxPreviewPage := 1
	doc.Find("table.ptd td, table.ptt td, div.gtb td").Each(func(_ int, td *goquery.Selection) {
		if text := strings.TrimSpace(td.Text()); text != "" {
			if p, err := strconv.Atoi(text); err == nil && p > maxPreviewPage {
				maxPreviewPage = p
			}
		}
	})

	if maxPreviewPage > 1 {
		for p := 1; p < maxPreviewPage; p++ {
			nextPageURL := fmt.Sprintf("%s/g/%s/%s/?p=%d", baseURL, gid, token, p)
			reqNext, _ := http.NewRequest("GET", nextPageURL, nil)
			reqNext.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

			respNext, err := client.Do(reqNext)
			if err == nil && respNext.StatusCode == 200 {
				docNext, err := goquery.NewDocumentFromReader(respNext.Body)
				respNext.Body.Close()
				if err == nil {
					extractPreviewsFromDoc(docNext)
				}
			}
		}
	}

	// 打印后端抓取 Debug 日志
	log.Printf("[EH-DETAIL-DEBUG] 成功抓取画廊 [%s] | 预览图切片: %d 张 | 社区评论: %d 条", gid, len(previewPages), len(comments))

	isFav := false
	var favIdx *int
	
	favLink := doc.Find("#favoritelink")
	if favLink.Length() > 0 {
		favText := strings.TrimSpace(favLink.Text())
		// 如果页面 HTML 上写着 Favorited
		if strings.Contains(favText, "Favorited") {
			isFav = true
			defaultIdx := 0
			favIdx = &defaultIdx
	
			// 匹配 Favorited (Favorite 0 ~ 9)
			for idx := 0; idx <= 9; idx++ {
				if strings.Contains(favText, fmt.Sprintf("Favorite %d", idx)) {
					i := idx
					favIdx = &i
					break
				}
			}
		}
	}
	
	// 赋值给 detail 结构体返回
	return &GalleryDetailResult{
		ID:           gid,
		Token:        token,
		Title:        title,
		SubTitle:     subTitle,
		CoverURL:     proxiedCover,
		Source:       "online",
		Category:     category,
		Uploader:     uploader,
		Rating:       rating,
		PageCount:    pageCount,
		UpdatedAt:    updatedAt,
		Tags:         tags,
		PreviewPages: previewPages,
		Comments:     comments,
		IsFavorite:   isFav,    // 🟢 填入真实状态
		FavIndex:     favIdx,   // 🟢 填入真实分类序号 (0~9)
	}, nil
}

func (s *EHService) FetchPopularList(account *models.AccountSetting) ([]OnlineComicDTO, error) {
	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}

	baseURL := "https://e-hentai.org/"
	if account.IsEx {
		baseURL = "https://exhentai.org/"
	}

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

	// 复用 DOM 解析逻辑
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
			proxiedCoverURL = "http://localhost:8080/api/v1/comics/cover-proxy?url=" + url.QueryEscape(rawCoverURL)
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