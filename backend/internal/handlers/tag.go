package handlers

import (
	"SakuHentai/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetTagEngineStatus 获取当前标签引擎配置、版本与自动更新周期
func GetTagEngineStatus(c *gin.Context) {
	cnVer, sortVer := services.GlobalTagEngine.GetVersions()
	c.JSON(http.StatusOK, gin.H{
		"enableCN":            services.GlobalTagEngine.EnableCN,
		"tagCNVersion":        cnVer,
		"enableSort":          services.GlobalTagEngine.EnableSort,
		"tagSortVersion":      sortVer,
		"updateCycleHours":    services.TagUpdateIntervalHours,
	})
}

// SyncTagTranslation 触发同步翻译库
func SyncTagTranslation(c *gin.Context) {
	go services.GlobalTagEngine.UpdateTranslation()
	c.JSON(http.StatusOK, gin.H{"message": "同步指令已下发，后台下载中..."})
}

// SyncTagCount 触发同步热度库
func SyncTagCount(c *gin.Context) {
	go services.GlobalTagEngine.UpdateCountData()
	c.JSON(http.StatusOK, gin.H{"message": "同步指令已下发，后台下载中..."})
}

// QueryTagSuggestions 搜索补全联想 API
func QueryTagSuggestions(c *gin.Context) {
	q := c.Query("q")
	limitStr := c.DefaultQuery("limit", "10")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 10
	}

	results := services.GlobalTagEngine.Suggest(q, limit)
	c.JSON(http.StatusOK, results)
}

func GetTagDictionary(c *gin.Context) {
	c.JSON(http.StatusOK, services.GlobalTagEngine.GetTagList())
}

// GetTagProgress 获取下载进度状态
func GetTagProgress(c *gin.Context) {
	transProgress, sortProgress := services.GlobalTagEngine.GetProgress()

	c.JSON(http.StatusOK, gin.H{
		"transProgress": transProgress,
		"sortProgress":  sortProgress,
	})
}

// GetProxyConfig 获取代理设置
func GetProxyConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"proxy": services.GetGlobalProxy(),
	})
}

// SetProxyConfig 更新代理设置
func SetProxyConfig(c *gin.Context) {
	var req struct {
		Proxy string `json:"proxy"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的参数"})
		return
	}

	if err := services.SetGlobalProxy(req.Proxy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "代理地址格式不正确: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "代理地址更新成功",
		"proxy":   services.GetGlobalProxy(),
	})
}