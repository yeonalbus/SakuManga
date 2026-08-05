package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"SakuHentai/internal/models"

	"github.com/PuerkitoBio/goquery"
)

// E 站抓取统一浏览器 UA
const ehReaderUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// ErrGalleryUnavailable 表示 E 站画廊不可用（已删除 / 不可用 / 版权下架）
type ErrGalleryUnavailable struct {
	Kind string // "removed" 已删除/不可用；"copyright" 版权下架
}

func (e *ErrGalleryUnavailable) Error() string {
	switch e.Kind {
	case "copyright":
		return "画廊因版权投诉已被 E 站下架（Copyright Violation）"
	case "removed":
		return "画廊已被删除或不可用（This gallery has been removed or is unavailable）"
	default:
		return "画廊不可用，可能已被删除或版权下架"
	}
}

// classifyGalleryUnavailable 根据 E 站不可用页 HTML 文本识别画廊真实状态
func classifyGalleryUnavailable(body string) *ErrGalleryUnavailable {
	lower := strings.ToLower(body)
	// 版权炮下架：明确提示知识产权侵权 / 被下架
	if strings.Contains(lower, "intellectual property infringement") ||
		strings.Contains(lower, "copyright violation") ||
		strings.Contains(lower, "taken down") {
		return &ErrGalleryUnavailable{Kind: "copyright"}
	}
	// 已删除 / 不可用：经典提示语
	if strings.Contains(lower, "removed or is unavailable") ||
		strings.Contains(lower, "gallery has been removed") {
		return &ErrGalleryUnavailable{Kind: "removed"}
	}
	return nil
}

// readBodyLimited 读取响应体（限制大小，仅用于不可用页识别）
func readBodyLimited(resp *http.Response) string {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
	return string(body)
}

// OnlinePagesResult 在线阅读器页列表结果
type OnlinePagesResult struct {
	Total int      `json:"total"`
	URLs  []string `json:"urls"`
}

// gdataResponse E 站 gdata JSON API 返回结构（仅解析所需字段）
type gdataResponse struct {
	GMetadata []struct {
		GID   int64    `json:"gid"`
		Token string   `json:"token"`
		Tags  []string `json:"tags"`
	} `json:"gmetadata"`
}

// FetchOnlinePageUrls 获取在线画廊每页的「原图 URL」列表
//
// 主方案：调用 E 站官方 gdata JSON API（一次 POST 拿回全部页原图地址，速度快）
// 兜底方案：gdata 失败/未解析出 file 标签时，逐页抓取 /s/ 页解析原图（并发受限）
func (s *EHService) FetchOnlinePageUrls(account *models.AccountSetting, gid, token string, setting *models.EHSetting) (*OnlinePagesResult, error) {
	urls, err := s.fetchPageURLsByGData(account, gid, token, setting)
	if err == nil && len(urls) > 0 {
		log.Printf("[EH-READER] 画廊 [%s] 通过 gdata 获取原图列表成功 | 共 %d 页", gid, len(urls))
		return &OnlinePagesResult{Total: len(urls), URLs: urls}, nil
	}

	log.Printf("[EH-READER-WARN] 画廊 [%s] gdata 获取页列表失败（%v），回退到逐页抓取 /s/ 页...", gid, err)
	return s.fetchPageURLsByHTML(account, gid, token, setting)
}

// fetchPageURLsByGData 调用 gdata API：先试 namespace=1，失败再试 namespace=0
func (s *EHService) fetchPageURLsByGData(account *models.AccountSetting, gid, token string, setting *models.EHSetting) ([]string, error) {
	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}
	baseURL := GetBaseURL(account, setting)

	// 1) namespace=1：file 标签带 file: 前缀
	urls, err := s.fetchPageURLsByGDataOnce(client, baseURL, gid, token, 1)
	if err == nil && len(urls) > 0 {
		return urls, nil
	}
	// 2) namespace=0：file 标签是裸 https:// URL
	urls2, err2 := s.fetchPageURLsByGDataOnce(client, baseURL, gid, token, 0)
	if err2 == nil && len(urls2) > 0 {
		return urls2, nil
	}
	if err == nil {
		err = err2
	}
	return nil, err
}

// fetchPageURLsByGDataOnce 单次调用 E 站 api.php 的 gdata 方法
func (s *EHService) fetchPageURLsByGDataOnce(client *http.Client, baseURL, gid, token string, namespace int) ([]string, error) {
	apiURL := strings.TrimSuffix(baseURL, "/") + "/api.php"
	payload := fmt.Sprintf(`{"method":"gdata","gidlist":[[%s,"%s"]],"namespace":%d}`, gid, token, namespace)

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	// 🟢 关键：E-Hentai API 文档明确要求 Content-Type 不能是 application/json，
	// 否则 api.php 会返回「空响应」（gmetadata 为空导致解析不到 file 标签）。
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", ehReaderUserAgent)
	// 🟢 gdata 要求 Referer 与请求域名一致
	req.Header.Set("Referer", baseURL)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body := readBodyLimited(resp)
		if gErr := classifyGalleryUnavailable(body); gErr != nil {
			return nil, gErr
		}
		return nil, fmt.Errorf("gdata 响应状态异常: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data gdataResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("gdata 响应解析失败(namespace=%d): %v | body: %s", namespace, err, truncateForLog(string(body), 300))
	}
	if len(data.GMetadata) == 0 {
		return nil, fmt.Errorf("gdata 未返回画廊元数据(namespace=%d) | body: %s", namespace, truncateForLog(string(body), 300))
	}

	// 解析原图 URL：
	// 1. 优先匹配 file: 前缀的标签（namespace=1 生效时）
	// 2. 兜底匹配 http(s):// 开头的裸 URL（namespace=0 时 file 标签就是纯 URL）
	urls := make([]string, 0, len(data.GMetadata[0].Tags))
	for _, tag := range data.GMetadata[0].Tags {
		var u string
		if strings.HasPrefix(tag, "file:") {
			u = strings.TrimSpace(strings.TrimPrefix(tag, "file:"))
		} else if strings.HasPrefix(tag, "http://") || strings.HasPrefix(tag, "https://") {
			u = strings.TrimSpace(tag)
		}
		if u != "" && (strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")) {
			urls = append(urls, u)
		}
	}

	if len(urls) == 0 {
		tags := data.GMetadata[0].Tags
		sample := tags
		if len(sample) > 10 {
			sample = sample[:10]
		}
		return nil, fmt.Errorf("gdata 未解析到 file 标签(namespace=%d) | gid=%d 标签数=%d 样例=%v | body=%s",
			namespace, data.GMetadata[0].GID, len(tags), sample, truncateForLog(string(body), 300))
	}
	return urls, nil
}

// previewLink 预览页中一个页面的 /s/ 链接
type previewLink struct {
	href string // /s/ 页面链接（用于逐页解析原图）
}

// fetchPageURLsByHTML 兜底方案：抓取详情页得到全部 /s/ 链接，再并发抓取原图 URL
//
// 并发优化（依据 readerdebug 实测确证）：
//  1. 原图 URL 每页的 host/目录/keystamp 签名/fileindex/文件名均不同，无法跨页推导，必须逐页抓 /s/ 解析；
//  2. 预览页：先串行抓 ?p=0 解析 table.ptt 确定总预览页数，其余页并发抓取并保序展平；
//  3. /s/ 页：并发 8 路并去掉固定 sleep（323 页实测从约 28s 降到约 14s）。
func (s *EHService) fetchPageURLsByHTML(account *models.AccountSetting, gid, token string, setting *models.EHSetting) (*OnlinePagesResult, error) {
	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}
	baseURL := GetBaseURL(account, setting)

	// 1. 抓全部预览页，得到完整 /s/ 链接列表（即画廊真实总页数）
	links, err := s.fetchAllPreviewLinks(client, baseURL, gid, token)
	if err != nil {
		return nil, err
	}

	// 2. 并发抓 /s/ 页解析原图 URL（尽力而为，失败的页留空，由前端就近补全）
	urls := make([]string, len(links))
	pageSem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for idx, href := range links {
		wg.Add(1)
		pageSem <- struct{}{}
		go func(i int, href string) {
			defer wg.Done()
			defer func() { <-pageSem }()
			if u, err := s.fetchOriginalImageURL(client, href); err == nil {
				urls[i] = u
			}
		}(idx, href)
	}
	wg.Wait()

	success := 0
	for _, u := range urls {
		if u != "" {
			success++
		}
	}
	log.Printf("[EH-READER] 画廊 [%s] 逐页解析完成：%d/%d 成功", gid, success, len(links))

	if success == 0 {
		return nil, fmt.Errorf("解析原图 URL 全部失败")
	}
	// 🎯 总页数取真实链接数（而非成功数），缺失页由前端按需就近补全
	return &OnlinePagesResult{Total: len(links), URLs: urls}, nil
}

// fetchAllPreviewLinks 抓取画廊全部预览页并展平出所有 /s/ 页面链接（不解析原图）
func (s *EHService) fetchAllPreviewLinks(client *http.Client, baseURL, gid, token string) ([]string, error) {
	const maxPreviewPageCap = 50 // 防御性上限，避免把非页码数字误当总页数导致循环失控

	// 1. 串行抓第一页预览页，确定预览总页数并收集首页链接
	firstLinks, maxPreviewPage, err := s.fetchPreviewPage(client, baseURL, gid, token, 0)
	if err != nil {
		log.Printf("[EH-READER-WARN] 画廊 [%s] 预览首页抓取失败: %v", gid, err)
		return nil, err
	}
	if len(firstLinks) == 0 {
		return nil, fmt.Errorf("未能从详情页解析到任何页面链接")
	}
	if maxPreviewPage < 1 {
		maxPreviewPage = 1
	}
	if maxPreviewPage > maxPreviewPageCap {
		maxPreviewPage = maxPreviewPageCap
	}
	log.Printf("[EH-READER-DEBUG] 画廊 [%s] 预览页共 %d 页，首页收集到 %d 个页面链接", gid, maxPreviewPage, len(firstLinks))

	// 2. 并发抓取其余预览页（?p=1..N-1），按页序展平保证页码顺序
	allLinks := make([][]previewLink, maxPreviewPage)
	allLinks[0] = firstLinks
	var wg sync.WaitGroup
	previewSem := make(chan struct{}, 4)
	for p := 1; p < maxPreviewPage; p++ {
		wg.Add(1)
		previewSem <- struct{}{}
		go func(page int) {
			defer wg.Done()
			defer func() { <-previewSem }()
			links, _, perr := s.fetchPreviewPage(client, baseURL, gid, token, page)
			if perr != nil {
				log.Printf("[EH-READER-WARN] 画廊 [%s] 预览页 %d 抓取失败: %v", gid, page, perr)
				return
			}
			allLinks[page] = links
		}(p)
	}
	wg.Wait()

	var links []string
	for _, pageLinks := range allLinks {
		for _, l := range pageLinks {
			links = append(links, l.href)
		}
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("未能从详情页解析到任何页面链接")
	}
	log.Printf("[EH-READER] 画廊 [%s] 预览页链接收集完成：共 %d 页", gid, len(links))
	return links, nil
}

// ── 在线画廊按页就近解析缓存 ──

// OnlineGalleryCache 单个画廊的页链接与已解析原图缓存
type OnlineGalleryCache struct {
	mu    sync.Mutex
	total int      // 画廊真实总页数
	links []string // 全部 /s/ 链接（下标即页码索引）
	urls  []string // 已解析原图 URL（空串=未解析）
}

var (
	onlineGalleryCaches   = make(map[string]*OnlineGalleryCache)
	onlineGalleryCachesMu sync.Mutex
)

// ensureOnlineGalleryCache 确保画廊链接索引已建立（仅抓预览页，不解析原图）
func (s *EHService) ensureOnlineGalleryCache(account *models.AccountSetting, gid, token string, setting *models.EHSetting) (*OnlineGalleryCache, error) {
	onlineGalleryCachesMu.Lock()
	if c, ok := onlineGalleryCaches[gid]; ok {
		onlineGalleryCachesMu.Unlock()
		return c, nil
	}
	onlineGalleryCachesMu.Unlock()

	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}
	baseURL := GetBaseURL(account, setting)
	links, err := s.fetchAllPreviewLinks(client, baseURL, gid, token)
	if err != nil {
		return nil, err
	}
	c := &OnlineGalleryCache{
		total: len(links),
		links: links,
		urls:  make([]string, len(links)),
	}
	onlineGalleryCachesMu.Lock()
	onlineGalleryCaches[gid] = c
	onlineGalleryCachesMu.Unlock()
	return c, nil
}

// FetchOnlinePageURL 就近解析指定页（1-based）的原图 URL；返回 total 供前端校准总页数
func (s *EHService) FetchOnlinePageURL(account *models.AccountSetting, gid, token string, setting *models.EHSetting, page int) (string, int, error) {
	c, err := s.ensureOnlineGalleryCache(account, gid, token, setting)
	if err != nil {
		return "", 0, err
	}
	idx := page - 1
	c.mu.Lock()
	defer c.mu.Unlock()
	if idx < 0 || idx >= len(c.links) {
		return "", c.total, fmt.Errorf("页码 %d 超出范围（共 %d 页）", page, c.total)
	}
	if c.urls[idx] != "" {
		return c.urls[idx], c.total, nil
	}
	client, err := s.BuildClient(account)
	if err != nil {
		return "", c.total, err
	}
	u, err := s.fetchOriginalImageURL(client, c.links[idx])
	if err != nil {
		return "", c.total, err
	}
	c.urls[idx] = u
	return u, c.total, nil
}

// fetchPreviewPage 抓取单个预览页（?p=N），返回该页的页面链接与预览总页数
func (s *EHService) fetchPreviewPage(client *http.Client, baseURL, gid, token string, page int) ([]previewLink, int, error) {
	pageURL := fmt.Sprintf("%sg/%s/%s/?p=%d", baseURL, gid, token, page)
	req, _ := http.NewRequest("GET", pageURL, nil)
	req.Header.Set("User-Agent", ehReaderUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body := readBodyLimited(resp)
		if gErr := classifyGalleryUnavailable(body); gErr != nil {
			return nil, 0, gErr
		}
		return nil, 0, fmt.Errorf("预览页状态异常: %d", resp.StatusCode)
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	var links []previewLink
	doc.Find("#gdt a[href*='/s/']").Each(func(_ int, sel *goquery.Selection) {
		if href, ok := sel.Attr("href"); ok && href != "" {
			links = append(links, previewLink{href: href})
		}
	})

	maxPreviewPage := 0
	doc.Find("table.ptt td").Each(func(_ int, td *goquery.Selection) {
		if text := strings.TrimSpace(td.Text()); text != "" {
			if n, perr := strconv.Atoi(text); perr == nil && n > maxPreviewPage {
				maxPreviewPage = n
			}
		}
	})
	return links, maxPreviewPage, nil
}

// fetchOriginalImageURL 抓取 /s/ 页面并解析出原图 URL（#i3 img 或 #img）
func (s *EHService) fetchOriginalImageURL(client *http.Client, sLink string) (string, error) {
	req, _ := http.NewRequest("GET", sLink, nil)
	req.Header.Set("User-Agent", ehReaderUserAgent)
	req.Header.Set("Referer", "https://exhentai.org/")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body := readBodyLimited(resp)
		if gErr := classifyGalleryUnavailable(body); gErr != nil {
			return "", gErr
		}
		return "", fmt.Errorf("页面状态异常: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	src := ""
	// 优先 #i3 img（原图容器）
	doc.Find("#i3 img").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		if v, ok := sel.Attr("src"); ok && isValidCoverURL(v) {
			src = v
			return false
		}
		return true
	})
	if src == "" {
		// 兜底 #img（旧版页面结构）
		doc.Find("#img").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
			if v, ok := sel.Attr("src"); ok && isValidCoverURL(v) {
				src = v
				return false
			}
			return true
		})
	}
	if src == "" {
		return "", fmt.Errorf("未解析到原图地址")
	}
	return src, nil
}

// truncateForLog 截断超长字符串用于日志诊断
func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
