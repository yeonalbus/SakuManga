package handlers

import (
	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
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
		Path string `json:"path"`
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

	newPath := models.ExtraScanPath{
		// 纳秒时间戳，避免同一秒内添加多条路径时主键冲突被误报为“已存在”
		ID:                fmt.Sprintf("path-%d", time.Now().UnixNano()),
		Path:              cleanPath,
		IncludeSubfolders: true,
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
		IncludeSubfolders bool `json:"includeSubfolders"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	database.DB.Model(&models.ExtraScanPath{}).Where("id = ?", id).Update("include_subfolders", req.IncludeSubfolders)
	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

func DeleteScanPath(c *gin.Context) {
	id := c.Param("id")
	database.DB.Where("id = ?", id).Delete(&models.ExtraScanPath{})
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
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