package handlers

import (
	"net/http"

	"SakuHentai/internal/version"

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
