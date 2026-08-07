package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuHentai/internal/middleware"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
)

// DownloadHandler 下载任务 HTTP 层
type DownloadHandler struct {
	db        *gorm.DB
	ehService *services.EHService
	manager   *services.DownloadManager
}

// NewDownloadHandler 构造下载 handler
func NewDownloadHandler(db *gorm.DB, ehService *services.EHService, manager *services.DownloadManager) *DownloadHandler {
	return &DownloadHandler{db: db, ehService: ehService, manager: manager}
}

// requireAccount 校验 E 站账号凭证
func (h *DownloadHandler) requireAccount(c *gin.Context) *models.AccountSetting {
	account := middleware.CurrentAccount(c)
	if account == nil || account.IPBMemberID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return nil
	}
	return account
}

// requireDownloadPermission 校验当前用户具备下载许可（admin 或 allowDownload=true）
// 下载任务创建/恢复/离线更新下载等写库操作均需此许可（默认关闭，由管理员在用户管理中开启）。
func requireDownloadPermission(c *gin.Context) bool {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return false
	}
	if user.Role != services.RoleAdmin && !user.AllowDownload {
		c.JSON(http.StatusForbidden, gin.H{"error": "无下载权限，请联系管理员开启"})
		return false
	}
	return true
}

// requireTaskAccess 校验当前用户对任务可访问/可操作（管理员或发起者本人）。
// 多用户权限：下载任务按发起者隔离，仅发起者本人可暂停/取消/改优先级等；
// 管理员作为中心制兜底可管理全部用户的任务。
func requireTaskAccess(c *gin.Context, task *models.DownloadTask) bool {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return false
	}
	if user.Role != services.RoleAdmin && task.UserID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "只能操作自己发起的下载任务"})
		return false
	}
	return true
}

// requireAdmin 校验当前用户为管理员（handler 内嵌校验，用于 GET/POST 同路径权限不同的接口）
func requireAdmin(c *gin.Context) bool {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return false
	}
	if user.Role != services.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员可执行此操作"})
		return false
	}
	return true
}

// ─────────────────────────────────────────────────────────────
// 任务 CRUD
// ─────────────────────────────────────────────────────────────

// CreateDownload 创建下载任务 POST /api/v1/downloads
func (h *DownloadHandler) CreateDownload(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	if h.requireAccount(c) == nil {
		return
	}
	if !requireDownloadPermission(c) {
		return
	}

	var p services.CreateDownloadParams
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误: " + err.Error()})
		return
	}
	p.UserID = user.ID

	task, err := h.manager.CreateTask(p)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// BatchCreateDownload 批量创建下载任务 POST /api/v1/downloads/batch
// 前端用于「批量加入下载队列」：一次提交多个 gid+token，统一按默认方案入队。
func (h *DownloadHandler) BatchCreateDownload(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	if h.requireAccount(c) == nil {
		return
	}
	if !requireDownloadPermission(c) {
		return
	}

	var p services.BatchCreateParams
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误: " + err.Error()})
		return
	}
	if len(p.Tasks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tasks 列表不能为空"})
		return
	}
	if len(p.Tasks) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "单次批量创建最多 500 条"})
		return
	}
	p.UserID = user.ID

	c.JSON(http.StatusOK, h.manager.CreateTasksBatch(p))
}

// ListDownloads 下载任务列表 GET /api/v1/downloads
// 权限：无下载许可用户直接拒绝（隐藏下载列表）；普通成员仅返回自己发起的任务；管理员返回全部。
func (h *DownloadHandler) ListDownloads(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	if user.Role != services.RoleAdmin && !user.AllowDownload {
		c.JSON(http.StatusForbidden, gin.H{"error": "无下载权限，请联系管理员开启"})
		return
	}
	var p services.DownloadListParams
	if err := c.ShouldBindQuery(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "非法请求参数"})
		return
	}
	// 普通成员只看自己的任务；管理员查看全部（userID=0 不过滤）
	var userID uint
	if user.Role != services.RoleAdmin {
		userID = user.ID
	}
	tasks, total, err := h.manager.ListTasks(p, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": tasks, "total": total, "page": p.Page, "size": p.Size})
}

// GetDownload 单个任务详情 GET /api/v1/downloads/:id
func (h *DownloadHandler) GetDownload(c *gin.Context) {
	task, err := h.manager.GetTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if !requireTaskAccess(c, task) {
		return
	}
	c.JSON(http.StatusOK, task)
}

// ─────────────────────────────────────────────────────────────
// 任务操作
// ─────────────────────────────────────────────────────────────

// PauseDownload 暂停 POST /api/v1/downloads/:id/pause
func (h *DownloadHandler) PauseDownload(c *gin.Context) {
	task, err := h.manager.GetTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if !requireTaskAccess(c, task) {
		return
	}
	task, err = h.manager.PauseTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// ResumeDownload 恢复 POST /api/v1/downloads/:id/resume
func (h *DownloadHandler) ResumeDownload(c *gin.Context) {
	if !requireDownloadPermission(c) {
		return
	}
	task, err := h.manager.GetTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if !requireTaskAccess(c, task) {
		return
	}
	task, err = h.manager.ResumeTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// CancelDownload 取消 POST /api/v1/downloads/:id/cancel
func (h *DownloadHandler) CancelDownload(c *gin.Context) {
	task, err := h.manager.GetTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if !requireTaskAccess(c, task) {
		return
	}
	task, err = h.manager.CancelTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// RetryDownload 重试 POST /api/v1/downloads/:id/retry
func (h *DownloadHandler) RetryDownload(c *gin.Context) {
	task, err := h.manager.GetTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if !requireTaskAccess(c, task) {
		return
	}
	task, err = h.manager.RetryTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// UnlockDownload GP 解锁 POST /api/v1/downloads/:id/unlock
func (h *DownloadHandler) UnlockDownload(c *gin.Context) {
	task, err := h.manager.GetTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if !requireTaskAccess(c, task) {
		return
	}
	task, err = h.manager.UnlockTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// SetDownloadPriority 修改任务优先级 POST /api/v1/downloads/:id/priority
// 提升优先级会触发抢占式调度：正在运行的低优先级任务被置回 queued（进度保留），
// 高优先级任务优先竞争全局线程额度（计划书 5.5）。
func (h *DownloadHandler) SetDownloadPriority(c *gin.Context) {
	if !requireDownloadPermission(c) {
		return
	}
	var req struct {
		Priority int `json:"priority"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误: " + err.Error()})
		return
	}
	task, err := h.manager.GetTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if !requireTaskAccess(c, task) {
		return
	}
	task, err = h.manager.SetTaskPriority(c.Param("id"), req.Priority)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// RestoreDownloads 恢复历史任务 POST /api/v1/downloads/restore
// 恢复会重新入队全部未完成任务（可能触发他人任务重跑），仅管理员可执行。
func (h *DownloadHandler) RestoreDownloads(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	count, err := h.manager.RestoreTasks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"restored": count})
}

// ─────────────────────────────────────────────────────────────
// GP 面板
// ─────────────────────────────────────────────────────────────

// GetGPInfo 获取 GP 面板信息 GET /api/v1/downloads/gp-info?gid=..&token=..
func (h *DownloadHandler) GetGPInfo(c *gin.Context) {
	account := h.requireAccount(c)
	if account == nil {
		return
	}

	gid := c.Query("gid")
	token := c.Query("token")
	if gid == "" || token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失，必需传递 gid 和 token"})
		return
	}

	setting := getEHSetting(h.db, account.ID)
	info, err := h.manager.GetGPInfo(account, setting, gid, token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

// ─────────────────────────────────────────────────────────────
// 下载设置
// ─────────────────────────────────────────────────────────────

// GetDownloadSettings 获取下载设置 GET /api/v1/downloads/settings
func (h *DownloadHandler) GetDownloadSettings(c *gin.Context) {
	c.JSON(http.StatusOK, h.manager.GetSettings())
}

// SaveDownloadSettings 保存下载设置 POST /api/v1/downloads/settings
// 下载设置为系统级配置（路径/并发/归档等），仅管理员可修改（GET 保持登录可读）。
func (h *DownloadHandler) SaveDownloadSettings(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	var s models.DownloadSetting
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误: " + err.Error()})
		return
	}
	saved, err := h.manager.SaveSettings(&s)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, saved)
}
