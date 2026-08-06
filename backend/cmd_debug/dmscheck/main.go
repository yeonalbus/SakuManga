// Package main 详情页关系 DOM 结构检查工具
//
// 打印一个 E 站详情页 HTML 中的 #dms（newer version 横幅）/ #gnd（Child galleries）/
// parent_ / child_ / #gdd Parent 行的完整结构与**顺序**，用于确认更新链的罗列方向
// （从旧到新还是从新到旧），以决定更新检测应取第一个还是最后一个新版本。
//
// 用法：cd backend && go run ./cmd_debug/dmscheck ../MangaExamlpe/Page/3990907.html
package main

import (
	"fmt"
	"os"

	"github.com/PuerkitoBio/goquery"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: dmscheck <详情页html>")
		os.Exit(1)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Printf("打开文件失败: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	doc, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		fmt.Printf("解析失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== #dms（newer version 横幅）节点 ===")
	doc.Find("#dms").Each(func(i int, s *goquery.Selection) {
		fmt.Printf("[%d] 横幅文本: %s\n", i, s.Text())
		s.Find("a[href*='/g/']").Each(func(j int, a *goquery.Selection) {
			href, _ := a.Attr("href")
			fmt.Printf("    #dms 链接[%d] href=%s 文本=%q\n", j, href, a.Text())
		})
	})

	fmt.Println("\n=== #gnd（Child galleries）链接 ===")
	doc.Find("#gnd a[href*='/g/']").Each(func(i int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		fmt.Printf("  [%d] href=%s 文本=%q\n", i, href, a.Text())
	})

	fmt.Println("\n=== a[id^='parent_'] / a[id^='child_'] 链接 ===")
	doc.Find("a[id^='parent_'], a[id^='child_']").Each(func(i int, a *goquery.Selection) {
		id, _ := a.Attr("id")
		href, _ := a.Attr("href")
		fmt.Printf("  [%d] id=%s href=%s\n", i, id, href)
	})

	fmt.Println("\n=== #gdd 元数据行 ===")
	doc.Find("#gdd tr").Each(func(i int, tr *goquery.Selection) {
		label := tr.Find("td.gdt1").Text()
		if label != "" {
			fmt.Printf("  gdd[%d] label=%q value=%q\n", i, label, tr.Find("td.gdt2").Text())
		}
	})
}
