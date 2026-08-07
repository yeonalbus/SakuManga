// Package router 统一注册全部 API 路由
//
// 将路由配置从 main.go 中解耦：main 只负责初始化数据库、加载代理配置与启动服务，
// 所有 Handler/Service 的装配与路由注册集中在本包完成，便于统一维护与扩展。
package router

import (
	"SakuHentai/internal/handlers"
	"SakuHentai/internal/middleware"
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
// 并在启动时装载 admin 账号以驱动榜单定时调度器。
func RegisterRoutes(r *gin.Engine, db *gorm.DB, ehService *services.EHService) {
	// ─── 1. 初始化各领域 Handler / Service ───
	authService := services.NewAuthService(db)
	authHandler := handlers.NewAuthHandler(db, authService)
	userHandler := handlers.NewUserHandler(db)
	serverHandler := handlers.NewServerHandler(db)

	accountHandler := handlers.NewAccountHandler(db, ehService)
	ehSettingHandler := handlers.NewEHSettingHandler(db, ehService)
	onlineHandler := handlers.NewOnlineComicHandler(db, ehService)
	libraryHandler := handlers.NewLibraryHandler(db)

	// 下载任务管理器：启动 worker 池并按设置恢复未完成任务
	downloadManager := services.NewDownloadManager(db, ehService)
	downloadManager.Start()
	downloadHandler := handlers.NewDownloadHandler(db, ehService, downloadManager)
	offlineHandler := handlers.NewOfflineHandler(db, ehService, downloadManager)

	toplistService := services.NewToplistService(ehService)
	favService := services.NewFavoritesService(ehService)

	// Tag 维护服务：启动调度器（启动时执行旧数据迁移，其后按东八区每日/每周触发）
	tagMaintainService := services.NewTagMaintainService(db, ehService)
	services.StartTagMaintainScheduler(db, tagMaintainService)
	tagMaintainHandler := handlers.NewTagMaintainHandler(db, tagMaintainService)

	// 每周自动更新扫描（Round4 任务四：周扫描 + Aged Status）：
	// 按设置周期性运行「更新检测 + 老化判定」，成功后自动入队新版本下载（autoUpdateGallery）
	updateScanHandler := handlers.NewUpdateScanHandler(db)
	services.StartUpdateScanScheduler(db, ehService, downloadManager)

	// 装载 admin 账号并启动榜单定时调度器（后台维护任务固定用 admin）
	toplistService.StartScheduler(services.LoadAdminAccount(db))

	toplistHandler := handlers.NewToplistHandler(db, toplistService)
	favHandler := handlers.NewFavoritesHandler(db, favService)

	// ─── 2. 公开路由（无需登录）───
	public := r.Group("/api/v1")
	{
		public.POST("/auth/login", authHandler.Login)

		// 封面/页图代理：浏览器 <img> 媒体加载无法携带 Authorization 头，故作公开路由，
		// 由 handler 内部做可选认证（优先当前用户凭证，未登录回退 admin 凭证代理图片）
		public.GET("/comics/cover-proxy", onlineHandler.ProxyCover)

		// 离线封面/页图：与在线 cover-proxy 同理，浏览器 <img> 无法携带 Authorization 头，
		// 故作公开路由（离线画廊数据本身为全局共享，无用户隔离）。
		public.GET("/comics/:id/cover", handlers.GetComicCover)
		public.GET("/comics/:id/pages", handlers.GetComicPages)
		public.GET("/comics/:id/page/:index", handlers.GetComicPageImage)
	}

	// ─── 3. 受保护路由（需登录）───
	api := r.Group("/api/v1")
	api.Use(middleware.AuthRequired(db))
	{
		api.POST("/auth/logout", authHandler.Logout)
		api.GET("/auth/me", authHandler.Me)

		// 路径管理（读写分离：扫描路径增删改/触发扫描为系统级写操作，仅管理员，见下方 admin 分组）
		api.GET("/scan-paths", handlers.GetScanPaths)
		api.GET("/scan-paths/:id/scan/progress", handlers.GetScanPathProgress)
		api.GET("/scan-paths/scan-progress", handlers.GetAllScanProgress)

		// 漫画数据与封面（封面/页图为公开路由，见上方 public 分组）
		api.GET("/comics/offline", handlers.GetOfflineComics)
		api.GET("/comics/:id", handlers.GetComicDetail)
		// 删除本地画廊为系统级写操作（仅管理员），见下方 admin 分组
		// 阅读次数上报（排行榜持久化，问题9）
		api.POST("/comics/:id/click", handlers.RecordComicClick)

		// 标签 API（只读查询开放给所有用户；数据同步为管理员）
		api.GET("/tags/status", handlers.GetTagEngineStatus)
		api.GET("/tags/suggest", handlers.QueryTagSuggestions)
		api.GET("/tags/dictionary", handlers.GetTagDictionary)

		// 前端错误日志上报（问题8 诊断辅助：浏览器无法写文件，由后端落盘到 logs/client.log）
		api.POST("/client/log", handlers.ReportClientLog)
		// 日志大小查询（清除为系统级写操作，仅管理员，见下方 admin 分组）
		api.GET("/client/log/size", handlers.GetClientLogSize)

		// 四类系统日志：查询 / 实时监控 / 开关设置读取（清理与设置保存为系统级写操作，仅管理员，见下方 admin 分组）
		api.GET("/logs/categories", handlers.GetLogCategories)
		api.GET("/logs/query", handlers.QueryLogs)
		api.GET("/logs/tail", handlers.TailLogs)
		api.GET("/logs/settings", handlers.GetLogSettings)

		// E站账户与偏好设置（绑定当前登录用户自己的 E 站凭证）
		api.GET("/account/settings", accountHandler.GetAccountSettings)
		api.POST("/account/settings", accountHandler.SaveAccountSettings)
		api.DELETE("/account/settings", accountHandler.ClearAccountSettings)

		// EH 专属站点偏好设置接口（按当前用户隔离）
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
		// 🎲 随机抽卡（离线 SQL RANDOM + 在线随机页采样）
		api.GET("/comics/random", onlineHandler.GetRandomComics)
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

		// 书架（按用户隔离）
		api.GET("/bookshelves", libraryHandler.GetBookshelves)
		api.POST("/bookshelves", libraryHandler.CreateBookshelf)
		api.PUT("/bookshelves/:id", libraryHandler.UpdateBookshelf)
		api.DELETE("/bookshelves/:id", libraryHandler.DeleteBookshelf)
		api.POST("/bookshelves/:id/comics", libraryHandler.AddComicToBookshelf)
		api.DELETE("/bookshelves/:id/comics", libraryHandler.RemoveComicFromBookshelf)

		// 历史（按用户隔离 + 上限淘汰）
		api.GET("/history", libraryHandler.GetHistory)
		api.POST("/history", libraryHandler.AddHistory)
		api.DELETE("/history", libraryHandler.ClearHistory)
		api.DELETE("/history/:id", libraryHandler.DeleteHistory)

		// 个人评分（按用户隔离）
		api.GET("/ratings", libraryHandler.GetRatings)
		api.GET("/ratings/:comicId", libraryHandler.GetComicRating)
		api.PUT("/ratings/:comicId", libraryHandler.SetComicRating)
		api.DELETE("/ratings/:comicId", libraryHandler.DeleteComicRating)

		// 阅读清单（每用户每来源一个队列）
		api.GET("/reading-list", libraryHandler.GetReadingList)
		api.PUT("/reading-list", libraryHandler.SaveReadingList)

		// 下载任务与 GP 面板（许可校验在 handler 内：admin 或 allowDownload）
		api.POST("/downloads", downloadHandler.CreateDownload)
		api.GET("/downloads", downloadHandler.ListDownloads)
		api.POST("/downloads/batch", downloadHandler.BatchCreateDownload)
		api.GET("/downloads/gp-info", downloadHandler.GetGPInfo)
		api.GET("/downloads/settings", downloadHandler.GetDownloadSettings)
		// POST /downloads/settings 保存与 POST /downloads/restore 恢复为系统级操作，见下方 admin 分组
		api.GET("/downloads/:id", downloadHandler.GetDownload)
		api.POST("/downloads/:id/pause", downloadHandler.PauseDownload)
		api.POST("/downloads/:id/resume", downloadHandler.ResumeDownload)
		api.POST("/downloads/:id/cancel", downloadHandler.CancelDownload)
		api.POST("/downloads/:id/retry", downloadHandler.RetryDownload)
		api.POST("/downloads/:id/unlock", downloadHandler.UnlockDownload)
		api.POST("/downloads/:id/priority", downloadHandler.SetDownloadPriority)

		// ─── 3.1 管理员分组（用户管理 / 服务器 / 系统级设置 / 离线更新维护）───
		// Round3-任务2：离线更新检测 + 维护查重从登录组移入仅管理员分组
		admin := api.Group("")
		admin.Use(middleware.AdminOnly())
		{
			// 系统级写操作（中心制：仅管理员可修改数据库 / 系统配置 / 清理日志）
			admin.DELETE("/comics/:id", handlers.DeleteOfflineComic) // 删除本地画廊（记录 + 可选物理文件）
			admin.POST("/scan-paths", handlers.AddScanPath)
			admin.PUT("/scan-paths/:id", handlers.UpdateScanPath)
			admin.DELETE("/scan-paths/:id", handlers.DeleteScanPath)
			admin.POST("/scan-paths/:id/scan", handlers.TriggerScanPath)
			admin.DELETE("/client/log", handlers.ClearClientLog)
			admin.DELETE("/logs", handlers.DeleteLogs)
			admin.POST("/logs/settings", handlers.SaveLogSettings)
			admin.POST("/downloads/settings", downloadHandler.SaveDownloadSettings)
			admin.POST("/downloads/restore", downloadHandler.RestoreDownloads)

			// 用户管理
			admin.GET("/users", userHandler.ListUsers)
			admin.POST("/users", userHandler.CreateUser)
			admin.PUT("/users/:id", userHandler.UpdateUser)
			admin.PUT("/users/:id/password", userHandler.ResetPassword)
			admin.DELETE("/users/:id", userHandler.DeleteUser)

			// 服务器与存储配置
			admin.GET("/server/setting", serverHandler.GetServerSetting)
			admin.POST("/server/setting", serverHandler.SaveServerSetting)

			// 统一代理配置（系统级，仅管理员）
			admin.GET("/network/proxy", handlers.GetProxyHandler)
			admin.POST("/network/proxy", handlers.SetProxyHandler)

			// Tag 引擎数据同步（系统级，仅管理员）
			admin.POST("/tags/sync/translation", handlers.SyncTagTranslation)
			admin.POST("/tags/sync/count", handlers.SyncTagCount)
			admin.GET("/tags/progress", handlers.GetTagProgress)

			// 🏷️ Tag 维护（双轨三态：设置 / 手动刷新 / 手动写回 / 进度轮询）
			admin.GET("/offline/tags/setting", tagMaintainHandler.GetSetting)
			admin.POST("/offline/tags/setting", tagMaintainHandler.SaveSetting)
			admin.POST("/offline/tags/refresh", tagMaintainHandler.RefreshTags)
			admin.POST("/offline/tags/writeback", tagMaintainHandler.Writeback)
			admin.GET("/offline/tags/progress", tagMaintainHandler.GetProgress)

			// 单本 tag 增删落库（详情页编辑，仅管理员可改本地 Tag）
			admin.PUT("/comics/:id/tags", tagMaintainHandler.EditComicTags)

			// 管理员查看成员历史（可按 userId / source 过滤）
			admin.GET("/admin/history", libraryHandler.AdminGetHistory)

			// 离线更新检测 + 维护查重（Round3-任务2：由登录组移入仅管理员）
			admin.POST("/offline/updates/check", offlineHandler.CheckOfflineUpdates)
			admin.GET("/offline/updates/check/progress", offlineHandler.GetCheckUpdatesProgress)
			admin.GET("/offline/updates/check/result", offlineHandler.GetCheckUpdatesResult)
			admin.GET("/offline/updates", offlineHandler.ListOfflineUpdates)
			admin.POST("/offline/updates/download", offlineHandler.DownloadUpdate)
			// 需求 3(2)：画廊被删/移除项「移出更新列表」（仅清标记，保留本地文件）
			admin.POST("/offline/updates/:id/dismiss", offlineHandler.DismissOfflineUpdate)

			// 每周自动更新扫描设置（Round4 任务四：周扫描 + Aged Status）
			admin.GET("/offline/update-scan/setting", updateScanHandler.GetSetting)
			admin.POST("/offline/update-scan/setting", updateScanHandler.SaveSetting)
			admin.GET("/offline/maintain", offlineHandler.GetMaintainDedup)
			admin.GET("/offline/maintain/progress", offlineHandler.GetMaintainProgress)
			admin.GET("/offline/maintain/result", offlineHandler.GetMaintainResult)
			admin.GET("/offline/maintain/unsynced", offlineHandler.GetMaintainUnsynced)
			admin.POST("/offline/maintain/remove", offlineHandler.RemoveDedup)
		}
	}
}
