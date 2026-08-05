// Package webui 内嵌前端构建产物（Vite dist/），提供单 exe 打包所需的静态文件服务与 SPA 回退。
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var distFS embed.FS

// RegisterRoutes 将内嵌前端挂载到 gin 路由：
//   - 真实存在的静态文件（JS/CSS/图片/favicon 等）直接返回；
//   - 其余非 /api 路径统一回退到 index.html，配合 Vue Router 的 createWebHistory() 模式；
//   - /api 路径未命中时返回 404 JSON，避免吞掉前端错误提示。
//
// 必须在 router.RegisterRoutes 之后调用，确保 API 路由优先匹配。
func RegisterRoutes(r *gin.Engine) {
	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("读取内嵌前端资源失败: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(dist))
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// API 未命中 -> 404 JSON
		if strings.HasPrefix(path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
			return
		}

		// 静态文件存在 -> 直接返回
		name := strings.TrimPrefix(path, "/")
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(dist, name); err == nil {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		// SPA 回退：改写路径到 index.html 再交给文件服务器
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
