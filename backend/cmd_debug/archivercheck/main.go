// Package main 归档下载 404 诊断工具
//
// 目的：更新下载失败（archiver.php 返回 404）的根因定位。
// 用 Test/manga.db 中的 admin 账号凭证，分别请求表站 / 里站的：
//
//	① 画廊详情页 /g/{gid}/{token}/     → 判断画廊是否存在 / 是否 Ex-only / token 是否有效
//	② archiver.php?gid=&token=        → 判断归档是否可访问（404 根因）
//
// 并打印各自 HTTP 状态码与响应体去标签片段，与 download_archive.go 的真实请求保持同参。
//
// 用法：cd backend && go run ./cmd_debug/archivercheck -gid 4086937 -token c9967316cd
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
)

const ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

func main() {
	dbPath := flag.String("db", "../Test/manga.db", "sqlite 数据库路径（相对 backend/）")
	gid := flag.String("gid", "4086937", "要检查的画廊 gid")
	token := flag.String("token", "c9967316cd", "要检查的画廊 token")
	flag.Parse()

	db, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{})
	if err != nil {
		fmt.Printf("打开数据库失败: %v\n", err)
		os.Exit(1)
	}

	// 1. admin 账号凭证
	account := services.LoadAdminAccount(db)
	fmt.Printf("账号: IsEx=%v Site(Account)=%q igneous=%q member=%q\n",
		account.IsEx, account.Site, account.Igneous, account.IPBMemberID)

	// 2. EHSetting（loadEHSetting 未导出，直接查表）
	var ehSetting models.EHSetting
	if err := db.Where("user_id = ?", account.ID).Order("id ASC").First(&ehSetting).Error; err != nil {
		fmt.Printf("读取 EHSetting(user_id=%d) 失败: %v（回退默认 e-hentai）\n", account.ID, err)
		ehSetting = models.EHSetting{Site: "e-hentai"}
	}
	fmt.Printf("EHSetting: Site=%q PreferRedirect=%v SelectedProfile=%q\n",
		ehSetting.Site, ehSetting.PreferRedirect, ehSetting.SelectedProfile)

	// 3. 客户端（与下载引擎同参数：cookie + UA + 代理）
	client, err := (&services.EHService{}).BuildClient(account)
	if err != nil {
		fmt.Printf("构建客户端失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n======== 目标画廊 gid=%s token=%s ========\n\n", *gid, *token)

	domains := []string{"e-hentai.org", "exhentai.org"}
	for _, host := range domains {
		base := "https://" + host
		fmt.Printf("────── %s ──────\n", base)

		// ① 画廊详情页
		gURL := fmt.Sprintf("%s/g/%s/%s/", base, *gid, *token)
		probe(client, "画廊详情页", gURL)

		// ② archiver.php（GET 首访）
		aURL := fmt.Sprintf("%s/archiver.php?gid=%s&token=%s", base, *gid, *token)
		probe(client, "archiver.php", aURL)

		fmt.Println()
	}
}

// probe 请求 URL 并打印状态码 + 响应体片段 + 关键特征识别
func probe(client *http.Client, label, url string) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("  [%s] 构造请求失败: %v\n", label, err)
		return
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", "https://e-hentai.org/")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("  [%s] 请求失败: %v\n", label, err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	elapsed := time.Since(start).Round(time.Millisecond)

	plain := strings.Join(strings.Fields(strings.ToLower(string(body))), " ")
	if len(plain) > 500 {
		plain = plain[:500]
	}
	// 关键特征识别
	traits := []string{}
	detect := func(key string, label string) {
		if strings.Contains(plain, key) {
			traits = append(traits, label)
		}
	}
	detect("gallery not found", "画廊不存在(gallery not found)")
	detect("gallery has been removed", "画廊已删除")
	detect("not logged in", "未登录")
	detect("forbidden", "无权限(forbidden)")
	detect("archiver_key", "归档表单(archiver_key)")
	detect("download original archive", "归档表单(原图)")
	detect("download resample archive", "归档表单(压缩图)")
	detect("being generated", "归档生成中")
	detect("do not have enough funds", "资金不足")
	detect("hathdl", "H@H Downloader 表单")
	detect("star detective", "画廊标题命中(Star Detective)")
	detect("たんプリ", "画廊标题命中(たんプリ)")

	fmt.Printf("  [%s] %d  %s  (%s)\n", label, resp.StatusCode, url, elapsed)
	if len(traits) > 0 {
		fmt.Printf("      特征: %s\n", strings.Join(traits, ", "))
	}
	fmt.Printf("      片段: %q\n", plain)
}
