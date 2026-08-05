package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"SakuHentai/internal/middleware"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// UserHandler 用户管理（仅管理员）
type UserHandler struct {
	db *gorm.DB
}

func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{db: db}
}

// ListUsers 成员列表（不含密码）
func (h *UserHandler) ListUsers(c *gin.Context) {
	var users []models.User
	if err := h.db.Order("id ASC").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询成员失败"})
		return
	}
	result := make([]gin.H, 0, len(users))
	for i := range users {
		result = append(result, userPublic(&users[i]))
	}
	c.JSON(http.StatusOK, gin.H{"users": result})
}

// CreateUser 新增成员（用户名/初始密码/角色/下载许可）
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req struct {
		Username      string `json:"username" binding:"required"`
		Password      string `json:"password" binding:"required"`
		Role          string `json:"role"`
		AllowDownload bool   `json:"allowDownload"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名与初始密码为必填"})
		return
	}
	role := req.Role
	if role != services.RoleAdmin && role != services.RoleMember {
		role = services.RoleMember
	}
	hash, err := services.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}
	user := models.User{
		Username:      req.Username,
		PasswordHash:  hash,
		Role:          role,
		AllowDownload: req.AllowDownload,
	}
	if err := h.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "用户名已存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "成员已创建", "user": userPublic(&user)})
}

// UpdateUser 修改用户名、角色、下载许可
func (h *UserHandler) UpdateUser(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var user models.User
	if err := h.db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	var req struct {
		Username      *string `json:"username"`
		Role          *string `json:"role"`
		AllowDownload *bool   `json:"allowDownload"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}
	// 禁止通过此接口把 admin 降级，防止误操作锁死系统
	if user.Role == services.RoleAdmin && req.Role != nil && *req.Role != services.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "不能修改 admin 的角色"})
		return
	}
	// 修改用户名：去空格 + 非空校验 + 唯一性校验（排除自身）
	if req.Username != nil {
		name := strings.TrimSpace(*req.Username)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "用户名不能为空"})
			return
		}
		var cnt int64
		if err := h.db.Model(&models.User{}).Where("username = ? AND id <> ?", name, user.ID).Count(&cnt).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户名失败"})
			return
		}
		if cnt > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "用户名已存在"})
			return
		}
		user.Username = name
	}
	if req.Role != nil && (*req.Role == services.RoleAdmin || *req.Role == services.RoleMember) {
		user.Role = *req.Role
	}
	if req.AllowDownload != nil {
		user.AllowDownload = *req.AllowDownload
	}
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "成员已更新", "user": userPublic(&user)})
}

// ResetPassword 重置密码
func (h *UserHandler) ResetPassword(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var user models.User
	if err := h.db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新密码为必填"})
		return
	}
	hash, err := services.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}
	user.PasswordHash = hash
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "密码已重置"})
}

// DeleteUser 删除成员（禁止删自己 / 禁止删 admin），并清理其个人数据
func (h *UserHandler) DeleteUser(c *gin.Context) {
	cur := middleware.CurrentUser(c)
	id, _ := strconv.Atoi(c.Param("id"))
	if cur != nil && cur.ID == uint(id) {
		c.JSON(http.StatusForbidden, gin.H{"error": "不能删除当前登录账号"})
		return
	}
	var user models.User
	if err := h.db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	if user.Role == services.RoleAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "不能删除管理员账号"})
		return
	}
	if err := h.db.Delete(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}
	// 清理该用户的会话与个人数据
	uid := uint(id)
	h.db.Delete(&models.UserSession{}, "user_id = ?", uid)
	h.db.Delete(&models.Bookshelf{}, "user_id = ?", uid)
	h.db.Delete(&models.HistoryRecord{}, "user_id = ?", uid)
	h.db.Delete(&models.ComicRating{}, "user_id = ?", uid)
	h.db.Delete(&models.ReadingList{}, "user_id = ?", uid)
	h.db.Delete(&models.FavoriteState{}, "user_id = ?", uid)
	h.db.Delete(&models.EHSetting{}, "user_id = ?", uid)
	h.db.Delete(&models.EHProfile{}, "user_id = ?", uid)
	c.JSON(http.StatusOK, gin.H{"message": "成员已删除"})
}
