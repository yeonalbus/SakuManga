package main

import (
	"net/http"

	"SakuHentai/internal/database"
	"SakuHentai/internal/handlers"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"

	"github.com/gin-gonic/gin"
)

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
	
	// 👈 注意：这里将原先未定义的 db 替换为 database.DB
	// (如果你的 database 包使用的是 database.GetDB()，请替换为 database.GetDB().AutoMigrate(...))
	database.DB.AutoMigrate(&models.AccountSetting{})

	// 2. 初始化 Router
	r := gin.Default()
	r.Use(Cors())

	// 3. 初始化 Service 与 Handler
	ehService := services.NewEHService()
	accountHandler := handlers.NewAccountHandler(database.DB, ehService)
	toplistService := services.NewToplistService(ehService)
	favService := services.NewFavoritesService(ehService)

	var account models.AccountSetting
	database.DB.First(&account, 1)
	toplistService.StartScheduler(&account) // 👈 启动 0 点定时器

	onlineHandler := handlers.NewOnlineComicHandler(database.DB, ehService)
	toplistHandler := handlers.NewToplistHandler(database.DB, toplistService)
	favHandler := handlers.NewFavoritesHandler(database.DB, favService)

	api := r.Group("/api/v1")
	{
		// 路径管理
		api.GET("/scan-paths", handlers.GetScanPaths)
		api.POST("/scan-paths", handlers.AddScanPath)
		api.PUT("/scan-paths/:id", handlers.UpdateScanPath)
		api.DELETE("/scan-paths/:id", handlers.DeleteScanPath)
		api.POST("/scan-paths/:id/scan", handlers.TriggerScanPath)

		// 漫画数据与封面
		api.GET("/comics/offline", handlers.GetOfflineComics)
		api.GET("/comics/:id/cover", handlers.GetComicCover)
		api.GET("/comics/:id", handlers.GetComicDetail)

		// 阅读器
		api.GET("/comics/:id/pages", handlers.GetComicPages)
		api.GET("/comics/:id/page/:index", handlers.GetComicPageImage)

		// 标签 API
		api.GET("/tags/status", handlers.GetTagEngineStatus)
		api.POST("/tags/sync/translation", handlers.SyncTagTranslation)
		api.POST("/tags/sync/count", handlers.SyncTagCount)
		api.GET("/tags/suggest", handlers.QueryTagSuggestions)
		api.GET("/tags/progress", handlers.GetTagProgress)
		api.GET("/tags/dictionary", handlers.GetTagDictionary)
		
		// 代理
		api.GET("/network/proxy", handlers.GetProxyConfig)
		api.POST("/network/proxy", handlers.SetProxyConfig)

		// E站账户
		api.GET("/account/settings", accountHandler.GetAccountSettings)
		api.POST("/account/settings", accountHandler.SaveAccountSettings)
		api.DELETE("/account/settings", accountHandler.ClearAccountSettings)

		// 在线画廊与封面代理
		api.GET("/comics/online", onlineHandler.GetOnlineComics)
		api.GET("/comics/cover-proxy", onlineHandler.ProxyCover)
		api.GET("/comics/online/popular", onlineHandler.GetOnlinePopular)
		api.GET("/comics/online/detail", onlineHandler.GetOnlineComicDetail)
		api.GET("/comics/online/toplist", toplistHandler.GetToplist)
		api.GET("/comics/online/favorites", favHandler.GetOnlineFavorites)
		api.POST("/comics/online/favorite", favHandler.AddFavorite)
    	api.DELETE("/comics/online/favorite", favHandler.RemoveFavorite)
	}

	r.Run(":8080")
}