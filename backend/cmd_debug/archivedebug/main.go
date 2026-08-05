// 调试工具：抓取 archiver.php 页面原始 HTML 与表单结构
//
// 用法：cd backend && go run ./cmd_debug/archivedebug [gid] [token]
// 说明：从本地 DB（manga.db）读取账号 ID=1 的凭证，GET 指定画廊的 archiver.php，
// 打印全部 <form> 结构（input/select/textarea/JS 按钮）与含关键字（hathdl / archiver_key /
// do_hathdl / dltype）的原始 HTML 行，用于定位「创建归档后未找到 archiver_key」问题。
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"

	"github.com/PuerkitoBio/goquery"
)

const ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

var kwRe = regexp.MustCompile(`(?i)(hathdl|archiver_key|do_hathdl|dltype|hath|download|form|input|onclick|href)`)
var stripTagRe = regexp.MustCompile(`(?s)<[^>]*>`)

func stripTags(html string) string {
	return stripTagRe.ReplaceAllString(html, " ")
}

func main() {
	gid := "4098402"
	token := ""
	if len(os.Args) >= 2 {
		gid = os.Args[1]
	}
	// 只有 10 位 hex 才视为 token，否则从 DB 读取
	if len(os.Args) >= 3 && len(os.Args[2]) == 10 {
		token = os.Args[2]
	}
	// hathdl 标志：扫描参数（hathdl 之后的下一个参数为 xres）
	doHathdl := false
	hathdlXres := "org"
	for i, a := range os.Args {
		if a == "hathdl" {
			doHathdl = true
			if i+1 < len(os.Args) && len(os.Args[i+1]) > 0 {
				hathdlXres = os.Args[i+1]
			}
		}
	}

	database.InitDB()
	services.InitProxyConfig()

	var account models.AccountSetting
	if err := database.DB.First(&account, 1).Error; err != nil {
		log.Fatalf("无账号: %v", err)
	}
	var setting models.EHSetting
	database.DB.First(&setting, 1)
	// 若未传 token，尝试从 download_tasks 读取
	if token == "" {
		var t models.DownloadTask
		if err := database.DB.Where("g_id = ?", gid).Order("updated_at desc").First(&t).Error; err == nil && t.Token != "" {
			token = t.Token
			fmt.Printf("从 download_tasks 读取 token=%s\n", token)
		}
	}
	fmt.Printf("账号 IPBMemberID=%s gid=%s token=%s\n", account.IPBMemberID, gid, token)
	if token == "" {
		log.Fatalf("未提供 token")
	}

	svc := services.NewEHService()
	client, err := svc.BuildClient(&account)
	if err != nil {
		log.Fatal(err)
	}

	base := "https://e-hentai.org/archiver.php"
	url := base + "?gid=" + gid + "&token=" + token
	fmt.Printf("\n===== GET %s =====\n", url)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("status=%d len=%d finalURL=%s\n", resp.StatusCode, len(body), resp.Request.URL.String())
	dump(body)

	// 1. 去标签文本预览
	text := strings.Join(strings.Fields(stripTags(string(body))), " ")
	if len(text) > 800 {
		text = text[:800]
	}
	fmt.Printf("\n===== 去标签文本(前800) =====\n%s\n", text)

	// 2. 全部 <form> 结构
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err == nil {
		fmt.Println("\n===== <form> 结构 =====")
		doc.Find("form").Each(func(i int, s *goquery.Selection) {
			action, _ := s.Attr("action")
			method, _ := s.Attr("method")
			id, _ := s.Attr("id")
			fmt.Printf("form#%d id=%q action=%q method=%q\n", i, id, action, method)
			s.Find("input, select, textarea, button").Each(func(_ int, el *goquery.Selection) {
				tag := goquery.NodeName(el)
				name, _ := el.Attr("name")
				val, _ := el.Attr("value")
				typ, _ := el.Attr("type")
				onclick, _ := el.Attr("onclick")
				if tag == "input" && typ == "hidden" {
					fmt.Printf("   %s name=%q value=%q\n", tag, name, val)
				} else {
					fmt.Printf("   %s name=%q value=%q type=%q onclick=%q text=%q\n",
						tag, name, val, typ, onclick, strings.TrimSpace(el.Text()))
				}
			})
			fmt.Println()
		})
	}

	// 3. 含关键字的原始 HTML 行
	fmt.Println("===== 原始HTML关键行 =====")
	for _, line := range strings.Split(string(body), "\n") {
		if kwRe.MatchString(line) {
			fmt.Printf("  %s\n", strings.TrimSpace(line))
		}
	}

	// 可选：提交 H@H Downloader 表单（不跟随重定向），观察返回
	if doHathdl {
		xres := hathdlXres
		// 真实浏览器提交：gid/token 在 form action 的 query 中，body 只带 hathdl_xres
		formData := "hathdl_xres=" + xres
		postURL := base + "?gid=" + gid + "&token=" + token
		req2, _ := http.NewRequest("POST", postURL, strings.NewReader(formData))
		req2.Header.Set("User-Agent", ua)
		req2.Header.Set("Referer", url)
		req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		noRedirect := &http.Client{
			Jar: client.Jar,
			CheckRedirect: func(r *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		resp2, err2 := noRedirect.Do(req2)
		if err2 != nil {
			log.Fatalf("POST hathdl 失败: %v", err2)
		}
		defer resp2.Body.Close()
		b2, _ := io.ReadAll(resp2.Body)
		fmt.Printf("\n===== POST hathdl_xres=%s =====\nstatus=%d len=%d loc=%q\n",
			xres, resp2.StatusCode, len(b2), resp2.Header.Get("Location"))
		dump(b2)
	}
}

// dump 打印 HTML 的去标签文本与含关键字原始行
func dump(body []byte) {
	text := strings.Join(strings.Fields(stripTags(string(body))), " ")
	if len(text) > 800 {
		text = text[:800]
	}
	fmt.Printf("\n===== 去标签文本(前800) =====\n%s\n", text)
	fmt.Println("===== 原始HTML关键行 =====")
	for _, line := range strings.Split(string(body), "\n") {
		if kwRe.MatchString(line) {
			fmt.Printf("  %s\n", strings.TrimSpace(line))
		}
	}
}
