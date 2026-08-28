package services

import (
	"log"
	"strings"
	"time"

	"SakuHentai/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// 画质升级（图片质量升级功能）
//
// 目标：把「低清/非最优版本」的本子升级为「归档原图（archiveOriginal）」下载方案。
// 检测范围（元数据为唯一标准，不做文件尺寸自动判定）：
//   - 仅 SakuHentai 自己下载导入的本子：scan_path_id 为空（下载导入）
//     + gid/token 非空（下载时写入 metadata/ametadata/ComicInfo.xml 的 E 站元数据）；
//   - 未被标记「画廊已被删除/移除」（removed_status = false，被删画廊无法下载）；
//   - 有效下载方案 ≠ archiveOriginal。有效方案解析（ResolveEffectiveDownloadScheme）：
//     1. 新数据：直接取 download_scheme（下载完成时回填，最准）；
//     2. 存量数据（download_scheme 空）：查 download_tasks 该 gid 最近一次「已完成」任务
//        的 mode/archiveType（即下载任务列表展示的「归档·原图 / 归档·压缩 / 画廊」）；
//     3. 仍查不到任务记录（存量任务被清理等）：按本地落地目录名推断——
//        目录名带 "archive - " 前缀 = 归档下载（视为归档原图，排除）；
//        普通目录 = 画廊下载（纳入）。
//
// 升级动作 = 用本地记录的 gid/token 创建「归档原图」下载任务（UpdateForComicID 关联），
// 完成后由 finalizeUpdate 按升级语义（ForceDeleteOriginal）自动删除旧版。
// ─────────────────────────────────────────────────────────────

// BuildTaskSchemeIndex 从 download_tasks 构建 gid → 下载方案 索引。
// 仅统计「已完成」（completed）任务（下载成功才代表实际落地版本；cancelled/error 不代表）。
// 同一 gid 多条完成记录时取 updated_at 最新一条（重复下载/更新后以最新方案为准）。
func BuildTaskSchemeIndex(db *gorm.DB) map[string]string {
	idx := map[string]string{}
	if db == nil {
		return idx
	}
	var tasks []models.DownloadTask
	if err := db.Select("g_id", "mode", "archive_type", "updated_at").
		Where("status = ?", models.DownloadCompleted).
		Find(&tasks).Error; err != nil {
		log.Printf("%s [upgrade] 构建任务方案索引失败: %v", dlWarnTag, err)
		return idx
	}
	latest := map[string]time.Time{}
	for _, t := range tasks {
		if t.GID == "" {
			continue
		}
		ts := t.UpdatedAt
		if prev, ok := latest[t.GID]; ok && !ts.After(prev) {
			continue
		}
		latest[t.GID] = ts
		idx[t.GID] = taskSchemeValue(t.Mode, t.ArchiveType)
	}
	return idx
}

// taskSchemeValue 将下载任务（mode + archiveType）映射为四值下载方案
func taskSchemeValue(mode models.DownloadMode, archiveType models.ArchiveType) string {
	if mode == models.DownloadModeArchive {
		if archiveType == models.ArchiveTypeResample {
			return string(models.DefaultSchemeArchiveResample)
		}
		return string(models.DefaultSchemeArchiveOriginal)
	}
	return string(models.DefaultSchemeGallery)
}

// isArchiveDownloadPath 判断本地落地目录是否为「归档下载解压」形态：
// SakuHentai 归档下载目录固定命名为 "archive - <gid> - <title>"（download_archive.go），
// 画廊下载目录为 "<gid> - <title>"，据此区分（仅兜底推断用）。
func isArchiveDownloadPath(localPath string) bool {
	return strings.Contains(localPath, `\archive - `) || strings.Contains(localPath, `/archive - `)
}

// ResolveEffectiveDownloadScheme 解析漫画的有效下载方案（升级判定用）：
//  1. download_scheme 非空（新数据回填）→ 直接取；
//  2. 否则查 taskScheme 索引（gid → 最近一次完成任务的方案，即下载列表展示的方案）；
//  3. 仍未命中 → 按落地目录名推断：archive - 前缀 = 归档下载（视为归档原图），
//     普通目录 = 画廊下载。
//
// 推断仅用于判定，不写回数据库（download_scheme 仍为空，未来真实下载完成后再回填）。
func ResolveEffectiveDownloadScheme(c *models.OfflineComic, taskScheme map[string]string) string {
	if c == nil {
		return ""
	}
	if c.DownloadScheme != "" {
		return c.DownloadScheme
	}
	if taskScheme != nil {
		if s, ok := taskScheme[c.GID]; ok && s != "" {
			return s
		}
	}
	if isArchiveDownloadPath(c.LocalPath) {
		return string(models.DefaultSchemeArchiveOriginal)
	}
	return string(models.DefaultSchemeGallery)
}

// UpgradeCandidate 升级候选漫画 DTO（列表接口返回）
type UpgradeCandidate struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	CoverURL       string    `json:"coverUrl"`
	Category       string    `json:"category,omitempty"`
	PageCount      int       `json:"pageCount"`
	FileSize       int64     `json:"fileSize"`
	LocalPath      string    `json:"localPath,omitempty"`
	GID            string    `json:"gid,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
	SourceMode     string    `json:"sourceMode"`
	DownloadScheme string    `json:"downloadScheme"` // 有效方案（存量经任务索引/目录名推断后的值，非原始字段值）
	Upgrading      bool      `json:"upgrading"`      // 该 gid 已有进行中的下载任务（含升级）
}

// upgradeActiveStatuses 视为「进行中/未完成」的任务状态（创建任务时的 gid 去重集合）
var upgradeActiveStatuses = []string{
	string(models.DownloadQueued),
	string(models.DownloadDownloading),
	string(models.DownloadPaused),
	string(models.DownloadError),
	string(models.DownloadErrorLock),
}

// ListUpgradeCandidates 列出全部画质升级候选（纯本地查询，不联网）。
// 基础池 = 下载导入 + 有 gid/token + 未被删除/移除；有效方案在 Go 层判定，
// 非 archiveOriginal（含推断值）才进入候选。返回按更新时间倒序。
func ListUpgradeCandidates(db *gorm.DB) ([]UpgradeCandidate, error) {
	if db == nil {
		return nil, nil
	}
	var comics []models.OfflineComic
	if err := db.
		Where("(scan_path_id = '' OR scan_path_id IS NULL)").
		Where("g_id != '' AND token != ''").
		Where("removed_status = ?", false).
		Order("updated_at desc").
		Find(&comics).Error; err != nil {
		return nil, err
	}
	// 方案判定：download_scheme → 任务索引 → 目录名推断（内存完成，避免 N+1）
	taskScheme := BuildTaskSchemeIndex(db)

	// 进行中任务 gid 集合（供 Upgrading 标注：同 gid 已有未完成任务时禁用升级按钮）
	activeGIDs := map[string]bool{}
	if len(comics) > 0 {
		var tasks []models.DownloadTask
		if err := db.Select("g_id").Where("status IN ?", upgradeActiveStatuses).Find(&tasks).Error; err == nil {
			for _, t := range tasks {
				activeGIDs[t.GID] = true
			}
		}
	}

	out := make([]UpgradeCandidate, 0, len(comics))
	for _, c := range comics {
		eff := ResolveEffectiveDownloadScheme(&c, taskScheme)
		if eff == string(models.DefaultSchemeArchiveOriginal) {
			continue // 已达标（归档原图），不进入升级范围
		}
		out = append(out, UpgradeCandidate{
			ID:             c.ID,
			Title:          c.Title,
			CoverURL:       c.CoverURL,
			Category:       c.Category,
			PageCount:      c.PageCount,
			FileSize:       c.FileSize,
			LocalPath:      c.LocalPath,
			GID:            c.GID,
			UpdatedAt:      c.UpdatedAt,
			SourceMode:     c.SourceMode,
			DownloadScheme: eff,
			Upgrading:      activeGIDs[c.GID],
		})
	}
	return out, nil
}

// markDownloadScheme 下载任务完成后将下载方案回填到刚入库的离线漫画记录。
// localPath 为落地路径（画廊=destDir，归档=解压目录），scheme 为四值下载方案。
// 由下载引擎在 ScanAndSaveDirectory 后调用；扫描入库不覆盖该字段（scanner.go 保留）。
func markDownloadScheme(db *gorm.DB, localPath, scheme string) {
	if db == nil || localPath == "" || scheme == "" {
		return
	}
	if err := db.Model(&models.OfflineComic{}).
		Where("local_path = ?", localPath).
		Update("download_scheme", scheme).Error; err != nil {
		log.Printf("%s [upgrade] 回填下载方案失败 %q（scheme=%s）: %v", dlWarnTag, localPath, scheme, err)
		return
	}
	log.Printf("%s [upgrade] 已回填下载方案：%q → %s", dlLogTag, localPath, scheme)
}
