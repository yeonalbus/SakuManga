package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuManga/internal/middleware"
	"SakuManga/internal/services"
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
			"ipb_pass_hash": user.IPBPassHash, // 明文回显（需求：编辑弹窗/主界面展示，改 sk/igneous 时无需重填）
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
		IPBPassHash string `json:"ipb_pass_hash"` // 留空表示保持现有 Hash 不变（编辑弹窗已回显，无需重填）
		Igneous     string `json:"igneous"`
		SK          string `json:"sk"` // 🟢 sk 会话 Cookie：E 站首页实时性的关键（缺少时返回约 2h 前的缓存）
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失，ipb_member_id 为必填"})
		return
	}

	account := middleware.CurrentAccount(c)
	account.IPBMemberID = req.IPBMemberID
	if req.IPBPassHash != "" {
		account.IPBPassHash = req.IPBPassHash
	}
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

// RefreshCookies 使用当前（或请求内传入的）凭证刷新 sk / igneous 并返回，不落库。
// 供「更新 Cookie 凭证」弹窗的「刷新凭证」按钮调用：前端拿到新值回填表单，确认后再保存。
func (h *AccountHandler) RefreshCookies(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 表单里可能已修改 member_id/pass_hash 尚未保存：优先用请求内传入的最新值
	var req struct {
		IPBMemberID string `json:"ipb_member_id"`
		IPBPassHash string `json:"ipb_pass_hash"`
	}
	_ = c.ShouldBindJSON(&req)

	account := middleware.CurrentAccount(c)
	if req.IPBMemberID != "" {
		account.IPBMemberID = req.IPBMemberID
	}
	if req.IPBPassHash != "" {
		account.IPBPassHash = req.IPBPassHash
	}
	if account.IPBMemberID == "" || account.IPBPassHash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 ipb_member_id / ipb_pass_hash，无法刷新凭证"})
		return
	}

	igneous, sk, err := h.ehService.RefreshCookies(account)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if igneous == "" && sk == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "刷新失败，未能获取到新凭证（凭证可能已过期或网络异常）"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"igneous": igneous, "sk": sk})
}

// LoginWithPassword 使用 E 站账号密码内部登录（免 F12 复制 Cookie），
// 成功后将凭证保存到当前用户，失败（验证码/风控）返回错误由前端引导手动粘贴。
func (h *AccountHandler) LoginWithPassword(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入 E 站账号与密码"})
		return
	}

	result, err := h.ehService.LoginWithPassword(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 落库到当前用户
	user.IPBMemberID = result.IPBMemberID
	user.IPBPassHash = result.IPBPassHash
	user.Igneous = result.Igneous
	user.SK = result.SK
	user.IsEx = result.IsEx
	user.UpdatedAt = time.Now()
	if err := h.db.Save(user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存凭证到数据库失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "登录成功，E 站凭证已保存",
		"isEx":    result.IsEx,
		"data": gin.H{
			"ipb_member_id": user.IPBMemberID,
			"igneous":       user.Igneous,
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
