package services

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"SakuHentai/internal/models"

	"github.com/PuerkitoBio/goquery"
)

// ============================================================
// EH 设置相关服务：图片配额 / 资产(GP, Credits, Hath) / 我的标签
// 全部为“实时直连 E 站”读取与上传，不落库。
// ============================================================

var (
	// 图片配额（E 站 2025+ 新格式，表站 home.php）：形如
	// "You are currently at 1,234 towards your account limit of 5,000"
	quotaRegex0 = regexp.MustCompile(`(?i)currently\s+at\s+([\d,]+)\s+towards\s+your\s+account\s+limit\s+of\s+([\d,]+)`)
	// 图片配额：形如 "You are at 1,234 of your 5,000 image limit"
	quotaRegex1 = regexp.MustCompile(`(?i)at\s+([\d,]+)\s+of your\s+([\d,]+)\s+image limit`)
	// 图片配额：形如 "image limit: 1234 / 5000"
	quotaRegex2 = regexp.MustCompile(`(?i)image\s+limit[:\s]+([\d,]+)\s*/\s*([\d,]+)`)
	// 图片配额：形如 "1,234 / 5,000 image limit"
	quotaRegex3 = regexp.MustCompile(`(?i)([\d,]+)\s*/\s*([\d,]+)\s+image\s+limit`)

	// 资产数值提取
	assetGpRegex      = regexp.MustCompile(`(?i)\bGP\s*[:=]?\s*([\d,]+(?:\.\d+)?)`)
	assetCreditsRegex = regexp.MustCompile(`(?i)\bCredits?\s*[:=]?\s*([\d,]+(?:\.\d+)?)`)
	assetHathRegex    = regexp.MustCompile(`(?i)\bHath\s*[:=]?\s*([\d,]+(?:\.\d+)?)`)
	// 资产：Hath 当前余额（hathperks.php）形如 "You currently have 8775.15 Hath."（数字在 Hath 前）
	assetHathTextRegex = regexp.MustCompile(`(?i)you\s+currently\s+have\s+([\d,]+(?:\.\d+)?)\s+Hath`)

	// 市场数据（exchange.php?t=gp）：2025 版表站不再展示个人 GP/Credits 余额，
	// 唯一数据来源为交易所市场挂单总量：
	//   Buy GP 表单区  "Buy GP! Available: 31,309 Credits"（市场可买 Credits 总量）
	//   Sell GP 表单区 "Sell GP! Available: 55,910 kGP"（市场可卖 GP 总量，单位 kGP）
	exchangeCreditsRegex = regexp.MustCompile(`(?i)Buy\s+GP!?\s+Available:\s*([\d,]+)\s*Credits`)
	exchangeGpRegex      = regexp.MustCompile(`(?i)Sell\s+GP!?\s+Available:\s*([\d,]+)\s*kGP`)

	// 去除 HTML 标签（仅保留文本内容，便于用正则匹配数字）
	stripTagRegex = regexp.MustCompile(`(?s)<[^>]*>`)
)

// fetchHTML 以伪装浏览器请求头抓取指定 URL 的 HTML 文本
func (s *EHService) fetchHTML(client *http.Client, urlStr string) (string, error) {
	req, _ := http.NewRequest("GET", urlStr, nil)
	req.Close = true
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	resp, err := client.Do(req)
	if err != nil && strings.Contains(err.Error(), "EOF") {
		reqRetry, _ := http.NewRequest("GET", urlStr, nil)
		reqRetry.Close = true
		reqRetry.Header = req.Header
		resp, err = client.Do(reqRetry)
	}
	if err != nil {
		return "", fmt.Errorf("请求 E 站失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("E 站响应状态异常: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取 E 站响应失败: %v", err)
	}
	return string(body), nil
}

// parseCommaInt 解析带千分位逗号的整数
func parseCommaInt(s string) (int, error) {
	return strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(s), ",", ""))
}

// stripHTMLTags 去除 HTML 标签，仅保留可见文本
func stripHTMLTags(html string) string {
	return stripTagRegex.ReplaceAllString(html, " ")
}

// parseQuota 从首页 HTML 中解析图片配额
func parseQuota(html string) (current, max int) {
	text := stripHTMLTags(html)
	for _, re := range []*regexp.Regexp{quotaRegex0, quotaRegex1, quotaRegex2, quotaRegex3} {
		if m := re.FindStringSubmatch(text); len(m) >= 3 {
			c, errC := parseCommaInt(m[1])
			mm, errM := parseCommaInt(m[2])
			if errC == nil && errM == nil && mm > 0 && c >= 0 {
				return c, mm
			}
		}
	}
	return 0, 0
}

// matchAsset 从纯文本中提取资产数值
func matchAsset(text, pattern string) string {
	re := regexp.MustCompile(pattern)
	if m := re.FindStringSubmatch(text); len(m) > 1 {
		return m[1]
	}
	return ""
}

// parseAssets 从 HTML 中解析 GP / Credits / Hath
func parseAssets(html string) (gp, credits, hath string) {
	text := stripHTMLTags(html)
	// Hath 余额优先匹配精确格式（hathperks.php："You currently have 8775.15 Hath."）
	hath = ""
	if m := assetHathTextRegex.FindStringSubmatch(text); len(m) > 1 {
		hath = m[1]
	} else {
		hath = matchAsset(text, assetHathRegex.String())
	}
	return matchAsset(text, assetGpRegex.String()),
		matchAsset(text, assetCreditsRegex.String()),
		hath
}

// parseMarketAssets 从 exchange.php?t=gp 解析市场可用的 GP（kGP）与 Credits 总量。
// 注意：这些是交易所市场挂单量，非个人账户余额（2025 版表站已无个人余额展示）。
func parseMarketAssets(html string) (gp, credits string) {
	text := stripHTMLTags(html)
	// 兼容 &nbsp; 与普通空白
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "\u00a0", " ")
	return matchAsset(text, exchangeGpRegex.String()),
		matchAsset(text, exchangeCreditsRegex.String())
}

// FetchEHUserStatus 读取图片配额与资产（GP / Credits / Hath）
//
// 注意：配额与资产仅表站 e-hentai.org 提供（里站 exhentai.org 的 home.php
// 查询不稳定 / 不展示），因此此处固定使用表站域名，忽略站点偏好。
func (s *EHService) FetchEHUserStatus(account *models.AccountSetting, setting *models.EHSetting) (*models.EHUserStatus, error) {
	if account == nil || account.IPBMemberID == "" || account.IPBPassHash == "" {
		return nil, errors.New("未绑定 E 站账号或 Cookie 无效，无法读取配额与资产")
	}

	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}

	// 配额与资产仅表站可查：固定 e-hentai.org，不随 Site 偏好切换
	const statusBaseURL = "https://e-hentai.org/"
	status := &models.EHUserStatus{}

	// 1. 表站 My Home 页：解析资产（主）与配额（兜底）
	if html, err := s.fetchHTML(client, statusBaseURL+"home.php"); err == nil {
		status.AssetGP, status.AssetCredits, status.AssetHath = parseAssets(html)
		if status.CurrentQuota == 0 && status.MaxQuota == 0 {
			status.CurrentQuota, status.MaxQuota = parseQuota(html)
		}
	}

	// 2. 表站首页：解析配额（主），资产作为兜底
	if html, err := s.fetchHTML(client, statusBaseURL); err == nil {
		if status.CurrentQuota == 0 && status.MaxQuota == 0 {
			status.CurrentQuota, status.MaxQuota = parseQuota(html)
		}
		if status.AssetGP == "" && status.AssetCredits == "" && status.AssetHath == "" {
			status.AssetGP, status.AssetCredits, status.AssetHath = parseAssets(html)
		}
	}

	// 3. 表站 Hath Perks 页：Hath 当前余额（"You currently have X Hath."）
	//    2025 版表站 home.php 不再展示资产余额，Hath 余额仅此页提供
	if html, err := s.fetchHTML(client, statusBaseURL+"hathperks.php"); err == nil {
		if status.AssetHath == "" {
			_, _, status.AssetHath = parseAssets(html)
		}
	}

	// 4. 表站 GP Exchange 页：市场可用的 GP（kGP）与 Credits 总量。
	//    2025 版表站不展示个人 GP/Credits 余额，采用交易所市场挂单量作为资产参考值
	//    （用户确认采用此方案）
	if html, err := s.fetchHTML(client, statusBaseURL+"exchange.php?t=gp"); err == nil {
		gp, credits := parseMarketAssets(html)
		if status.AssetGP == "" {
			status.AssetGP = gp
		}
		if status.AssetCredits == "" {
			status.AssetCredits = credits
		}
	}

	return status, nil
}

// extractMyTags 从 mytags 页解析 Watched / Hidden 标签列表。
//
// 2025+ 新版 mytags 布局（ehg_mytags.c.js）：
//   - 每个标签一行 <div id="usertag_XXX">，XXX 为数字 ID；
//   - 标签名在 <div id="tagpreview_XXX" title="命名空间:名称">：title 为完整标签名
//     （如 "language:chinese"），文本可能是缩写（如 "chinese" / "m:males only"）；
//   - Watched 状态：<input id="tagwatch_XXX" checked> 带 checked 属性；
//   - Hidden 状态：  <input id="taghide_XXX" checked> 带 checked 属性。
// 旧版布局使用 .tagwatch/.taghide class，新版已改为上述 id 结构（这也是此前
// 读取为空的根因）。
func extractMyTags(doc *goquery.Document) (watched, hidden []string) {
	wseen := map[string]bool{}
	hseen := map[string]bool{}

	doc.Find("div[id^=\"usertag_\"]").Each(func(_ int, row *goquery.Selection) {
		id, _ := row.Attr("id")
		// 跳过新增输入行（usertag_0）与批量操作区（usertags_mass）
		if id == "usertag_0" || id == "usertags_mass" || id == "" {
			return
		}

		name := ""
		// 优先取 tagpreview 的 title 属性（完整命名空间:名称）
		row.Find("div[id^=\"tagpreview_\"]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
			if t, ok := s.Attr("title"); ok && strings.TrimSpace(t) != "" {
				name = strings.TrimSpace(t)
			} else if t := strings.TrimSpace(s.Text()); t != "" {
				name = t
			}
			return false
		})
		// 兜底：从标签链接 /tag/命名空间:名称 解析
		if name == "" {
			row.Find("a[href*=\"/tag/\"]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
				if href, ok := s.Attr("href"); ok {
					if u, err := url.Parse(href); err == nil {
						if p := strings.TrimPrefix(u.Path, "/tag/"); p != "" {
							name = p
						}
					}
				}
				return false
			})
		}
		if name == "" {
			return
		}

		isWatched := false
		row.Find("input[id^=\"tagwatch_\"]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
			if _, ok := s.Attr("checked"); ok {
				isWatched = true
			}
			return false
		})

		isHidden := false
		row.Find("input[id^=\"taghide_\"]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
			if _, ok := s.Attr("checked"); ok {
				isHidden = true
			}
			return false
		})

		switch {
		case isWatched && !wseen[name]:
			wseen[name] = true
			watched = append(watched, name)
		case isHidden && !hseen[name]:
			hseen[name] = true
			hidden = append(hidden, name)
		}
	})

	return watched, hidden
}

// FetchMyTags 从 E 站 mytags 页读取关注与隐藏的标签
func (s *EHService) FetchMyTags(account *models.AccountSetting, setting *models.EHSetting) (*models.EHMyTags, error) {
	if account == nil || account.IPBMemberID == "" || account.IPBPassHash == "" {
		return nil, errors.New("未绑定 E 站账号或 Cookie 无效，无法读取我的标签")
	}

	client, err := s.BuildClient(account)
	if err != nil {
		return nil, err
	}

	baseURL := GetBaseURL(account, setting)
	html, err := s.fetchHTML(client, baseURL+"mytags")
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("解析我的标签页 HTML 失败: %v", err)
	}

	watched, hidden := extractMyTags(doc)
	return &models.EHMyTags{
		Watched: watched,
		Hidden:  hidden,
	}, nil
}

// postMyTagsForm 以表单方式 POST 到 mytags 页（新版表单机制，JS 中
// do_usertags_post / do_tagset_post 均为提交 hidden action + 各字段）。
func (s *EHService) postMyTagsForm(client *http.Client, baseURL string, form url.Values) error {
	req, _ := http.NewRequest("POST", baseURL+"mytags", strings.NewReader(form.Encode()))
	req.Close = true
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Referer", baseURL+"mytags")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("上传到 E 站 mytags 失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("E 站 mytags 响应状态异常: %d", resp.StatusCode)
	}
	return nil
}

// AddMyTag 向 E 站 mytags 页上传，添加一个关注/隐藏标签。
//
// 2025+ 新版表单（ehg_mytags.c.js）：新增行输入后回车触发
// do_usertags_post("add")，提交 hidden usertag_action=add 与新增行字段
// tagname_new / tagwatch_new(勾选=on) / taghide_new(勾选=on) / tagweight_new。
// 旧版 watchadd/watchsub 参数已废弃，不再使用。
func (s *EHService) AddMyTag(account *models.AccountSetting, setting *models.EHSetting, action, tag string) error {
	if action != "watch" && action != "hide" {
		return errors.New("action 参数仅支持 watch / hide")
	}
	if account == nil || account.IPBMemberID == "" || account.IPBPassHash == "" {
		return errors.New("未绑定 E 站账号或 Cookie 无效")
	}

	client, err := s.BuildClient(account)
	if err != nil {
		return err
	}

	baseURL := GetBaseURL(account, setting)
	form := url.Values{}
	form.Set("usertag_action", "add")
	form.Set("tagname_new", tag)
	if action == "watch" {
		form.Set("tagwatch_new", "on")
	} else {
		form.Set("taghide_new", "on")
	}
	form.Set("tagweight_new", "10")

	return s.postMyTagsForm(client, baseURL, form)
}

// findMyTagRowID 在 mytags 页中查找指定标签（tagpreview 的 title 全名，
// 如 "language:chinese"）所在行的数字 ID（如 "3437"），供新版批量删除使用
// （modify_usertags[]=<ID>）。未找到返回空串。
func findMyTagRowID(doc *goquery.Document, tag string) string {
	needle := strings.TrimSpace(tag)
	found := ""
	doc.Find("div[id^=\"usertag_\"]").EachWithBreak(func(_ int, row *goquery.Selection) bool {
		id, _ := row.Attr("id")
		if id == "usertag_0" || id == "usertags_mass" || id == "" {
			return true
		}
		row.Find("div[id^=\"tagpreview_\"]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
			if t, ok := s.Attr("title"); ok && strings.TrimSpace(t) == needle {
				found = strings.TrimPrefix(id, "usertag_")
				return false
			}
			return true
		})
		return found == ""
	})
	return found
}

// RemoveMyTag 从 E 站 mytags 页移除一个关注/隐藏标签。
//
// 2025+ 新版表单：无旧版 tagaction=unwatch/unhide 参数，唯一删除机制为
// 批量删除（JS do_usertags_mass）：提交 hidden usertag_action=mass +
// usertag_target=0 + modify_usertags[]=<数字ID>。因此先抓取 mytags 页，
// 解析出标签对应的数字 ID 后再提交。
func (s *EHService) RemoveMyTag(account *models.AccountSetting, setting *models.EHSetting, action, tag string) error {
	if account == nil || account.IPBMemberID == "" || account.IPBPassHash == "" {
		return errors.New("未绑定 E 站账号或 Cookie 无效")
	}

	client, err := s.BuildClient(account)
	if err != nil {
		return err
	}

	baseURL := GetBaseURL(account, setting)

	// 先抓取 mytags 页解析标签的数字 ID
	html, err := s.fetchHTML(client, baseURL+"mytags")
	if err != nil {
		return fmt.Errorf("读取我的标签页失败: %v", err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return fmt.Errorf("解析我的标签页 HTML 失败: %v", err)
	}
	tagID := findMyTagRowID(doc, tag)
	if tagID == "" {
		return fmt.Errorf("未在 E 站 mytags 中找到标签 %q，可能已不存在", tag)
	}

	form := url.Values{}
	form.Set("usertag_action", "mass")
	form.Set("usertag_target", "0")
	form.Add("modify_usertags[]", tagID)

	return s.postMyTagsForm(client, baseURL, form)
}

// CreateMyTagset 在 E 站新建一个 Tagset。
//
// 新版机制（ehg_mytags.c.js 的 do_tagset_create）：提交 tagset_form，
// hidden tagset_action=create + tagset_name=<新名称>。原站通过 JS prompt
// 触发（页面上无独立「新建」按钮），因此应用内提供该能力。
// 注意：新建的 Tagset 默认未选中，标签仍归属当前选中的 Tagset（原站通过
// mytags 页顶部下拉或 ?tagset=N 切换）。
func (s *EHService) CreateMyTagset(account *models.AccountSetting, setting *models.EHSetting, name string) error {
	if name = strings.TrimSpace(name); name == "" {
		return errors.New("tagset 名称不能为空")
	}
	if account == nil || account.IPBMemberID == "" || account.IPBPassHash == "" {
		return errors.New("未绑定 E 站账号或 Cookie 无效")
	}

	client, err := s.BuildClient(account)
	if err != nil {
		return err
	}

	baseURL := GetBaseURL(account, setting)
	form := url.Values{}
	form.Set("tagset_action", "create")
	form.Set("tagset_name", name)

	return s.postMyTagsForm(client, baseURL, form)
}
