package services

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// 下载任务批量操作 + 状态统计（Round47）
//
// 设计口径：
//   - 批量操作只作用于「当前用户可访问 + 指定状态 + 指定模式」的任务；
//   - 状态页签是唯一入口，后端再次校验 action 与 status 的对应关系；
//   - pause/resume/retry/cancel 复用单任务方法，保证与单条操作完全一致的语义
//     （取消仍会清理损坏压缩包、暂停仍会停止引擎并保留续传数据）；
//   - unlock 按用户分组只做一次 GP / 配额校验，再逐任务解锁，避免 20+ 个任务
//     逐个请求 /eh/status 造成长时间阻塞与 E 站限流。
// ─────────────────────────────────────────────────────────────

// ErrInvalidBatchAction 批量操作参数非法（handler 映射为 400）
var ErrInvalidBatchAction = errors.New("无效的批量操作参数")

// BatchActionParams 批量操作查询条件
type BatchActionParams struct {
	Action string // pause | resume | retry | cancel | unlock
	Status string // 页签状态（queued / downloading / paused / error / error_lock / ...）
	Mode   string // all / gallery / archive（空等同 all）
	UserID uint   // >0：仅该用户的任务；0：管理员查看全部
}

// BatchActionResult 批量操作聚合结果
type BatchActionResult struct {
	Affected int      `json:"affected"`
	Failed   int      `json:"failed"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors"`
}

// DownloadStats 各状态任务数量（页签徽标 + 页头总览）
type DownloadStats struct {
	Counts map[string]int64 `json:"counts"`
	Total  int64            `json:"total"`
	Active int64            `json:"active"` // 未结束：排队/下载/暂停/出错/锁定
}

// 每个 action 允许的来源状态。前端只会传页签对应状态，后端仍做白名单校验，
// 避免直接调用接口把合法状态之外的任务批量改写。
var batchActionAllowedStatuses = map[string]map[string]bool{
	"pause":  {"queued": true, "downloading": true},
	"resume": {"paused": true},
	"retry":  {"error": true},
	"cancel": {"queued": true, "downloading": true, "paused": true, "error": true, "error_lock": true},
	"unlock": {"error_lock": true},
}

// BatchAction 执行批量操作。
func (m *DownloadManager) BatchAction(p BatchActionParams) (*BatchActionResult, error) {
	action := strings.ToLower(strings.TrimSpace(p.Action))
	status := strings.ToLower(strings.TrimSpace(p.Status))
	allowed, ok := batchActionAllowedStatuses[action]
	if !ok {
		return nil, fmt.Errorf("%w: 不支持的操作 %q", ErrInvalidBatchAction, p.Action)
	}
	if !allowed[status] {
		return nil, fmt.Errorf("%w: %s 不支持对 %s 状态批量操作", ErrInvalidBatchAction, action, status)
	}

	mode := strings.ToLower(strings.TrimSpace(p.Mode))
	if mode == "" {
		mode = "all"
	}
	if mode != "all" && mode != "gallery" && mode != "archive" {
		return nil, fmt.Errorf("%w: 无效的下载模式 %q", ErrInvalidBatchAction, p.Mode)
	}

	q := m.db.Model(&models.DownloadTask{}).Where("status = ?", status)
	if p.UserID > 0 {
		q = q.Where("user_id = ?", p.UserID)
	}
	if mode != "all" {
		q = q.Where("mode = ?", mode)
	}
	var tasks []models.DownloadTask
	if err := q.Order("created_at asc").Find(&tasks).Error; err != nil {
		return nil, err
	}

	result := &BatchActionResult{Errors: []string{}}
	if action == "unlock" {
		return m.batchUnlock(tasks, result)
	}

	for i := range tasks {
		task := &tasks[i]
		var err error
		switch action {
		case "pause":
			_, err = m.PauseTask(task.ID)
		case "resume":
			_, err = m.ResumeTask(task.ID)
		case "retry":
			_, err = m.RetryTask(task.ID)
		case "cancel":
			_, err = m.CancelTask(task.ID)
		}
		if err != nil {
			if isBatchSkipError(err) {
				result.Skipped++
				continue
			}
			result.Failed++
			appendBatchError(result, task, err)
			continue
		}
		result.Affected++
	}

	log.Printf("%s 批量操作 %s（status=%s mode=%s）：处理 %d / 失败 %d / 跳过 %d",
		dlLogTag, action, status, mode, result.Affected, result.Failed, result.Skipped)
	return result, nil
}

// batchUnlock 按任务发起者分组做一次配额校验，再逐个执行解锁。
// 普通用户的锁定任务通常同属一个 E 站账户；管理员批量操作多用户任务时按组处理。
func (m *DownloadManager) batchUnlock(tasks []models.DownloadTask, result *BatchActionResult) (*BatchActionResult, error) {
	if len(tasks) == 0 {
		return result, nil
	}

	groups := make(map[uint][]models.DownloadTask)
	order := make([]uint, 0, 2)
	for _, t := range tasks {
		if _, ok := groups[t.UserID]; !ok {
			order = append(order, t.UserID)
		}
		groups[t.UserID] = append(groups[t.UserID], t)
	}

	for _, userID := range order {
		group := groups[userID]
		if m.ehService == nil {
			result.Failed += len(group)
			appendBatchGroupError(result, len(group), "解锁服务不可用（E 站服务未初始化）")
			continue
		}
		account := loadUserAccount(m.db, userID)
		if account.IPBMemberID == "" {
			result.Failed += len(group)
			appendBatchGroupError(result, len(group), "未绑定 E 站账号凭证，无法校验配额")
			continue
		}
		ehSetting := loadEHSetting(m.db, userID)
		status, err := m.ehService.FetchEHUserStatus(account, ehSetting)
		if err != nil {
			result.Failed += len(group)
			appendBatchGroupError(result, len(group), "校验配额失败: "+err.Error())
			log.Printf("%s 批量解锁用户 %d 校验配额失败: %v", dlWarnTag, userID, err)
			continue
		}
		log.Printf("%s 批量解锁用户 %d：GP=%s Credits=%s Hath=%s 配额 %d/%d",
			dlLogTag, userID, status.AssetGP, status.AssetCredits, status.AssetHath, status.CurrentQuota, status.MaxQuota)

		for i := range group {
			task := &group[i]
			if err := m.unlockTaskAfterQuotaCheck(task); err != nil {
				if isBatchSkipError(err) {
					result.Skipped++
					continue
				}
				result.Failed++
				appendBatchError(result, task, err)
				continue
			}
			result.Affected++
		}
	}

	log.Printf("%s 批量解锁：处理 %d / 失败 %d / 跳过 %d",
		dlLogTag, result.Affected, result.Failed, result.Skipped)
	return result, nil
}

// TaskStats 统计各状态任务数量（mode 为空 / all 表示不过滤模式；userID=0 表示管理员全部）。
func (m *DownloadManager) TaskStats(mode string, userID uint) (*DownloadStats, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	q := m.db.Model(&models.DownloadTask{})
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	if mode != "" && mode != "all" {
		q = q.Where("mode = ?", mode)
	}

	type statusRow struct {
		Status string
		Cnt    int64
	}
	var rows []statusRow
	if err := q.Select("status, COUNT(*) AS cnt").Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}

	counts := map[string]int64{
		string(models.DownloadQueued):      0,
		string(models.DownloadDownloading): 0,
		string(models.DownloadPaused):      0,
		string(models.DownloadError):       0,
		string(models.DownloadErrorLock):   0,
		string(models.DownloadCompleted):   0,
		string(models.DownloadCancelled):   0,
	}
	var total int64
	for _, row := range rows {
		if _, ok := counts[row.Status]; ok {
			counts[row.Status] = row.Cnt
			total += row.Cnt
		}
	}

	stats := &DownloadStats{Counts: counts, Total: total}
	stats.Active = counts[string(models.DownloadQueued)] +
		counts[string(models.DownloadDownloading)] +
		counts[string(models.DownloadPaused)] +
		counts[string(models.DownloadError)] +
		counts[string(models.DownloadErrorLock)]
	return stats, nil
}

// isBatchSkipError 任务在批量执行期间状态已变化 / 被删除：不计失败，计入 skipped。
func isBatchSkipError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "不允许") || strings.Contains(msg, "任务不存在")
}

func appendBatchError(result *BatchActionResult, task *models.DownloadTask, err error) {
	const maxErrors = 10
	if len(result.Errors) >= maxErrors {
		return
	}
	name := task.Title
	if name == "" {
		name = task.ID
	}
	result.Errors = append(result.Errors, fmt.Sprintf("%s（%s）：%v", name, task.ID, err))
}

func appendBatchGroupError(result *BatchActionResult, count int, msg string) {
	const maxErrors = 10
	if len(result.Errors) >= maxErrors {
		return
	}
	result.Errors = append(result.Errors, fmt.Sprintf("%d 个任务：%s", count, msg))
}
