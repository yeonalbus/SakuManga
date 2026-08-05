// 调试工具：验证 E 站图片配额与资产解析（仅在表站 e-hentai.org 查询）
//
// 用法：cd backend && go run ./cmd_debug/statusdebug
// 说明：从本地 DB（manga.db）读取账号 ID=1 的凭证，抓取表站相关页面，
//
//	打印去标签后的文本与正则匹配结果，并列出 home.php 中的链接以定位资产所在页面。
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"

	"github.com/PuerkitoBio/goquery"
)

var stripTagDebugRe = regexp.MustCompile(`(?s)<[^>]*>`)

// dumpSection 打印请求结果（状态码/最终URL）与去标签后文本的关键上下文、正则匹配结果
// maxPreview<=0 表示打印全文，否则仅打印前 maxPreview 字符
func dumpSection(label, html string, statusCode int, finalURL string, patterns []string, keywords []string, maxPreview int) {
	text := strings.Join(strings.Fields(stripTagDebugRe.ReplaceAllString(html, " ")), " ")
	fmt.Printf("===== %s =====\n", label)
	fmt.Printf("  status=%d finalURL=%s rawLen=%d textLen=%d\n", statusCode, finalURL, len(html), len(text))
	if maxPreview > 0 && len(text) > maxPreview {
		fmt.Printf("  [前%d字] %s\n", maxPreview, text[:maxPreview])
	} else {
		fmt.Printf("  [全文] %s\n", text)
	}
	for _, p := range patterns {
		re := regexp.MustCompile(p)
		if m := re.FindString(text); m != "" {
			fmt.Printf("  [MATCH ] /%s/  => %q\n", p, m)
		} else {
			fmt.Printf("  [MISS  ] /%s/\n", p)
		}
	}
	fmt.Println()
}

// dumpLinks 打印页面中的链接（含文本），用于定位资产所在子页
func dumpLinks(label, html string) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		fmt.Printf("[%s] 解析链接失败: %v\n", label, err)
		return
	}
	fmt.Printf("===== %s 页面链接 =====\n", label)
	doc.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		text := strings.TrimSpace(sel.Text())
		if text != "" {
			fmt.Printf("  [%s] -> %s\n", text, href)
		}
	})
	fmt.Println()
}

// dumpRawLines 打印原始 HTML 中命中关键词的行（不去标签，保留属性/JS 内容），
// 用于发现藏在标签属性或脚本变量中的资产数据。
func dumpRawLines(label, html string, keywords []string, maxLines int) {
	fmt.Printf("===== %s 原始HTML关键行 =====\n", label)
	shown := 0
	for _, line := range strings.Split(html, "\n") {
		lower := strings.ToLower(line)
		hit := false
		for _, kw := range keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				hit = true
				break
			}
		}
		if hit {
			fmt.Printf("  RAW> %s\n", strings.TrimSpace(line))
			shown++
			if shown >= maxLines {
				fmt.Printf("  ...（达到上限 %d 行）\n", maxLines)
				break
			}
		}
	}
	fmt.Println()
}

func main() {
	database.InitDB()
	services.InitProxyConfig()

	var account models.AccountSetting
	if err := database.DB.First(&account, 1).Error; err != nil {
		log.Fatalf("无账号（请先在前端绑定并保存 E 站凭证）: %v", err)
	}
	var setting models.EHSetting
	database.DB.First(&setting, 1)
	fmt.Printf("账号 IPBMemberID=%s isEx=%v setting.Site=%s setting.PreferRedirect=%v\n",
		account.IPBMemberID, account.IsEx, setting.Site, setting.PreferRedirect)

	svc := services.NewEHService()
	client, err := svc.BuildClient(&account)
	if err != nil {
		log.Fatal(err)
	}

	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	fetch := func(urlStr string) (string, int, string) {
		req, _ := http.NewRequest("GET", urlStr, nil)
		req.Close = true
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("[%s] 请求失败: %v\n", urlStr, err)
			return "", 0, ""
		}
		body, _ := io.ReadAll(resp.Body)
		finalURL := resp.Request.URL.String()
		resp.Body.Close()
		return string(body), resp.StatusCode, finalURL
	}

	patterns := []string{
		`(?i)currently\s+at\s+[\d,]+\s+towards\s+your\s+account\s+limit\s+of\s+[\d,]+`,
		`(?i)at\s+[\d,]+\s+of your\s+[\d,]+\s+image limit`,
		`(?i)you\s+currently\s+have\s+[\d,]+(?:\.\d+)?\s+Hath`,
		`(?i)Buy\s+GP!?\s+Available:\s*[\d,]+\s*Credits`,
		`(?i)Sell\s+GP!?\s+Available:\s*[\d,]+\s*kGP`,
		`(?i)\bGP\s*[:=]?\s*[\d,]+(?:\.\d+)?`,
		`(?i)\bCredits?\s*[:=]?\s*[\d,]+(?:\.\d+)?`,
		`(?i)\bHath\s*[:=]?\s*[\d,]+(?:\.\d+)?`,
	}
	keywords := []string{"account limit", "image limit", "GP", "Credits", "Hath", "Welcome back"}

	// 表站 My Home：全文 + 链接（定位资产所在子页）+ 原始 HTML 关键行（找属性/JS 中的资产）
	html, code, finalURL := fetch("https://e-hentai.org/home.php")
	dumpSection("https://e-hentai.org/home.php (My Home 全文)", html, code, finalURL, patterns, keywords, 0)
	dumpLinks("home.php", html)
	dumpRawLines("home.php", html, []string{"GP", "Credits", "Hath", "quota", "account limit", "balance", "currency", "spending"}, 80)

	// 候选资产页面：Hath Perks（确认 Hath 余额）/ GP·Hath Exchange（全文找个人余额提示）/ Credit Log / 赏金
	// 2025 版首页与画廊列表页已匿名化（无资产栏），资产分布在各自子页。
	balanceKeywords := []string{"you currently have", "you have ", "your gp", "your credits", "your hath", "balance", "wallet", "GP:", "Credits:", "Hath:", "Unspent"}
	for _, u := range []string{
		"https://e-hentai.org/hathperks.php",
		"https://e-hentai.org/exchange.php?t=gp",
		"https://e-hentai.org/exchange.php?t=hath",
		"https://e-hentai.org/logs.php?t=credits",
		"https://e-hentai.org/logs.php?t=gp",
		"https://e-hentai.org/logs.php",
		"https://e-hentai.org/bounty.php",
		"https://e-hentai.org/stats.php",
	} {
		h, c, f := fetch(u)
		maxP := 4000
		if strings.Contains(u, "exchange") {
			maxP = 0 // 交易所页打印全文，确认是否含个人余额提示
		}
		dumpSection(u, h, c, f, patterns, keywords, maxP)
		dumpRawLines(u, h, balanceKeywords, 60)
	}

	// bitcoin.php（Donations）有 #bal 余额区，用 goquery 提取其文本
	if bh, bc, _ := fetch("https://e-hentai.org/bitcoin.php"); bc == 200 {
		fmt.Printf("===== bitcoin.php status=%d（#bal 余额区） =====\n", bc)
		bdoc, _ := goquery.NewDocumentFromReader(strings.NewReader(bh))
		bdoc.Find("#bal").Each(func(_ int, sel *goquery.Selection) {
			fmt.Printf("  #bal 全文: %s\n", strings.Join(strings.Fields(sel.Text()), " "))
		})
		fmt.Println()
	}

	// 画廊详情页：顶部可能显示 GP/Credits/Hath 资产栏（从 Popular 页提取第一个 /g/ 链接）
	if gh, gc, _ := fetch("https://e-hentai.org/popular"); gc == 200 {
		gdoc, _ := goquery.NewDocumentFromReader(strings.NewReader(gh))
		first := ""
		gdoc.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
			if first != "" {
				return
			}
			href, _ := sel.Attr("href")
			if ok, _ := regexp.MatchString(`/g/\d+/`, href); ok {
				first = href
			}
		})
		if first != "" {
			gallURL := first
			if !strings.HasPrefix(gallURL, "http") {
				gallURL = "https://e-hentai.org" + gallURL
			}
			h, c, f := fetch(gallURL)
			dumpSection("画廊详情页 "+gallURL, h, c, f, patterns, keywords, 5000)
			dumpRawLines(gallURL, h, balanceKeywords, 60)
		}
	}

	// 调用真实服务解析，验证修复后结果
	status, err := svc.FetchEHUserStatus(&account, &setting)
	if err != nil {
		log.Fatalf("FetchEHUserStatus 失败: %v", err)
	}
	fmt.Printf("FetchEHUserStatus 结果 => 配额: %d / %d | GP: %s | Credits: %s | Hath: %s\n",
		status.CurrentQuota, status.MaxQuota, status.AssetGP, status.AssetCredits, status.AssetHath)

	// ===== 我的标签（mytags）验证 =====
	mytagsURL := services.GetBaseURL(&account, &setting) + "mytags"
	if mh, mc, mf := fetch(mytagsURL); mc == 200 {
		fmt.Printf("===== %s (My Tags, status=%d finalURL=%s) =====\n", mytagsURL, mc, mf)
		mdoc, _ := goquery.NewDocumentFromReader(strings.NewReader(mh))
		mdoc.Find("div[id^=\"usertag_\"]").Each(func(_ int, row *goquery.Selection) {
			id, _ := row.Attr("id")
			if id == "usertag_0" || id == "usertags_mass" {
				fmt.Printf("  [%s] （新增行/操作区，跳过）\n", id)
				return
			}
			title := ""
			row.Find("div[id^=\"tagpreview_\"]").Each(func(_ int, s *goquery.Selection) {
				title, _ = s.Attr("title")
			})
			watch := ""
			row.Find("input[id^=\"tagwatch_\"]").Each(func(_ int, s *goquery.Selection) {
				if _, ok := s.Attr("checked"); ok {
					watch = "WATCHED"
				}
			})
			hide := ""
			row.Find("input[id^=\"taghide_\"]").Each(func(_ int, s *goquery.Selection) {
				if _, ok := s.Attr("checked"); ok {
					hide = "HIDDEN"
				}
			})
			fmt.Printf("  [%s] title=%q %s %s\n", id, title, watch, hide)
		})
		fmt.Println()
	} else {
		fmt.Printf("===== %s 抓取失败: status=%d =====\n\n", mytagsURL, mc)
	}

	// 调用真实服务解析 mytags，验证修复后的 FetchMyTags
	if mt, err := svc.FetchMyTags(&account, &setting, 0); err != nil {
		fmt.Printf("FetchMyTags 失败: %v\n", err)
	} else {
		fmt.Printf("FetchMyTags 结果 => Watched: %v | Hidden: %v\n", mt.Watched, mt.Hidden)
	}

	// 抓取 ehg_mytags.c.js 研究 usertag_action / tagset_action 提交机制（新建 tagset 依据）
	jsURL := "https://exhentai.org/z/0381/ehg_mytags.c.js"
	if js, jc, _ := fetch(jsURL); jc == 200 {
		fmt.Printf("===== %s (status=%d, len=%d) =====\n", jsURL, jc, len(js))
		jsKeys := []string{"usertag_action", "tagset_action", "tagset_name", "do_usertags", "do_tagset",
			"change_tagset", "create", "newtagset", "tagset_form", "usertag_form"}
		for _, line := range strings.Split(js, "\n") {
			lower := strings.ToLower(line)
			hit := false
			for _, kw := range jsKeys {
				if strings.Contains(lower, kw) {
					hit = true
					break
				}
			}
			if hit {
				fmt.Printf("  JS> %s\n", strings.TrimSpace(line))
			}
		}
		fmt.Println()
	} else {
		fmt.Printf("===== 抓取 %s 失败: status=%d =====\n\n", jsURL, jc)
	}
}
