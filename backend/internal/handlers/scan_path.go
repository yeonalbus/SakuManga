package handlers

import (
	"SakuManga/internal/database"
	"SakuManga/internal/models"
	"SakuManga/internal/services"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func GetScanPaths(c *gin.Context) {
	var paths []models.ExtraScanPath
	database.DB.Find(&paths)
	c.JSON(http.StatusOK, paths)
}

func AddScanPath(c *gin.Context) {
	var req struct {
		Path                string `json:"path"`
		Name                string `json:"name,omitempty"`      // 可配置显示名（问题3 来源标签）
		EnableOfflineUpdate *bool  `json:"enableOfflineUpdate"` // 是否参与离线更新/维护查重（问题4）
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的路径"})
		return
	}

	// 规范化路径（Windows 下 / 与 \ 视为同一目录，去除尾部多余分隔符）
	cleanPath := filepath.Clean(req.Path)

	// 精确判断是否已存在（兼容历史遗留的正/反斜杠、大小写差异记录）
	var existing []models.ExtraScanPath
	database.DB.Find(&existing)
	for _, p := range existing {
		if strings.EqualFold(filepath.Clean(p.Path), cleanPath) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "该路径已存在"})
			return
		}
	}

	// 新增路径默认开启离线更新/维护（D7）
	enableOfflineUpdate := true
	if req.EnableOfflineUpdate != nil {
		enableOfflineUpdate = *req.EnableOfflineUpdate
	}

	newPath := models.ExtraScanPath{
		// 纳秒时间戳，避免同一秒内添加多条路径时主键冲突被误报为“已存在”
		ID:                  fmt.Sprintf("path-%d", time.Now().UnixNano()),
		Path:                cleanPath,
		Name:                req.Name,
		IncludeSubfolders:   true,
		EnableOfflineUpdate: enableOfflineUpdate,
	}

	if err := database.DB.Create(&newPath).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "添加路径失败"})
		return
	}

	c.JSON(http.StatusOK, newPath)
}

func UpdateScanPath(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		IncludeSubfolders   *bool  `json:"includeSubfolders"`
		Name                string `json:"name,omitempty"`
		EnableOfflineUpdate *bool  `json:"enableOfflineUpdate"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	updates := map[string]interface{}{}
	if req.IncludeSubfolders != nil {
		updates["include_subfolders"] = *req.IncludeSubfolders
	}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.EnableOfflineUpdate != nil {
		updates["enable_offline_update"] = *req.EnableOfflineUpdate
	}
	if len(updates) > 0 {
		database.DB.Model(&models.ExtraScanPath{}).Where("id = ?", id).Updates(updates)
	}
	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// GetScanPathImpact 查询删除该路径会连带清理的漫画数量（前端二次确认文案用）。
func GetScanPathImpact(c *gin.Context) {
	id := c.Param("id")
	var count int64
	if err := database.DB.Model(&models.OfflineComic{}).
		Where("scan_path_id = ?", id).
		Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "统计失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"comicCount": count})
}

// DeleteScanPath 删除一条额外的画廊扫描路径，**并连带清理该路径下已入库的漫画记录**。
//
// 历史缺陷（Round53 修复）：本接口原先只删 extra_scan_paths 一行，
// offline_comics 里 ScanPathID 指向该路径的记录全部残留——离线库照常展示、
// 来源标签退化成「下载」、且不再命中「关闭离线维护」集合而被重新纳入联网更新检测。
// 完整流程（幂等 / 扫描中拒绝 / 先清漫画再删路径）见 services.DeleteScanPathWithComics。
func DeleteScanPath(c *gin.Context) {
	id := c.Param("id")

	removed, err := services.DeleteScanPathWithComics(database.DB, id)
	if err != nil {
		if errors.Is(err, services.ErrScanPathInProgress) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "removedComics": removed})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功", "removedComics": removed})
}

// TriggerScanPath 异步启动一次扫描（全量 / 增量）。
// 启动成功立即返回进度对象；若该路径已在扫描中则返回 409。
func TriggerScanPath(c *gin.Context) {
	id := c.Param("id")
	var targetPath models.ExtraScanPath
	if err := database.DB.First(&targetPath, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到该路径记录"})
		return
	}

	var req struct {
		Mode string `json:"mode"` // full | incremental，缺省为 full
	}
	_ = c.ShouldBindJSON(&req)
	if req.Mode == "" {
		req.Mode = "full"
	}

	progress, err := services.GetScanManager().StartScan(
		targetPath.ID, targetPath.Path, targetPath.IncludeSubfolders, req.Mode,
	)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, progress.Snapshot())
}

// GetScanPathProgress 查询指定路径的扫描进度；若该路径从未启动过扫描返回 404。
func GetScanPathProgress(c *gin.Context) {
	id := c.Param("id")
	progress := services.GetScanManager().GetProgress(id)
	if progress == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "该路径暂无扫描任务"})
		return
	}
	c.JSON(http.StatusOK, progress.Snapshot())
}

// GetAllScanProgress 查询全部路径的扫描进度（供前端统一轮询）。
func GetAllScanProgress(c *gin.Context) {
	c.JSON(http.StatusOK, services.GetScanManager().GetAllProgress())
}
