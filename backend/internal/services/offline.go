package services

import (
	"SakuHentai/internal/models"
	"crypto/md5"
	"encoding/hex"
	"errors"
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
	return checkUpdatesWithProgress(db, ehService, nil)
}

// CheckUpdatesWithProgress 带进度回调的更新检测（问题3：长任务进度可感知，供 handler 异步任务使用）
func CheckUpdatesWithProgress(db *gorm.DB, ehService *EHService, onProgress OfflineProgressFn) (*UpdateCheckResult, error) {
	return checkUpdatesWithProgress(db, ehService, onProgress)
}

// checkUpdatesWithProgress 带进度回调的更新检测（问题3：长任务进度可感知）
func checkUpdatesWithProgress(db *gorm.DB, ehService *EHService, onProgress OfflineProgressFn) (*UpdateCheckResult, error) {
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

	// 问题3：联网核对进度（含 gid 的漫画数）
	netTotal := 0
	for i := range comics {
		if comics[i].GID != "" {
			netTotal++
		}
	}
	netDone := 0

	for i := range comics {
		c := &comics[i]
		if c.GID == "" {
			continue
		}
		netDone++
		if onProgress != nil {
			onProgress(netDone, netTotal, c.Title, "在线更新核对")
		}
		if _, ok := gidMap[c.GID]; !ok {
			gidMap[c.GID] = c
		}

		// ── A. 联网核对在线详情 ──
		// 关键：归档下载物（ametadata/ComicInfo.xml）磁盘上不落任何 parent/child/newVersion 关系，
		// 因此「子孙关系 / 更新版本」必须从在线详情页 HTML 提取（见 FetchGalleryDetail）。
		detail, err := ehService.FetchGalleryDetail(account, c.GID, c.Token, ehSetting)
		if err != nil {
			// 需求 3(2)：区分「画廊被删」与「网络故障」——removed/copyright 持久化标记并跳过，
			// 其余 HTTP/网络错误为临时故障，仅 log（下次重试）。
			var gu *ErrGalleryUnavailable
			if errors.As(err, &gu) && (gu.Kind == "removed" || gu.Kind == "copyright") {
				c.RemovedStatus = true
				c.RemovedAt = time.Now().UnixMilli()
				changed[c.ID] = true
				log.Printf("%s [update] 漫画 %q(gid=%s) 已被删除/移除（%s），标记 RemovedStatus 并排除后续扫描",
					dlLogTag, c.Title, c.GID, gu.Kind)
			} else {
				log.Printf("%s [update] 漫画 %q(gid=%s) 在线详情拉取失败（跳过）: %v", dlWarnTag, c.Title, c.GID, err)
			}
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

// ─────────────────────────────────────────────────────────────
// Aged Status 老化判定（Round4 任务四：365 天老化规则）
//
// E 站规则：发布超 365 天的画廊无法再通过 Gallery Manager Update 生成子画廊，
// 此类画廊只扫描一次。本函数遍历 publishedAt < now-365d 且 agedCheckedAt == 0 的漫画：
//   - 联网核对在线详情，若发现新版本，取最新版的发布时间——
//     最新版仍在 365 天内 → 正常标记 needsUpdate（不设 AgedStatus）；
//     无新版本，或最新版也已超 365 天 → 设 AgedStatus=true 并排除后续扫描。
//   - 无论结果如何都更新 AgedCheckedAt，防止重复扫描。
// ─────────────────────────────────────────────────────────────

// AgeCheckResult 老化判定结果
type AgeCheckResult struct {
	Checked     int `json:"checked"`     // 参与老化判定的漫画数
	Aged        int `json:"aged"`        // 标记为老化的漫画数
	NeedsUpdate int `json:"needsUpdate"` // 判定为有可更新新版（仍在窗口内）的漫画数
}

// AgeCheckWithProgress 带进度回调的老化判定（供周扫描/手动触发共用）
func AgeCheckWithProgress(db *gorm.DB, ehService *EHService, onProgress OfflineProgressFn) (*AgeCheckResult, error) {
	return ageCheckWithProgress(db, ehService, onProgress)
}

// ageCheckWithProgress 老化判定实现（一次性：只处理 publishedAt 超 365 天且从未判定过的漫画）
func ageCheckWithProgress(db *gorm.DB, ehService *EHService, onProgress OfflineProgressFn) (*AgeCheckResult, error) {
	if db == nil || ehService == nil {
		return nil, fmt.Errorf("非法参数：db / ehService 不能为空")
	}
	account := LoadAdminAccount(db)
	if account.IPBMemberID == "" {
		return nil, fmt.Errorf("请先绑定并保存 E 站账户凭证")
	}
	ehSetting := loadEHSetting(db, LoadAdminUserID(db))

	cutoff := time.Now().AddDate(0, 0, -365)

	var comics []models.OfflineComic
	if err := db.Where("g_id != '' AND aged_status = ? AND aged_checked_at = ? AND published_at < ?",
		false, 0, cutoff).Order("published_at asc").Find(&comics).Error; err != nil {
		return nil, fmt.Errorf("读取老化候选漫画失败: %v", err)
	}
	// 与常规更新检测一致：已关闭离线维护的额外路径漫画不参与
	comics = filterOfflineUpdateEnabled(db, comics)

	result := &AgeCheckResult{Checked: len(comics)}
	nowMs := time.Now().UnixMilli()
	for i := range comics {
		c := &comics[i]
		if onProgress != nil {
			onProgress(i+1, len(comics), c.Title, "老化判定")
		}

		aged := false
		detail, err := ehService.FetchGalleryDetail(account, c.GID, c.Token, ehSetting)
		if err != nil {
			// 需求 3(2)：removed/copyright → 持久化标记删除并跳过本次老化判定（后续扫描已过滤）。
			var gu *ErrGalleryUnavailable
			if errors.As(err, &gu) && (gu.Kind == "removed" || gu.Kind == "copyright") {
				c.RemovedStatus = true
				c.RemovedAt = time.Now().UnixMilli()
				c.UpdatedAt = time.Now()
				if err := db.Save(c).Error; err != nil {
					log.Printf("%s [update] 保存漫画 %s 删除标记失败: %v", dlErrTag, c.ID, err)
				}
				log.Printf("%s [update] 漫画 %q(gid=%s) 已被删除/移除（%s），标记 RemovedStatus 并排除后续扫描",
					dlLogTag, c.Title, c.GID, gu.Kind)
				time.Sleep(1200 * time.Millisecond)
				continue
			}
			log.Printf("%s [update] 老化判定漫画 %q(gid=%s) 在线详情拉取失败（按无新版处理）: %v",
				dlWarnTag, c.Title, c.GID, err)
		} else if detail != nil {
			if latest, has := latestGalleryVersion(detail); has {
				if pub := parseGalleryAddedAt(latest.AddedAt); pub != nil && pub.Before(cutoff) {
					// 最新版也已超 365 天 → 老化（此链条已无法再更新）
					aged = true
				} else {
					// 最新版仍在 365 天内（或发布时间未知，保守按可更新处理）→ 正常标记更新
					markOfflineUpdate(c, latest.GID, latest.Token, buildUpdateNote(latest.GID, detail.Children))
					result.NeedsUpdate++
					log.Printf("%s [update] 漫画 %q(gid=%s) 有新版 gid=%s（发布时间 %s），仍在可更新窗口内",
						dlLogTag, c.Title, c.GID, latest.GID, latest.AddedAt)
				}
			} else {
				// 无新版本 → 老化
				aged = true
			}
		} else {
			aged = true
		}

		if aged {
			if clearOfflineUpdate(c) {
				log.Printf("%s [update] 清除漫画 %q(gid=%s) 的更新标记（最新版也已超 365 天）",
					dlLogTag, c.Title, c.GID)
			}
			c.AgedStatus = true
			result.Aged++
			log.Printf("%s [update] 漫画 %q(gid=%s) 已老化（发布超 365 天且无窗口内新版），标记 AgedStatus 并排除后续扫描",
				dlLogTag, c.Title, c.GID)
		}

		c.AgedCheckedAt = nowMs
		c.UpdatedAt = time.Now()
		if err := db.Save(c).Error; err != nil {
			log.Printf("%s [update] 保存漫画 %s 老化状态失败: %v", dlErrTag, c.ID, err)
		}
		time.Sleep(1200 * time.Millisecond)
	}

	log.Printf("%s [update] 老化判定完成：检查 %d 个，标记老化 %d 个，可更新 %d 个",
		dlLogTag, result.Checked, result.Aged, result.NeedsUpdate)
	return result, nil
}

// latestGalleryVersion 取详情页中的最新版画廊关系（Children 从旧到新排列，最后一位为最新版）
func latestGalleryVersion(detail *GalleryDetailResult) (GalleryRelation, bool) {
	if detail.NewVersionGID != "" && detail.NewVersionGID != detail.ID {
		for _, ch := range detail.Children {
			if ch.GID == detail.NewVersionGID {
				return ch, true
			}
		}
		return GalleryRelation{GID: detail.NewVersionGID, Token: detail.NewVersionToken}, true
	}
	if n := len(detail.Children); n > 0 {
		return detail.Children[n-1], true
	}
	return GalleryRelation{}, false
}

// parseGalleryAddedAt 解析详情页关系列表中的发布时间字符串（如 "2026-07-30 12:13"）
func parseGalleryAddedAt(s string) *time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return &t
		}
	}
	return nil
}

// RunUpdateScanWithProgress 执行一轮完整更新扫描：常规更新检测 + 老化判定，供周调度与手动触发共用。
// 老化判定可能清除部分 needsUpdate（最新版也已超 365 天），故结束后从 DB 重建真实待更新列表。
func RunUpdateScanWithProgress(db *gorm.DB, ehService *EHService, onProgress OfflineProgressFn) (*UpdateCheckResult, *AgeCheckResult, error) {
	result, err := checkUpdatesWithProgress(db, ehService, onProgress)
	if err != nil {
		return nil, nil, err
	}
	aged, aerr := ageCheckWithProgress(db, ehService, onProgress)
	if aerr != nil {
		log.Printf("%s [update] 老化判定失败: %v", dlErrTag, aerr)
	}
	var fresh []models.OfflineComic
	if rerr := db.Where("needs_update = ? AND aged_status = ?", true, false).Order("updated_at desc").Find(&fresh).Error; rerr == nil {
		result.NeedsUpdate = fresh
	}
	return result, aged, nil
}

// RunUpdateScan 无进度回调版本（等价于 RunUpdateScanWithProgress(db, eh, nil)）
func RunUpdateScan(db *gorm.DB, ehService *EHService) (*UpdateCheckResult, *AgeCheckResult, error) {
	return RunUpdateScanWithProgress(db, ehService, nil)
}

// AutoEnqueueUpdates 自动为所有待更新漫画入队下载（autoUpdateGallery=true 时调用）。
// 手动触发与周扫描共用同一套方案选择逻辑，返回 (入队数, 跳过数)。
func AutoEnqueueUpdates(db *gorm.DB, manager *DownloadManager, result *UpdateCheckResult) (enqueued, skipped int) {
	if manager == nil || result == nil {
		return 0, 0
	}
	userID := LoadAdminUserID(db)
	for i := range result.NeedsUpdate {
		comic := result.NeedsUpdate[i]
		params, err := BuildUpdateDownloadParams(db, manager, &comic, "", userID)
		if err != nil {
			log.Printf("%s [update] 自动更新跳过漫画 %s（%s）: %v", dlWarnTag, comic.ID, comic.Title, err)
			skipped++
			continue
		}
		if _, err := manager.CreateTask(params); err != nil {
			// 已存在进行中任务或创建失败：跳过但不算致命错误
			log.Printf("%s [update] 自动更新漫画 %s 入队失败: %v", dlWarnTag, comic.ID, err)
			skipped++
			continue
		}
		enqueued++
	}
	return enqueued, skipped
}

// BuildUpdateDownloadParams 根据漫画的更新信息构造下载参数（手动更新与自动更新共用同一套方案选择逻辑）
func BuildUpdateDownloadParams(db *gorm.DB, manager *DownloadManager, comic *models.OfflineComic, modeOverride string, userID uint) (CreateDownloadParams, error) {
	// 优先使用检测到的新版 gid/token；同 gid 扩充时 token 可能已变更
	gid := comic.NewGID
	if gid == "" {
		gid = comic.GID
	}
	token := comic.NewToken
	if token == "" {
		token = comic.Token
	}
	if gid == "" || token == "" {
		return CreateDownloadParams{}, errors.New("该漫画缺少新版 gid/token，无法下载更新")
	}

	// 选择更新下载方案：请求覆盖 > 自动更新方案 > 归档
	setting := manager.GetSettings()
	mode := modeOverride
	if mode == "" {
		mode = setting.AutoUpdateScheme
	}
	if mode != string(models.DownloadModeGallery) && mode != string(models.DownloadModeArchive) {
		mode = string(models.DownloadModeArchive)
	}

	archiveType := ""
	if mode == string(models.DownloadModeArchive) {
		if setting.DefaultDownloadScheme == models.DefaultSchemeArchiveResample {
			archiveType = string(models.ArchiveTypeResample)
		} else {
			archiveType = string(models.ArchiveTypeOriginal)
		}
	}

	return CreateDownloadParams{
		UserID:           userID,
		GID:              gid,
		Token:            token,
		Title:            comic.Title,
		CoverURL:         comic.CoverURL,
		Mode:             models.DownloadMode(mode),
		ArchiveType:      models.ArchiveType(archiveType),
		UpdateForComicID: comic.ID,
	}, nil
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

// ClearOfflineUpdateByGID 按下载任务的新 GID 反向匹配更新列表并清除更新标记（需求 2）。
// 覆盖两种场景：
//   - 漫画自身 gid = 下载 gid：用户在「更新」页手动下载了标记需要更新的漫画本体；
//   - 漫画 new_gid = 下载 gid：用户下载了检测到的新版本，父画廊的更新标记应一并消除。
// 复用 clearOfflineUpdate 的字段清空语义，避免与维护查重、老化判定产生新状态冲突。
// 返回清除的记录数。
func ClearOfflineUpdateByGID(db *gorm.DB, gid string) (int64, error) {
	if gid == "" {
		return 0, nil
	}
	// 仅拉取标记了需要更新的漫画（数量少），gid 匹配在 Go 层按字段判断，
	// 避免依赖 gorm 对 GID/NewGID 的列名映射（g_id / new_g_id）导致的 SQL 耦合。
	var comics []models.OfflineComic
	if err := db.Where("needs_update = ?", true).Find(&comics).Error; err != nil {
		return 0, err
	}
	cleared := int64(0)
	for i := range comics {
		if comics[i].GID != gid && comics[i].NewGID != gid {
			continue
		}
		if !clearOfflineUpdate(&comics[i]) {
			continue
		}
		comics[i].UpdatedAt = time.Now()
		if err := db.Save(&comics[i]).Error; err != nil {
			return cleared, err
		}
		cleared++
	}
	if cleared > 0 {
		log.Printf("%s [update] 下载完成 gid=%s：清除 %d 个漫画的更新标记（需求 2）", dlLogTag, gid, cleared)
	}
	return cleared, nil
}

// ClearOfflineUpdateByComicID 清除指定漫画的更新标记（需求 3(2) 前端「移出列表」）。
// 仅清除更新标记（保留本地文件与记录），供「画廊已被删除/移除」项清理列表；
// 保留 RemovedStatus 标记，使后续更新检测/维护查重仍跳过该画廊（避免重复联网）。
// 返回是否发生了清除。
func ClearOfflineUpdateByComicID(db *gorm.DB, comicID string) (bool, error) {
	if db == nil || comicID == "" {
		return false, nil
	}
	var comic models.OfflineComic
	if err := db.First(&comic, "id = ?", comicID).Error; err != nil {
		return false, err
	}
	if !clearOfflineUpdate(&comic) {
		return false, nil
	}
	comic.UpdatedAt = time.Now()
	if err := db.Save(&comic).Error; err != nil {
		return false, err
	}
	log.Printf("%s [update] 移出更新列表：comic=%s gid=%s（需求 3(2)）", dlLogTag, comic.ID, comic.GID)
	return true, nil
}

// ReconcileResult 下载完成后数据对账结果（需求 3(1)）
type ReconcileResult struct {
	DedupItems         []DedupItem `json:"dedupItems"`         // GID 去重建议（复用 DedupItem，供维护页提示）
	ParentGIDWritten   int         `json:"parentGIDWritten"`   // 回写 ParentGID 的条数
	PageCountCorrected int         `json:"pageCountCorrected"` // 校正 PageCount 的条数
	AgedReset          int         `json:"agedReset"`          // 复位 Aged 状态的条数
}

// ReconcileOfflineAfterDownload 下载完成后主动对账数据库（需求 3(1)）。
//
// 下载任务 completed 收尾调用。下载引擎已通过 ScanAndSaveDirectory 将本次下载的漫画入库
// （download_gallery.go / download_archive.go），metadata / ametadata / ComicInfo.xml 已落在
// 落地目录或压缩包内。这里按 task.GID 查询数据库中全部同 GID 本地记录，读取其落地
// 目录/压缩包内的元数据做四步轻量对账（不重复扫描）：
//  1. GID 去重：同 GID 且 local_path 不同 → 判定为重复来源，复用维护查重规则 2 的
//     「保留文件夹形态」语义生成去重建议（DedupItem），提示用户去维护页处理。
//  2. ParentGID 回写：metadata 的 parent_gid 非空且本地为空 → 回写，供维护规则 3 与更新 B 段复用。
//  3. PageCount 校正：metadata filecount > 本地 PageCount → 更新。
//  4. Aged 复位：曾标记 AgedStatus=true → 复位 AgedStatus/AgedCheckedAt，重新参与后续扫描。
func ReconcileOfflineAfterDownload(db *gorm.DB, task *models.DownloadTask) (*ReconcileResult, error) {
	result := &ReconcileResult{DedupItems: []DedupItem{}}
	if db == nil || task == nil || task.GID == "" {
		return result, nil
	}

	var comics []models.OfflineComic
	if err := db.Where("g_id = ?", task.GID).Find(&comics).Error; err != nil {
		return result, fmt.Errorf("下载后对账：读取同 GID(%s) 记录失败: %v", task.GID, err)
	}
	if len(comics) == 0 {
		return result, nil
	}

	// ── 2/3/4 步：逐条读取落地元数据做字段对账 ──
	for i := range comics {
		c := &comics[i]
		meta := readOfflineMetadataFromPath(c)
		changed := false

		// 2. ParentGID 回写：metadata parent_gid 非空且本地为空 → 回写
		if c.ParentGID == "" && meta.ParentGID != "" {
			c.ParentGID = meta.ParentGID
			result.ParentGIDWritten++
			changed = true
		}
		// 3. PageCount 校正：metadata filecount 更完整时采用
		if meta.FileCount > c.PageCount {
			c.PageCount = meta.FileCount
			result.PageCountCorrected++
			changed = true
		}
		// 4. Aged 复位：本次成功下载新版（或确认该 GID 仍存在），复位老化状态
		if c.AgedStatus {
			c.AgedStatus = false
			c.AgedCheckedAt = 0
			result.AgedReset++
			changed = true
		}

		if changed {
			c.UpdatedAt = time.Now()
			if err := db.Save(c).Error; err != nil {
				return result, err
			}
		}
	}

	// ── 1. GID 去重：同 GID 且 local_path 不同 → 复用规则 2「保留文件夹形态」语义 ──
	keepIdx := -1
	for i := range comics {
		if comics[i].SourceMode == "gallery" || !IsArchive(comics[i].LocalPath) {
			keepIdx = i
			break
		}
	}
	if keepIdx == -1 {
		keepIdx = 0
	}
	for i := range comics {
		if i == keepIdx {
			continue
		}
		result.DedupItems = append(result.DedupItems, DedupItem{
			Comic:     comics[i],
			Reason:    fmt.Sprintf("同 GID(%s) 重复：建议保留文件夹形态，删除该重复项", task.GID),
			Keep:      false,
			PairComic: &comics[keepIdx],
		})
	}

	if len(result.DedupItems) > 0 {
		log.Printf("%s [reconcile] 下载完成 gid=%s：发现 %d 条同 GID 重复记录，建议去维护页处理",
			dlLogTag, task.GID, len(result.DedupItems))
	}
	if result.ParentGIDWritten > 0 || result.PageCountCorrected > 0 || result.AgedReset > 0 {
		log.Printf("%s [reconcile] 下载完成 gid=%s：回写 ParentGID=%d 校正 PageCount=%d 复位 Aged=%d",
			dlLogTag, task.GID, result.ParentGIDWritten, result.PageCountCorrected, result.AgedReset)
	}
	return result, nil
}

// readOfflineMetadataFromPath 读取落地记录内的元数据（目录 → ParseDirMetadata；归档 → ParseZipMetadata）。
func readOfflineMetadataFromPath(c *models.OfflineComic) *ParsedMetadata {
	if c == nil || c.LocalPath == "" {
		return &ParsedMetadata{Tags: []string{}}
	}
	if IsArchive(c.LocalPath) {
		return ParseZipMetadata(c.LocalPath)
	}
	return ParseDirMetadata(c.LocalPath)
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
// Round4 任务一：新增 PairComic —— 成对对象（对比视图双列展示：同 GID 保留↔删除、父子版本 新版↔旧版）。
type DedupItem struct {
	Comic     models.OfflineComic  `json:"comic"`
	Reason    string               `json:"reason"` // 重复原因
	Keep      bool                 `json:"keep"`   // 是否建议保留（true=保留，false=建议删除）
	PairComic *models.OfflineComic `json:"pairComic,omitempty"` // 成对对象（Round4 任务一）
}

// DedupResult 查重结果
type DedupResult struct {
	Items      []DedupItem `json:"items"`      // 需要处理的项（含建议保留项与建议删除项）
	FinishedAt int64       `json:"finishedAt"` // 结果生成时间戳(ms)
	Stale      bool        `json:"stale"`      // 是否已过期（删除操作后置 true，提示前端重新扫描）
}

// ErrComicNotFound 漫画记录不存在（幽灵文件容错：记录可能已被其他设备删除）
var ErrComicNotFound = errors.New("未找到漫画记录")

// MaintainDedup 本地维护查重
//
// 规则：
//   - 同 GID 多份：保留文件夹形态（gallery），建议删除压缩包形态（archive）
//   - 归档文件 hash 相同：内容完全相同，建议删除重复项
//   - 父画廊关系：旧版（父画廊）被新版取代，建议删除旧版（支持在线发现：磁盘元数据
//     parent/child 关系为空，需联网核对详情页；ehService 为空或未绑账号时退化为纯本地）
//   - 文件夹内容签名相同（无 gid/hash/parent 元数据的复制型重复）→ 删除复制项（问题3修复）
func MaintainDedup(db *gorm.DB, ehService *EHService) (*DedupResult, error) {
	return maintainDedupWithProgress(db, ehService, nil, true)
}

// MaintainDedupWithProgress 带进度回调的维护查重（问题3：长任务进度可感知，供 handler 异步任务使用）
// forceFull=true 时忽略 parent_checked_at 增量标记，强制对全部符合条件的漫画做在线父子关系发现（需求1 兜底）。
func MaintainDedupWithProgress(db *gorm.DB, ehService *EHService, onProgress OfflineProgressFn, forceFull bool) (*DedupResult, error) {
	return maintainDedupWithProgress(db, ehService, onProgress, forceFull)
}

// maintainDedupWithProgress 带进度回调的本地维护查重（问题3：长任务进度可感知）
func maintainDedupWithProgress(db *gorm.DB, ehService *EHService, onProgress OfflineProgressFn, forceFull bool) (*DedupResult, error) {
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
	pairID := map[string]string{}        // Round4 任务一：comic id → 成对对象 comic id（对比视图双列展示）

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
			// Round4 任务一：记录成对关系（保留项 ↔ 删除项），对比视图双列展示
			pairID[group[i].ID] = group[keepIdx].ID
			if pairID[group[keepIdx].ID] == "" {
				pairID[group[keepIdx].ID] = group[i].ID
			}
		}
	}

	// ── 2. 归档文件 hash 查重（仅 .zip/.cbz 等归档）──
	hashGroups := map[string][]models.OfflineComic{}
	hashTotal := 0
	for i := range comics {
		if comics[i].SourceMode == "archive" && comics[i].LocalPath != "" && IsArchive(comics[i].LocalPath) {
			hashTotal++
		}
	}
	hashDone := 0
	for i := range comics {
		c := &comics[i]
		if c.SourceMode != "archive" || c.LocalPath == "" {
			continue
		}
		if !IsArchive(c.LocalPath) {
			continue
		}
		hashDone++
		if onProgress != nil {
			onProgress(hashDone, hashTotal, c.Title, "归档 Hash 计算")
		}
		h, err := hashFile(c.LocalPath)
		if err != nil {
			log.Printf("%s [maintain] 计算 %q hash 失败: %v", dlWarnTag, c.LocalPath, err)
			continue
		}
		c.FileHash = h
		_ = db.Model(c).Update("file_hash", h)
		hashGroups[h] = append(hashGroups[h], *c)
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
			// Round4 任务一：与同组首个归档互为成对对象（对比视图双列展示）
			pairID[c.ID] = group[0].ID
			if pairID[group[0].ID] == "" {
				pairID[group[0].ID] = c.ID
			}
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
			// 问题3：在线父子关系发现阶段进度（联网抓取最长，逐本限流 1.2s）
			fetchTotal := 0
			for i := range comics {
				c := &comics[i]
				// 需求1 兜底：已核对过父画廊关系的漫画（parent_checked_at>0）默认增量跳过；
				// forceFull=true 时忽略该标记，强制全量联网重抓。
				if c.GID == "" || c.Token == "" || c.ParentGID != "" || removeSet[c.ID] || (!forceFull && c.ParentCheckedAt != 0) {
					continue
				}
				fetchTotal++
			}
			fetchDone := 0
			for i := range comics {
				c := &comics[i]
				if c.GID == "" || c.Token == "" || c.ParentGID != "" || removeSet[c.ID] || (!forceFull && c.ParentCheckedAt != 0) {
					continue
				}
				fetchDone++
				if onProgress != nil {
					onProgress(fetchDone, fetchTotal, c.Title, "在线父子关系发现")
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
					// Round4 任务一：旧版 ↔ 新版 互为成对对象（对比视图双列展示）
					pairID[c.ID] = successor.ID
					if pairID[successor.ID] == "" {
						pairID[successor.ID] = c.ID
					}
					log.Printf("%s [maintain] 漫画 %q(gid=%s) 被新版 %q(gid=%s) 取代，建议删除旧版",
						dlLogTag, c.Title, c.GID, successor.Title, successor.GID)
				}
				// 需求1 兜底：无论本次是否发现父画廊/新版，都记录在线核对时间戳，
				// 下次维护查重据此增量跳过已核对漫画，避免对同一批内容重复联网抓取。
				now := time.Now().UnixMilli()
				_ = db.Model(c).Update("parent_checked_at", now)
				c.ParentCheckedAt = now

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
			// Round4 任务一：旧版 p ↔ 新版 c 互为成对对象（对比视图双列展示）
			pairID[p.ID] = c.ID
			if pairID[c.ID] == "" {
				pairID[c.ID] = p.ID
			}
		}
	}
		// ── 4. 文件夹内容签名查重（问题3修复）──
		// 针对「无 gid/hash/parent 元数据的复制型文件夹重复」：
		//   递归收集文件夹内所有图片的「相对路径|大小」生成内容签名，签名相同 = 内容完全一致。
		//   签名缓存进 file_hash（与规则2的归档 hash 互斥：仅 gallery 形态使用）。
		sigGroups := map[string][]models.OfflineComic{}
		sigTotal := 0
		for i := range comics {
			c := &comics[i]
			if c.SourceMode == "archive" || !isFolderPath(c.LocalPath) {
				continue
			}
			if removeSet[c.ID] {
				continue
			}
			sigTotal++
		}
		sigDone := 0
		for i := range comics {
			c := &comics[i]
			if c.SourceMode == "archive" || !isFolderPath(c.LocalPath) {
				continue
			}
			if removeSet[c.ID] {
				continue
			}
			sigDone++
			if onProgress != nil {
				onProgress(sigDone, sigTotal, c.Title, "文件夹内容签名")
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
				_ = db.Model(c).Updates(map[string]interface{}{"file_hash": sig, "file_modified_at": maxMod})
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
				// Round4 任务一：与同组保留项互为成对对象（对比视图双列展示）
				pairID[group[i].ID] = group[keepIdx].ID
				if pairID[group[keepIdx].ID] == "" {
					pairID[group[keepIdx].ID] = group[i].ID
				}
			}
		}
	
		// ── 组装结果 ──
		// 删除标记优先于保留标记：同一漫画若同时命中“被新版取代(删)”与“是某旧版的新版(留)”，一律判删，
		// 确保多版本链（如 4019697→4051934→4086937）只保留最新版一份。
	for i := range comics {
		c := comics[i]
		var pair *models.OfflineComic
		if pid := pairID[c.ID]; pid != "" {
			pair = findComicByID(comics, pid)
		}
		if removeSet[c.ID] {
			result.Items = append(result.Items, DedupItem{Comic: c, Reason: reasonMap[c.ID], Keep: false, PairComic: pair})
			continue
		}
		if keepSet[c.ID] {
			result.Items = append(result.Items, DedupItem{Comic: c, Reason: "建议保留", Keep: true, PairComic: pair})
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

// findComicByID 在 comics 切片中按 id 查找漫画（返回最新值指针；成对对象解析用，Round4 任务一）。
func findComicByID(comics []models.OfflineComic, id string) *models.OfflineComic {
	for i := range comics {
		if comics[i].ID == id {
			return &comics[i]
		}
	}
	return nil
}

// filterOfflineUpdateEnabled 过滤掉「已关闭离线维护」的额外路径下的漫画（问题4）。
//
// 规则：
//   - 下载导入的漫画（ScanPathID == ""）始终参与更新检测/查重；
//   - 额外路径的漫画仅当其路径 EnableOfflineUpdate == true 时参与；
//   - 已老化漫画（AgedStatus=true）不参与更新检测（Round4 任务四，只扫描一次）；
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
		if c.AgedStatus {
			continue // 已老化：排除后续扫描
		}
		if c.RemovedStatus {
			continue // 画廊已被删除/移除：排除后续扫描（需求 3(2)，避免重复联网）
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
		// 幽灵文件容错：记录不存在（可能已被其他设备删除）→ 返回哨兵错误，调用方视为「已删除」
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrComicNotFound
		}
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
	// 需求4：删除记录视为书库变更，记录时间戳供「队列空闲>1min」自动增量维护查重判断。
	MarkLibraryChanged()
	log.Printf("%s [maintain] 已删除漫画 %q（id=%s）", dlLogTag, comic.Title, comicID)
	return nil
}

// RemoveDedupComic 删除重复漫画（查重维护入口）：委托 DeleteOfflineComic 统一处理历史记录与文件
func RemoveDedupComic(db *gorm.DB, comicID string, deleteFile bool) error {
	return DeleteOfflineComic(db, comicID, deleteFile)
}

// RemoveDedupComics 批量删除重复漫画（查重维护入口）：逐个委托 DeleteOfflineComic，
// 单项失败不中断整体，返回实际删除数量与首个错误（便于前端展示“删除 N 项，部分失败”）。
func RemoveDedupComics(db *gorm.DB, comicIDs []string, deleteFile bool) (int, error) {
	if len(comicIDs) == 0 {
		return 0, fmt.Errorf("未指定要删除的漫画")
	}
	deleted := 0
	var firstErr error
	for _, id := range comicIDs {
		if err := DeleteOfflineComic(db, id, deleteFile); err != nil {
			// 幽灵文件容错：记录已不存在（可能已由其他设备删除）→ 视为已删除，不计为失败
			if errors.Is(err, ErrComicNotFound) {
				log.Printf("%s [maintain] 漫画 %s 记录已不存在（视为已删除）", dlWarnTag, id)
				deleted++
				continue
			}
			log.Printf("%s [maintain] 批量删除失败（comic %s）: %v", dlErrTag, id, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		deleted++
	}
	return deleted, firstErr
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
