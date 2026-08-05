package main

import (
	"net/http"

	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"SakuHentai/internal/router"
	"SakuHentai/internal/services"

	"github.com/gin-gonic/gin"
)

// Cors 跨域中间件：允许前端开发服务器跨域访问后端 API
func Cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func main() {
	// 1. 初始化数据库
	database.InitDB()

	// 启动时加载 config.json 中的代理配置
	services.InitProxyConfig()

	// 启动标签引擎：加载本地翻译/热度数据，若缺失或非最新则自动下载（含 24 小时自动更新周期）
	services.InitTagEngine()

	database.DB.AutoMigrate(&models.AccountSetting{})

	// 2. 初始化 Router 并挂载中间件
	r := gin.Default()
	r.Use(Cors())

	// 3. 初始化 E-Hentai 抓取服务，并注册全部 API 路由（路由配置见 internal/router）
	ehService := services.NewEHService()
	router.RegisterRoutes(r, database.DB, ehService)

	// 4. 显式指定监听双栈 / IPv4 0.0.0.0
	r.Run("0.0.0.0:8081")
}
