package handlers

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuManga/internal/middleware"
	"SakuManga/internal/models"
	"SakuManga/internal/services"
)

// 下载/更新日志统一前缀（与 services 包保持一致，便于验收 grep）
const (
	dlLogTag  = "[DOWNLOAD]"
	dlWarnTag = "[DOWNLOAD-WARN]"
	dlErrTag  = "[DOWNLOAD-ERROR]"
)

// OfflineHandler 离线更新检测 + 维护查重 HTTP 层
type OfflineHandler struct {
	db        *gorm.DB
	ehService *services.EHService
	manager   *services.DownloadManager
}

// NewOfflineHandler 构造离线 handler
func NewOfflineHandler(db *gorm.DB, ehService *services.EHService, manager *services.DownloadManager) *OfflineHandler {
	return &OfflineHandler{db: db, ehService: ehService, manager: manager}
}

// ─────────────────────────────────────────────────────────────
// 更新检测
// ─────────────────────────────────────────────────────────────

// CheckOfflineUpdates 运行一次更新检测 POST /api/v1/offline/updates/check
//
// 联网逐画廊核对在线详情（限流退避内置），可能耗时较长，由前端异步调用。
// 检测内部固定使用管理员账号（后台维护任务），用户仅作为触发入口。
func (h *OfflineHandler) CheckOfflineUpdates(c *gin.Context) {
	if !services.StartOfflineTask(services.OfflineTaskUpdate) {
		c.JSON(http.StatusConflict, gin.H{"error": "已有离线维护任务正在运行，请稍后再试"})
		return
	}

	go func() {
		// Round4 任务四：手动触发同样执行「常规更新检测 + 老化判定」完整扫描
		result, _, err := services.RunUpdateScanWithProgress(h.db, h.ehService, services.OfflineUpdateProgressSink)
		if err != nil {
			services.FinishOfflineTask(err)
			return
		}
		services.StoreUpdateCheckResult(result)

		// 自动更新画廊（autoUpdateGallery）：检测到新版后立即按所选方案入队下载
		if h.manager.GetSettings().AutoUpdateGallery {
			enqueued, skipped := services.AutoEnqueueUpdates(h.db, h.manager, result)
			log.Printf("%s [update] 检测完成：需要更新 %d 个，自动入队 %d 个，跳过 %d 个（autoUpdateGallery=true）",
				dlLogTag, len(result.NeedsUpdate), enqueued, skipped)
		}
		services.FinishOfflineTask(nil)
	}()

	c.JSON(http.StatusAccepted, gin.H{"started": true})
}

// GetCheckUpdatesProgress 更新检测进度轮询 GET /api/v1/offline/updates/check/progress
func (h *OfflineHandler) GetCheckUpdatesProgress(c *gin.Context) {
	c.JSON(http.StatusOK, services.GetOfflineTaskProgress())
}

// GetCheckUpdatesResult 更新检测结果读取 GET /api/v1/offline/updates/check/result
func (h *OfflineHandler) GetCheckUpdatesResult(c *gin.Context) {
	c.JSON(http.StatusOK, services.GetUpdateCheckResult())
}

// ListOfflineUpdates 列出需要更新的漫画 GET /api/v1/offline/updates
func (h *OfflineHandler) ListOfflineUpdates(c *gin.Context) {
	var comics []models.OfflineComic
	if err := h.db.Where("needs_update = ?", true).Order("updated_at desc").Find(&comics).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": comics, "total": len(comics)})
}

// DismissOfflineUpdate 将漫画移出更新列表 POST /api/v1/offline/updates/:id/dismiss
// 仅清除更新标记（保留本地文件与记录），供「画廊已被删除/移除」项清理列表。需求 3(2)
func (h *OfflineHandler) DismissOfflineUpdate(c *gin.Context) {
	id := c.Param("id")
	if _, err := services.ClearOfflineUpdateByComicID(h.db, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "未找到漫画记录"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"dismissed": true})
}

// downloadUpdateReq 更新下载请求体
type downloadUpdateReq struct {
	ComicID string `json:"comicId"`
	Mode    string `json:"mode"` // 可选：gallery | archive（覆盖自动更新方案）
}

// buildUpdateParams 根据漫画的更新信息构造下载参数（手动更新与自动更新共用同一套方案选择逻辑）
//
// Round4 任务四：方案选择逻辑下沉到 services.BuildUpdateDownloadParams，与自动入队复用同一实现，避免重复。
func (h *OfflineHandler) buildUpdateParams(comic *models.OfflineComic, modeOverride string, userID uint) (services.CreateDownloadParams, error) {
	return services.BuildUpdateDownloadParams(h.db, h.manager, comic, modeOverride, userID)
}

// DownloadUpdate 为需要更新的漫画启动新版下载 POST /api/v1/offline/updates/download
func (h *OfflineHandler) DownloadUpdate(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	if !requireDownloadPermission(c) {
		return
	}

	var req downloadUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ComicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误，必需传递 comicId"})
		return
	}

	var comic models.OfflineComic
	if err := h.db.First(&comic, "id = ?", req.ComicID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到漫画记录"})
		return
	}
	if !comic.NeedsUpdate {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该漫画当前无需更新（needsUpdate=false）"})
		return
	}

	// 构造更新下载参数（方案选择与自动更新共用同一逻辑）
	params, err := h.buildUpdateParams(&comic, req.Mode, user.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := h.manager.CreateTask(params)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

// ─────────────────────────────────────────────────────────────
// 维护查重
// ─────────────────────────────────────────────────────────────

// GetMaintainDedup 异步启动维护查重 GET /api/v1/offline/maintain
//
// 维护查重逐画廊联网核对，可能耗时数十分钟，改为异步任务 + 进度轮询：
//  1. 本接口立即返回 202（启动结果）；
//  2. 前端轮询 GET /offline/maintain/progress 获取进度；
//  3. 完成后前端读取 GET /offline/maintain/result 获取结果。
func (h *OfflineHandler) GetMaintainDedup(c *gin.Context) {
	if !services.StartOfflineTask(services.OfflineTaskMaintain) {
		c.JSON(http.StatusConflict, gin.H{"error": "已有离线维护任务正在运行，请稍后再试"})
		return
	}
	// 需求1：?full=true 强制全量在线核对（忽略 parent_checked_at 增量标记，联网逐本重抓）
	full := c.DefaultQuery("full", "") == "true"

	go func() {
		result, err := services.MaintainDedupWithProgress(h.db, h.ehService, services.OfflineMaintainProgressSink, full)
		if err != nil {
			services.FinishOfflineTask(err)
			return
		}
		services.StoreMaintainDedupResult(result, full)
		services.FinishOfflineTask(nil)
	}()

	c.JSON(http.StatusAccepted, gin.H{"started": true})
}

// GetMaintainProgress 维护查重进度轮询 GET /api/v1/offline/maintain/progress
func (h *OfflineHandler) GetMaintainProgress(c *gin.Context) {
	c.JSON(http.StatusOK, services.GetOfflineTaskProgress())
}

// GetMaintainResult 维护查重结果读取 GET /api/v1/offline/maintain/result
func (h *OfflineHandler) GetMaintainResult(c *gin.Context) {
	// Round42 D6：结果持久化——进程重启后内存缓存为空，先从快照回填，前端即可秒开上次结果
	services.EnsureMaintainResultLoaded(h.db)
	c.JSON(http.StatusOK, services.GetMaintainDedupResult())
}

// GetMaintainUnsynced 书库变更同步状态读取 GET /api/v1/offline/maintain/unsynced
// 需求4：前端进入维护界面时据此判断是否自动触发增量查重（下载/更新/删除后结果未反映变更）。
func (h *OfflineHandler) GetMaintainUnsynced(c *gin.Context) {
	services.EnsureMaintainResultLoaded(h.db)
	c.JSON(http.StatusOK, services.GetMaintainUnsyncedStatus())
}

// GetDedupSetting 读取查重设置 GET /api/v1/offline/dedup/setting
// Round42 D2：目前仅「联网复核」一项，默认关闭（无记录时返回默认值）。
func (h *OfflineHandler) GetDedupSetting(c *gin.Context) {
	c.JSON(http.StatusOK, services.GetDedupSetting(h.db))
}

// SaveDedupSetting 保存查重设置 POST /api/v1/offline/dedup/setting
//
// Round44：新增 deleteFileDefault（维护页「移除时删除本地文件」勾选的记忆）。
// 字段为指针 → 支持部分更新（前端只传其中一项时另一项保持原值）。
func (h *OfflineHandler) SaveDedupSetting(c *gin.Context) {
	var req struct {
		OnlineVerify      *bool `json:"onlineVerify"`
		DeleteFileDefault *bool `json:"deleteFileDefault"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
		return
	}
	s := services.GetDedupSetting(h.db)
	if req.OnlineVerify != nil {
		s.OnlineVerify = *req.OnlineVerify
	}
	if req.DeleteFileDefault != nil {
		s.DeleteFileDefault = *req.DeleteFileDefault
	}
	if err := services.SaveDedupSetting(h.db, s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, services.GetDedupSetting(h.db))
}

// ─────────────────────────────────────────────────────────────
// Round29：任务控制（暂停 / 继续 / 取消）
//
// 作用于单槽位当前任务（更新检测或维护查重，互斥），三个页面共用：
//   离线更新检测（OfflineUpdate） / 本地书库维护（OfflineMaintain） /
//   更新扫描设置「立即扫描」。暂停为协作式：当前漫画处理完后生效。
// ─────────────────────────────────────────────────────────────

// PauseOfflineTask 暂停当前任务 POST /api/v1/offline/task/pause
func (h *OfflineHandler) PauseOfflineTask(c *gin.Context) {
	if !services.PauseOfflineTask() {
		c.JSON(http.StatusConflict, gin.H{"error": "当前没有可暂停的运行中任务"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"paused": true})
}

// ResumeOfflineTask 继续被暂停的任务 POST /api/v1/offline/task/resume
func (h *OfflineHandler) ResumeOfflineTask(c *gin.Context) {
	if !services.ResumeOfflineTask() {
		c.JSON(http.StatusConflict, gin.H{"error": "当前没有处于暂停状态的任务"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"resumed": true})
}

// CancelOfflineTask 取消任务（running / paused 均可）POST /api/v1/offline/task/cancel
// 取消为协作式：任务在当前漫画处理完后于下一检查点退出；取消结果由进度轮询
// 呈现为 status=cancelled。
func (h *OfflineHandler) CancelOfflineTask(c *gin.Context) {
	if !services.CancelOfflineTask() {
		c.JSON(http.StatusConflict, gin.H{"error": "当前没有可取消的运行中/暂停中任务"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": true})
}

// removeDedupReq 删除重复项请求体
type removeDedupReq struct {
	ComicID    string   `json:"comicId"`
	ComicIDs   []string `json:"comicIds"`   // 批量删除：传多个 comicId，一次删除
	DeleteFile bool     `json:"deleteFile"` // 是否同时物理删除本地文件
}

// RemoveDedup 删除重复漫画 POST /api/v1/offline/maintain/remove
//
// 支持两种删除方式：
//   - 单个删除：传 comicId（兼容旧版）
//   - 批量删除：传 comicIds 数组，一次提交多个，避免反复“删除→刷新”
func (h *OfflineHandler) RemoveDedup(c *gin.Context) {
	var req removeDedupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
		return
	}
	// 批量删除：comicIds 非空则优先批量
	if len(req.ComicIDs) > 0 {
		deleted, err := services.RemoveDedupComics(h.db, req.ComicIDs, req.DeleteFile)
		// 幽灵文件修复：无论成败都使维护/更新结果缓存失效，避免残留过期数据
		services.InvalidateMaintainDedupResult(req.ComicIDs)
		// Round33：定向重算疑似重复簇（已删项不再残留在簇成员中），缓存即时可信
		services.SyncMaintainDedupClusters(h.db)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "deleted": deleted})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": deleted})
		return
	}
	if req.ComicID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误，必需传递 comicId 或 comicIds"})
		return
	}
	if err := services.RemoveDedupComic(h.db, req.ComicID, req.DeleteFile); err != nil {
		// 幽灵文件容错：记录已不存在（可能已被其他设备删除）→ 视为删除成功，
		// 返回 alreadyDeleted=true，前端据此移除本地列表项而非报错。
		if errors.Is(err, services.ErrComicNotFound) {
			services.InvalidateMaintainDedupResult([]string{req.ComicID})
			services.SyncMaintainDedupClusters(h.db)
			c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": 1, "alreadyDeleted": true})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	services.InvalidateMaintainDedupResult([]string{req.ComicID})
	services.SyncMaintainDedupClusters(h.db)
	c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": 1})
}

// ClearRemovedStatus 批量清除画廊移除标记 POST /api/v1/offline/maintain/clear-removed
//
// D4：失效画廊修复后（重新上传 / 更换源），一键清除全部 removed_status 标记并复位
// parent_checked_at（此前增量核对已跳过），使这些画廊重新参与维护查重/更新检测。
// 前端随后触发 forceFull 全量在线重新匹配。返回清除的条数。
func (h *OfflineHandler) ClearRemovedStatus(c *gin.Context) {
	cleared, err := services.ClearRemovedStatus(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 结果缓存失效：标记 stale=true 提示重新扫描（清除标记不改变 items 本身）
	services.InvalidateMaintainDedupResult(nil)
	c.JSON(http.StatusOK, gin.H{"ok": true, "cleared": cleared})
}

// ─────────────────────────────────────────────────────────────
// Round26 O2：忽略标记（疑似重复组 / 父画廊更新提示）
// ─────────────────────────────────────────────────────────────

// createIgnoreReq 创建忽略条目请求体
type createIgnoreReq struct {
	Type     string `json:"type"`     // title | gid | comic
	TitleKey string `json:"titleKey"` // type=title：归一化核心名
	Artist   string `json:"artist"`   // type=title：画师
	GID      string `json:"gid"`      // type=gid：父画廊 gid
	ComicID  string `json:"comicId"`  // type=comic：本地漫画 id（组内成员级忽略）
	Note     string `json:"note"`     // 备注（可选）
}

// CreateIgnore 新增忽略条目 POST /api/v1/offline/ignore
func (h *OfflineHandler) CreateIgnore(c *gin.Context) {
	var req createIgnoreReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
		return
	}
	rec, err := services.CreateIgnore(h.db, req.Type, req.TitleKey, req.Artist, req.GID, req.ComicID, req.Note)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Round33：忽略即时生效——定向重算结果缓存的疑似重复簇，
	// 避免「忽略后重进页面簇又复活」而被迫整份重新扫描
	services.SyncMaintainDedupClusters(h.db)
	c.JSON(http.StatusOK, gin.H{"ok": true, "ignore": rec})
}

// ListIgnores 忽略清单 GET /api/v1/offline/ignore/list
//
// Round44：返回体附带每条忽略的成员视图（宽松口径 + 同组标注 + 新增感知），
// 供忽略清单独立页展示成员卡片；同时给出 matchedCount / groupCount / newCount。
func (h *OfflineHandler) ListIgnores(c *gin.Context) {
	list, err := services.ListIgnoresWithMembers(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": list})
}

// AckIgnoreNewMembers 确认新增 POST /api/v1/offline/ignore/:id/ack
//
// Round44 新增感知：被忽略的作品下出现"忽略之后新入库"的本子时，该簇会重新出现在维护页；
// 用户在忽略清单页确认已知悉后，把成员快照刷新为当前匹配集合，之后继续静默。
func (h *OfflineHandler) AckIgnoreNewMembers(c *gin.Context) {
	id := c.Param("id")
	rec, err := services.AckIgnoreSnapshot(h.db, id)
	if err != nil {
		if errors.Is(err, services.ErrIgnoreNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 快照更新后该簇回到静默态 → 定向同步结果缓存（无需重新扫描）
	services.SyncMaintainDedupClusters(h.db)
	c.JSON(http.StatusOK, gin.H{"ok": true, "ignore": rec})
}

// RestoreIgnore 恢复（删除）忽略条目 POST /api/v1/offline/ignore/:id/restore
func (h *OfflineHandler) RestoreIgnore(c *gin.Context) {
	id := c.Param("id")
	if err := services.RestoreIgnore(h.db, id); err != nil {
		if errors.Is(err, services.ErrIgnoreNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Round33：恢复即时生效——定向重算结果缓存的疑似重复簇，恢复的组立即回到列表
	services.SyncMaintainDedupClusters(h.db)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
