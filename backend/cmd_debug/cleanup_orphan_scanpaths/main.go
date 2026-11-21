// Package main 一次性清理工具：删除「路径记录已不存在」的额外扫描路径残留漫画。
//
// 背景（Round53 修复）：
//
//	修复前 DELETE /api/v1/admin/scan-paths/:id 只删 extra_scan_paths 一行，
//	offline_comics 中 scan_path_id 指向该路径的记录全部成为「孤儿」——仍出现在离线库、
//	来源标签因查不到路径名而退化成「下载」、且不再命中「关闭离线维护」集合，
//	会被重新纳入联网更新检测与本地查重。
//
// 本工具只处理「scan_path_id 非空且路径表里已不存在」的记录；路径仍存在的漫画一律不动；
// **磁盘文件一律保留**。历史/书架/阅读清单引用清理与线上删除逻辑完全一致
// （直接复用 services.DeleteOfflineComicsByScanPath）。
//
// 用法（默认只读统计，不动库）：
//
//	cd backend && go run ./cmd_debug/cleanup_orphan_scanpaths -db Y:\SakuManga\manga.db
//
// 真正执行（务必先备份 manga.db，并让珱垣停止线上进程，避免 SQLite 写锁冲突）：
//
//	cd backend && go run ./cmd_debug/cleanup_orphan_scanpaths -db Y:\SakuManga\manga.db -apply
//
// 执行后会把 xp_meta 置脏：线上进程下次查询词云时自动全量重建统计，清掉已删本的贡献。
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"SakuManga/internal/models"
	"SakuManga/internal/services"
)

// orphanGroup 孤儿来源分组（scan_path_id → 记录数）
type orphanGroup struct {
	ScanPathID string `gorm:"column:scan_path_id"`
	Cnt        int64  `gorm:"column:cnt"`
}

func main() {
	dbPath := flag.String("db", "", `manga.db 路径（线上：Y:\SakuManga\manga.db）`)
	apply := flag.Bool("apply", false, "真正执行删除（默认只统计、不动库）")
	sample := flag.Int("sample", 3, "每组打印的样例路径条数")
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

	// 1. 只读统计：scan_path_id 非空且路径表中不存在
	var orphans []orphanGroup
	if err := db.Table("offline_comics").
		Select("scan_path_id, COUNT(*) AS cnt").
		Where("scan_path_id IS NOT NULL AND scan_path_id <> ''").
		Where("scan_path_id NOT IN (SELECT id FROM extra_scan_paths)").
		Group("scan_path_id").
		Order("cnt DESC").
		Scan(&orphans).Error; err != nil {
		fmt.Fprintf(os.Stderr, "统计孤儿记录失败（表结构异常？）: %v\n", err)
		os.Exit(1)
	}

	var total int64
	for _, o := range orphans {
		total += o.Cnt
	}

	var pathTotal int64
	_ = db.Model(&models.ExtraScanPath{}).Count(&pathTotal)

	fmt.Printf("数据库: %s\n", *dbPath)
	fmt.Printf("现存扫描路径 %d 条，孤儿来源 %d 组，共 %d 本漫画记录\n", pathTotal, len(orphans), total)

	if len(orphans) == 0 {
		fmt.Println("没有需要清理的残留记录。")
		return
	}

	for _, o := range orphans {
		fmt.Printf("\n- 已删除的路径 ID: %s（%d 本）\n", o.ScanPathID, o.Cnt)
		var samples []string
		_ = db.Table("offline_comics").
			Select("local_path").
			Where("scan_path_id = ?", o.ScanPathID).
			Limit(*sample).
			Pluck("local_path", &samples).Error
		for _, s := range samples {
			fmt.Printf("    样例: %s\n", s)
		}
	}

	if !*apply {
		fmt.Println("\n[只读模式] 未做任何修改。确认无误后加 -apply 执行清理。")
		fmt.Println("[提醒] 执行前请先备份 manga.db，并停止线上进程（避免 SQLite 写锁冲突）。")
		return
	}

	// 2. 执行清理：复用线上删除逻辑（记录 + 引用清理；磁盘文件一律保留）
	fmt.Println("\n========== 开始清理（磁盘文件保留）==========")
	var removedTotal int
	var failed int
	for _, o := range orphans {
		n, err := services.DeleteOfflineComicsByScanPath(db, o.ScanPathID)
		if err != nil {
			failed++
			log.Printf("清理来源 %s 失败: %v", o.ScanPathID, err)
			continue
		}
		removedTotal += n
		fmt.Printf("- %s: 已清理 %d 本\n", o.ScanPathID, n)
	}

	// 3. XP 统计置脏：线上进程下次查询自动全量重建，清掉已删本的贡献。
	//    （本工具在进程外运行，没有 GlobalXpCloud，无法直接差分重算。）
	if err := db.Exec(`UPDATE xp_meta SET dirty = 1 WHERE id = 1`).Error; err != nil {
		log.Printf("置脏 xp_meta 失败（可忽略，下次手动维护/重启后重建）: %v", err)
	}

	fmt.Printf("\n完成：共清理 %d 本漫画记录（预期 %d 本），失败来源 %d 组。\n", removedTotal, total, failed)
	if removedTotal != int(total) {
		fmt.Println("[注意] 清理数量与统计不一致，请复查日志。")
	}
}
