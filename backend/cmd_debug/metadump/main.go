// Package main 元数据解析验证工具（问题3 回归）
//
// 用法：
//
//	cd backend && go run ./cmd_debug/metadump "文件夹1" ["文件夹2" ...]
//
// 对每个参数：若它本身是漫画文件夹（含 metadata/ametadata/ComicInfo.xml）则直接解析；
// 若是容器目录（含若干子文件夹），则递归其直接子文件夹逐个解析。
// 打印解析出的 标题/gid/token/父gid/分类/标签数/页数/大小/发布时间，
// 用于验证 metadata/ametadata/ComicInfo.xml 的稳健解析（画廊 gallery 包裹层、tags 字符串/数组、gid 数字/字符串）。
// 不带参数时默认解析项目根目录下的 MangaExamlpe 样例目录。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"SakuHentai/internal/services"
)

func main() {
	dirs := os.Args[1:]
	if len(dirs) == 0 {
		dirs = []string{"../MangaExamlpe"}
	}
	for _, d := range dirs {
		info, err := os.Stat(d)
		if err != nil || !info.IsDir() {
			fmt.Printf("跳过（非目录）: %s\n\n", d)
			continue
		}
		// 判断该目录是否本身就是漫画文件夹（直接含元数据文件）
		if hasMetaFile(d) {
			dump(d)
			continue
		}
		// 否则视为容器目录，递归其直接子文件夹
		entries, err := os.ReadDir(d)
		if err != nil {
			fmt.Printf("读取失败: %s (%v)\n\n", d, err)
			continue
		}
		found := 0
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			sub := filepath.Join(d, e.Name())
			if hasMetaFile(sub) {
				found++
				dump(sub)
			} else {
				fmt.Printf("目录: %s  （无元数据文件，跳过）\n\n", sub)
			}
		}
		if found == 0 {
			fmt.Printf("目录: %s  （未发现含元数据的子文件夹）\n\n", d)
		}
	}
}

// hasMetaFile 判断目录内是否直接存在 metadata/ametadata/ComicInfo.xml 等元数据文件
func hasMetaFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch filepath.Base(e.Name()) {
		case "metadata", "ametadata", "info.json", "comicinfo.xml", "ComicInfo.xml":
			return true
		}
	}
	return false
}

func dump(dir string) {
	meta := services.ParseDirMetadata(dir)
	fmt.Printf("目录: %s\n", dir)
	fmt.Printf("  title       = %q\n", meta.Title)
	fmt.Printf("  title_jpn   = %q\n", meta.TitleJpn)
	fmt.Printf("  gid         = %q\n", meta.GID)
	fmt.Printf("  token       = %q\n", meta.Token)
	fmt.Printf("  parent_gid  = %q\n", meta.ParentGID)
	fmt.Printf("  category    = %q\n", meta.Category)
	fmt.Printf("  file_count  = %d\n", meta.FileCount)
	fmt.Printf("  file_size   = %d\n", meta.FileSize)
	fmt.Printf("  publish     = %q\n", meta.PublishTime)
	fmt.Printf("  tags(%d)     = %v\n", len(meta.Tags), meta.Tags)
	fmt.Println()
}
