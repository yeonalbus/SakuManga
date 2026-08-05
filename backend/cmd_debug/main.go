package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
)

func main() {
	database.InitDB()
	services.InitProxyConfig()

	// 用户浏览器提供的 sk
	userSK := "REDACTED_EH_SK"

	// 1. 持久化 sk 到 DB（账号 ID=1）
	var account models.AccountSetting
	if err := database.DB.First(&account, 1).Error; err != nil {
		log.Fatalf("无账号: %v", err)
	}
	account.SK = userSK
	if err := database.DB.Save(&account).Error; err != nil {
		log.Fatalf("保存 sk 失败: %v", err)
	}
	fmt.Printf("已持久化 sk=%s\n", userSK)

	// 2. 从 DB 重新读取，确认已落库
	var account2 models.AccountSetting
	database.DB.First(&account2, 1)
	fmt.Printf("DB 中 sk=%s\n", account2.SK)

	// 3. 走应用真实路径 BuildClient 抓取首页验证实时性
	svc := services.NewEHService()
	client, err := svc.BuildClient(&account2)
	if err != nil {
		log.Fatal(err)
	}
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	dateRe := regexp.MustCompile(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}`)
	fmt.Printf("当前 UTC 时间: %s\n", time.Now().UTC().Format("2006-01-02 15:04:05"))

	for _, urlStr := range []string{"https://exhentai.org/", "https://exhentai.org/?f_search=&t=12345"} {
		req, _ := http.NewRequest("GET", urlStr, nil)
		req.Close = true
		req.Header.Set("User-Agent", ua)
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("[%s] 失败: %v\n", urlStr, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		bodyStr := string(body)
		idx := strings.Index(bodyStr, "/g/")
		date := "无画廊"
		if idx >= 0 {
			seg := bodyStr[idx : idx+2000]
			if d := dateRe.FindAllString(seg, 1); len(d) > 0 {
				date = d[0]
			}
		}
		fmt.Printf("[%s] 首条日期=%s 大小=%d\n", urlStr, date, len(body))
	}
}
