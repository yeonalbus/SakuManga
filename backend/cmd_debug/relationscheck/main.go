// Package main 画廊关系选择器离线验证工具
//
// 读取用户从 E 站抓取的真实详情页 HTML（MangaExamlpe/Page/*.html），
// 调用 services.ParseGalleryRelationsFromHTML 复用与在线流程完全一致的选择器逻辑，
// 验证 #dms（更新版横幅）/ #gnd（子画廊）/ #gdd-Parent / a[id^='parent_'] / a[id^='child_']
// 在真实 DOM 上能否命中。
//
// 用法：cd backend && go run ./cmd_debug/relationscheck -dir ../MangaExamlpe/Page
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"SakuHentai/internal/services"
)

func main() {
	dir := flag.String("dir", "../MangaExamlpe/Page", "存放详情页 HTML 的目录")
	flag.Parse()

	files, err := filepath.Glob(filepath.Join(*dir, "*.html"))
	if err != nil {
		fmt.Printf("读取目录失败: %v\n", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Printf("目录 %s 下没有 html 文件\n", *dir)
		os.Exit(1)
	}
	sort.Strings(files)

	allOK := true
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			fmt.Printf("[%s] 读取失败: %v\n", filepath.Base(f), err)
			allOK = false
			continue
		}
		rel, err := services.ParseGalleryRelationsFromHTML(data)
		if err != nil {
			fmt.Printf("[%s] 解析失败: %v\n", filepath.Base(f), err)
			allOK = false
			continue
		}

		fmt.Printf("==== %s ====\n", filepath.Base(f))
		fmt.Printf("  父画廊(ParentGID):      %-10s token=%s\n", rel.ParentGID, rel.ParentToken)
		fmt.Printf("  更新版(NewVersionGID):  %-10s token=%s\n", rel.NewVersionGID, rel.NewVersionToken)
		fmt.Printf("  子画廊(Children):       %d 个\n", len(rel.Children))
		for _, ch := range rel.Children {
			fmt.Printf("      - gid=%s token=%s\n", ch.GID, ch.Token)
		}
		if rel.ParentGID == "" && rel.NewVersionGID == "" && len(rel.Children) == 0 {
			fmt.Println("  ❌ 未发现任何关系，选择器未命中！")
			allOK = false
		} else {
			fmt.Println("  ✅ 关系解析命中")
		}
	}

	if !allOK {
		fmt.Println("\n存在未命中的情况，请检查选择器。")
		os.Exit(1)
	}
	fmt.Println("\n全部关系解析通过。")
}
