// Package main 调试工具：查询 Test/manga.db 中 offline_comics 的关系字段现状
// （验证 查重/更新检测 返回 0 时，DB 中 parent_g_id / needs_update 等字段的真实值）
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"SakuHentai/internal/models"
)

func main() {
	dbPath := flag.String("db", "../Test/manga.db", "sqlite 数据库路径（相对 backend/ 目录）")
	flag.Parse()

	if _, err := os.Stat(*dbPath); err != nil {
		log.Fatalf("数据库不存在 %q: %v", *dbPath, err)
	}
	db, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}

	var comics []models.OfflineComic
	if err := db.Order("g_id asc").Find(&comics).Error; err != nil {
		log.Fatalf("查询 offline_comics 失败: %v", err)
	}

	fmt.Printf("共 %d 条离线漫画记录:\n", len(comics))
	for i, c := range comics {
		pub := ""
		if c.PublishedAt != nil {
			pub = c.PublishedAt.Format("2006-01-02 15:04")
		}
		fmt.Printf("\n[%d] id=%s\n", i+1, c.ID)
		fmt.Printf("    title     = %q\n", c.Title)
		fmt.Printf("    gid       = %q\n", c.GID)
		fmt.Printf("    token     = %q\n", c.Token)
		fmt.Printf("    parentGID = %q\n", c.ParentGID)
		fmt.Printf("    sourceMode= %q\n", c.SourceMode)
		fmt.Printf("    pageCount = %d\n", c.PageCount)
		fmt.Printf("    published = %s\n", pub)
		fmt.Printf("    needsUpdate= %v  newGID=%q  newToken=%q  note=%q\n", c.NeedsUpdate, c.NewGID, c.NewToken, c.UpdateNote)
		fmt.Printf("    localPath = %q\n", c.LocalPath)
	}
}
