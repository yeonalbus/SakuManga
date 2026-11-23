// Package main 一次性清理工具：统计并删除「不可读归档」（D4：rar/7z）的离线记录。
//
// 背景：
//
//	D4 落地后扫描器不再把 rar/7z 作为候选，但历史版本曾把它们入库 → 形成
//	「记录存在、0 页、无封面、打不开」的死本子。
//
// 本工具只处理 `source_mode = 'archive'` 且 LocalPath 以 .rar/.7z 结尾的记录；
// **磁盘文件一律保留**；引用清理/XP 统计复用线上删除逻辑（services.RemoveDedupComics）。
//
// 用法（默认只读统计，不动库）：
//
//	cd backend && go run ./cmd_debug/cleanup_unsupported_archives -db Y:\SakuManga\manga.db
//
// 真正执行（务必先备份 manga.db，并停止线上进程，避免 SQLite 写锁冲突）：
//
//	cd backend && go run ./cmd_debug/cleanup_unsupported_archives -db Y:\SakuManga\manga.db -apply
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"SakuManga/internal/models"
	"SakuManga/internal/services"
)

func main() {
	dbPath := flag.String("db", "", `manga.db 路径（线上：Y:\SakuManga\manga.db）`)
	apply := flag.Bool("apply", false, "真正执行删除（默认只统计、不动库）")
	sample := flag.Int("sample", 10, "打印样例条数")
	flag.Parse()

	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "缺少 -db 参数")
		flag.Usage()
		os.Exit(2)
	}
	if _, err := os.Stat(*dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "数据库不存在或不可读: %v\n", err)
		os.Exit(2)
	}

	db, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开数据库失败: %v\n", err)
		os.Exit(1)
	}
	if sqlDB, err := db.DB(); err == nil {
		defer sqlDB.Close()
	}

	// 统计：归档形态且扩展名为 rar/7z（SQL 里无法 Lower 扩展名，先取出再过滤）
	var comics []models.OfflineComic
	if err := db.Select("id", "title", "local_path", "source_mode").
		Where("source_mode = ?", "archive").
		Find(&comics).Error; err != nil {
		fmt.Fprintf(os.Stderr, "读取离线记录失败: %v\n", err)
		os.Exit(1)
	}

	ids := make([]string, 0, 16)
	samples := make([]string, 0, 16)
	for _, c := range comics {
		lower := strings.ToLower(c.LocalPath)
		if strings.HasSuffix(lower, ".rar") || strings.HasSuffix(lower, ".7z") {
			ids = append(ids, c.ID)
			if len(samples) < *sample {
				samples = append(samples, fmt.Sprintf("%s | %s", c.ID, c.LocalPath))
			}
		}
	}

	fmt.Printf("数据库: %s\n", *dbPath)
	fmt.Printf("不可读归档（rar/7z）离线记录: %d 条\n", len(ids))
	for _, s := range samples {
		fmt.Printf("  - %s\n", s)
	}

	if len(ids) == 0 {
		fmt.Println("没有需要清理的记录。")
		return
	}

	if !*apply {
		fmt.Println("\n[只读模式] 未做任何修改。确认无误后加 -apply 执行清理。")
		fmt.Println("[提醒] 执行前请先备份 manga.db，并停止线上进程（避免 SQLite 写锁冲突）。")
		return
	}

	fmt.Println("\n========== 开始清理（磁盘文件保留）==========")
	deleted, err := services.RemoveDedupComics(db, ids, false)
	if err != nil {
		log.Printf("清理过程中出现错误（已删除 %d 条）: %v", deleted, err)
	}
	if err := db.Exec(`UPDATE xp_meta SET dirty = 1 WHERE id = 1`).Error; err != nil {
		log.Printf("置脏 xp_meta 失败（可忽略）: %v", err)
	}
	fmt.Printf("\n完成：已清理 %d 条（预期 %d 条）。\n", deleted, len(ids))
}
