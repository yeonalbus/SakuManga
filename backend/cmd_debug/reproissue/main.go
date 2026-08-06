// Package main 复现脚本（问题7 / 问题8 最小复现）
//
// 运行方式（必须在 backend/ 目录下执行，内部使用相对路径读取 manga.db 与 ./data）：
//
//	cd backend && go run ./cmd_debug/reproissue
//
// 用途：
//  1. 问题8：用用户输入的真实内容（"3#ARA大羅 REBOOT"、"#"、"3#" 等）驱动 Suggest，
//     验证后端不 panic；并 dump 联想返回的 JSON 载荷，校验 name 恒为字符串、count 恒为数字——
//     即前端「联想渲染崩溃」只可能由前端侧类型异常触发，现已由 TagChip/SearchBar 防御代码兜底。
//  2. 问题7：枚举本地离线漫画，为带 GID 的漫画生成「离线 → 在线画廊详情」跳转所需参数
//     （id=gid & token=token），验证该链路所需数据是否齐备。
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
)

func main() {
	// 仅加载本地磁盘数据（不触发网络下载 / 后台 goroutine）
	database.InitDB()
	services.InitProxyConfig()
	services.GlobalTagEngine.LoadFromDisk()

	fmt.Println("========== 问题8：联想 Suggest 复现 ==========")
	reproSuggest()

	fmt.Println("\n========== 问题7：离线 → 在线画廊跳转链路 ==========")
	reproOfflineToOnline()

	fmt.Println("\n========== 迁移状态：阶段1新增列是否已落库 ==========")
	checkSchema()

	fmt.Println("\n========== 问题4/回归：本地维护失效排查 ==========")
	dumpMaintainFacts()

	fmt.Println("\n========== 问题3/规则4：文件夹内容签名查重干跑 ==========")
	verifyRule4()
}

// reproSuggest 用真实问题输入驱动 Suggest，验证后端不 panic 且载荷类型恒定。
func reproSuggest() {
	queries := []string{
		"3#ARA大羅 REBOOT", // 用户报告触发消失的完整输入
		"3#",               // 触发点：键入第 2 个字符
		"#",                // 单独 #：用户反馈正常
		"3#ARA",
		"大羅",
		"REBOOT",
		"#ARA",
		"",
	}
	for _, q := range queries {
		// 捕获 panic，确认后端是否可能成为崩溃源头
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("[PANIC] query=%q → %v\n", q, r)
				}
			}()
			items := services.GlobalTagEngine.Suggest(q, 8)
			fmt.Printf("query=%q → %d 条\n", q, len(items))
			// dump 前 3 条 JSON 载荷，并校验类型（模拟前端收到的数据）
			for i, it := range items {
				if i >= 3 {
					break
				}
				b, _ := json.Marshal(it)
				if bad := checkItemJSONTypes(b); len(bad) > 0 {
					fmt.Printf("  ⚠️ [类型异常] %v | %s\n", bad, b)
				} else {
					fmt.Printf("  ✅ 类型正常 | %s\n", b)
				}
			}
		}()
	}
	fmt.Println("结论：若上表无 PANIC / 无类型异常，则后端联想数据恒定安全，前端已由防御代码兜底。")
}

// checkItemJSONTypes 把 Suggest 输出还原成前端收到的 JSON，校验 name 必须为字符串、count 必须为数字。
func checkItemJSONTypes(raw []byte) []string {
	var bad []string
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return []string{"JSON 解析失败"}
	}
	if v, ok := m["name"]; ok {
		if _, isStr := v.(string); !isStr {
			bad = append(bad, fmt.Sprintf("name 非字符串: %v(%T)", v, v))
		}
	} else {
		bad = append(bad, "缺少 name 字段")
	}
	if v, ok := m["count"]; ok {
		if _, isNum := v.(float64); !isNum {
			bad = append(bad, fmt.Sprintf("count 非数字: %v(%T)", v, v))
		}
	} else {
		bad = append(bad, "缺少 count 字段")
	}
	return bad
}

// reproOfflineToOnline 枚举带 GID 的离线漫画，验证「离线 → 在线详情页」跳转参数是否齐备。
func reproOfflineToOnline() {
	// 注意：模型字段 GID 经 GORM 默认命名策略落库为 g_id 列（JSON tag 才是 gid），
	// 原始 SQL 查询必须用列名 g_id。
	col := "g_id"
	var count int64
	if err := database.DB.Model(&models.OfflineComic{}).Where(col+" <> ''").Count(&count).Error; err != nil {
		// 阶段1新增列尚未 AutoMigrate 落库时，offline_comics 表可能没有该列
		if strings.Contains(err.Error(), "no such column") {
			fmt.Printf("⚠️ offline_comics 表缺少 %q 列：阶段1新增字段尚未落库。\n", extractMissingColumn(err.Error()))
			fmt.Println("   → 请重启后端服务触发 GORM AutoMigrate 加列后，再运行本脚本验证。")
			fmt.Println("   → 当前表结构字段如下：")
			dumpOfflineComicColumns()
			return
		}
		log.Fatalf("统计带 GID 的离线漫画失败: %v", err)
	}
	if count == 0 {
		fmt.Println("⚠️ 本地无带 GID 的离线漫画，无法验证跳转链路（跳过）。")
		return
	}

	var comics []models.OfflineComic
	if err := database.DB.
		Select("id", "title", col, "token").
		Where(col + " <> ''").
		Order("updated_at DESC").
		Limit(10).
		Find(&comics).Error; err != nil {
		log.Fatalf("查询离线漫画失败: %v", err)
	}

	missingToken := 0
	fmt.Printf("本地共 %d 本带 GID 的离线漫画，取前 %d 本验证链路:\n", count, len(comics))
	for _, c := range comics {
		link := fmt.Sprintf("/online/detail?id=%s&token=%s", c.GID, c.Token)
		flag := "✅"
		if c.Token == "" {
			flag = "⚠️"
			missingToken++
		}
		fmt.Printf("  %s id=%s | gid=%s | token=%q | title=%q\n", flag, c.ID, c.GID, c.Token, truncate(c.Title, 32))
		if c.Token == "" {
			fmt.Printf("      → 该漫画缺失 token，跳转链接 %q 将无法加载在线画廊\n", link)
		}
	}

	if missingToken > 0 {
		fmt.Printf("⚠️ 有 %d 本带 GID 的离线漫画缺失 token（在线详情需 token 才能通过 E 站鉴权）。\n", missingToken)
	} else {
		fmt.Println("✅ 抽查样本均具备 gid + token，离线 → 在线详情跳转所需参数齐备。")
	}
}

// extractMissingColumn 从 SQLite 报错信息中提取缺失列名，如 "no such column: gid" → "gid"
func extractMissingColumn(errMsg string) string {
	const marker = "no such column: "
	idx := strings.Index(errMsg, marker)
	if idx == -1 {
		return "?"
	}
	rest := errMsg[idx+len(marker):]
	if i := strings.IndexAny(rest, " ()"); i != -1 {
		rest = rest[:i]
	}
	return rest
}

// dumpOfflineComicColumns 通过 PRAGMA 打印 offline_comics 当前表结构，便于确认已有哪些列。
func dumpOfflineComicColumns() {
	rows, err := database.DB.Raw("PRAGMA table_info(offline_comics)").Rows()
	if err != nil {
		fmt.Printf("  （无法读取表结构: %v）\n", err)
		return
	}
	defer rows.Close()
	cols := []string{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			continue
		}
		cols = append(cols, name)
	}
	fmt.Printf("  %v\n", cols)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// dumpMaintainFacts 排查「本地维护失效」：额外路径开关值、离线漫画的 gid/parent/hash/路径分布、
// 带 gid 漫画的路径归属与对应开关、以及同名标题样本（潜在重复）。
func dumpMaintainFacts() {
	fmt.Println("── 1. extra_scan_paths（路径维护开关实际值）──")
	var paths []struct {
		ID                  string `gorm:"column:id"`
		Path                string `gorm:"column:path"`
		Name                string `gorm:"column:name"`
		EnableOfflineUpdate bool   `gorm:"column:enable_offline_update"`
	}
	database.DB.Table("extra_scan_paths").Find(&paths)
	if len(paths) == 0 {
		fmt.Println("  （无额外扫描路径记录）")
	}
	for _, p := range paths {
		flag := "✅ 参与维护"
		if !p.EnableOfflineUpdate {
			flag = "❌ 已关闭维护"
		}
		fmt.Printf("  id=%q name=%q path=%q → %s\n", p.ID, p.Name, p.Path, flag)
	}

	fmt.Println("── 2. offline_comics 概览 ──")
	var total, withGid, withParent, withHash, withScan int64
	database.DB.Table("offline_comics").Count(&total)
	database.DB.Table("offline_comics").Where("g_id != ''").Count(&withGid)
	// 注意：GORM 将 ParentGID 映射为 parent_g_id 列（不是 parent_gid）——此前排查脚本拼错列名导致误报「no such column」
	database.DB.Table("offline_comics").Where("parent_g_id != ''").Count(&withParent)
	database.DB.Table("offline_comics").Where("file_hash != ''").Count(&withHash)
	database.DB.Table("offline_comics").Where("scan_path_id != ''").Count(&withScan)
	fmt.Printf("  总数=%d | 含gid=%d | 含parent_gid=%d | 含file_hash=%d | 含scan_path_id=%d\n",
		total, withGid, withParent, withHash, withScan)

	fmt.Println("── 3. source_mode / 归档形态分布 ──")
	var modes []map[string]any
	database.DB.Raw("SELECT COALESCE(source_mode,'') AS m, COUNT(*) AS n FROM offline_comics GROUP BY COALESCE(source_mode,'')").Scan(&modes)
	for _, mo := range modes {
		fmt.Printf("  source_mode=%q → %v 本\n", mo["m"], mo["n"])
	}
	var zipCount, folderCount int64
	database.DB.Table("offline_comics").Where("lower(local_path) LIKE '%.zip' OR lower(local_path) LIKE '%.cbz'").Count(&zipCount)
	database.DB.Table("offline_comics").Count(&folderCount)
	fmt.Printf("  local_path 为 .zip/.cbz 归档：%d 本（仅这类才走「归档 hash 查重」）\n", zipCount)

	fmt.Println("── 4. 带 gid 漫画的路径归属 + 对应开关（CheckUpdates 应在此集合上执行）──")
	var rows []struct {
		ID         string `gorm:"column:id"`
		Title      string `gorm:"column:title"`
		GID        string `gorm:"column:g_id"`
		ScanPathID string `gorm:"column:scan_path_id"`
		SourceMode string `gorm:"column:source_mode"`
	}
	database.DB.Table("offline_comics").Where("g_id != ''").Find(&rows)
	if len(rows) == 0 {
		fmt.Println("  （本地无带 gid 的漫画 → 更新检测天然为 0，且同 GID 查重也天然无效）")
	}
	disabled := map[string]bool{}
	for _, p := range paths {
		if !p.EnableOfflineUpdate {
			disabled[p.ID] = true
		}
	}
	droppedByFilter := 0
	for _, r := range rows {
		enable := "（下载导入/无路径）"
		if r.ScanPathID != "" && disabled[r.ScanPathID] {
			enable = "❌ 已被『关闭维护路径』过滤"
			droppedByFilter++
		} else if r.ScanPathID != "" {
			enable = "✅ 路径参与维护"
		}
		fmt.Printf("  gid=%s scan_path_id=%q → %s | mode=%q | %q\n",
			r.GID, r.ScanPathID, enable, r.SourceMode, truncate(r.Title, 24))
	}
	fmt.Printf("  其中被开关过滤掉的带 gid 漫画：%d / %d\n", droppedByFilter, len(rows))
	if droppedByFilter > 0 {
		fmt.Println("  ⚠️ 根因：存在『关闭维护』的路径，但其下漫画并未从维护中豁免（或路径开关值被迁移成 false）。")
		fmt.Println("  → 处理：把这些路径的 enable_offline_update 置为 true（或确认预期），重启后维护即可命中。")
	}

	fmt.Println("── 5. 同名标题样本（潜在重复，供判断用户所见重复项类型）──")
	var dup []map[string]any
	if err := database.DB.Raw(
		"SELECT title, COUNT(*) AS n FROM offline_comics GROUP BY title HAVING COUNT(*) > 1 ORDER BY n DESC LIMIT 10",
	).Scan(&dup).Error; err != nil {
		fmt.Println("  查询失败:", err)
	} else if len(dup) == 0 {
		fmt.Println("  （无同名标题）")
	} else {
		for _, d := range dup {
			fmt.Printf("  %q ×%v\n", d["title"], d["n"])
		}
	}
}

// verifyRule4 调用 MaintainDedup 做规则4干跑，验证「文件夹内容完全相同」类重复能否被检出。
// 说明：干跑会像正式维护一样把内容签名缓存写回 file_hash/file_modified_at（幂等，可安全重复执行）；
//       真正删除文件需走 RemoveDedup，本函数只读判定、不删除任何文件。
func verifyRule4() {
	// 干跑仅验证规则4（文件夹内容签名），传 nil ehService 走纯本地逻辑，不联网
	result, err := services.MaintainDedup(database.DB, nil)
	if err != nil {
		fmt.Printf("⚠️ 查重执行失败: %v\n", err)
		return
	}
	var keepCount, removeCount, rule4Count int
	for _, it := range result.Items {
		if it.Keep {
			keepCount++
			continue
		}
		removeCount++
		if strings.Contains(it.Reason, "文件夹内容完全相同") {
			rule4Count++
			fmt.Printf("  🗑️ [规则4] %q | %s\n", truncate(it.Comic.Title, 32), it.Reason)
		}
	}
	fmt.Printf("干跑汇总：建议保留 %d 项，建议删除 %d 项（其中规则4命中 %d 项）\n", keepCount, removeCount, rule4Count)
	if rule4Count == 0 {
		fmt.Println("  ⚠️ 未命中规则4 —— 可能原因：")
		fmt.Println("    ① 当前 manga.db 里没有文件夹形态的复制品（复制品在服务端实际使用的另一个库中，见 main.go 启动日志的 DB 路径）；")
		fmt.Println("    ② 复制品位于『已关闭维护』的额外路径下（被 filterOfflineUpdateEnabled 过滤）；")
		fmt.Println("    ③ 复制品对应文件夹已在本地被移动/删除（LocalPath 指向的目录不存在）。")
	}
}

// checkSchema 打印 offline_comics / extra_scan_paths 的实际列，确认阶段1新增列已由 AutoMigrate 落库。
func checkSchema() {
	type needCol struct {
		table string
		col   string
		note  string
	}
	needs := []needCol{
		{"offline_comics", "title_jpn", "问题2 日语原名"},
		{"offline_comics", "added_at", "问题1 入库时间排序"},
		{"offline_comics", "published_at", "问题1 发布时间排序"},
		{"offline_comics", "file_modified_at", "问题1 文件修改时间排序"},
		{"offline_comics", "scan_path_id", "问题3 来源识别"},
		{"offline_comics", "g_id", "问题7 离线→在线跳转"},
		{"offline_comics", "parent_g_id", "问题7 父画廊GID（GORM列名）"},
		{"offline_comics", "token", "问题7 在线跳转 token"},
		{"offline_comics", "file_hash", "规则2 归档hash / 规则4 内容签名"},
		{"offline_comics", "source_mode", "归档形态 gallery/archive"},
		{"extra_scan_paths", "name", "问题3 来源标签名"},
		{"extra_scan_paths", "enable_offline_update", "问题4 路径维护开关"},
	}
	for _, n := range needs {
		cols := tableColumns(n.table)
		has := containsStr(cols, n.col)
		mark := "✅"
		if !has {
			mark = "❌"
		}
		fmt.Printf("  %s %s.%s（%s）%v\n", mark, n.table, n.col, n.note, has)
	}
}

// tableColumns 返回某张表的全部列名。
func tableColumns(table string) []string {
	rows, err := database.DB.Raw("PRAGMA table_info(" + table + ")").Rows()
	if err != nil {
		return nil
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			continue
		}
		cols = append(cols, name)
	}
	return cols
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
