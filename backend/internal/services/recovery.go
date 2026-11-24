package services

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

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

	// 预加载阵亡 gid 集合（Round54 阶段3）：确定性失败已入阵亡的本子不再重入候选，
	// 避免「恢复失败 → 阵亡 → 下次对账又进候选 → 再恢复再失败」的循环。
	fallenGIDs := map[string]bool{}
	var fallenGIDList []string
	if err := db.Model(&models.FallenComic{}).Where("g_id <> ''").Pluck("g_id", &fallenGIDList).Error; err != nil {
		return nil, err
	}
	for _, g := range fallenGIDList {
		fallenGIDs[g] = true
	}

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
		case fallenGIDs[c.GID]:
			// 已在阵亡名单（恢复确定性失败）：保持阵亡，不重入候选
			continue
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

// ─────────────────────────────────────────────────────────────
// Round54 阶段2：恢复会话
//
// 设计要点（见设计文档第 8/9/10 节）：
//   - 恢复任务 = 更新任务语义（UpdateForComicID 关联旧记录，绕过「已下载」去重）
//   - 恢复中判定由「未终结恢复任务」推导，不新增状态表（避免状态漂移）
//   - 锁粒度按扫描路径：只锁恢复队列本子原属路径
//   - 取消保留候选清单，可幂等重新入队
// ─────────────────────────────────────────────────────────────

// RecoveryGroup 恢复任务在 download_tasks.group 中的标记
const RecoveryGroup = "recovery"

// 恢复方案（UI 档位；与设计文档 8.1 档位映射一致）
const (
	RecoverySchemeArchiveOriginal = "archive-original" // H@H 原图（归档原图，耗 GP）
	RecoverySchemeArchiveResample = "archive-resample" // H@H 压缩（归档压缩）
	RecoverySchemeGallery         = "gallery"          // 画廊逐图（不耗 GP 但慢）
)

// 恢复相关哨兵错误（handler 据此映射状态码）
var (
	ErrRecoveryNoCandidates  = errors.New("没有可恢复的候选（请先对扫描路径执行全量扫描）")
	ErrRecoveryInProgress    = errors.New("恢复任务进行中，请等待完成或先取消")
	ErrRecoverySchemeInvalid = errors.New("无效的恢复方案（仅支持 archive-original / archive-resample / gallery）")
)

// recoveryUnfinishedStatuses 视为「恢复进行中」的任务状态集合。
// error / error_lock 视为可重试（网络/配额失败留队列），确定性失败由钩子转为 cancelled。
func recoveryUnfinishedStatuses() []string {
	return []string{
		string(models.DownloadQueued),
		string(models.DownloadDownloading),
		string(models.DownloadPaused),
		string(models.DownloadError),
		string(models.DownloadErrorLock),
	}
}

// RecoveryStatus 恢复功能状态（前端按钮置灰/进度展示用）
type RecoveryStatus struct {
	Phase          string   `json:"phase"`          // idle | ready | recovering
	CandidateCount int64    `json:"candidateCount"` // 恢复候选数
	ActiveTasks    int64    `json:"activeTasks"`    // 未终结恢复任务数
	LockedPathIDs  []string `json:"lockedPathIds"`  // 恢复中锁定的扫描路径
}

// GetRecoveryStatus 推导恢复状态（无状态表，全部由查询得出，重启自动恢复语义）。
func GetRecoveryStatus(db *gorm.DB) (*RecoveryStatus, error) {
	if db == nil {
		return nil, errors.New("恢复状态查询失败：db 为空")
	}
	st := &RecoveryStatus{LockedPathIDs: []string{}}

	if err := db.Model(&models.RecoveryCandidate{}).Count(&st.CandidateCount).Error; err != nil {
		return nil, err
	}

	statuses := recoveryUnfinishedStatuses()
	if err := db.Model(&models.DownloadTask{}).
		Where("\"group\" = ? AND status IN ?", RecoveryGroup, statuses).
		Count(&st.ActiveTasks).Error; err != nil {
		return nil, err
	}

	// 锁定路径 = 存在未终结恢复任务、且其 g_id 仍在候选表中的扫描路径
	if err := db.Raw(`SELECT DISTINCT c.scan_path_id FROM recovery_candidates c
		WHERE c.scan_path_id <> '' AND EXISTS (
			SELECT 1 FROM download_tasks t
			WHERE t."group" = ? AND t.g_id = c.g_id AND t.status IN ?
		)`, RecoveryGroup, statuses).Scan(&st.LockedPathIDs).Error; err != nil {
		return nil, err
	}

	switch {
	case st.ActiveTasks > 0:
		st.Phase = "recovering"
	case st.CandidateCount > 0:
		st.Phase = "ready"
	default:
		st.Phase = "idle"
	}
	return st, nil
}

// IsRecoveryLockedPath 该扫描路径是否被恢复任务锁定（扫描守卫用）
func IsRecoveryLockedPath(db *gorm.DB, pathID string) (bool, error) {
	if db == nil || strings.TrimSpace(pathID) == "" {
		return false, nil
	}
	var count int64
	err := db.Model(&models.RecoveryCandidate{}).
		Where("scan_path_id = ?", pathID).
		Where(`EXISTS (SELECT 1 FROM download_tasks t
			WHERE t."group" = ? AND t.g_id = recovery_candidates.g_id AND t.status IN ?)`,
			RecoveryGroup, recoveryUnfinishedStatuses()).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListRecoveryCandidates 分页读取恢复候选（按创建时间升序）
func ListRecoveryCandidates(db *gorm.DB, page, size int) ([]models.RecoveryCandidate, int64, error) {
	if size <= 0 || size > 500 {
		size = 50
	}
	if page <= 0 {
		page = 1
	}
	var total int64
	if err := db.Model(&models.RecoveryCandidate{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.RecoveryCandidate
	err := db.Order("created_at ASC, g_id ASC").
		Offset((page - 1) * size).Limit(size).
		Find(&items).Error
	return items, total, err
}

// RecoveryStartParams 启动恢复入参
type RecoveryStartParams struct {
	Scheme string `json:"scheme"` // archive-original | archive-resample | gallery
	UserID uint   `json:"userId"` // 任务发起者（执行时使用其 E 站凭证）
}

// RecoveryStartResult 启动恢复统计
type RecoveryStartResult struct {
	Created int      `json:"created"`
	Skipped int      `json:"skipped"` // 已有未终结同 gid 任务（幂等）
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors"`
}

// recoverySchemeToMode 档位 → 下载模式/归档类型映射（设计文档 8.1）
func recoverySchemeToMode(scheme string) (models.DownloadMode, models.ArchiveType, error) {
	switch scheme {
	case RecoverySchemeArchiveOriginal:
		return models.DownloadModeArchive, models.ArchiveTypeOriginal, nil
	case RecoverySchemeArchiveResample:
		return models.DownloadModeArchive, models.ArchiveTypeResample, nil
	case RecoverySchemeGallery:
		return models.DownloadModeGallery, "", nil
	}
	return "", "", ErrRecoverySchemeInvalid
}

// StartRecovery 把全部恢复候选批量转为下载任务并入队。
//
// 语义：
//   - 每条候选 → CreateTask(UpdateForComicID=候选.ComicID, Group="recovery",
//     ForceDeleteOriginal=true)。UpdateForComicID 非空 → 绕过「已下载」去重，
//     且完成后 finalizeUpdate 负责删旧记录、迁移引用、清候选/阵亡（阶段3钩子）。
//   - 幂等：已有未终结同 gid 恢复任务 → CreateTask 返回 errTaskAlreadyExists，计入 skipped。
//   - 无候选 / 恢复中 → 哨兵错误（handler 映射 400/409）。
func (m *DownloadManager) StartRecovery(p RecoveryStartParams) (*RecoveryStartResult, error) {
	mode, archiveType, err := recoverySchemeToMode(p.Scheme)
	if err != nil {
		return nil, err
	}

	status, err := GetRecoveryStatus(m.db)
	if err != nil {
		return nil, err
	}
	if status.ActiveTasks > 0 {
		return nil, ErrRecoveryInProgress
	}
	if status.CandidateCount == 0 {
		return nil, ErrRecoveryNoCandidates
	}

	var cands []models.RecoveryCandidate
	if err := m.db.Order("created_at ASC, g_id ASC").Find(&cands).Error; err != nil {
		return nil, err
	}

	res := &RecoveryStartResult{Errors: []string{}}
	for _, c := range cands {
		_, err := m.CreateTask(CreateDownloadParams{
			GID:                 c.GID,
			Token:               c.Token,
			Title:               c.Title,
			CoverURL:            c.CoverURL,
			Mode:                mode,
			ArchiveType:         archiveType,
			UpdateForComicID:    c.ComicID,
			ForceDeleteOriginal: true, // 恢复 = 旧记录指向失效文件，完成后必须无条件删旧
			Group:               RecoveryGroup,
			UserID:              p.UserID,
		})
		if err != nil {
			if errors.Is(err, errTaskAlreadyExists) || errors.Is(err, errAlreadyDownloaded) {
				res.Skipped++
			} else {
				res.Failed++
				res.Errors = append(res.Errors, fmt.Sprintf("gid=%s: %v", c.GID, err))
			}
			continue
		}
		res.Created++
	}
	log.Printf("%s [recovery] 启动恢复（%s）：候选 %d，创建 %d，跳过 %d，失败 %d",
		dlLogTag, p.Scheme, len(cands), res.Created, res.Skipped, res.Failed)
	return res, nil
}

// RecoveryCancelParams 取消恢复入参（GIDs 为空 = 全部取消）
type RecoveryCancelParams struct {
	GIDs []string `json:"gids"`
}

// RecoveryCancelResult 取消恢复统计
type RecoveryCancelResult struct {
	Cancelled int      `json:"cancelled"`
	Errors    []string `json:"errors"`
}

// CancelRecovery 取消未终结的恢复任务（单条或全部）。
// 候选清单全部保留（设计 8.3）：取消后可再次点「恢复」按 g_id 幂等重入。
func (m *DownloadManager) CancelRecovery(p RecoveryCancelParams) (*RecoveryCancelResult, error) {
	q := m.db.Where("\"group\" = ? AND status IN ?", RecoveryGroup, recoveryUnfinishedStatuses())
	if len(p.GIDs) > 0 {
		q = q.Where("g_id IN ?", p.GIDs)
	}
	var tasks []models.DownloadTask
	if err := q.Find(&tasks).Error; err != nil {
		return nil, err
	}

	res := &RecoveryCancelResult{Errors: []string{}}
	for _, t := range tasks {
		if _, err := m.CancelTask(t.ID); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("gid=%s: %v", t.GID, err))
			continue
		}
		res.Cancelled++
	}
	log.Printf("%s [recovery] 取消恢复任务：命中 %d，成功 %d，失败 %d",
		dlLogTag, len(tasks), res.Cancelled, len(res.Errors))
	return res, nil
}

// convertCandidatesToFallen 把某扫描路径下的恢复候选转为阵亡条目
// （reason=path-removed），并清空候选行。移除路径时调用（设计第 11 节）。
//
// 候选自包含快照直接搬入阵亡名单；按 dedupe_key upsert 幂等，
// 保留用户已存在的 dismissed 状态。
func convertCandidatesToFallen(db *gorm.DB, scanPathID, sourcePath string) (int, error) {
	if db == nil || strings.TrimSpace(scanPathID) == "" {
		return 0, nil
	}
	var cands []models.RecoveryCandidate
	if err := db.Where("scan_path_id = ?", scanPathID).Find(&cands).Error; err != nil {
		return 0, err
	}
	if len(cands) == 0 {
		return 0, nil
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		for _, c := range cands {
			f := models.FallenComic{
				DedupeKey:  "gid:" + c.GID,
				GID:        c.GID,
				ComicID:    c.ComicID,
				Title:      c.Title,
				TitleJpn:   c.TitleJpn,
				CoverURL:   c.CoverURL,
				Category:   c.Category,
				FileSize:   c.FileSize,
				PageCount:  c.PageCount,
				Reason:     models.FallenReasonPathRemoved,
				SourcePath: sourcePath,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "dedupe_key"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"g_id", "comic_id", "title", "title_jpn", "cover_url", "category",
					"file_size", "page_count", "reason", "source_path", "updated_at",
				}),
			}).Create(&f).Error; err != nil {
				return err
			}
		}
		return tx.Where("scan_path_id = ?", scanPathID).Delete(&models.RecoveryCandidate{}).Error
	})
	if err != nil {
		return 0, err
	}
	return len(cands), nil
}

// ─────────────────────────────────────────────────────────────
// Round54 阶段3：完成/失败钩子 + 阵亡名单查询
// ─────────────────────────────────────────────────────────────

// classifyRecoveryFailure 恢复任务失败分类：
//   - 确定性失败（removed/copyright/404/密码压缩包解压失败）→ 返回阵亡原因，true
//   - 可重试失败（网络/EOF/配额锁定等）→ 返回 "", false（留队列重试，设计 6.4）
//
// 404 判定说明：archiver.php 对已删除/被替换的画廊返回 404
// （错误文本「archiver.php 返回状态码 404」），属于确定性失败；
// 旧 gid/token 因画廊被替换而 404 时同样无法自动找回新 gid，归入阵亡。
func classifyRecoveryFailure(msg string) (string, bool) {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "removed") ||
		strings.Contains(lower, "copyright") ||
		strings.Contains(lower, "not found") ||
		strings.Contains(lower, "http 404") ||
		strings.Contains(msg, "状态码 404") ||
		strings.Contains(msg, "已被删除") ||
		strings.Contains(msg, "不可用"):
		return models.FallenReasonRemoved, true
	case strings.Contains(msg, "解压失败") ||
		strings.Contains(lower, "unlock") ||
		strings.Contains(lower, "password"):
		return models.FallenReasonUnlockFail, true
	}
	return "", false
}

// recordRecoveryFallen 恢复任务确定性失败 → 写入阵亡名单（按 g_id upsert），
// 并从恢复候选移除该条目（避免再次入队）。快照优先取候选表（自包含）。
func (m *DownloadManager) recordRecoveryFallen(task *models.DownloadTask, reason string) error {
	if task == nil || task.GID == "" {
		return errors.New("恢复任务缺少 g_id，无法记入阵亡")
	}

	f := models.FallenComic{
		DedupeKey: "gid:" + task.GID,
		GID:       task.GID,
		Title:     task.Title,
		CoverURL:  task.CoverURL,
		Reason:    reason,
	}
	var cand models.RecoveryCandidate
	if err := m.db.First(&cand, "g_id = ?", task.GID).Error; err == nil {
		f.ComicID = cand.ComicID
		f.Title = cand.Title
		f.TitleJpn = cand.TitleJpn
		f.CoverURL = cand.CoverURL
		f.Category = cand.Category
		f.FileSize = cand.FileSize
		f.PageCount = cand.PageCount
		f.SourcePath = cand.LocalPath
	}

	if err := m.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "dedupe_key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"comic_id", "title", "title_jpn", "cover_url", "category",
			"file_size", "page_count", "reason", "source_path", "updated_at",
		}),
	}).Create(&f).Error; err != nil {
		return err
	}
	return m.db.Where("g_id = ?", task.GID).Delete(&models.RecoveryCandidate{}).Error
}

// cleanupRecoveryArtifacts 恢复完成后清理候选与阵亡条目（按 g_id）
func (m *DownloadManager) cleanupRecoveryArtifacts(gid string) {
	if gid == "" {
		return
	}
	if err := m.db.Where("g_id = ?", gid).Delete(&models.RecoveryCandidate{}).Error; err != nil {
		log.Printf("%s [recovery] 清理候选 gid=%s 失败: %v", dlWarnTag, gid, err)
	}
	if err := m.db.Where("g_id = ?", gid).Delete(&models.FallenComic{}).Error; err != nil {
		log.Printf("%s [recovery] 清理阵亡 gid=%s 失败: %v", dlWarnTag, gid, err)
	}
}

// recoveredRecordAlive 恢复任务完成的防御校验：按 g_id 确认新记录已入库且文件有效。
// 只有确认新记录存在，才允许删除旧记录（防悬空，设计 9.1）。
func (m *DownloadManager) recoveredRecordAlive(gid string) bool {
	if gid == "" {
		return false
	}
	var comics []models.OfflineComic
	if err := m.db.Where("g_id = ?", gid).Find(&comics).Error; err != nil {
		return false
	}
	for i := range comics {
		if pathExistsConservative(comics[i].LocalPath) {
			return true
		}
	}
	return false
}

// FallenListParams 阵亡名单查询参数
type FallenListParams struct {
	Page int `json:"page"`
	Size int `json:"size"`
}

// ListFallenComics 分页读取全局阵亡名单。
// 排序（设计第 12 节）：未 dismissed 按创建时间升序在前；dismissed 按 dismissed_at 升序置底。
func ListFallenComics(db *gorm.DB, p FallenListParams) ([]models.FallenComic, int64, error) {
	if p.Size <= 0 || p.Size > 500 {
		p.Size = 50
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	var total int64
	if err := db.Model(&models.FallenComic{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.FallenComic
	err := db.Order("CASE WHEN dismissed THEN 1 ELSE 0 END ASC, dismissed_at ASC, created_at ASC").
		Offset((p.Page - 1) * p.Size).Limit(p.Size).
		Find(&items).Error
	return items, total, err
}

// SetFallenDismissed 用户手动 x toggle（按条目 id：no-ehentai 条目无 g_id，id 更通用）。
// dismissed=true → 划线 + 置底（保留显示）；false → 撤销划线，回到原位置。
// 注意：撤销时必须写 NULL，GORM 的 map/struct 更新会忽略 nil（实测无效），故用显式 SQL。
func SetFallenDismissed(db *gorm.DB, id uint, dismissed bool) error {
	var f models.FallenComic
	if err := db.First(&f, "id = ?", id).Error; err != nil {
		return err
	}
	if dismissed {
		return db.Model(&models.FallenComic{}).Where("id = ?", id).
			Updates(map[string]interface{}{"dismissed": true, "dismissed_at": time.Now()}).Error
	}
	return db.Exec("UPDATE fallen_comics SET dismissed = 0, dismissed_at = NULL WHERE id = ?", id).Error
}
