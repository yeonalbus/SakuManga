// Package router 统一注册全部 API 路由
//
// 将路由配置从 main.go 中解耦：main 只负责初始化数据库、加载代理配置与启动服务，
// 所有 Handler/Service 的装配与路由注册集中在本包完成，便于统一维护与扩展。
package router

import (
	"SakuHentai/internal/handlers"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterRoutes 注册全部 API 路由
//
// 参数:
//   - r:        gin 引擎实例
//   - db:       数据库连接（已 AutoMigrate）
//   - ehService: E-Hentai 抓取服务（被账号/EH 设置/在线画廊等多个领域复用）
//
// 说明: 内部会根据需要创建 Toplist/Favorites 等服务与对应 Handler，
// 并在启动时装载默认账号以驱动榜单定时调度器。
func RegisterRoutes(r *gin.Engine, db *gorm.DB, ehService *services.EHService) {
	// ─── 1. 初始化各领域 Handler / Service ───
	accountHandler := handlers.NewAccountHandler(db, ehService)
	ehSettingHandler := handlers.NewEHSettingHandler(db, ehService)
	onlineHandler := handlers.NewOnlineComicHandler(db, ehService)

	// 下载任务管理器：启动 worker 池并按设置恢复未完成任务
	downloadManager := services.NewDownloadManager(db, ehService)
	downloadManager.Start()
	downloadHandler := handlers.NewDownloadHandler(db, ehService, downloadManager)
	offlineHandler := handlers.NewOfflineHandler(db, ehService, downloadManager)

	toplistService := services.NewToplistService(ehService)
	favService := services.NewFavoritesService(ehService)

	// 读取默认账号（id=1）并启动榜单定时调度器
	var account models.AccountSetting
	db.First(&account, 1)
	toplistService.StartScheduler(&account)

	toplistHandler := handlers.NewToplistHandler(db, toplistService)
	favHandler := handlers.NewFavoritesHandler(db, favService)

	// ─── 2. 注册 /api/v1 分组路由 ───
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

		// 🟢 统一代理配置接口 (使用重构的 services 联动 handler)
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
		api.GET("/comics/online/pages", onlineHandler.GetOnlineComicPages)
		api.GET("/comics/online/page", onlineHandler.GetOnlinePageByIndex)
		api.GET("/comics/online/toplist", toplistHandler.GetToplist)
		api.GET("/comics/online/favorites", favHandler.GetOnlineFavorites)
		api.POST("/comics/online/favorites/sort", favHandler.ChangeSortOrder)
		api.POST("/comics/online/favorite", favHandler.AddFavorite)
		api.DELETE("/comics/online/favorite", favHandler.RemoveFavorite)

		// 订阅界面
		api.GET("/online/watched", onlineHandler.GetWatchedComics)

		// 下载任务与 GP 面板
		api.POST("/downloads", downloadHandler.CreateDownload)
		api.GET("/downloads", downloadHandler.ListDownloads)
		api.GET("/downloads/gp-info", downloadHandler.GetGPInfo)
		api.GET("/downloads/settings", downloadHandler.GetDownloadSettings)
		api.POST("/downloads/settings", downloadHandler.SaveDownloadSettings)
		api.POST("/downloads/restore", downloadHandler.RestoreDownloads)
		api.GET("/downloads/:id", downloadHandler.GetDownload)
		api.POST("/downloads/:id/pause", downloadHandler.PauseDownload)
		api.POST("/downloads/:id/resume", downloadHandler.ResumeDownload)
		api.POST("/downloads/:id/cancel", downloadHandler.CancelDownload)
		api.POST("/downloads/:id/retry", downloadHandler.RetryDownload)
		api.POST("/downloads/:id/unlock", downloadHandler.UnlockDownload)

		// 离线更新检测 + 维护查重
		api.POST("/offline/updates/check", offlineHandler.CheckOfflineUpdates)
		api.GET("/offline/updates", offlineHandler.ListOfflineUpdates)
		api.POST("/offline/updates/download", offlineHandler.DownloadUpdate)
		api.GET("/offline/maintain", offlineHandler.GetMaintainDedup)
		api.POST("/offline/maintain/remove", offlineHandler.RemoveDedup)
	}
}
