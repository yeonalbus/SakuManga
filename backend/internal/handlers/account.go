package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuHentai/internal/middleware"
	"SakuHentai/internal/services"
)

type AccountHandler struct {
	db        *gorm.DB
	ehService *services.EHService
}

func NewAccountHandler(db *gorm.DB, ehService *services.EHService) *AccountHandler {
	return &AccountHandler{db: db, ehService: ehService}
}

// GetAccountSettings 返回当前登录用户的 E 站凭证状态
func (h *AccountHandler) GetAccountSettings(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	if user.IPBMemberID == "" {
		c.JSON(http.StatusOK, gin.H{
			"isLoggedIn": false,
			"data":       nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"isLoggedIn": true,
		"data": gin.H{
			"ipb_member_id": user.IPBMemberID,
			"igneous":       user.Igneous,
			"sk":            user.SK,
			"isEx":          user.IsEx,
			"updatedAt":     user.UpdatedAt,
		},
	})
}

// SaveAccountSettings 校验并保存当前登录用户的 E 站凭证到 User 表
func (h *AccountHandler) SaveAccountSettings(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

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

	account := middleware.CurrentAccount(c)
	account.IPBMemberID = req.IPBMemberID
	account.IPBPassHash = req.IPBPassHash
	account.Igneous = req.Igneous
	account.SK = req.SK

	// 校验并尝试自动抓取 igneous
	isEx, err := h.ehService.VerifyAccount(account)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 写回当前用户的 User 记录
	user.IPBMemberID = account.IPBMemberID
	user.IPBPassHash = account.IPBPassHash
	user.Igneous = account.Igneous
	user.SK = account.SK
	user.IsEx = isEx
	user.UpdatedAt = time.Now()
	if err := h.db.Save(user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存凭证到数据库失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "凭证校验并保存成功",
		"isEx":    isEx,
		"data": gin.H{
			"ipb_member_id": user.IPBMemberID,
			"igneous":       user.Igneous, // 返回给前端自动填充的 igneous
			"isEx":          user.IsEx,
		},
	})
}

// ClearAccountSettings 清除当前登录用户的 E 站凭证
func (h *AccountHandler) ClearAccountSettings(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	user.IPBMemberID = ""
	user.IPBPassHash = ""
	user.Igneous = ""
	user.SK = ""
	user.IsEx = false
	user.UpdatedAt = time.Now()
	if err := h.db.Save(user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "清除凭证失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "凭证已清除"})
}
