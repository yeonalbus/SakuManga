// Package middleware 提供认证与权限中间件
package middleware

import (
	"net/http"
	"strings"

	"SakuHentai/internal/models"
	"SakuHentai/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ContextUserKey 存放当前登录用户的 gin.Context 键
const ContextUserKey = "currentUser"

// ContextTokenKey 存放当前请求的会话 token 的 gin.Context 键
const ContextTokenKey = "currentToken"

// AuthRequired 解析 Authorization: Bearer <token>，加载用户并注入 context
func AuthRequired(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		token := ""
		if strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		}
		// 兜底：浏览器 <img>/<video>/<audio> 等媒体加载无法携带 Authorization 头，
		// 允许通过 query 传递 token（封面代理等场景）
		if token == "" {
			token = c.Query("token")
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		var session models.UserSession
		if err := db.Where("token = ?", token).First(&session).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "会话已失效，请重新登录"})
			return
		}
		var user models.User
		if err := db.First(&user, session.UserID).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
			return
		}
		c.Set(ContextTokenKey, token)
		c.Set(ContextUserKey, &user)
		c.Next()
	}
}

// AdminOnly 校验当前用户为管理员（须挂载在 AuthRequired 之后）
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := c.Get(ContextUserKey)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		u, _ := user.(*models.User)
		if u == nil || u.Role != services.RoleAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "仅管理员可执行此操作"})
			return
		}
		c.Next()
	}
}

// CurrentUser 从 context 中取出当前登录用户（未登录返回 nil）
func CurrentUser(c *gin.Context) *models.User {
	v, ok := c.Get(ContextUserKey)
	if !ok {
		return nil
	}
	u, _ := v.(*models.User)
	return u
}

// CurrentAccount 从当前登录用户构造 AccountSetting（handler 层快速取用 E 站凭证）
// 用户未登录时返回 nil；凭证未绑定则返回一个空账号（由调用方自行判定 IPBMemberID==""）
func CurrentAccount(c *gin.Context) *models.AccountSetting {
	u := CurrentUser(c)
	if u == nil {
		return nil
	}
	return &models.AccountSetting{
		ID:          u.ID,
		IPBMemberID: u.IPBMemberID,
		IPBPassHash: u.IPBPassHash,
		Igneous:     u.Igneous,
		SK:          u.SK,
		IsEx:        u.IsEx,
	}
}

// CurrentToken 从 context 中取出当前会话 token（未登录返回空串）
func CurrentToken(c *gin.Context) string {
	v, ok := c.Get(ContextTokenKey)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
