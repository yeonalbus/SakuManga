package services

import (
	"SakuHentai/internal/models"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// 离线更新检测 + 维护查重（计划第 6 步）
//
// 1. CheckUpdates：联网核对每个离线漫画的在线详情
//    - 在线页数 > 本地页数 → 该画廊被扩充，标记 NeedsUpdate（NewGID/NewToken）
//    - 本地已下载「父画廊关系的更新版」→ 父画廊过时，标记其 NeedsUpdate
// 2. MaintainDedup：本地查重
//    - 同 GID 多份 → 建议保留文件夹形态、删除压缩包形态
//    - 归档文件 hash 相同 → 内容完全相同，删除重复
//    - 父画廊关系 → 旧版被新版取代
//    - 文件夹内容签名相同（无 gid/hash/parent 元数据的复制型重复）→ 删除复制项（问题3修复）
// ─────────────────────────────────────────────────────────────

// UpdateCheckResult 更新检测结果
type UpdateCheckResult struct {
	Checked     int                   `json:"checked"`     // 检测的漫画数量（联网核对）
	NeedsUpdate []models.OfflineComic `json:"needsUpdate"` // 需要更新的漫画（含标记信息）
	ParentFound int                   `json:"parentFound"` // 通过父画廊关系发现的新版本数量
}

// CheckUpdates 检测所有离线漫画的更新
//
// 联网核对会逐个请求在线详情，为避免触发限流，请求间隔 ~1.2s。
func CheckUpdates(db *gorm.DB, ehService *EHService) (*UpdateCheckResult, error) {
	if db == nil || ehService == nil {
		return nil, fmt.Errorf("非法参数：db / ehService 不能为空")
	}

	account := LoadAdminAccount(db)
	if account.IPBMemberID == "" {
		return nil, fmt.Errorf("请先绑定并保存 E 站账户凭证")
	}
	ehSetting := loadEHSetting(db, LoadAdminUserID(db))

	var comics []models.OfflineComic
	if err := db.Where("g_id != ''").Order("updated_at desc").Find(&comics).Error; err != nil {
		return nil, fmt.Errorf("读取离线漫画失败: %v", err)
	}
	// 问题4：按「离线维护」开关过滤——已关闭的额外路径下的漫画不参与更新检测
	comics = filterOfflineUpdateEnabled(db, comics)

	result := &UpdateCheckResult{Checked: len(comics), NeedsUpdate: []models.OfflineComic{}}
	gidMap := map[string]*models.OfflineComic{} // gid → 本地漫画（父画廊关系查重用）
	changed := map[string]bool{}                // comic id → 是否被标记
	fetchedOnline := map[string]bool{}          // comic id → A 段是否成功联网核对（B 段本地兜底跳过）

	log.Printf("%s [update] 开始更新检测：共 %d 个离线漫画（含 gid）", dlLogTag, len(comics))

	for i := range comics {
		c := &comics[i]
		if c.GID == "" {
			continue
		}
		if _, ok := gidMap[c.GID]; !ok {
			gidMap[c.GID] = c
		}

		// ── A. 联网核对在线详情 ──
		// 关键：归档下载物（ametadata/ComicInfo.xml）磁盘上不落任何 parent/child/newVersion 关系，
		// 因此「子孙关系 / 更新版本」必须从在线详情页 HTML 提取（见 FetchGalleryDetail）。
		detail, err := ehService.FetchGalleryDetail(account, c.GID, c.Token, ehSetting)
		if err != nil {
			log.Printf("%s [update] 漫画 %q(gid=%s) 在线详情拉取失败（跳过）: %v", dlWarnTag, c.Title, c.GID, err)
		} else if detail != nil {
			fetchedOnline[c.ID] = true
			// A1. 详情页罗列更新版本（#gnd "newer versions" / #dms）→ 本画廊已被取代，A→C 更新到最新版。
			//     若最新版已在本地（gidMap 命中），无需下载更新：清除旧的更新标记，交由维护查重界面删除旧版。
			if detail.NewVersionGID != "" && detail.NewVersionGID != c.GID {
				if _, ok := gidMap[detail.NewVersionGID]; ok {
					log.Printf("%s [update] 漫画 %q(gid=%s) 的最新版 gid=%s 本地已存在，跳过更新标记（交由维护查重）",
						dlLogTag, c.Title, c.GID, detail.NewVersionGID)
					if clearOfflineUpdate(c) {
						changed[c.ID] = true
					}
				} else {
					markOfflineUpdate(c, detail.NewVersionGID, detail.NewVersionToken,
						buildUpdateNote(detail.NewVersionGID, detail.Children))
					result.ParentFound++
					changed[c.ID] = true
					log.Printf("%s [update] 漫画 %q(gid=%s) 需要更新：%s", dlLogTag, c.Title, c.GID, c.UpdateNote)
				}
			} else if len(detail.Children) > 0 && detail.Children[len(detail.Children)-1].GID != c.GID {
				// A2. 兜底：存在更新版 → 标记更新到最新版（Children 最后一个，#gnd 从旧到新排列）
				latest := detail.Children[len(detail.Children)-1]
				if _, ok := gidMap[latest.GID]; ok {
					log.Printf("%s [update] 漫画 %q(gid=%s) 的最新版 gid=%s 本地已存在，跳过更新标记（交由维护查重）",
						dlLogTag, c.Title, c.GID, latest.GID)
					if clearOfflineUpdate(c) {
						changed[c.ID] = true
					}
				} else {
					markOfflineUpdate(c, latest.GID, latest.Token, buildUpdateNote(latest.GID, detail.Children))
					result.ParentFound++
					changed[c.ID] = true
					log.Printf("%s [update] 漫画 %q(gid=%s) 需要更新：%s", dlLogTag, c.Title, c.GID, c.UpdateNote)
				}
			}
			// A3. 详情页存在父画廊 → 回写 ParentGID（供本地查重规则3 与 B 段复用）
			if detail.ParentGID != "" && detail.ParentGID != c.GID {
				if c.ParentGID == "" || c.ParentGID != detail.ParentGID {
					c.ParentGID = detail.ParentGID
					changed[c.ID] = true
					log.Printf("%s [update] 漫画 %q(gid=%s) 记录父画廊 gid=%s", dlLogTag, c.Title, c.GID, detail.ParentGID)
				}
			}
			// A4. 在线页数 > 本地页数 → 原画廊被扩充（同一 gid 增量）
			if detail.PageCount > 0 && c.PageCount > 0 && detail.PageCount > c.PageCount {
				markOfflineUpdate(c, detail.ID, detail.Token,
					fmt.Sprintf("原画廊新增了 %d 页（在线 %d 页 > 本地 %d 页）", detail.PageCount-c.PageCount, detail.PageCount, c.PageCount))
				changed[c.ID] = true
				log.Printf("%s [update] 漫画 %q(gid=%s) 需要更新：%s", dlLogTag, c.Title, c.GID, c.UpdateNote)
			}
		}
		// 限流退避
		time.Sleep(1200 * time.Millisecond)
	}

	// ── B. 父画廊关系检测（本地，无网络）──
	// 仅对 A 段未标记的漫画生效：A 段已通过在线详情确认 A→C 更新到最新版（备注含中间链条），
	// B 段不得覆盖——否则多版本链会被降级为只更新到直接子版本（如 4019697 → 4051934 而非 4086937）。
	// B 段作为本地兜底，覆盖「无在线 HTML / 在线详情拉取失败」时仍能由 ParentGID 检出父子关系。
	for i := range comics {
		c := &comics[i]
		if c.ParentGID == "" || c.ParentGID == c.GID {
			continue
		}
		if p, ok := gidMap[c.ParentGID]; ok && p.GID != c.GID {
			// A 段已成功联网核对过父画廊 → 父画廊更新状态已由 A 段决定
			//（含“最新版本地已存在 → 无需更新”），B 段本地兜底必须跳过，避免降级到中间版本。
			if fetchedOnline[p.ID] {
				log.Printf("%s [update] 父画廊 %q(gid=%s) 已由 A 段联网核对，B 段本地兜底跳过",
					dlLogTag, p.Title, p.GID)
				continue
			}
			if p.NeedsUpdate {
				log.Printf("%s [update] 父画廊 %q(gid=%s) 已被 A 段标记更新到最新版 gid=%s，B 段跳过避免降级",
					dlLogTag, p.Title, p.GID, p.NewGID)
				continue
			}
			p.NeedsUpdate = true
			p.NewGID = c.GID
			p.NewToken = c.Token
			p.UpdateNote = fmt.Sprintf("检测到更新版（父画廊关系）：新版本 %q", c.Title)
			changed[p.ID] = true
			result.ParentFound++
			log.Printf("%s [update] 父画廊 %q(gid=%s) 已被 %q(gid=%s) 取代，标记更新",
				dlLogTag, p.Title, p.GID, c.Title, c.GID)
		}
	}

	// ── 保存变更 ──
	if len(changed) > 0 {
		for i := range comics {
			c := &comics[i]
			if !changed[c.ID] {
				continue
			}
			c.UpdatedAt = time.Now()
			if err := db.Save(c).Error; err != nil {
				log.Printf("%s [update] 保存漫画 %s 更新标记失败: %v", dlErrTag, c.ID, err)
			}
		}
	}

	// 收集需要更新的漫画（已标记）
	for i := range comics {
		if comics[i].NeedsUpdate {
			result.NeedsUpdate = append(result.NeedsUpdate, comics[i])
		}
	}

	log.Printf("%s [update] 更新检测完成：检查 %d 个，需更新 %d 个（父画廊关系发现 %d 个）",
		dlLogTag, result.Checked, len(result.NeedsUpdate), result.ParentFound)
	return result, nil
}

// markOfflineUpdate 标记漫画需要更新到新版
func markOfflineUpdate(c *models.OfflineComic, newGID, newToken, note string) {
	c.NeedsUpdate = true
	c.NewGID = newGID
	c.NewToken = newToken
	c.UpdateNote = note
}

// clearOfflineUpdate 清除漫画的更新标记（本地已存在最新版 / 更新已完成）。返回是否发生了清除。
func clearOfflineUpdate(c *models.OfflineComic) bool {
	if !c.NeedsUpdate && c.NewGID == "" && c.NewToken == "" && c.UpdateNote == "" {
		return false
	}
	c.NeedsUpdate = false
	c.NewGID = ""
	c.NewToken = ""
	c.UpdateNote = ""
	return true
}

// buildUpdateNote 构造 A→C 更新备注：更新目标 = 最新版（latestGID），并附带中间链条
// （#gnd 从旧到新罗列，去掉最新版自身）与各自 added 发布时间。示例：
//
//	"检测到更新版本：最新版 gid=4086937，中间版本：4012290(2026-06-26 12:48) → 4019697(2026-06-29 14:47) → 4051934(2026-07-14 14:20)"
func buildUpdateNote(latestGID string, children []GalleryRelation) string {
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

// DedupItem 查重建议项
type DedupItem struct {
	Comic  models.OfflineComic `json:"comic"`
	Reason string              `json:"reason"` // 重复原因
	Keep   bool                `json:"keep"`   // 是否建议保留（true=保留，false=建议删除）
}

// DedupResult 查重结果
type DedupResult struct {
	Items []DedupItem `json:"items"` // 需要处理的项（含建议保留项与建议删除项）
}

// MaintainDedup 本地维护查重
//
// 规则：
//   - 同 GID 多份：保留文件夹形态（gallery），建议删除压缩包形态（archive）
//   - 归档文件 hash 相同：内容完全相同，建议删除重复项
//   - 父画廊关系：旧版（父画廊）被新版取代，建议删除旧版（支持在线发现：磁盘元数据
//     parent/child 关系为空，需联网核对详情页；ehService 为空或未绑账号时退化为纯本地）
//   - 文件夹内容签名相同（无 gid/hash/parent 元数据的复制型重复）→ 删除复制项（问题3修复）
func MaintainDedup(db *gorm.DB, ehService *EHService) (*DedupResult, error) {
	if db == nil {
		return nil, fmt.Errorf("非法参数：db 不能为空")
	}

	var comics []models.OfflineComic
	if err := db.Where("source = ?", models.SourceOffline).Order("updated_at desc").Find(&comics).Error; err != nil {
		return nil, fmt.Errorf("读取离线漫画失败: %v", err)
	}
	// 问题4：按「离线维护」开关过滤——已关闭的额外路径下的漫画不参与查重
	comics = filterOfflineUpdateEnabled(db, comics)

	result := &DedupResult{Items: []DedupItem{}}
	keepSet := map[string]bool{}   // 建议保留的 comic id
	removeSet := map[string]bool{} // 建议删除的 comic id
	reasonMap := map[string]string{}
	totalBytes := make(map[string]int64) // 计算建议删除释放的空间用

	// ── 1. 同 GID 分组查重 ──
	gidGroups := map[string][]models.OfflineComic{}
	for _, c := range comics {
		if c.GID == "" {
			continue
		}
		gidGroups[c.GID] = append(gidGroups[c.GID], c)
	}
	for gid, group := range gidGroups {
		if len(group) < 2 {
			continue
		}
		// 优先保留文件夹形态（gallery）
		keepIdx := -1
		for i := range group {
			if group[i].SourceMode == "gallery" || !strings.HasSuffix(strings.ToLower(group[i].LocalPath), ".zip") {
				keepIdx = i
				break
			}
		}
		if keepIdx == -1 {
			keepIdx = 0
		}
		for i := range group {
			if i == keepIdx {
				keepSet[group[i].ID] = true
				continue
			}
			removeSet[group[i].ID] = true
			reasonMap[group[i].ID] = fmt.Sprintf("同 GID(%s) 重复：建议保留文件夹形态，删除该重复项", gid)
			totalBytes[group[i].ID] = group[i].FileSize
		}
	}

	// ── 2. 归档文件 hash 查重（仅 .zip/.cbz 等归档）──
	hashGroups := map[string][]models.OfflineComic{}
	for _, c := range comics {
		if c.SourceMode != "archive" || c.LocalPath == "" {
			continue
		}
		if !IsArchive(c.LocalPath) {
			continue
		}
		h, err := hashFile(c.LocalPath)
		if err != nil {
			log.Printf("%s [maintain] 计算 %q hash 失败: %v", dlWarnTag, c.LocalPath, err)
			continue
		}
		c.FileHash = h
		_ = db.Model(&c).Update("file_hash", h)
		hashGroups[h] = append(hashGroups[h], c)
	}
	for _, group := range hashGroups {
		if len(group) < 2 {
			continue
		}
		// 保留第一个，其余建议删除
		for i := 1; i < len(group); i++ {
			c := group[i]
			// 若已被同 GID 规则标记保留，跳过
			if keepSet[c.ID] {
				continue
			}
			removeSet[c.ID] = true
			reasonMap[c.ID] = fmt.Sprintf("归档内容完全相同（hash=%s）：删除重复项", c.FileHash)
			totalBytes[c.ID] = c.FileSize
		}
	}

	// ── 3. 父画廊关系查重（含在线关系发现）──
	gidToComic := map[string]*models.OfflineComic{}
	for i := range comics {
		if comics[i].GID != "" {
			gidToComic[comics[i].GID] = &comics[i]
		}
	}

	// 3a. 在线父子关系发现（磁盘元数据无 parent/child 关系，需联网核对详情页）
	//    - 本画廊详情有父画廊 → 回写 ParentGID（供 3b 本地查重用）
	//    - 本画廊详情有子画廊/新版（且本地存在）→ 本画廊是旧版，标记删除、保留新版
	//    仅对 ParentGID 为空且未被标记删除的漫画抓取，避免重复请求；
	//    账号未绑定时跳过在线发现（本地规则1/2/4 仍可用）。
	if ehService != nil {
		account := LoadAdminAccount(db)
		if account.IPBMemberID != "" {
			ehSetting := loadEHSetting(db, LoadAdminUserID(db))
			for i := range comics {
				c := &comics[i]
				if c.GID == "" || c.Token == "" || c.ParentGID != "" || removeSet[c.ID] {
					continue
				}
				detail, err := ehService.FetchGalleryDetail(account, c.GID, c.Token, ehSetting)
				if err != nil || detail == nil {
					log.Printf("%s [maintain] 漫画 %q(gid=%s) 在线详情拉取失败（跳过在线发现）: %v",
						dlWarnTag, c.Title, c.GID, err)
					time.Sleep(1200 * time.Millisecond)
					continue
				}
				// 回写父画廊关系
				if detail.ParentGID != "" && detail.ParentGID != c.GID {
					if c.ParentGID == "" || c.ParentGID != detail.ParentGID {
						c.ParentGID = detail.ParentGID
						_ = db.Model(c).Update("parent_g_id", detail.ParentGID)
						log.Printf("%s [maintain] 漫画 %q(gid=%s) 在线发现父画廊 gid=%s",
							dlLogTag, c.Title, c.GID, detail.ParentGID)
					}
				}
				// 本画廊已被更新版/子画廊取代（本地存在新版 → 旧版建议删除）
				var successor *models.OfflineComic
				if detail.NewVersionGID != "" && detail.NewVersionGID != c.GID {
					if s, ok := gidToComic[detail.NewVersionGID]; ok && s.ID != c.ID {
						successor = s
					}
				}
				if successor == nil {
					for _, ch := range detail.Children {
						if ch.GID == "" || ch.GID == c.GID {
							continue
						}
						if s, ok := gidToComic[ch.GID]; ok && s.ID != c.ID {
							// A→C：不 break，取最后一个本地存在的更新版（最新版）
							successor = s
						}
					}
				}
				if successor != nil {
					removeSet[c.ID] = true
					reasonMap[c.ID] = fmt.Sprintf("检测到更新版（父画廊关系）%q：旧版可删除", successor.Title)
					totalBytes[c.ID] = c.FileSize
					if !removeSet[successor.ID] {
						keepSet[successor.ID] = true
					}
					log.Printf("%s [maintain] 漫画 %q(gid=%s) 被新版 %q(gid=%s) 取代，建议删除旧版",
						dlLogTag, c.Title, c.GID, successor.Title, successor.GID)
				}
				// 限流退避
				time.Sleep(1200 * time.Millisecond)
			}
		}
	}

	// 3b. 本地父画廊关系查重（含 3a 在线回写的 ParentGID）
	for i := range comics {
		c := &comics[i]
		if c.ParentGID == "" || c.ParentGID == c.GID {
			continue
		}
		if p, ok := gidToComic[c.ParentGID]; ok && p.ID != c.ID {
			// 父画廊 p 是旧版，被 c 取代
			removeSet[p.ID] = true
			reasonMap[p.ID] = fmt.Sprintf("已被更新版（父画廊关系）%q 取代，旧版可删除", c.Title)
			totalBytes[p.ID] = p.FileSize
			// 旧版 p 一旦判定删除，绝不再作为“建议保留”输出
			delete(keepSet, p.ID)
			// 新版建议保留（但若 c 自身也是被删除项，不得覆盖删除标记）
			if !removeSet[c.ID] {
				keepSet[c.ID] = true
			}
		}
	}
		// ── 4. 文件夹内容签名查重（问题3修复）──
		// 针对「无 gid/hash/parent 元数据的复制型文件夹重复」：
		//   递归收集文件夹内所有图片的「相对路径|大小」生成内容签名，签名相同 = 内容完全一致。
		//   签名缓存进 file_hash（与规则2的归档 hash 互斥：仅 gallery 形态使用）。
		sigGroups := map[string][]models.OfflineComic{}
		for i := range comics {
			c := &comics[i]
			if c.SourceMode == "archive" || !isFolderPath(c.LocalPath) {
				continue
			}
			if removeSet[c.ID] {
				continue
			}
			// 快路径：已有签名且文件夹目录 mtime 未超过已记录的最新文件 mtime → 内容未变，直接复用
			if c.FileHash != "" && !c.FileModifiedAt.IsZero() {
				if fi, err := os.Stat(c.LocalPath); err == nil && !fi.ModTime().After(c.FileModifiedAt) {
					sigGroups[c.FileHash] = append(sigGroups[c.FileHash], *c)
					continue
				}
			}
			sig, maxMod, err := folderSignature(c.LocalPath)
			if err != nil {
				log.Printf("%s [maintain] 计算 %q 内容签名失败: %v", dlWarnTag, c.LocalPath, err)
				continue
			}
			if c.FileHash != sig || maxMod.After(c.FileModifiedAt) {
				c.FileHash = sig
				c.FileModifiedAt = maxMod
				_ = db.Model(&c).Updates(map[string]interface{}{"file_hash": sig, "file_modified_at": maxMod})
			}
			sigGroups[sig] = append(sigGroups[sig], *c)
		}
		for sig, group := range sigGroups {
			if len(group) < 2 {
				continue
			}
			keepIdx := -1
			for i := range group {
				if keepSet[group[i].ID] && !removeSet[group[i].ID] {
					keepIdx = i
					break
				}
			}
			if keepIdx == -1 {
				keepIdx = 0
			}
			for i := range group {
				if i == keepIdx {
					keepSet[group[i].ID] = true
					continue
				}
				if removeSet[group[i].ID] {
					continue
				}
				removeSet[group[i].ID] = true
				reasonMap[group[i].ID] = fmt.Sprintf("文件夹内容完全相同（%s，共 %d 份）：删除复制项", shortHash(sig), len(group))
				totalBytes[group[i].ID] = group[i].FileSize
			}
		}
	
		// ── 组装结果 ──
		// 删除标记优先于保留标记：同一漫画若同时命中“被新版取代(删)”与“是某旧版的新版(留)”，一律判删，
		// 确保多版本链（如 4019697→4051934→4086937）只保留最新版一份。
	for i := range comics {
		c := comics[i]
		if removeSet[c.ID] {
			result.Items = append(result.Items, DedupItem{Comic: c, Reason: reasonMap[c.ID], Keep: false})
			continue
		}
		if keepSet[c.ID] {
			result.Items = append(result.Items, DedupItem{Comic: c, Reason: "建议保留", Keep: true})
		}
	}

	// 记录汇总日志
	var keepCount, removeCount int
	for _, it := range result.Items {
		if it.Keep {
			keepCount++
		} else {
			removeCount++
		}
	}
	log.Printf("%s [maintain] 维护查重完成：建议保留 %d 项，建议删除 %d 项", dlLogTag, keepCount, removeCount)
	return result, nil
}

// filterOfflineUpdateEnabled 过滤掉「已关闭离线维护」的额外路径下的漫画（问题4）。
//
// 规则：
//   - 下载导入的漫画（ScanPathID == ""）始终参与更新检测/查重；
//   - 额外路径的漫画仅当其路径 EnableOfflineUpdate == true 时参与；
//   - 读取路径配置失败时保守处理：按全量参与（不静默丢数据）。
func filterOfflineUpdateEnabled(db *gorm.DB, comics []models.OfflineComic) []models.OfflineComic {
	var paths []models.ExtraScanPath
	if err := db.Select("id", "enable_offline_update").Find(&paths).Error; err != nil {
		log.Printf("%s [offline] 读取额外路径配置失败，按全量参与处理: %v", dlWarnTag, err)
		return comics
	}
	disabled := make(map[string]bool, len(paths))
	for _, p := range paths {
		if !p.EnableOfflineUpdate {
			disabled[p.ID] = true
		}
	}
	filtered := comics[:0]
	for _, c := range comics {
		if c.ScanPathID != "" && disabled[c.ScanPathID] {
			continue
		}
		filtered = append(filtered, c)
	}
	return filtered
}

// DeleteOfflineComic 删除本地画廊：
//   - 可选物理删除本地文件
//   - 删除数据库记录
//
// 说明：书架与历史记录均为前端 localStorage 功能（后端数据库无对应表），
// 由前端删除方法在本地同步清理引用，后端无需（也无法）处理。
func DeleteOfflineComic(db *gorm.DB, comicID string, deleteFile bool) error {
	var comic models.OfflineComic
	if err := db.First(&comic, "id = ?", comicID).Error; err != nil {
		return fmt.Errorf("未找到漫画记录: %v", err)
	}

	// 1. 可选：物理删除本地文件
	if deleteFile && comic.LocalPath != "" {
		if err := os.RemoveAll(comic.LocalPath); err != nil {
			return fmt.Errorf("删除本地文件失败 %q: %v", comic.LocalPath, err)
		}
		log.Printf("%s [maintain] 已删除本地文件 %q（comic %s）", dlLogTag, comic.LocalPath, comicID)
	}

	// 2. 删除数据库记录
	if err := db.Delete(&comic).Error; err != nil {
		return fmt.Errorf("删除漫画记录失败: %v", err)
	}
	log.Printf("%s [maintain] 已删除漫画 %q（id=%s）", dlLogTag, comic.Title, comicID)
	return nil
}

// RemoveDedupComic 删除重复漫画（查重维护入口）：委托 DeleteOfflineComic 统一处理历史记录与文件
func RemoveDedupComic(db *gorm.DB, comicID string, deleteFile bool) error {
	return DeleteOfflineComic(db, comicID, deleteFile)
}

// hashFile 计算文件 md5（归档查重用，分块读取避免大内存占用）
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	buf := make([]byte, 256*1024)
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// isFolderPath 判断本地路径是否为存在的文件夹。
func isFolderPath(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// folderSignature 计算文件夹内容签名（问题3规则4）：
// 递归收集文件夹内所有图片文件的「相对路径|字节大小」，排序后序列化再取 md5。
// 两个内容完全一致的文件夹（含子目录结构）得到相同签名，从而可识别复制型重复；
// 同时返回文件夹内最新文件的修改时间，作为后续增量缓存依据。
func folderSignature(path string) (string, time.Time, error) {
	var entries []string
	var maxMod time.Time
	err := filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil // 跳过无法访问的条目（权限/中断），不阻断整体
		}
		if fi.IsDir() || !IsImage(fi.Name()) {
			return nil
		}
		rel, rerr := filepath.Rel(path, p)
		if rerr != nil {
			rel = p
		}
		entries = append(entries, fmt.Sprintf("%s|%d", filepath.ToSlash(rel), fi.Size()))
		if fi.ModTime().After(maxMod) {
			maxMod = fi.ModTime()
		}
		return nil
	})
	if err != nil {
		return "", maxMod, err
	}
	sort.Strings(entries)
	h := md5.New()
	for _, e := range entries {
		_, _ = io.WriteString(h, e)
		_, _ = io.WriteString(h, "\n")
	}
	return hex.EncodeToString(h.Sum(nil)), maxMod, nil
}

// shortHash 取签名前 8 位用于提示文案。
func shortHash(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
