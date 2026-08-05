package handlers

import (
	"net/http"

	"SakuHentai/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type EHSettingHandler struct {
	db *gorm.DB
}

func NewEHSettingHandler(db *gorm.DB) *EHSettingHandler {
	return &EHSettingHandler{db: db}
}

// GetEHSettings 获取 E 站偏好配置
func (h *EHSettingHandler) GetEHSettings(c *gin.Context) {
	var setting models.EHSetting
	
	// 仅按 ID=1 查询，不存在时才创建
	if err := h.db.First(&setting, 1).Error; err != nil {
		setting = models.EHSetting{
			ID:             1,
			Site:           "e-hentai",
			PreferRedirect: true,
		}
		if createErr := h.db.Create(&setting).Error; createErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "初始化设置失败: " + createErr.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, setting)
}

// SaveEHSettings 更新并保存 E 站偏好配置
func (h *EHSettingHandler) SaveEHSettings(c *gin.Context) {
	var setting models.EHSetting
	if err := c.ShouldBindJSON(&setting); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败: " + err.Error()})
		return
	}

	setting.ID = 1 // 锁定单例 ID

	if err := h.db.Save(&setting).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存 E 站设置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "设置更新成功", "data": setting})
}