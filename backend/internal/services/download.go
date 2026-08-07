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
	// maxArchiveAutoUnlock 归档任务「遇到锁自动消耗 GP 解锁」的单任务重试上限。
	// 达到上限后不再自动解锁（任务保持 error_lock 等待手动解锁），避免无限消耗 GP。
	maxArchiveAutoUnlock = 3
)

// errTaskAlreadyExists：批量创建下载任务时用于区分「gid 去重跳过」与「真正失败」的错误哨兵
var errTaskAlreadyExists = errors.New("task already exists")

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

// BatchCreateParams 批量创建下载任务入参（前端 POST /api/v1/downloads/batch）
type BatchCreateParams struct {
	Tasks       []BatchCreateItem   `json:"tasks"`
	Mode        models.DownloadMode `json:"mode"`        // 统一下载模式：gallery / archive
	ArchiveType models.ArchiveType  `json:"archiveType"` // 归档类型：original / resample（仅 archive 模式生效）
	Priority    int                 `json:"priority"`
	Group       string              `json:"group"`
	UserID      uint                `json:"userId"` // 任务发起者（决定执行时使用谁的 E 站凭证）
}

// BatchCreateItem 批量创建中的单个画廊
type BatchCreateItem struct {
	GID      string `json:"gid"`
	Token    string `json:"token"`
	Title    string `json:"title"`
	CoverURL string `json:"coverUrl"`
}

// BatchCreateResult 批量创建统计结果
type BatchCreateResult struct {
	Created int      `json:"created"`
	Skipped int      `json:"skipped"` // 已存在进行中任务（gid 去重）
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors"`
	TaskIDs []string `json:"taskIds"`
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

	// 调度门控：控制「同一优先级并发」
	// 真实并发由调度器按下载设置决定，worker 池数量仅作上限兜底
	schedMu        sync.Mutex
	runningTotal   int         // 当前运行任务总数
	runningArchive int         // 当前运行的归档任务数（仅统计/日志）
	runningByPri   map[int]int // 各优先级正在运行的任务数
	runningTasks   map[string]int // 任务ID -> 优先级（正在运行，供抢占式调度遍历）

	// 归档引擎运行时管理：全局线程配额池 + 正在运行的归档引擎（动态调整/立即中断）
	archivePool    *archiveThreadPool
	activeArchives map[string]*archiveDownloader // 任务ID -> 正在运行的归档引擎

	// 画廊引擎运行时管理：正在运行的画廊引擎（暂停/取消/抢占时立即中断）
	activeGalleries map[string]*galleryDownloader // 任务ID -> 正在运行的画廊引擎
}

// NewDownloadManager 构造下载任务管理器
func NewDownloadManager(db *gorm.DB, ehService *EHService) *DownloadManager {
	return &DownloadManager{
		db:             db,
		ehService:      ehService,
		queue:          make(chan string, 256),
		workers:        16, // worker 池仅作并发上限，实际并发由调度门控（downloadAllGalleriesSamePriority / 归档线程配额）控制
		quit:           make(chan struct{}),
		runningByPri:   make(map[int]int),
		runningTasks:   make(map[string]int),
		archivePool:    newArchiveThreadPool(defaultMaxArchiveThreads),
		activeArchives: make(map[string]*archiveDownloader),
		activeGalleries: make(map[string]*galleryDownloader),
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

	// 抢占让位：markRunning 与引擎启动之间可能被高优先级任务抢占（DB 状态已被置回 queued），
	// 此时让位退出，由抢占流程负责重新入队，避免低优先级任务在抢占后仍启动引擎。
	var chk models.DownloadTask
	if err := m.db.First(&chk, "id = ?", taskID).Error; err == nil && chk.Status != models.DownloadDownloading {
		log.Printf("%s [sched] 任务 %s 在引擎启动前被抢占（状态=%s），让位", dlWarnTag, taskID, chk.Status)
		return
	}
	task = chk

	m.dispatchEngine(&task)

	// 离线更新任务收尾：下载成功后按设置清理旧版
	m.finalizeUpdate(&task)

	// 无 H@H 自动降级：归档更新任务失败（非配额锁定）时按设置改用画廊引擎重试
	m.fallbackUpdateToGallery(&task)

	// 需求2：任何下载任务完成后，主动清除更新列表中同 GID 的更新标记。
	// 覆盖：手动下载标记需更新的漫画本体（gid 匹配）+ 下载检测到的新版本时父画廊的标记（new_gid 匹配）。
	if task.Status == models.DownloadCompleted && task.GID != "" {
		if _, err := ClearOfflineUpdateByGID(m.db, task.GID); err != nil {
			log.Printf("%s [update] 下载完成 gid=%s 清除更新标记失败: %v", dlErrTag, task.GID, err)
		}
	}

	// 需求3(1)：下载完成后主动对账数据库（GID 去重 / ParentGID 回写 / PageCount 校正 / Aged 复位）。
	// 依赖 ScanAndSaveDirectory 已入库的本地记录与其落地目录/压缩包内 metadata，做轻量比对。
	if task.Status == models.DownloadCompleted && task.GID != "" {
		if _, err := ReconcileOfflineAfterDownload(m.db, &task); err != nil {
			log.Printf("%s [reconcile] 下载完成 gid=%s 数据对账失败: %v", dlErrTag, task.GID, err)
		}
	}

	// 抢占恢复：任务被高优先级任务抢占后 DB 状态被置回 queued（进度保留，可断点续传），
	// 自动重新入队以再次调度。降级路径（fallbackArchiveToGallery 会改 Mode + 自行入队）排除在外。
	var post models.DownloadTask
	if err := m.db.First(&post, "id = ?", taskID).Error; err == nil &&
		post.Mode == task.Mode && post.Status == models.DownloadQueued {
		log.Printf("%s [sched] 任务 %s 被抢占后重新入队（优先级=%d 进度=%d/%d）",
			dlLogTag, taskID, post.Priority, post.DoneFiles, post.TotalFiles)
		m.Enqueue(taskID)
	}
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
		return nil, fmt.Errorf("%w（gid=%s 已有进行中任务 %s）", errTaskAlreadyExists, p.GID, existing.ID)
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
	// 新任务入队：优先级高于正在运行任务时抢占（计划书 5.5.2）
	m.preemptLowerPriority(task.Priority)
	return task, nil
}

// CreateTasksBatch 批量创建下载任务并入队（逐条复用 CreateTask，聚合统计）
func (m *DownloadManager) CreateTasksBatch(p BatchCreateParams) *BatchCreateResult {
	res := &BatchCreateResult{Errors: []string{}}
	for i, item := range p.Tasks {
		if item.GID == "" || item.Token == "" {
			res.Failed++
			res.Errors = append(res.Errors, fmt.Sprintf("[%d] gid 与 token 不能为空", i+1))
			continue
		}
		task, err := m.CreateTask(CreateDownloadParams{
			GID:         item.GID,
			Token:       item.Token,
			Title:       item.Title,
			CoverURL:    item.CoverURL,
			Mode:        p.Mode,
			ArchiveType: p.ArchiveType,
			Priority:    p.Priority,
			Group:       p.Group,
			UserID:      p.UserID,
		})
		if err != nil {
			if errors.Is(err, errTaskAlreadyExists) {
				res.Skipped++
			} else {
				res.Failed++
				res.Errors = append(res.Errors, fmt.Sprintf("[%d] %v", i+1, err))
			}
			continue
		}
		res.Created++
		res.TaskIDs = append(res.TaskIDs, task.ID)
	}
	log.Printf("%s 批量创建任务：共 %d 条，成功 %d，跳过 %d，失败 %d",
		dlLogTag, len(p.Tasks), res.Created, res.Skipped, res.Failed)
	return res
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
	// 立即中断运行中的归档下载（分块 Range 请求取消，.part 保留供恢复续传）
	m.stopArchiveDownload(taskID)
	// 同时中断运行中的画廊下载（图片请求取消，.part 保留供恢复续传）
	m.stopGalleryDownload(taskID)
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
	// 恢复的高优先级任务入队时抢占正在运行的低优先级任务
	m.preemptLowerPriority(task.Priority)
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
	// 立即中断运行中的归档下载（分块 Range 请求取消）
	m.stopArchiveDownload(taskID)
	// 同时中断运行中的画廊下载（图片请求取消）
	m.stopGalleryDownload(taskID)
	log.Printf("%s 取消任务 %s（gid=%s）", dlLogTag, taskID, task.GID)
	return &task, nil
}

// SetTaskPriority 修改任务优先级并触发抢占调度（计划书 5.5 / 5.6）。
// 提升优先级时抢占正在运行的低优先级任务为其让路；
// queued 任务重新入队按新优先级竞争槽位（downloading 任务由 worker 退出时自动重新入队，paused 保持暂停）。
func (m *DownloadManager) SetTaskPriority(taskID string, priority int) (*models.DownloadTask, error) {
	var task models.DownloadTask
	if err := m.db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, errors.New("任务不存在")
	}
	if task.Status == models.DownloadCompleted || task.Status == models.DownloadCancelled {
		return nil, fmt.Errorf("当前状态 %s 不允许修改优先级", task.Status)
	}
	if priority == task.Priority {
		return &task, nil
	}
	old := task.Priority
	task.Priority = priority
	task.UpdatedAt = time.Now()
	if err := m.db.Save(&task).Error; err != nil {
		return nil, err
	}
	log.Printf("%s 修改任务 %s（gid=%s）优先级 %d → %d", dlLogTag, taskID, task.GID, old, priority)

	// 优先级提升 → 抢占正在运行的低优先级任务，为本任务让路
	if priority > old {
		m.preemptLowerPriority(priority)
	}

	// queued 任务重新入队以按新优先级竞争槽位
	if task.Status == models.DownloadQueued {
		m.Enqueue(taskID)
	}
	return &task, nil
}

// RetryTask 重试任务（error / error_lock / cancelled → queued）
func (m *DownloadManager) RetryTask(taskID string) (*models.DownloadTask, error) {
	var task models.DownloadTask
	if err := m.db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, errors.New("任务不存在")
	}
	wasLocked := task.Status == models.DownloadErrorLock
	switch task.Status {
	case models.DownloadError, models.DownloadErrorLock, models.DownloadCancelled:
	default:
		return nil, fmt.Errorf("当前状态 %s 不允许重试（仅 error / error_lock / cancelled 可重试）", task.Status)
	}
	task.Status = models.DownloadQueued
	task.Error = ""
	task.AutoUnlockCount = 0 // 手动重试后重置自动解锁计数，重新获得自动解锁机会
	task.UpdatedAt = time.Now()
	if err := m.db.Save(&task).Error; err != nil {
		return nil, err
	}
	// 归档任务 error_lock：先取消原服务端 Session（IP 变更/多 IP 触发封锁），再从当前 IP 重新解锁
	if wasLocked && task.Mode == models.DownloadModeArchive {
		m.cancelArchiveSessionForTask(&task)
	}
	m.Enqueue(taskID)
	// 重试的高优先级任务入队时抢占正在运行的低优先级任务
	m.preemptLowerPriority(task.Priority)
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

	// 归档任务：先取消原服务端 Session（IP 变更/多 IP 触发封锁），再从当前 IP 重新解锁
	if task.Mode == models.DownloadModeArchive {
		m.cancelArchiveSessionForTask(&task)
	}

	task.Status = models.DownloadQueued
	task.Error = ""
	task.AutoUnlockCount = 0 // 手动解锁后重置自动解锁计数，重新获得自动解锁机会
	task.UpdatedAt = time.Now()
	if err := m.db.Save(&task).Error; err != nil {
		return nil, err
	}
	m.Enqueue(taskID)
	// 解锁的高优先级任务入队时抢占正在运行的低优先级任务（计划书 5.5.2）
	m.preemptLowerPriority(task.Priority)
	return &task, nil
}

// autoUnlockArchiveTask 自动 GP 解锁（需求：归档任务遇锁且设置开启时，自动解锁重试）。
// 每任务上限 maxArchiveAutoUnlock 次，达到上限后不再自动解锁（任务保持 error_lock 等待手动处理）。
// 仅修改任务状态为 queued 并落库，由 runTask 收尾的统一重新入队逻辑（post 检查）负责入队，
// 避免本方法主动 Enqueue 造成与 post 检查的重复入队。
// 返回 true 表示已自动解锁（本次失败不再写 error_lock）。
func (m *DownloadManager) autoUnlockArchiveTask(task *models.DownloadTask) bool {
	if task == nil || task.Mode != models.DownloadModeArchive {
		return false
	}
	if task.AutoUnlockCount >= maxArchiveAutoUnlock {
		log.Printf("%s [auto-unlock] 任务 %s 已达自动解锁上限（%d 次），进入 error_lock 等待手动解锁",
			dlWarnTag, task.ID, maxArchiveAutoUnlock)
		return false
	}
	task.AutoUnlockCount++
	// 先取消原服务端 Session（IP 变更/多 IP 触发封锁），再从当前 IP 重新解锁，对齐手动解锁流程
	m.cancelArchiveSessionForTask(task)
	task.Status = models.DownloadQueued
	task.Error = ""
	task.UpdatedAt = time.Now()
	if err := m.db.Save(task).Error; err != nil {
		log.Printf("%s [auto-unlock] 任务 %s 自动解锁状态保存失败: %v", dlErrTag, task.ID, err)
		return false
	}
	log.Printf("%s [auto-unlock] 任务 %s（gid=%s）自动消耗 GP 解锁，取消旧 Session 后重新入队（第 %d/%d 次）",
		dlLogTag, task.ID, task.GID, task.AutoUnlockCount, maxArchiveAutoUnlock)
	return true
}

// cancelArchiveSessionForTask 取消任务对应的 E 站服务端归档 Session（invalidate_sessions=1）。
// 仅归档任务生效：IP 变更/多 IP 使用触发 E 站 IP 封锁后，先废弃旧 IP 建立的 Session，
// 再从当前 IP 重新解锁（对齐 JHentai cancelArchive + downloadArchive(resume:true, reParse:true) 流程）。
// 取消失败不阻断解锁流程（E 站通常仍允许重新创建归档），仅记录日志。
func (m *DownloadManager) cancelArchiveSessionForTask(task *models.DownloadTask) {
	if task == nil || task.Mode != models.DownloadModeArchive {
		return
	}
	account := loadUserAccount(m.db, task.UserID)
	if account.IPBMemberID == "" {
		log.Printf("%s 取消任务 %s 归档 Session 跳过：未绑定 E 站账号凭证", dlWarnTag, task.ID)
		return
	}
	ehSetting := loadEHSetting(m.db, task.UserID)
	client, err := m.ehService.BuildClient(account)
	if err != nil {
		log.Printf("%s 取消任务 %s 归档 Session 失败（构建客户端）: %v", dlErrTag, task.ID, err)
		return
	}
	referer := GetBaseURL(account, ehSetting)
	if err := cancelArchiveSession(client, referer, task.GID, task.Token); err != nil {
		log.Printf("%s 取消任务 %s 归档 Session 失败: %v（将继续解锁流程）", dlWarnTag, task.ID, err)
		return
	}
	log.Printf("%s 已取消任务 %s（gid=%s）的服务端归档 Session，将从当前 IP 重新解锁", dlLogTag, task.ID, task.GID)
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
		ArchivePath:                   `downloads\Archive`,
		ExtractPath:                   `downloads\Gallery`,
		SingleImageSavePath:           `downloads\Gallery`,
		DefaultDownloadScheme:         models.DefaultSchemeArchiveOriginal,
		ConcurrentImageDownloads:      10,
		SpeedLimitImages:              99,
		SpeedLimitInterval:            "1s",
		DownloadAllGalleriesSamePriority: true,
		ArchiveThreads:                10,
		ControlArchiveConcurrency:     true,
		MaxArchiveConcurrency:         1,
		DeleteZipAfterArchiveDownload: true,
		AutoReduceThreadsOnEOF:        true,
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

	// 旧数据迁移：defaultDownloadScheme 替换旧版 defaultDownloadOriginal 布尔值
	// （旧布尔值 true=归档原图，false=归档压缩，保持用户既有偏好）
	if setting.DefaultDownloadScheme == "" {
		if setting.DefaultDownloadOriginal {
			setting.DefaultDownloadScheme = models.DefaultSchemeArchiveOriginal
		} else {
			setting.DefaultDownloadScheme = models.DefaultSchemeArchiveResample
		}
		setting.UpdatedAt = time.Now()
		if err := m.db.Save(&setting).Error; err != nil {
			log.Printf("%s 迁移默认下载配置失败: %v", dlErrTag, err)
		} else {
				log.Printf("%s 已迁移旧下载设置：默认下载配置=%s", dlLogTag, setting.DefaultDownloadScheme)
			}
		}
	
		// 旧数据迁移：maxArchiveConcurrency 为新增字段（默认 1），旧记录为 0 时补齐默认值
		if setting.MaxArchiveConcurrency == 0 {
			setting.MaxArchiveConcurrency = 1
			setting.UpdatedAt = time.Now()
			if err := m.db.Save(&setting).Error; err != nil {
				log.Printf("%s 迁移 maxArchiveConcurrency 默认值失败: %v", dlErrTag, err)
			} else {
				log.Printf("%s 已迁移下载设置：maxArchiveConcurrency=1", dlLogTag)
			}
		}
		return &setting
}

// SaveSettings 保存下载设置
func (m *DownloadManager) SaveSettings(s *models.DownloadSetting) (*models.DownloadSetting, error) {
	old := m.GetSettings()
	s.ID = 1
	s.UpdatedAt = time.Now()
	if err := m.db.Save(s).Error; err != nil {
		log.Printf("%s 保存下载设置失败: %v", dlErrTag, err)
		return nil, err
	}
	log.Printf("%s 保存下载设置: archivePath=%q extractPath=%q 并发图片=%d 速度=%d/%s 删除压缩包=%v 自动恢复=%v 自动更新画廊=%v 归档线程=%d 控制归档并发=%v",
		dlLogTag, s.ArchivePath, s.ExtractPath, s.ConcurrentImageDownloads, s.SpeedLimitImages, s.SpeedLimitInterval,
		s.DeleteZipAfterArchiveDownload, s.AutoResumeTasks, s.AutoUpdateGallery, s.ArchiveThreads, s.ControlArchiveConcurrency)

	// 归档线程数 / 并发控制开关 / 最大归档并发数变化 → 动态调整所有运行中的归档任务并唤醒额度池
	// 重新分配（对应 JHentai _onIsolateCountChange + _tryWakeWaitingTasks，计划书 5.6）
	if s.ArchiveThreads != old.ArchiveThreads || s.ControlArchiveConcurrency != old.ControlArchiveConcurrency ||
		s.MaxArchiveConcurrency != old.MaxArchiveConcurrency {
		m.notifyArchiveThreadsChange(s.ArchiveThreads)
	}
	return s, nil
}

// ─────────────────────────────────────────────────────────────
// 归档引擎运行时管理：注册 / 注销 / 立即中断 / 线程数动态调整
// ─────────────────────────────────────────────────────────────

// registerArchive 注册正在运行的归档引擎（供暂停/取消中断与线程数动态调整）
func (m *DownloadManager) registerArchive(g *archiveDownloader) {
	m.mu.Lock()
	m.activeArchives[g.task.ID] = g
	m.mu.Unlock()
}

// unregisterArchive 注销归档引擎
func (m *DownloadManager) unregisterArchive(taskID string) {
	m.mu.Lock()
	delete(m.activeArchives, taskID)
	m.mu.Unlock()
}

// stopArchiveDownload 立即中断指定归档任务的下载（暂停/取消时调用）：
// 置位本地停止标记 + 取消分块下载器 context，进行中的 Range 请求即刻中止。
// 同时唤醒线程配额池中排队等待的任务，使它们检查停止标记（否则被暂停的任务会一直阻塞在 acquire）。
func (m *DownloadManager) stopArchiveDownload(taskID string) {
	m.mu.Lock()
	g := m.activeArchives[taskID]
	m.mu.Unlock()
	if g != nil {
		g.stopDownload()
	}
	m.archivePool.wakeAll()
}

// ─────────────────────────────────────────────────────────────
// 画廊引擎运行时管理：注册 / 注销 / 立即中断
// ─────────────────────────────────────────────────────────────

// registerGallery 注册正在运行的画廊引擎（供暂停/取消/抢占中断）
func (m *DownloadManager) registerGallery(g *galleryDownloader) {
	m.mu.Lock()
	m.activeGalleries[g.task.ID] = g
	m.mu.Unlock()
}

// unregisterGallery 注销画廊引擎
func (m *DownloadManager) unregisterGallery(taskID string) {
	m.mu.Lock()
	delete(m.activeGalleries, taskID)
	m.mu.Unlock()
}

// stopGalleryDownload 立即中断指定画廊任务的下载（暂停/取消/抢占时调用）：
// 置位本地停止标记 + 取消共享 context，进行中的图片请求即刻中止（.part 保留供恢复续传）。
// 同时唤醒线程配额池中排队等待的任务，使它们检查停止标记（否则被暂停的任务会一直阻塞在 acquirePartial）。
func (m *DownloadManager) stopGalleryDownload(taskID string) {
	m.mu.Lock()
	g := m.activeGalleries[taskID]
	m.mu.Unlock()
	if g != nil {
		g.stopDownload()
	}
	m.archivePool.wakeAll()
}

// notifyArchiveThreadsChange 归档线程数设置变化时，动态调整所有运行中的归档任务的分块 worker 数
func (m *DownloadManager) notifyArchiveThreadsChange(newThreads int) {
	m.mu.Lock()
	list := make([]*archiveDownloader, 0, len(m.activeArchives))
	for _, g := range m.activeArchives {
		list = append(list, g)
	}
	m.mu.Unlock()
	for _, g := range list {
		g.onArchiveThreadsChange(newThreads)
	}
	// 唤醒排队任务重新竞争：线程数/并发上限变化后重算额度（计划书 5.6）
	m.archivePool.wakeAll()
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

	// Ex-only 画廊在表站 archiver.php 会返回 404（"this gallery is currently unavailable"），
	// 必须跟随站点配置（GetBaseURL：Site=exhentai && IsEx 时用里站）。
	base := strings.TrimSuffix(GetBaseURL(account, ehSetting), "/")
	archiverURL := base + "/archiver.php?gid=" + gid + "&token=" + token
	req, err := http.NewRequest("GET", archiverURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", base+"/")
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
