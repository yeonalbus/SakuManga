package handlers

import (
	"net/http"

	"SakuHentai/internal/middleware"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuthHandler 认证：登录 / 登出 / 当前用户
type AuthHandler struct {
	db          *gorm.DB
	authService *services.AuthService
}

func NewAuthHandler(db *gorm.DB, authService *services.AuthService) *AuthHandler {
	return &AuthHandler{db: db, authService: authService}
}

// userPublic 返回给前端的用户信息（不含密码哈希）
func userPublic(u *models.User) gin.H {
	return gin.H{
		"id":            u.ID,
		"username":      u.Username,
		"role":          u.Role,
		"allowDownload": u.AllowDownload,
		"isEx":          u.IsEx,
		"ipb_member_id": u.IPBMemberID,
		"createdAt":     u.CreatedAt,
	}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名与密码为必填"})
		return
	}
	token, user, err := h.authService.Login(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "user": userPublic(user)})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	token := middleware.CurrentToken(c)
	if token != "" {
		h.authService.Logout(token)
	}
	c.JSON(http.StatusOK, gin.H{"message": "已退出登录"})
}

func (h *AuthHandler) Me(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": userPublic(user)})
}
