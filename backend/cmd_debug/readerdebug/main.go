// 调试工具：诊断在线阅读页 URL 抓取链路
//
// 用法：cd backend && go run ./cmd_debug/readerdebug [gid] [token] [full]
// 说明：从本地 DB（manga.db）读取账号 ID=1 的凭证，抓取指定画廊的
//  1. 预览页 ?p=0，打印 #gdt 内链接与缩略图 <img> 的 src/data-src（判断缩略图推导可行性）
//  2. 第一个 /s/ 页，打印 #i3 img / #img 的原图 URL（验证 /s/ 解析是否有效）
//  3. 传入第三个参数 "full" 时，额外执行 FetchOnlinePageUrls 完整抓取
//     整个画廊全部 /s/ 页并计时（会真实请求全部页面，注意触发 E 站风控风险）
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"

	"github.com/PuerkitoBio/goquery"
)

const (
	ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

func main() {
	gid := "4098415"
	token := "fd9cd4a453"
	if len(os.Args) >= 2 {
		gid = os.Args[1]
	}
	if len(os.Args) >= 3 {
		token = os.Args[2]
	}

	database.InitDB()
	services.InitProxyConfig()

	var account models.AccountSetting
	if err := database.DB.First(&account, 1).Error; err != nil {
		log.Fatalf("无账号（请先在前端绑定并保存 E 站凭证）: %v", err)
	}
	var setting models.EHSetting
	database.DB.First(&setting, 1)
	fmt.Printf("账号 IPBMemberID=%s isEx=%v site=%s preferRedirect=%v\n",
		account.IPBMemberID, account.IsEx, setting.Site, setting.PreferRedirect)

	svc := services.NewEHService()

	// ---- 0. 完整抓取（可选）：仅当第 3 个参数为 "full" 时执行，走真实请求链
	// （gdata 失败 → HTML 兜底并发优化），计时验证。会抓整个画廊全部 /s/ 页。 ----
	if len(os.Args) >= 4 && os.Args[3] == "full" {
		fmt.Println("\n===== 完整抓取 FetchOnlinePageUrls（实测新并发优化） =====")
		start := time.Now()
		pages, perr := svc.FetchOnlinePageUrls(&account, gid, token, &setting)
		elapsed := time.Since(start)
		if perr != nil {
			fmt.Printf("完整抓取失败: %v\n", perr)
		} else {
			fmt.Printf("完整抓取成功: 共 %d 页 | 耗时 %s\n", pages.Total, elapsed)
			for i, u := range pages.URLs {
				if i >= 3 {
					fmt.Printf("  ...（其余省略）\n")
					break
				}
				fmt.Printf("  [%d] %s\n", i, u)
			}
		}
	}

	client, err := svc.BuildClient(&account)
	if err != nil {
		log.Fatal(err)
	}

	baseURL := services.GetBaseURL(&account, &setting)
	fetch := func(urlStr, referer string) string {
		req, _ := http.NewRequest("GET", urlStr, nil)
		req.Close = true
		req.Header.Set("User-Agent", ua)
		if referer != "" {
			req.Header.Set("Referer", referer)
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("[请求失败] %s: %v\n", urlStr, err)
			return ""
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("[GET] status=%d len=%d url=%s\n", resp.StatusCode, len(body), resp.Request.URL.String())
		return string(body)
	}

	// ---- 1. 预览页 ?p=0 ----
	previewURL := fmt.Sprintf("%sg/%s/%s/?p=0", baseURL, gid, token)
	body := fetch(previewURL, baseURL)
	if body == "" {
		return
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n===== 预览页 #gdt 前 5 个 /s/ 链接结构 =====")
	doc.Find("#gdt a[href*='/s/']").EachWithBreak(func(i int, sel *goquery.Selection) bool {
		if i >= 5 {
			return false
		}
		href, _ := sel.Attr("href")
		imgHTML, _ := sel.Html()
		src, _ := sel.Find("img").Attr("src")
		dataSrc, _ := sel.Find("img").Attr("data-src")
		style, _ := sel.Find("div[style]").Attr("style")
		fmt.Printf("[%d] href=%s\n", i, href)
		fmt.Printf("    imgHTML=%s\n", strings.TrimSpace(imgHTML))
		fmt.Printf("    src=%q data-src=%q style=%q\n", src, dataSrc, style)
		return true
	})

	// 预览页底部是否有分页（确认多页抓取）
	fmt.Println("\n===== 预览页分页 table.ptt =====")
	doc.Find("table.ptt td").Each(func(_ int, td *goquery.Selection) {
		fmt.Printf("  ptt-td=%q\n", strings.TrimSpace(td.Text()))
	})

	// ---- 2. 抓多个 /s/ 页，对比原图 URL 结构（keystamp/fileindex/文件名 是否可推导） ----
	var sHrefs []string
	doc.Find("#gdt a[href*='/s/']").Each(func(_ int, sel *goquery.Selection) {
		if h, ok := sel.Attr("href"); ok {
			sHrefs = append(sHrefs, h)
		}
	})
	// 补抓预览页 p=1 拿后续页 href
	for _, p := range []int{1} {
		u := fmt.Sprintf("%sg/%s/%s/?p=%d", baseURL, gid, token, p)
		b := fetch(u, baseURL)
		if b == "" {
			continue
		}
		d, _ := goquery.NewDocumentFromReader(strings.NewReader(b))
		d.Find("#gdt a[href*='/s/']").Each(func(_ int, sel *goquery.Selection) {
			if h, ok := sel.Attr("href"); ok {
				sHrefs = append(sHrefs, h)
			}
		})
	}
	fmt.Printf("\n===== 抓取 /s/ 页对比原图 URL（共收集 %d 个链接） =====\n", len(sHrefs))
	for _, idx := range []int{0, 1, 2, 19} {
		if idx >= len(sHrefs) {
			continue
		}
		href := sHrefs[idx]
		sBody := fetch(href, "https://exhentai.org/")
		if sBody == "" {
			continue
		}
		sDoc, err := goquery.NewDocumentFromReader(strings.NewReader(sBody))
		if err != nil {
			continue
		}
		got := ""
		sDoc.Find("#i3 img").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
			src, _ := sel.Attr("src")
			if src != "" {
				got = src
				return false
			}
			return true
		})
		if got == "" {
			sDoc.Find("#img").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
				src, _ := sel.Attr("src")
				if src != "" {
					got = src
					return false
				}
				return true
			})
		}
		fmt.Printf("  /s/[%d] %s\n     原图=%q\n", idx, href, got)
	}

	// ---- 3. 验证缩略图 → 原图推导（复制 deriveOriginalFromThumb 逻辑） ----
	fmt.Println("\n===== 缩略图推导原图（deriveOriginalFromThumb 复刻） =====")
	doc.Find("#gdt a[href*='/s/']").EachWithBreak(func(i int, sel *goquery.Selection) bool {
		if i >= 3 {
			return false
		}
		thumb := ""
		if img := sel.Find("img"); img.Length() > 0 {
			if ds, ok := img.Attr("data-src"); ok && ds != "" {
				thumb = ds
			} else if s2, ok := img.Attr("src"); ok {
				thumb = s2
			}
		}
		orig := ""
		if thumb != "" {
			thumb = strings.TrimSpace(thumb)
			if idx := strings.Index(thumb, "/h/"); idx >= 0 {
				cand := thumb[:idx] + thumb[idx+2:]
				if strings.HasPrefix(cand, "http://") || strings.HasPrefix(cand, "https://") {
					orig = cand
				}
			}
		}
		if orig == "" {
			fmt.Printf("  [%d] 缩略图=%q -> 推导失败\n", i, thumb)
		} else {
			fmt.Printf("  [%d] 缩略图=%q -> 推导原图=%q\n", i, thumb, orig)
		}
		return true
	})
}
