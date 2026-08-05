package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
)

type AccountHandler struct {
	db        *gorm.DB
	ehService *services.EHService
}

func NewAccountHandler(db *gorm.DB, ehService *services.EHService) *AccountHandler {
	return &AccountHandler{db: db, ehService: ehService}
}

func (h *AccountHandler) GetAccountSettings(c *gin.Context) {
	var setting models.AccountSetting
	if err := h.db.First(&setting, 1).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"isLoggedIn": false,
			"data":       nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"isLoggedIn": true,
		"data": gin.H{
			"ipb_member_id": setting.IPBMemberID,
			"igneous":       setting.Igneous,
			"sk":            setting.SK,
			"isEx":          setting.IsEx,
			"updatedAt":     setting.UpdatedAt,
		},
	})
}

func (h *AccountHandler) SaveAccountSettings(c *gin.Context) {
	var req struct {
		IPBMemberID string `json:"ipb_member_id" binding:"required"`
		IPBPassHash string `json:"ipb_pass_hash" binding:"required"`
		Igneous     string `json:"igneous"`
		SK          string `json:"sk"` // 🟢 sk 会话 Cookie：E 站首页实时性的关键（缺少时返回约 2h 前的缓存）
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失，ipb_member_id 与 ipb_pass_hash 为必填"})
		return
	}

	setting := models.AccountSetting{
		ID:          1,
		IPBMemberID: req.IPBMemberID,
		IPBPassHash: req.IPBPassHash,
		Igneous:     req.Igneous,
		SK:          req.SK,
		UpdatedAt:   time.Now(),
	}

	// 校验并尝试自动抓取 igneous
	isEx, err := h.ehService.VerifyAccount(&setting)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	setting.IsEx = isEx

	// 存入数据库（包含自动填充的 Igneous）
	if err := h.db.Save(&setting).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存凭证到数据库失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "凭证校验并保存成功",
		"isEx":    isEx,
		"data": gin.H{
			"ipb_member_id": setting.IPBMemberID,
			"igneous":       setting.Igneous, // 返回给前端自动填充的 igneous
			"isEx":          setting.IsEx,
		},
	})
}

func (h *AccountHandler) ClearAccountSettings(c *gin.Context) {
	h.db.Delete(&models.AccountSetting{}, 1)
	c.JSON(http.StatusOK, gin.H{"message": "凭证已清除"})
}