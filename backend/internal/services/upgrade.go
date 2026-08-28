package services

import (
	"log"
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
//   - 下载方案 ≠ archiveOriginal：download_scheme 非 archiveOriginal 即纳入
//     （空 = 存量数据，本字段上线前下载，压缩/原图未知，同样纳入并标注「版本未知」）。
//
// 升级动作 = 用本地记录的 gid/token 创建「归档原图」下载任务（UpdateForComicID 关联），
// 完成后由 finalizeUpdate 按升级语义（ForceDeleteOriginal）自动删除旧版。
// ─────────────────────────────────────────────────────────────

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
	DownloadScheme string    `json:"downloadScheme"` // 空 = 存量未知版本
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
// 返回按更新时间倒序的候选列表。
func ListUpgradeCandidates(db *gorm.DB) ([]UpgradeCandidate, error) {
	if db == nil {
		return nil, nil
	}
	var comics []models.OfflineComic
	err := db.
		Where("(scan_path_id = '' OR scan_path_id IS NULL)").
		Where("g_id != '' AND token != ''").
		Where("removed_status = ?", false).
		// 方案未知（空/存量）或非归档原图 → 全部纳入；已达标（archiveOriginal）排除
		Where("(download_scheme = '' OR download_scheme IS NULL OR download_scheme != ?)", string(models.DefaultSchemeArchiveOriginal)).
		Order("updated_at desc").
		Find(&comics).Error
	if err != nil {
		return nil, err
	}

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
			DownloadScheme: c.DownloadScheme,
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
