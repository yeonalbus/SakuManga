package handlers

import (
	"SakuManga/internal/services"
	"net/http"
	"strconv"
	"strings"

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
	limitStr := c.DefaultQuery("limit", "20")
	limit, _ := strconv.Atoi(limitStr)
	// 联想词增多（配合前端可滚动下拉）：默认 20、上限 50，防异常大 limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	results := services.GlobalTagEngine.Suggest(q, limit)
	c.JSON(http.StatusOK, results)
}

// GetTagDictionary 获取完整翻译词典（性能优化：走 TagEngine 预序列化 + 预 gzip 压缩缓存，
// 零序列化/压缩开销；带强 ETag，浏览器二次访问 If-None-Match 命中直接 304）。
func GetTagDictionary(c *gin.Context) {
	plain, gz, etag := services.GlobalTagEngine.GetDictCache()
	if plain == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "标签词典未就绪，请稍后重试"})
		return
	}
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("ETag", etag)
	if c.GetHeader("If-None-Match") == etag {
		c.Status(http.StatusNotModified)
		return
	}
	// 客户端支持 gzip 且有预压缩缓存 → 直接返回压缩字节（gin-contrib/gzip 中间件
	// 检测到已有 Content-Encoding 会跳过，不双重压缩）
	if gz != nil && strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
		c.Header("Content-Encoding", "gzip")
		c.Data(http.StatusOK, "application/json; charset=utf-8", gz)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", plain)
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