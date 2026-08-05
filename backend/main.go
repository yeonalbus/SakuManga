package main

import (
	"fmt"
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

	// 2. 确保初始管理员存在（users 表为空时创建 admin/admin123，E 站凭证置空，日志打印账密）
	if err := services.EnsureInitialAdmin(database.DB); err != nil {
		panic("创建初始管理员失败: " + err.Error())
	}

	// 3. 初始化 Router 并挂载中间件
	r := gin.Default()
	r.Use(Cors())

	// 4. 初始化 E-Hentai 抓取服务，并注册全部 API 路由（路由配置见 internal/router）
	ehService := services.NewEHService()
	router.RegisterRoutes(r, database.DB, ehService)

	// 5. 读取服务器配置（监听地址 + 端口），无记录则用默认值 0.0.0.0:8081
	var setting models.ServerSetting
	if err := database.DB.First(&setting, 1).Error; err != nil {
		setting = models.ServerSetting{ID: 1, BindHost: "0.0.0.0", Port: 8081, HistoryLimit: 200}
	}
	addr := fmt.Sprintf("%s:%d", setting.BindHost, setting.Port)
	r.Run(addr)
}
