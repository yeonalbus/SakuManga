package handlers

import (
	"net/http"

	"SakuManga/internal/services"
	"SakuManga/internal/version"

	"github.com/gin-gonic/gin"
)

// GetSystemVersion 返回服务端版本与构建标识 GET /api/v1/system/version
// 供前端「关于」页展示与部署后新旧版本核对（Round24）。
// 构建标识格式：YYYYMMDDHHMM-g<git短哈希>（由 build-release.bat 注入）；未注入为 "dev"。
func GetSystemVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"appVersion": version.AppVersion, // 产品版本（release 语义）
		"build":      version.Build,      // 构建标识（时间戳 + git 短哈希）
	})
}

// CheckUpdate 检测 GitHub 最新 release 版本 GET /api/v1/system/check-update?current=2.0.1
// 供设置「关于软件 → 版本」红点提醒使用；current 缺省用后端 AppVersion。
// 检测失败（网络/GitHub 不可达）返回 ok=false，前端静默处理。
func CheckUpdate(c *gin.Context) {
	current := c.DefaultQuery("current", version.AppVersion)
	c.JSON(http.StatusOK, services.CheckLatestRelease(current))
}
