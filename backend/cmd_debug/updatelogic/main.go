// Package main 更新检测 + 维护查重 判定逻辑离线仿真验证工具
//
// 不联网、不依赖账号：用 MangaExamlpe/Page 下的真实详情页 HTML + Test/manga.db 的真实
// 漫画记录，逐行复刻 services.CheckUpdates（A1/A2/A3 + B）与 services.MaintainDedup（3a/3b）
// 的判定逻辑，确定性验证 4019697→4051934 子孙对能否被正确检出。
//
// 用法：cd backend && go run ./cmd_debug/updatelogic
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
)

func main() {
	dbPath := flag.String("db", "../Test/manga.db", "sqlite 数据库路径（相对 backend/）")
	pageDir := flag.String("page", "../MangaExamlpe/Page", "详情页 HTML 目录")
	flag.Parse()

	db, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{})
	if err != nil {
		fmt.Printf("打开数据库失败: %v\n", err)
		os.Exit(1)
	}

	var comics []models.OfflineComic
	if err := db.Where("g_id != ''").Order("updated_at desc").Find(&comics).Error; err != nil {
		fmt.Printf("读取漫画失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("数据库漫画 %d 条（updated_at desc）：\n", len(comics))
	for _, c := range comics {
		fmt.Printf("  gid=%-8s token=%-12s parent=%-8s 标题=%q\n", c.GID, c.Token, c.ParentGID, c.Title)
	}

	// 扫描 HTML 目录 → gid → 关系
	htmlMap := map[string]*services.GalleryRelationSummary{}
	entries, err := os.ReadDir(*pageDir)
	if err != nil {
		fmt.Printf("读取 HTML 目录失败: %v\n", err)
		os.Exit(1)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		gid := strings.TrimSuffix(e.Name(), ".html")
		data, err := os.ReadFile(filepath.Join(*pageDir, e.Name()))
		if err != nil {
			continue
		}
		rel, err := services.ParseGalleryRelationsFromHTML(data)
		if err != nil {
			fmt.Printf("  HTML[%s] 解析失败: %v\n", e.Name(), err)
			continue
		}
		htmlMap[gid] = rel
		fmt.Printf("HTML[%s] parent=%s children=%d 个\n", gid, rel.ParentGID, len(rel.Children))
		for _, ch := range rel.Children {
			fmt.Printf("         child gid=%s token=%s added=%s\n", ch.GID, ch.Token, ch.AddedAt)
		}
	}

	fmt.Println("\n──────── 仿真 CheckUpdates（A1/A2/A3 回写 + B 本地）────────")
	simulateCheckUpdates(comics, htmlMap)
	fmt.Println("\n──────── 仿真 MaintainDedup（3a 在线发现 + 3b 本地查重）────────")
	simulateMaintainDedup(comics, htmlMap)
}

// simulateCheckUpdates 复刻 services.CheckUpdates 的判定逻辑
func simulateCheckUpdates(comics []models.OfflineComic, htmlMap map[string]*services.GalleryRelationSummary) {
	work := make([]models.OfflineComic, len(comics))
	copy(work, comics)

	// A 段（逐漫画判定 + 回写 ParentGID）
	for i := range work {
		c := &work[i]
		if c.GID == "" {
			continue
		}
		rel := htmlMap[c.GID]
		if rel == nil {
			continue
		}
		// A1：newer version 横幅 → 本画廊已被新版取代（A→C 一次更新到最新版）
		if rel.NewVersionGID != "" && rel.NewVersionGID != c.GID {
			c.NeedsUpdate = true
			c.NewGID = rel.NewVersionGID
			c.NewToken = rel.NewVersionToken
			c.UpdateNote = buildNote(rel.NewVersionGID, rel.Children)
			fmt.Printf("  A1 %s(gid=%s) → 需要更新到最新版 gid=%s\n", c.Title, c.GID, rel.NewVersionGID)
		} else if len(rel.Children) > 0 && rel.Children[len(rel.Children)-1].GID != c.GID {
			// A2：本画廊存在更新版（子画廊关系）→ 更新到最新版（Children 最后一个）
			latest := rel.Children[len(rel.Children)-1]
			c.NeedsUpdate = true
			c.NewGID = latest.GID
			c.NewToken = latest.Token
			c.UpdateNote = buildNote(latest.GID, rel.Children)
			fmt.Printf("  A2 %s(gid=%s) → 需要更新到最新版 gid=%s\n", c.Title, c.GID, latest.GID)
		}
		// A3：回写父画廊关系（供 B 段本地查重用）
		if rel.ParentGID != "" && rel.ParentGID != c.GID {
			c.ParentGID = rel.ParentGID
			fmt.Printf("  A3 %s(gid=%s) 回写父画廊 parent=%s\n", c.Title, c.GID, rel.ParentGID)
		}
	}

	// B 段：父画廊关系本地查重（依赖 A3 回写后的 ParentGID）
	gidMap := map[string]*models.OfflineComic{}
	for i := range work {
		if work[i].GID != "" {
			gidMap[work[i].GID] = &work[i]
		}
	}
	for i := range work {
		c := &work[i]
		if c.ParentGID == "" || c.ParentGID == c.GID {
			continue
		}
		if p, ok := gidMap[c.ParentGID]; ok && p.GID != c.GID {
			if p.NeedsUpdate {
				fmt.Printf("  B  %s(gid=%s) 已被 A 段标记更新到最新版 gid=%s，B 段跳过避免降级\n", p.Title, p.GID, p.NewGID)
				continue
			}
			p.NeedsUpdate = true
			p.NewGID = c.GID
			p.NewToken = c.Token
			p.UpdateNote = fmt.Sprintf("检测到更新版（父画廊关系）：新版本 %q", c.Title)
			fmt.Printf("  B  %s(gid=%s) 被 %s(gid=%s) 取代，标记更新\n", p.Title, p.GID, c.Title, c.GID)
		}
	}

	fmt.Println("  ── 最终 NeedsUpdate ──")
	for i := range work {
		if work[i].NeedsUpdate {
			fmt.Printf("    ✅ %s(gid=%s) needsUpdate=true → 新版本 gid=%s token=%s\n       note: %s\n",
				work[i].Title, work[i].GID, work[i].NewGID, work[i].NewToken, work[i].UpdateNote)
		}
	}
}

// simulateMaintainDedup 复刻 services.MaintainDedup 规则3（3a 在线发现 + 3b 本地查重）
func simulateMaintainDedup(comics []models.OfflineComic, htmlMap map[string]*services.GalleryRelationSummary) {
	work := make([]models.OfflineComic, len(comics))
	copy(work, comics)
	gidToComic := map[string]*models.OfflineComic{}
	for i := range work {
		if work[i].GID != "" {
			gidToComic[work[i].GID] = &work[i]
		}
	}
	keepSet := map[string]bool{}
	removeSet := map[string]bool{}
	reasonMap := map[string]string{}

	// 3a：在线关系发现（仅 ParentGID 为空且未标记删除的漫画）
	for i := range work {
		c := &work[i]
		if c.GID == "" || c.Token == "" || c.ParentGID != "" || removeSet[c.ID] {
			continue
		}
		rel := htmlMap[c.GID]
		if rel == nil {
			continue
		}
		// 回写父画廊关系
		if rel.ParentGID != "" && rel.ParentGID != c.GID {
			c.ParentGID = rel.ParentGID
			fmt.Printf("  3a %s(gid=%s) 回写父画廊 parent=%s\n", c.Title, c.GID, rel.ParentGID)
		}
		// 本画廊被更新版/子画廊取代（本地存在新版 → 旧版建议删除）
		var successor *models.OfflineComic
		if rel.NewVersionGID != "" && rel.NewVersionGID != c.GID {
			if s, ok := gidToComic[rel.NewVersionGID]; ok && s.ID != c.ID {
				successor = s
			}
		}
		if successor == nil {
			for _, ch := range rel.Children {
				if ch.GID == "" || ch.GID == c.GID {
					continue
				}
				if s, ok := gidToComic[ch.GID]; ok && s.ID != c.ID {
					successor = s // A→C：不 break，取最后一个本地存在的更新版（最新版）
				}
			}
		}
		if successor != nil {
			removeSet[c.ID] = true
			reasonMap[c.ID] = fmt.Sprintf("检测到更新版（父画廊关系）%q：旧版可删除", successor.Title)
			keepSet[successor.ID] = true
			fmt.Printf("  3a %s(gid=%s) 被新版 %s(gid=%s) 取代 → 删除旧版\n", c.Title, c.GID, successor.Title, successor.GID)
		}
	}

	// 3b：本地父画廊关系查重（含 3a 回写的 ParentGID）
	for i := range work {
		c := &work[i]
		if c.ParentGID == "" || c.ParentGID == c.GID {
			continue
		}
		if p, ok := gidToComic[c.ParentGID]; ok && p.ID != c.ID {
			removeSet[p.ID] = true
			reasonMap[p.ID] = fmt.Sprintf("已被更新版（父画廊关系）%q 取代，旧版可删除", c.Title)
			keepSet[c.ID] = true
			fmt.Printf("  3b %s(gid=%s) 的父画廊 %s(gid=%s) 被取代 → 删除父画廊\n", c.Title, c.GID, p.Title, p.GID)
		}
	}

	fmt.Println("  ── 查重结果 ──")
	for i := range work {
		c := work[i]
		switch {
		case keepSet[c.ID]:
			fmt.Printf("    ✅ 保留 %s(gid=%s)\n", c.Title, c.GID)
		case removeSet[c.ID]:
			fmt.Printf("    🗑  删除 %s(gid=%s)  原因: %s\n", c.Title, c.GID, reasonMap[c.ID])
		}
	}
}

// buildNote 复刻 services.buildUpdateNote：A→C 更新备注（最新版 + 中间链条及 added 时间）
func buildNote(latestGID string, children []services.GalleryRelation) string {
	note := fmt.Sprintf("检测到更新版本：最新版 gid=%s", latestGID)
	mids := make([]string, 0, len(children))
	for _, ch := range children {
		if ch.GID == "" || ch.GID == latestGID {
			continue
		}
		if ch.AddedAt != "" {
			mids = append(mids, fmt.Sprintf("%s(%s)", ch.GID, ch.AddedAt))
		} else {
			mids = append(mids, ch.GID)
		}
	}
	if len(mids) > 0 {
		note += "，中间版本：" + strings.Join(mids, " → ")
	}
	return note
}
