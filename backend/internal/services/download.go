package services

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"SakuHentai/internal/models"

	"github.com/PuerkitoBio/goquery"
	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// 下载日志统一前缀（便于验收时 grep 定位）
//   [DOWNLOAD]        常规流程
//   [DOWNLOAD-WARN]   警告（不影响主流程）
//   [DOWNLOAD-ERROR]  错误（任务失败/异常）
//   [ARCHIVER]        archiver.php 相关
// ─────────────────────────────────────────────────────────────
const (
	dlLogTag  = "[DOWNLOAD]"
	dlWarnTag = "[DOWNLOAD-WARN]"
	dlErrTag  = "[DOWNLOAD-ERROR]"
	dlArcTag  = "[ARCHIVER]"
)

// CreateDownloadParams 创建下载任务的入参（前端 POST /api/v1/downloads）
type CreateDownloadParams struct {
	GID              string              `json:"gid"`
	Token            string              `json:"token"`
	Title            string              `json:"title"`
	CoverURL         string              `json:"coverUrl"`
	Mode             models.DownloadMode `json:"mode"`
	ArchiveType      models.ArchiveType  `json:"archiveType"`
	Priority         int                 `json:"priority"`
	Group            string              `json:"group"`
	UpdateForComicID string              `json:"updateForComicId,omitempty"` // 离线更新下载：被更新漫画 ID
	UserID           uint                `json:"userId"`                      // 任务发起者（决定执行时使用谁的 E 站凭证）
}

// DownloadListParams 下载任务列表查询参数
type DownloadListParams struct {
	Status string `form:"status"` // "" 全部 | active 活动 | queued/downloading/paused/completed/error/error_lock/cancelled
	Mode   string `form:"mode"`   // gallery | archive
	Page   int    `form:"page"`
	Size   int    `form:"size"`
}

// DownloadManager 下载任务管理器：
//   - SQLite 持久化任务记录 + 内存 worker 队列（后端重启可恢复）
//   - 负责任务生命周期：queued → downloading → completed / error / error_lock / paused / cancelled
//   - 引擎分派：gallery / archive 由第 2/3 步实现，当前为占位并在日志中明示
type DownloadManager struct {
	db        *gorm.DB
	ehService *EHService

	queue   chan string // 待执行任务 ID 队列
	workers int
	quit    chan struct{}
	mu      sync.Mutex

	// 调度门控：控制「同一优先级并发」与「归档并发上限」
	// 真实并发由调度器按下载设置决定，worker 池数量仅作上限兜底
	schedMu        sync.Mutex
	runningTotal   int         // 当前运行任务总数
	runningArchive int         // 当前运行的归档任务数
	runningByPri   map[int]int // 各优先级正在运行的任务数
}

// NewDownloadManager 构造下载任务管理器
func NewDownloadManager(db *gorm.DB, ehService *EHService) *DownloadManager {
	return &DownloadManager{
		db:           db,
		ehService:    ehService,
		queue:        make(chan string, 256),
		workers:      16, // worker 池仅作并发上限，实际并发由调度门控（downloadAllGalleriesSamePriority / archiveThreads）控制
		quit:         make(chan struct{}),
		runningByPri: make(map[int]int),
	}
}

// Start 启动 worker 池，并按设置恢复未完成任务
func (m *DownloadManager) Start() {
	for i := 0; i < m.workers; i++ {
		go m.worker(i)
	}

	setting := m.GetSettings()
	if setting.AutoResumeTasks {
		if n, err := m.RestoreTasks(); err == nil {
			log.Printf("%s 启动时自动恢复未完成任务 %d 个", dlLogTag, n)
		} else {
			log.Printf("%s 启动时恢复未完成任务失败: %v", dlErrTag, err)
		}
	} else {
		log.Printf("%s 自动恢复任务未开启（autoResumeTasks=false），启动时不恢复历史任务", dlLogTag)
	}

	log.Printf("%s 下载任务管理器已启动（worker=%d，队列容量=256）", dlLogTag, m.workers)
}

// worker 从队列取出任务并执行
func (m *DownloadManager) worker(id int) {
	log.Printf("%s worker#%d 已就绪", dlLogTag, id)
	for {
		select {
		case <-m.quit:
			log.Printf("%s worker#%d 退出", dlLogTag, id)
			return
		case taskID := <-m.queue:
			m.runTask(taskID)
		}
	}
}

// runTask 执行单个任务（状态机入口）
func (m *DownloadManager) runTask(taskID string) {
	var task models.DownloadTask
	if err := m.db.First(&task, "id = ?", taskID).Error; err != nil {
		log.Printf("%s [worker] 任务 %s 不存在或已被删除: %v", dlErrTag, taskID, err)
		return
	}

	if task.Status != models.DownloadQueued {
		log.Printf("%s [worker] 任务 %s 当前状态为 %s（非 queued），跳过执行", dlWarnTag, taskID, task.Status)
		return
	}

	log.Printf("%s [worker] 开始处理任务 id=%s gid=%s title=%q mode=%s",
		dlLogTag, task.ID, task.GID, task.Title, task.Mode)

	// 调度门控：按设置等待可执行槽位（优先级并发 / 归档并发上限）
	m.acquireSlot(&task)
	defer m.releaseSlot(&task)

	// 等待槽位期间任务可能被用户暂停/取消，重新校验状态避免覆盖用户操作
	var latest models.DownloadTask
	if err := m.db.First(&latest, "id = ?", taskID).Error; err != nil {
		log.Printf("%s [worker] 任务 %s 等待槽位期间不存在或已被删除: %v", dlErrTag, taskID, err)
		return
	}
	if latest.Status != models.DownloadQueued {
		log.Printf("%s [worker] 任务 %s 等待槽位期间状态变为 %s，放弃执行", dlWarnTag, taskID, latest.Status)
		return
	}
	task = latest

	if err := m.markRunning(&task); err != nil {
		return
	}

	m.dispatchEngine(&task)

	// 离线更新任务收尾：下载成功后按设置清理旧版
	m.finalizeUpdate(&task)

	// 无 H@H 自动降级：归档更新任务失败（非配额锁定）时按设置改用画廊引擎重试
	m.fallbackUpdateToGallery(&task)
}

// finalizeUpdate 离线更新任务完成后处理旧版漫画（按 AutoUpdateDeleteOriginal 设置）
func (m *DownloadManager) finalizeUpdate(task *models.DownloadTask) {
	if task == nil || task.UpdateForComicID == "" {
		return
	}
	// 只处理成功完成的任务
	if task.Status != models.DownloadCompleted {
		log.Printf("%s [update] 任务 %s 更新漫画 %s 未完成（状态=%s），跳过旧版清理",
			dlWarnTag, task.ID, task.UpdateForComicID, task.Status)
		return
	}

	var old models.OfflineComic
	if err := m.db.First(&old, "id = ?", task.UpdateForComicID).Error; err != nil {
		log.Printf("%s [update] 任务 %s 未找到被更新漫画 %s（可能已删除）: %v", dlWarnTag, task.ID, task.UpdateForComicID, err)
		return
	}

	setting := m.GetSettings()
	if setting.AutoUpdateDeleteOriginal && old.LocalPath != "" {
		if err := os.RemoveAll(old.LocalPath); err != nil {
			log.Printf("%s [update] 任务 %s 删除旧版文件夹失败 %q: %v", dlErrTag, task.ID, old.LocalPath, err)
		} else {
			log.Printf("%s [update] 任务 %s 已删除旧版文件夹 %q（autoUpdateDeleteOriginal=true）", dlLogTag, task.ID, old.LocalPath)
		}
		if err := m.db.Delete(&old).Error; err != nil {
			log.Printf("%s [update] 任务 %s 删除旧版记录失败: %v", dlErrTag, task.ID, err)
		}
		return
	}

	// 不删除文件：仅标记已更新，避免误删
	old.NeedsUpdate = false
	old.NewGID = ""
	old.NewToken = ""
	old.UpdateNote = ""
	if err := m.db.Save(&old).Error; err != nil {
		log.Printf("%s [update] 任务 %s 更新漫画标记失败: %v", dlErrTag, task.ID, err)
	}
	log.Printf("%s [update] 任务 %s 更新完成（保留旧版，仅清除更新标记）", dlLogTag, task.ID)
}

// fallbackUpdateToGallery 自动更新任务归档下载失败时按设置降级为画廊下载（无 H@H 场景）
//
// 仅在普通错误（error）下触发；配额/限流锁定（error_lock）不降级，避免在配额不足时继续消耗资源。
func (m *DownloadManager) fallbackUpdateToGallery(task *models.DownloadTask) {
	if task == nil || task.UpdateForComicID == "" {
		return
	}
	if task.Status != models.DownloadError {
		return // 成功 / 锁定 / 暂停 / 取消 均不降级
	}
	if task.Mode != models.DownloadModeArchive {
		return // 只有归档任务需要降级；画廊失败不再二次降级（避免循环）
	}
	if !m.GetSettings().AutoUpdateFallbackToGallery {
		log.Printf("%s [update] 任务 %s 归档下载失败，但 autoUpdateFallbackToGallery=false，不降级为画廊",
			dlWarnTag, task.ID)
		return
	}
	m.fallbackArchiveToGallery(task, "归档下载失败，autoUpdateFallbackToGallery=true")
}

// fallbackArchiveToGallery 将归档下载任务降级为画廊逐图下载并重新入队。
// 触发场景：
//   - 仅支持 H@H Downloader 的画廊（无法直接 HTTP 下载 zip，见 archiveDownloader.run）
//   - 自动更新任务归档下载失败且 autoUpdateFallbackToGallery=true（见 fallbackUpdateToGallery）
func (m *DownloadManager) fallbackArchiveToGallery(task *models.DownloadTask, reason string) {
	if task == nil || task.Mode != models.DownloadModeArchive {
		return
	}

	log.Printf("%s [update] 任务 %s 归档下载不可用，降级为画廊逐图下载（gid=%s）：%s",
		dlWarnTag, task.ID, task.GID, reason)

	task.Mode = models.DownloadModeGallery
	task.ArchiveType = ""
	task.Status = models.DownloadQueued
	task.Error = "" // 画廊引擎重新执行时会重写错误
	task.DoneFiles = 0
	task.DoneBytes = 0
	task.Speed = 0
	task.UpdatedAt = time.Now()
	if err := m.db.Save(task).Error; err != nil {
		log.Printf("%s [update] 任务 %s 降级保存失败: %v", dlErrTag, task.ID, err)
		return
	}
	log.Printf("%s [update] 任务 %s 已降级为画廊下载并重新入队", dlLogTag, task.ID)
	m.Enqueue(task.ID)
}

// markRunning 将任务置为下载中
func (m *DownloadManager) markRunning(task *models.DownloadTask) error {
	task.Status = models.DownloadDownloading
	task.Error = ""
	task.Speed = 0
	task.UpdatedAt = time.Now()
	if err := m.db.Save(task).Error; err != nil {
		log.Printf("%s 任务 %s 更新为 downloading 失败: %v", dlErrTag, task.ID, err)
		return err
	}
	return nil
}

// dispatchEngine 按模式分派到具体下载引擎
func (m *DownloadManager) dispatchEngine(task *models.DownloadTask) {
	switch task.Mode {
	case models.DownloadModeGallery:
		log.Printf("%s [gallery] 任务 %s 进入画廊下载引擎", dlLogTag, task.ID)
		m.runGalleryEngine(task)
	case models.DownloadModeArchive:
		log.Printf("%s [archive] 任务 %s 进入归档下载引擎", dlLogTag, task.ID)
		m.runArchiveEngine(task)
	default:
		m.failTask(task, "未知下载模式: "+string(task.Mode))
	}
}

// failTask 将任务置为错误状态并记录原因
func (m *DownloadManager) failTask(task *models.DownloadTask, msg string) {
	log.Printf("%s 任务 %s 失败: %s", dlErrTag, task.ID, msg)
	task.Status = models.DownloadError
	task.Error = msg
	task.UpdatedAt = time.Now()
	m.db.Save(task)
}

// ─────────────────────────────────────────────────────────────
// 任务 CRUD
// ─────────────────────────────────────────────────────────────

// CreateTask 创建下载任务并入队
func (m *DownloadManager) CreateTask(p CreateDownloadParams) (*models.DownloadTask, error) {
	if p.GID == "" || p.Token == "" {
		return nil, errors.New("gid 与 token 不能为空")
	}
	if p.Mode != models.DownloadModeGallery && p.Mode != models.DownloadModeArchive {
		return nil, fmt.Errorf("无效的下载模式: %s（仅支持 gallery / archive）", p.Mode)
	}
	if p.Mode == models.DownloadModeArchive {
		if p.ArchiveType != models.ArchiveTypeOriginal && p.ArchiveType != models.ArchiveTypeResample {
			p.ArchiveType = models.ArchiveTypeOriginal
		}
	} else {
		p.ArchiveType = ""
	}

	// gid 去重：同一 gid 存在未完成任务时拒绝重复创建
	var existing models.DownloadTask
	err := m.db.Where("g_id = ? AND status IN ?", p.GID, []string{
		string(models.DownloadQueued), string(models.DownloadDownloading),
		string(models.DownloadPaused), string(models.DownloadError), string(models.DownloadErrorLock),
	}).First(&existing).Error
	if err == nil {
		return nil, fmt.Errorf("该画廊（gid=%s）已有进行中的下载任务 %s，请勿重复创建", p.GID, existing.ID)
	}

	setting := m.GetSettings()
	task := &models.DownloadTask{
		ID:               newTaskID(),
		GID:              p.GID,
		Token:            p.Token,
		Title:            p.Title,
		CoverURL:         p.CoverURL,
		Mode:             p.Mode,
		ArchiveType:      p.ArchiveType,
		Status:           models.DownloadQueued,
		Priority:         p.Priority,
		Group:            p.Group,
		ArchivePath:      setting.ArchivePath,
		ExtractPath:      setting.ExtractPath,
		UpdateForComicID: p.UpdateForComicID,
		UserID:           p.UserID, // 任务发起者（执行时加载其 E 站凭证）
	}

	if err := m.db.Create(task).Error; err != nil {
		log.Printf("%s 创建任务失败: %v", dlErrTag, err)
		return nil, err
	}

	log.Printf("%s 创建任务 id=%s gid=%s title=%q mode=%s archiveType=%s group=%q archivePath=%q extractPath=%q",
		dlLogTag, task.ID, task.GID, task.Title, task.Mode, task.ArchiveType, task.Group, task.ArchivePath, task.ExtractPath)

	m.Enqueue(task.ID)
	return task, nil
}

// Enqueue 将任务 ID 放入队列
func (m *DownloadManager) Enqueue(taskID string) {
	select {
	case m.queue <- taskID:
		log.Printf("%s 任务 %s 已入队", dlLogTag, taskID)
	default:
		log.Printf("%s 任务 %s 入队失败（队列已满），任务保持 queued 等待后续处理", dlWarnTag, taskID)
	}
}

// PauseTask 暂停任务（queued / downloading / error_lock → paused）
func (m *DownloadManager) PauseTask(taskID string) (*models.DownloadTask, error) {
	var task models.DownloadTask
	if err := m.db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, errors.New("任务不存在")
	}
	switch task.Status {
	case models.DownloadQueued, models.DownloadDownloading, models.DownloadErrorLock:
	default:
		return nil, fmt.Errorf("当前状态 %s 不允许暂停", task.Status)
	}
	task.Status = models.DownloadPaused
	task.UpdatedAt = time.Now()
	if err := m.db.Save(&task).Error; err != nil {
		return nil, err
	}
	log.Printf("%s 暂停任务 %s（gid=%s）", dlLogTag, taskID, task.GID)
	return &task, nil
}

// ResumeTask 恢复任务（paused → queued）
func (m *DownloadManager) ResumeTask(taskID string) (*models.DownloadTask, error) {
	var task models.DownloadTask
	if err := m.db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, errors.New("任务不存在")
	}
	if task.Status != models.DownloadPaused {
		return nil, fmt.Errorf("当前状态 %s 不允许恢复（仅 paused 可恢复）", task.Status)
	}
	task.Status = models.DownloadQueued
	task.Error = ""
	task.UpdatedAt = time.Now()
	if err := m.db.Save(&task).Error; err != nil {
		return nil, err
	}
	m.Enqueue(taskID)
	log.Printf("%s 恢复任务 %s（gid=%s）", dlLogTag, taskID, task.GID)
	return &task, nil
}

// errTaskStopped 引擎检测到任务被取消/暂停时的中止信号
var errTaskStopped = errors.New("任务已取消或暂停，中止下载")

// taskStopped 从数据库重新读取任务状态，判断是否已被取消/暂停。
// 引擎内存中的 g.task.Status 在取消/暂停时不会更新，若以内存状态写库会覆盖用户操作，因此必须回查 DB。
func (m *DownloadManager) taskStopped(taskID string) bool {
	var st string
	if err := m.db.Model(&models.DownloadTask{}).
		Select("status").Where("id = ?", taskID).Scan(&st).Error; err != nil {
		return false
	}
	return st == string(models.DownloadCancelled) || st == string(models.DownloadPaused)
}

// CancelTask 取消任务（任意非终态 → cancelled）
func (m *DownloadManager) CancelTask(taskID string) (*models.DownloadTask, error) {
	var task models.DownloadTask
	if err := m.db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, errors.New("任务不存在")
	}
	if task.Status == models.DownloadCompleted || task.Status == models.DownloadCancelled {
		return nil, fmt.Errorf("当前状态 %s 不允许取消", task.Status)
	}
	task.Status = models.DownloadCancelled
	task.UpdatedAt = time.Now()
	if err := m.db.Save(&task).Error; err != nil {
		return nil, err
	}
	log.Printf("%s 取消任务 %s（gid=%s）", dlLogTag, taskID, task.GID)
	return &task, nil
}

// RetryTask 重试任务（error / error_lock / cancelled → queued）
func (m *DownloadManager) RetryTask(taskID string) (*models.DownloadTask, error) {
	var task models.DownloadTask
	if err := m.db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, errors.New("任务不存在")
	}
	switch task.Status {
	case models.DownloadError, models.DownloadErrorLock, models.DownloadCancelled:
	default:
		return nil, fmt.Errorf("当前状态 %s 不允许重试（仅 error / error_lock / cancelled 可重试）", task.Status)
	}
	task.Status = models.DownloadQueued
	task.Error = ""
	task.UpdatedAt = time.Now()
	if err := m.db.Save(&task).Error; err != nil {
		return nil, err
	}
	m.Enqueue(taskID)
	log.Printf("%s 重试任务 %s（gid=%s）", dlLogTag, taskID, task.GID)
	return &task, nil
}

// UnlockTask GP 解锁（error_lock → 校验余额 → queued）
// 用户需先在 E 站用 Credits 兑换 GP 或等待配额恢复，本端仅重新拉取 /eh/status 校验后重试。
func (m *DownloadManager) UnlockTask(taskID string) (*models.DownloadTask, error) {
	var task models.DownloadTask
	if err := m.db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, errors.New("任务不存在")
	}
	if task.Status != models.DownloadErrorLock {
		return nil, fmt.Errorf("当前状态 %s 不允许解锁（仅 error_lock 可解锁）", task.Status)
	}

	account := loadUserAccount(m.db, task.UserID)
	if account.IPBMemberID == "" {
		return nil, errors.New("未绑定 E 站账号凭证，无法校验配额")
	}
	ehSetting := loadEHSetting(m.db, task.UserID)

	status, err := m.ehService.FetchEHUserStatus(account, ehSetting)
	if err != nil {
		log.Printf("%s 解锁任务 %s 校验配额失败: %v", dlErrTag, taskID, err)
		return nil, errors.New("校验配额失败: " + err.Error())
	}
	log.Printf("%s 解锁任务 %s：GP=%s Credits=%s Hath=%s 配额 %d/%d",
		dlLogTag, taskID, status.AssetGP, status.AssetCredits, status.AssetHath, status.CurrentQuota, status.MaxQuota)

	task.Status = models.DownloadQueued
	task.Error = ""
	task.UpdatedAt = time.Now()
	if err := m.db.Save(&task).Error; err != nil {
		return nil, err
	}
	m.Enqueue(taskID)
	return &task, nil
}

// ListTasks 查询任务列表（支持状态/模式过滤 + 分页）
func (m *DownloadManager) ListTasks(p DownloadListParams) ([]models.DownloadTask, int64, error) {
	q := m.db.Model(&models.DownloadTask{})

	if p.Status != "" {
		if p.Status == "active" {
			q = q.Where("status IN ?", []string{
				string(models.DownloadQueued), string(models.DownloadDownloading),
				string(models.DownloadPaused), string(models.DownloadError), string(models.DownloadErrorLock),
			})
		} else {
			q = q.Where("status = ?", p.Status)
		}
	}
	if p.Mode != "" && p.Mode != "all" {
		q = q.Where("mode = ?", p.Mode)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if p.Size <= 0 {
		p.Size = 50
	}
	if p.Size > 200 {
		p.Size = 200
	}
	if p.Page <= 0 {
		p.Page = 1
	}

	var tasks []models.DownloadTask
	if err := q.Order("created_at desc").Offset((p.Page - 1) * p.Size).Limit(p.Size).Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

// GetTask 查询单个任务
func (m *DownloadManager) GetTask(taskID string) (*models.DownloadTask, error) {
	var task models.DownloadTask
	if err := m.db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, errors.New("任务不存在")
	}
	return &task, nil
}

// RestoreTasks 扫描未完成任务重新入队（queued / downloading 恢复；paused 保持暂停）
func (m *DownloadManager) RestoreTasks() (int, error) {
	var tasks []models.DownloadTask
	if err := m.db.Where("status IN ?", []string{
		string(models.DownloadQueued), string(models.DownloadDownloading),
	}).Find(&tasks).Error; err != nil {
		return 0, err
	}
	count := 0
	for i := range tasks {
		tasks[i].Status = models.DownloadQueued
		tasks[i].Error = ""
		tasks[i].UpdatedAt = time.Now()
		if err := m.db.Save(&tasks[i]).Error; err != nil {
			log.Printf("%s 恢复任务 %s 失败: %v", dlErrTag, tasks[i].ID, err)
			continue
		}
		m.Enqueue(tasks[i].ID)
		count++
	}
	log.Printf("%s 恢复历史任务 %d 个", dlLogTag, count)
	return count, nil
}

// ─────────────────────────────────────────────────────────────
// 下载设置（单例 ID=1）
// ─────────────────────────────────────────────────────────────

// defaultDownloadSetting 默认下载设置（与前端 downloadSettings 默认值保持一致）
func defaultDownloadSetting() models.DownloadSetting {
	return models.DownloadSetting{
		ArchivePath:                   `G:\EhentaiWebProject\Download_ZIP`,
		ExtractPath:                   `G:\EhentaiWebProject\Gallery`,
		SingleImageSavePath:           `G:\EhentaiWebProject\Gallery`,
		DefaultDownloadOriginal:       true,
		ConcurrentImageDownloads:      10,
		SpeedLimitImages:              99,
		SpeedLimitInterval:            "1s",
		DownloadAllGalleriesSamePriority: true,
		ArchiveThreads:                10,
		ControlArchiveConcurrency:     true,
		DeleteZipAfterArchiveDownload: true,
		AutoResumeTasks:               true,
	}
}

// GetSettings 获取下载设置（不存在则创建默认值）
func (m *DownloadManager) GetSettings() *models.DownloadSetting {
	var setting models.DownloadSetting
	if err := m.db.First(&setting, 1).Error; err != nil {
		setting = defaultDownloadSetting()
		setting.ID = 1
		setting.UpdatedAt = time.Now()
		if err := m.db.Create(&setting).Error; err != nil {
			log.Printf("%s 初始化下载设置失败: %v", dlErrTag, err)
			return &setting
		}
		log.Printf("%s 已创建默认下载设置（archivePath=%s extractPath=%s）", dlLogTag, setting.ArchivePath, setting.ExtractPath)
		return &setting
	}

	// 旧数据迁移：早期版本无 archiveThreads / controlArchiveConcurrency / downloadAllGalleriesSamePriority 字段，
	// AutoMigrate 补列后旧记录这些字段为 0/false。UI 不提供 archiveThreads=0 选项，
	// 故据此用默认值补齐一次并保存，避免新调度语义（并行下载 / 归档并发限制）失效。
	if setting.ArchiveThreads == 0 {
		def := defaultDownloadSetting()
		setting.ArchiveThreads = def.ArchiveThreads
		setting.ControlArchiveConcurrency = def.ControlArchiveConcurrency
		setting.DownloadAllGalleriesSamePriority = def.DownloadAllGalleriesSamePriority
		setting.UpdatedAt = time.Now()
		if err := m.db.Save(&setting).Error; err != nil {
			log.Printf("%s 迁移下载设置默认值失败: %v", dlErrTag, err)
		} else {
			log.Printf("%s 已迁移旧下载设置：补齐新字段默认值（archiveThreads=%d 控制归档并发=%v 同优先级并行=%v）",
				dlLogTag, setting.ArchiveThreads, setting.ControlArchiveConcurrency, setting.DownloadAllGalleriesSamePriority)
		}
	}
	return &setting
}

// SaveSettings 保存下载设置
func (m *DownloadManager) SaveSettings(s *models.DownloadSetting) (*models.DownloadSetting, error) {
	s.ID = 1
	s.UpdatedAt = time.Now()
	if err := m.db.Save(s).Error; err != nil {
		log.Printf("%s 保存下载设置失败: %v", dlErrTag, err)
		return nil, err
	}
	log.Printf("%s 保存下载设置: archivePath=%q extractPath=%q 并发图片=%d 速度=%d/%s 删除压缩包=%v 自动恢复=%v 自动更新画廊=%v",
		dlLogTag, s.ArchivePath, s.ExtractPath, s.ConcurrentImageDownloads, s.SpeedLimitImages, s.SpeedLimitInterval,
		s.DeleteZipAfterArchiveDownload, s.AutoResumeTasks, s.AutoUpdateGallery)
	return s, nil
}

// ─────────────────────────────────────────────────────────────
// GP 面板：账户资产 + archiver.php 真实报价
// ─────────────────────────────────────────────────────────────

// GetGPInfo 组合账户余额与 archiver.php 报价（GP 面板数据源）
func (m *DownloadManager) GetGPInfo(account *models.AccountSetting, ehSetting *models.EHSetting, gid, token string) (*models.DownloadGPInfo, error) {
	info := &models.DownloadGPInfo{}

	status, err := m.ehService.FetchEHUserStatus(account, ehSetting)
	if err != nil {
		log.Printf("%s 获取账户资产失败（GP 面板降级为仅返回 archiver 报价）: %v", dlWarnTag, err)
	} else {
		info.GP = status.AssetGP
		info.Credits = status.AssetCredits
		info.Hath = status.AssetHath
		info.QuotaUsed = status.CurrentQuota
		info.QuotaMax = status.MaxQuota
		log.Printf("%s GP 面板余额：GP=%s Credits=%s Hath=%s 配额 %d/%d", dlLogTag, info.GP, info.Credits, info.Hath, info.QuotaUsed, info.QuotaMax)
	}

	if gid != "" && token != "" {
		arc, err := m.QueryArchiveInfo(account, ehSetting, gid, token)
		if err != nil {
			log.Printf("%s 获取 archiver 报价失败（GP 面板不展示归档成本）: %v", dlWarnTag, err)
		} else {
			info.Archive = arc
		}
	}
	return info, nil
}

// QueryArchiveInfo 抓取 archiver.php 并解析原图/压缩图两个方案的 Download Cost 与 Estimated Size。
//
// 页面样例（用户提供）：
//
//	Download Cost:   Free!
//	Estimated Size:   18.56 MiB
//	Download Cost:   Free!
//	Estimated Size:   1.72 MiB
//
// 解析策略：按页面文本顺序扫描 "Download Cost: X" 与其后续 "Estimated Size: Y"，
// 第一组为原图、第二组为压缩图；无 H@H 时 Cost 为实际 GP/Credits 数值。
func (m *DownloadManager) QueryArchiveInfo(account *models.AccountSetting, ehSetting *models.EHSetting, gid, token string) (*models.ArchiveInfo, error) {
	client, err := m.ehService.BuildClient(account)
	if err != nil {
		return nil, err
	}

	archiverURL := "https://e-hentai.org/archiver.php?gid=" + gid + "&token=" + token
	req, err := http.NewRequest("GET", archiverURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://e-hentai.org/")
	req.AddCookie(&http.Cookie{Name: "inline_set", Value: "ts_l"})

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 archiver.php 失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("archiver.php 返回状态码 %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析 archiver.php HTML 失败: %v", err)
	}

	html, _ := doc.Html()
	info := &models.ArchiveInfo{GID: gid, Token: token, Options: extractArchiveOptions(html)}

	if len(info.Options) == 0 {
		// 解析失败：可能归档已创建/进行中/页面结构变化 → 输出页面片段便于 debug
		plain := doc.Find("body").Text()
		plain = strings.Join(strings.Fields(plain), " ")
		if len(plain) > 300 {
			plain = plain[:300]
		}
		log.Printf("%s 未能解析到归档方案（gid=%s）：页面片段=%q", dlWarnTag, gid, plain)
	} else {
		for _, opt := range info.Options {
			log.Printf("%s 解析到归档方案 gid=%s %s: cost=%q size=%q", dlArcTag, gid, opt.Label, opt.Cost, opt.Size)
		}
	}
	return info, nil
}

// extractArchiveOptions 从 archiver.php 页面文本中提取归档方案列表
// 修复：真实 HTML 中「Download Cost:」与值之间夹着标签（如 <strong>Free!</strong>），
// 旧正则 [^<\r\n]+ 遇 < 即停止导致 cost/size 为空；改为去标签 + 还原实体 + 关键字边界截取。
func extractArchiveOptions(html string) []models.ArchiveDownloadOption {
	// ① 去除 HTML 标签并还原常见实体，将空白压缩为单个空格
	text := stripHTMLTags(html)
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&", "&")
	text = strings.Join(strings.Fields(text), " ")

	// ② 按关键字边界截取「Download Cost」「Estimated Size」的实际值
	//（Go 正则不支持 lookahead，故用手动截取；值截止到下一个相邻关键字）
	const costKeyword = "Download Cost"
	var costs, sizes []string
	for start := 0; ; {
		idx := findKeywordIndex(text, start, costKeyword)
		if idx < 0 {
			break
		}
		// cost 值截止到紧随其后的 Estimated Size；size 值截止到下一段关键字或结尾
		//（已解锁横幅 "You unlocked ... cancel_sessions() {...}" 也作为截止点，防止其文本混入报价值）
		if cost := extractArchiveValue(text, idx, "Estimated Size", "Download Cost", "Archive Download", "H@H Downloader", "Current Funds", "You unlocked"); cost != "" {
			costs = append(costs, cost)
		}
		if sizeIdx := findKeywordIndex(text, idx+len(costKeyword), "Estimated Size"); sizeIdx >= 0 {
			if size := extractArchiveValue(text, sizeIdx, "Download Cost", "Archive Download", "H@H Downloader", "Current Funds", "You unlocked"); size != "" {
				sizes = append(sizes, size)
			}
		}
		start = idx + len(costKeyword)
	}

	labels := []string{string(models.ArchiveTypeOriginal), string(models.ArchiveTypeResample)}
	names := []string{"原图", "压缩图"}

	opts := make([]models.ArchiveDownloadOption, 0, 2)
	for i := 0; i < len(costs) && i < len(sizes) && i < 2; i++ {
		opts = append(opts, models.ArchiveDownloadOption{
			Label: labels[i],
			Name:  names[i],
			Cost:  sanitizeArchiveValue(costs[i]),
			Size:  sanitizeArchiveValue(sizes[i]),
		})
	}
	return opts
}

// sanitizeArchiveValue 截断归档报价值中混入的横幅/脚本文本。
// 已解锁场景下 archiver.php 会在报价之后紧跟
// "You unlocked an original download of this archive on ... [cancel] function cancel_sessions() {...}",
// 若提取的 cost/size 未能在该横幅处截止，会把大段无关文本吞入字段。
// 此函数在首个已知横幅/脚本/关键字处截断，保留干净的 "Free!" / "87.24 MiB" 等值。
func sanitizeArchiveValue(v string) string {
	cut := []string{
		"You unlocked", "You have", "You already",
		"cancel", "function", "window.",
		"Current Funds", "H@H Downloader", "Archive Download",
		"Download Cost", "Estimated Size",
	}
	lower := strings.ToLower(v)
	for _, c := range cut {
		if i := strings.Index(lower, strings.ToLower(c)); i >= 0 {
			v = v[:i]
		}
	}
	return strings.TrimSpace(v)
}

// findKeywordIndex 在 text 中从 start 开始查找关键字（忽略大小写），返回字节下标；未找到返回 -1
func findKeywordIndex(text string, start int, keyword string) int {
	rel := strings.Index(strings.ToLower(text[start:]), strings.ToLower(keyword))
	if rel < 0 {
		return -1
	}
	return start + rel
}

// extractArchiveValue 从 start（关键字处）向后取「:」之后的值，值截止到任一终止关键字先命中处或文本结尾
func extractArchiveValue(text string, start int, terminators ...string) string {
	colon := strings.Index(text[start:], ":")
	if colon < 0 {
		return ""
	}
	valStart := start + colon + 1
	end := len(text)
	for _, t := range terminators {
		if ti := findKeywordIndex(text, valStart, t); ti >= 0 && ti < end {
			end = ti
		}
	}
	return strings.TrimSpace(text[valStart:end])
}

// ─────────────────────────────────────────────────────────────
// 内部工具
// ─────────────────────────────────────────────────────────────

// newTaskID 生成任务唯一 ID（毫秒时间戳 + 随机字节）
func newTaskID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%d-%x", time.Now().UnixMilli(), b)
}

// loadEHSetting 读取指定用户的 EHSetting（多用户下每用户一条，不存在则创建默认值）
func loadEHSetting(db *gorm.DB, userID uint) *models.EHSetting {
	var setting models.EHSetting
	if err := db.Where("user_id = ?", userID).First(&setting).Error; err != nil {
		setting = models.EHSetting{UserID: userID, Site: "e-hentai", PreferRedirect: true}
		if err := db.Create(&setting).Error; err != nil {
			log.Printf("%s 初始化 EHSetting 失败: %v", dlErrTag, err)
		}
	}
	return &setting
}

// loadUserAccount 按用户 ID 加载其 E 站凭证，构造 AccountSetting（下载任务执行用发起者账号）
func loadUserAccount(db *gorm.DB, userID uint) *models.AccountSetting {
	var u models.User
	if err := db.First(&u, userID).Error; err != nil {
		return &models.AccountSetting{}
	}
	return &models.AccountSetting{
		ID:          u.ID,
		IPBMemberID: u.IPBMemberID,
		IPBPassHash: u.IPBPassHash,
		Igneous:     u.Igneous,
		SK:          u.SK,
		IsEx:        u.IsEx,
	}
}
