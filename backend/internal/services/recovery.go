package services

import (
	"errors"
	"log"
	"os"
	"strings"
	"sync"

	"SakuManga/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ─────────────────────────────────────────────────────────────
// Round54：全量扫描「失效对账」阶段
//
// 背景：现有 scanDirectory 只做「遍历现有文件 → 入库」，从不对比数据库
// 记录与磁盘实际存在性；Q：用户误删整个目录/部分本子后，扫描发现不了
// 「数据库有记录、文件没了」。本文件补上扫描后的对账阶段：
//   - 根路径不存在（整条路径被删）→ 全部记录判失效
//   - 逐条 os.Stat(localPath) 判定存在性（网络盘不可达等非 IsNotExist
//     错误保守视为存在，避免整库误判）
//   - 失效且带 g_id/token 且未 marked removed → 恢复候选（recovery_candidates）
//   - 失效且无 g_id/token → 阵亡 no-ehentai
//   - 失效且 removed_status=1 → 阵亡 removed
//   - 文件重新存在 → 自愈清除遗留的候选/阵亡条目（阵亡保留用户 dismissed 状态）
//
// 只写 recovery_candidates / fallen_comics，绝不修改 offline_comics。
// ─────────────────────────────────────────────────────────────

// reconcileStatConcurrency 对账阶段并发 Stat 上限（网络盘 SMB 并发不宜过高）
const reconcileStatConcurrency = 16

// ScanReconcileResult 单次全量扫描失效对账的结果统计
// （与 offline.go 的 ReconcileResult（下载后数据对账）区分命名）
type ScanReconcileResult struct {
	Total       int  // 该扫描路径下的记录总数
	Missing     int  // 判定失效（文件不存在）的记录数
	Candidates  int  // 写入/更新的恢复候选数
	Fallen      int  // 写入/更新的阵亡条目数
	Healed      int  // 自愈：清除的候选/阵亡条目数
	PathMissing bool // 扫描根路径不存在（整条路径判失效）
}

// ReconcileScanPathAgainstDisk 对指定扫描路径执行失效对账。
//
// pathID：额外扫描路径 ID（offline_comics.scan_path_id）；rootPath：扫描根路径。
// progress 可为 nil（测试/内部调用不需要进度上报）。
//
// 注意：本函数不检查「扫描是否成功」——调用方（扫描管理器）负责编排时机；
// 根路径不存在时照常执行（所有 local_path 判失效）。
func ReconcileScanPathAgainstDisk(db *gorm.DB, pathID, rootPath string, progress *ScanProgress) (*ScanReconcileResult, error) {
	if db == nil {
		return nil, errors.New("对账失败：db 为空")
	}
	if strings.TrimSpace(pathID) == "" {
		return nil, errors.New("对账失败：pathID 为空")
	}

	res := &ScanReconcileResult{}

	// 根路径不存在 → 标记整条路径失效（本场景下所有记录的 local_path 也必然 Stat 失败）
	if strings.TrimSpace(rootPath) != "" {
		if _, err := os.Stat(rootPath); os.IsNotExist(err) {
			res.PathMissing = true
			if progress != nil {
				progress.setPathMissing(true)
			}
		}
	}

	var comics []models.OfflineComic
	if err := db.Where("scan_path_id = ?", pathID).Find(&comics).Error; err != nil {
		return nil, err
	}
	res.Total = len(comics)
	if progress != nil {
		progress.setReconcileTotal(len(comics))
	}

	// 并发 Stat（每记录一个轻量任务，信号量限并发；2601 条网络盘预计 <10s）
	exists := make([]bool, len(comics))
	sem := make(chan struct{}, reconcileStatConcurrency)
	var wg sync.WaitGroup
	for i := range comics {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			exists[i] = pathExistsConservative(comics[i].LocalPath)
		}(i)
	}
	wg.Wait()

	// 逐条分类写入
	for i := range comics {
		c := &comics[i]
		if progress != nil {
			progress.setCurrentTitle(c.Title)
			progress.incReconcileProcessed()
		}
		if exists[i] {
			healed, err := healRecoveredRecord(db, c)
			if err != nil {
				log.Printf("%s [reconcile] 自愈清除 %q 失败: %v", dlWarnTag, c.Title, err)
				continue
			}
			res.Healed += healed
			continue
		}

		res.Missing++
		switch {
		case c.GID == "" || c.Token == "":
			// 非 E 站来源：无法重下
			if err := upsertFallen(db, c, models.FallenReasonNoEhentai, rootPath); err != nil {
				return nil, err
			}
			res.Fallen++
		case c.RemovedStatus:
			// EH 版权/下架：可重下但已 404
			if err := upsertFallen(db, c, models.FallenReasonRemoved, rootPath); err != nil {
				return nil, err
			}
			res.Fallen++
		default:
			// 可恢复：进候选
			if err := upsertCandidate(db, c, pathID); err != nil {
				return nil, err
			}
			res.Candidates++
		}
	}

	log.Printf("%s [reconcile] 路径 %q 对账完成：记录 %d / 失效 %d / 候选 %d / 阵亡 %d / 自愈 %d（路径缺失=%v）",
		dlLogTag, rootPath, res.Total, res.Missing, res.Candidates, res.Fallen, res.Healed, res.PathMissing)
	return res, nil
}

// pathExistsConservative 判定本地漫画路径是否仍存在。
// 口径（设计 6.1）：仅 IsNotExist 判失效；空路径判失效；
// 权限/网络抖动等其他错误保守视为存在——避免网络盘短时不可达时把整库误判失效。
func pathExistsConservative(p string) bool {
	if strings.TrimSpace(p) == "" {
		return false
	}
	if _, err := os.Stat(p); err != nil {
		return !os.IsNotExist(err)
	}
	return true
}

// upsertCandidate 写入/更新恢复候选（按 g_id 幂等）。
func upsertCandidate(db *gorm.DB, c *models.OfflineComic, pathID string) error {
	cand := models.RecoveryCandidate{
		GID:        c.GID,
		Token:      c.Token,
		Title:      c.Title,
		TitleJpn:   c.TitleJpn,
		CoverURL:   c.CoverURL,
		Category:   c.Category,
		PageCount:  c.PageCount,
		FileSize:   c.FileSize,
		LocalPath:  c.LocalPath,
		ScanPathID: pathID,
		ComicID:    c.ID,
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "g_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"token", "title", "title_jpn", "cover_url", "category",
			"page_count", "file_size", "local_path", "scan_path_id", "comic_id", "updated_at",
		}),
	}).Create(&cand).Error
}

// fallenDedupeKey 阵亡条目去重键：有 g_id 用 gid，否则用 comic_id
// （SQLite 唯一索引对空值不去重，必须显式键保证 upsert 幂等）。
func fallenDedupeKey(c *models.OfflineComic) string {
	if c.GID != "" {
		return "gid:" + c.GID
	}
	return "cid:" + c.ID
}

// upsertFallen 写入/更新阵亡条目（按 dedupe_key 幂等）。
// 注意：不更新 dismissed / dismissed_at——保留用户手动 x 的状态（设计 6.3/第 12 节）。
func upsertFallen(db *gorm.DB, c *models.OfflineComic, reason, sourcePath string) error {
	f := models.FallenComic{
		DedupeKey:  fallenDedupeKey(c),
		GID:        c.GID,
		ComicID:    c.ID,
		Title:      c.Title,
		TitleJpn:   c.TitleJpn,
		CoverURL:   c.CoverURL,
		Category:   c.Category,
		FileSize:   c.FileSize,
		PageCount:  c.PageCount,
		Reason:     reason,
		SourcePath: sourcePath,
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "dedupe_key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"g_id", "comic_id", "title", "title_jpn", "cover_url", "category",
			"file_size", "page_count", "reason", "source_path", "updated_at",
		}),
	}).Create(&f).Error
}

// healRecoveredRecord 文件重新存在时清除误判条目（自愈）：
//   - 候选按 g_id 无条件清除（文件回来就不需要恢复）
//   - 阵亡仅清除未 dismissed 的（用户手动 x 的条目保留展示）
//
// 返回清除的条目数。
func healRecoveredRecord(db *gorm.DB, c *models.OfflineComic) (int, error) {
	healed := 0
	if c.GID != "" {
		cand := db.Where("g_id = ?", c.GID).Delete(&models.RecoveryCandidate{})
		if cand.Error != nil {
			return healed, cand.Error
		}
		healed += int(cand.RowsAffected)

		fallen := db.Where("g_id = ? AND dismissed = ?", c.GID, false).Delete(&models.FallenComic{})
		if fallen.Error != nil {
			return healed, fallen.Error
		}
		healed += int(fallen.RowsAffected)
		return healed, nil
	}

	// 无 g_id：阵亡条目按 dedupe_key 清除（仅未 dismissed）
	fallen := db.Where("dedupe_key = ? AND dismissed = ?", "cid:"+c.ID, false).Delete(&models.FallenComic{})
	if fallen.Error != nil {
		return healed, fallen.Error
	}
	healed += int(fallen.RowsAffected)
	return healed, nil
}
