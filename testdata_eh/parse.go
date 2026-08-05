package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	dateRe := regexp.MustCompile(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}`)
	glinkRe := regexp.MustCompile(`(?s)<a[^>]*class="glink"[^>]*>(.*?)</a>`)
	glinkRe2 := regexp.MustCompile(`(?s)class="glink"[^>]*>(.*?)</a>`)

	for _, f := range os.Args[1:] {
		data, err := os.ReadFile(f)
		if err != nil {
			fmt.Printf("=== %s: 读取失败 %v\n", f, err)
			continue
		}
		text := string(data)

		var title string
		m := glinkRe.FindStringSubmatch(text)
		if len(m) > 1 {
			title = strings.TrimSpace(m[1])
		} else if m2 := glinkRe2.FindStringSubmatch(text); len(m2) > 1 {
			title = strings.TrimSpace(m2[1])
		} else {
			title = "(未找到 glink)"
		}

		// 找第一个画廊卡片块的日期（gl6c 附近）
		dates := dateRe.FindAllString(text, 8)

		// 检查是否有限流/错误页
		isLimit := strings.Contains(text, "You have exceeded") || strings.Contains(text, "blocked") || strings.Contains(text, "rate limit")

		fmt.Printf("=== %s ===\n", f)
		fmt.Printf("  标题: %s\n", title)
		fmt.Printf("  日期匹配: %v\n", dates)
		fmt.Printf("  疑似限流/错误页: %v\n", isLimit)
		fmt.Println()
	}
}
