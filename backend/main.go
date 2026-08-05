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

	// 启动时加载 config.json 中的代理配置
	services.InitProxyConfig()

	database.DB.AutoMigrate(&models.AccountSetting{})

	// 2. 初始化 Router
	r := gin.Default()
	r.Use(Cors())

	// 3. 初始化 Service 与 Handler
	ehService := services.NewEHService()
	accountHandler := handlers.NewAccountHandler(database.DB, ehService)
	ehSettingHandler := handlers.NewEHSettingHandler(database.DB, ehService)
	toplistService := services.NewToplistService(ehService)
	favService := services.NewFavoritesService(ehService)

	var account models.AccountSetting
	database.DB.First(&account, 1)
	toplistService.StartScheduler(&account)

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

		// 🟢 统一代理配置接口 (使用我们重构的 services 联动 handler)
		api.GET("/network/proxy", handlers.GetProxyHandler)
		api.POST("/network/proxy", handlers.SetProxyHandler)

		// E站账户与偏好设置
		api.GET("/account/settings", accountHandler.GetAccountSettings)
		api.POST("/account/settings", accountHandler.SaveAccountSettings)
		api.DELETE("/account/settings", accountHandler.ClearAccountSettings)

		// EH 专属站点偏好设置接口
		api.GET("/eh/settings", ehSettingHandler.GetEHSettings)
		api.POST("/eh/settings", ehSettingHandler.SaveEHSettings)

		// uconfig.php 代理：应用内读取/修改/保存 E 站配置（含 profile 管理）
		api.GET("/eh/uconfig", ehSettingHandler.GetUConfig)
		api.POST("/eh/uconfig", ehSettingHandler.SaveUConfig)

		// Profile 管理（兼容保留，站点配置请使用 uconfig 接口）
		api.GET("/eh/profiles", ehSettingHandler.GetProfiles)
		api.POST("/eh/profiles", ehSettingHandler.CreateProfile)
		api.PUT("/eh/profiles/:id", ehSettingHandler.UpdateProfile)
		api.DELETE("/eh/profiles/:id", ehSettingHandler.DeleteProfile)
		api.POST("/eh/profiles/:id/select", ehSettingHandler.SelectProfile)

		// 图片配额与资产（GP / Credits / Hath）
		api.GET("/eh/status", ehSettingHandler.GetEHUserStatus)

		// 我的标签（关注 / 隐藏，直连 E 站读取与上传）
		api.GET("/eh/mytags", ehSettingHandler.GetMyTags)
		api.POST("/eh/mytags", ehSettingHandler.AddMyTag)
		api.POST("/eh/mytags/remove", ehSettingHandler.RemoveMyTag)
		api.POST("/eh/mytags/tagset", ehSettingHandler.CreateMyTagset)

		// 在线画廊与封面代理
		api.GET("/comics/online", onlineHandler.GetOnlineComics)
		api.GET("/comics/cover-proxy", onlineHandler.ProxyCover)
		api.GET("/comics/online/popular", onlineHandler.GetOnlinePopular)
		api.GET("/comics/online/detail", onlineHandler.GetOnlineComicDetail)
		api.GET("/comics/online/previews", onlineHandler.GetOnlineComicPreviews)
		api.GET("/comics/online/toplist", toplistHandler.GetToplist)
		api.GET("/comics/online/favorites", favHandler.GetOnlineFavorites)
		api.POST("/comics/online/favorites/sort", favHandler.ChangeSortOrder)
		api.POST("/comics/online/favorite", favHandler.AddFavorite)
		api.DELETE("/comics/online/favorite", favHandler.RemoveFavorite)

		// 订阅界面
		api.GET("/online/watched", onlineHandler.GetWatchedComics)
	}

	// 显式指定监听双栈 / IPv4 0.0.0.0
	r.Run("0.0.0.0:8081")
}