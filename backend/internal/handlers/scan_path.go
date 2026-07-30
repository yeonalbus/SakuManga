package handlers

import (
	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
	"net/http"
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

	newPath := models.ExtraScanPath{
		ID:                "path-" + time.Now().Format("20060102150405"),
		Path:              req.Path,
		IncludeSubfolders: true,
	}

	if err := database.DB.Create(&newPath).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该路径已存在"})
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

func TriggerScanPath(c *gin.Context) {
	id := c.Param("id")
	var targetPath models.ExtraScanPath
	if err := database.DB.First(&targetPath, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到该路径记录"})
		return
	}

	count, err := services.ScanAndSaveDirectory(targetPath.Path, targetPath.IncludeSubfolders)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "扫描路径失败: " + err.Error()})
		return
	}

	now := time.Now().UnixMilli()
	database.DB.Model(&targetPath).Updates(map[string]interface{}{
		"last_scanned": now,
		"comic_count":  count,
	})

	c.JSON(http.StatusOK, gin.H{
		"lastScanned": now,
		"comicCount":  count,
	})
}