package handlers

import (
	"net/http"

	"SakuHentai/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ServerHandler 服务器与存储配置（监听地址 / 端口 / 历史上限）
type ServerHandler struct {
	db *gorm.DB
}

func NewServerHandler(db *gorm.DB) *ServerHandler {
	return &ServerHandler{db: db}
}

// getOrCreateServerSetting 获取或初始化服务器配置单例（ID=1）
func getOrCreateServerSetting(db *gorm.DB) *models.ServerSetting {
	var s models.ServerSetting
	if err := db.First(&s, 1).Error; err != nil {
		s = models.ServerSetting{ID: 1, BindHost: "0.0.0.0", Port: 8081, HistoryLimit: 200}
		db.Create(&s)
	}
	return &s
}

func (h *ServerHandler) GetServerSetting(c *gin.Context) {
	s := getOrCreateServerSetting(h.db)
	c.JSON(http.StatusOK, gin.H{"setting": s})
}

func (h *ServerHandler) SaveServerSetting(c *gin.Context) {
	s := getOrCreateServerSetting(h.db)
	var req struct {
		BindHost     string `json:"bindHost"`
		Port         int    `json:"port"`
		HistoryLimit int    `json:"historyLimit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}
	if req.BindHost != "" {
		s.BindHost = req.BindHost
	}
	if req.Port > 0 && req.Port <= 65535 {
		s.Port = req.Port
	}
	if req.HistoryLimit > 0 {
		s.HistoryLimit = req.HistoryLimit
	}
	if err := h.db.Save(s).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "服务器配置已保存，重启服务后生效", "setting": s})
}
