package services

import (
	"SakuHentai/internal/models"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
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

	result := &UpdateCheckResult{Checked: len(comics), NeedsUpdate: []models.OfflineComic{}}
	gidMap := map[string]*models.OfflineComic{} // gid → 本地漫画（父画廊关系查重用）
	changed := map[string]bool{}                // comic id → 是否被标记

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
		detail, err := ehService.FetchGalleryDetail(account, c.GID, c.Token, ehSetting)
		if err != nil {
			log.Printf("%s [update] 漫画 %q(gid=%s) 在线详情拉取失败（跳过）: %v", dlWarnTag, c.Title, c.GID, err)
		} else if detail != nil && detail.PageCount > 0 && c.PageCount > 0 && detail.PageCount > c.PageCount {
			c.NeedsUpdate = true
			c.NewGID = detail.ID
			c.NewToken = detail.Token
			c.UpdateNote = fmt.Sprintf("原画廊新增了 %d 页（在线 %d 页 > 本地 %d 页）", detail.PageCount-c.PageCount, detail.PageCount, c.PageCount)
			changed[c.ID] = true
			log.Printf("%s [update] 漫画 %q(gid=%s) 需要更新：%s", dlLogTag, c.Title, c.GID, c.UpdateNote)
		}
		// 限流退避
		time.Sleep(1200 * time.Millisecond)
	}

	// ── B. 父画廊关系检测（本地，无网络）──
	for i := range comics {
		c := &comics[i]
		if c.ParentGID == "" || c.ParentGID == c.GID {
			continue
		}
		if p, ok := gidMap[c.ParentGID]; ok && p.GID != c.GID {
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
//   - 父画廊关系：旧版（父画廊）被新版取代，建议删除旧版
func MaintainDedup(db *gorm.DB) (*DedupResult, error) {
	if db == nil {
		return nil, fmt.Errorf("非法参数：db 不能为空")
	}

	var comics []models.OfflineComic
	if err := db.Where("source = ?", models.SourceOffline).Order("updated_at desc").Find(&comics).Error; err != nil {
		return nil, fmt.Errorf("读取离线漫画失败: %v", err)
	}

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

	// ── 3. 父画廊关系查重 ──
	gidToComic := map[string]*models.OfflineComic{}
	for i := range comics {
		if comics[i].GID != "" {
			gidToComic[comics[i].GID] = &comics[i]
		}
	}
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
			// 新版建议保留
			keepSet[c.ID] = true
		}
	}

	// ── 组装结果 ──
	for i := range comics {
		c := comics[i]
		if keepSet[c.ID] {
			result.Items = append(result.Items, DedupItem{Comic: c, Reason: "建议保留", Keep: true})
			continue
		}
		if removeSet[c.ID] {
			result.Items = append(result.Items, DedupItem{Comic: c, Reason: reasonMap[c.ID], Keep: false})
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
