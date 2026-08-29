package handlers

import (
	"net/http"

	"SakuManga/internal/services" // 👈 引入 services

	"github.com/gin-gonic/gin"
)

// GetProxyHandler GET /api/v1/network/proxy
// 返回手动配置（proxy）、实际生效代理（effective，手动优先、系统兜底）与来源（manual/system/none）
func GetProxyHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"proxy":     services.GetProxyURL(),
		"effective": services.GetEffectiveProxy(),
		"source":    services.GetProxySource(),
	})
}

type SetProxyReq struct {
	Proxy string `json:"proxy"`
}

// SetProxyHandler POST /api/v1/network/proxy
func SetProxyHandler(c *gin.Context) {
	var req SetProxyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败"})
		return
	}

	if err := services.SetProxyURL(req.Proxy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "代理设置已更新",
		"proxy":   req.Proxy,
	})
}