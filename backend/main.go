package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"SakuHentai/internal/router"
	"SakuHentai/internal/services"
	"SakuHentai/internal/tray"
	"SakuHentai/webui"

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
	// --headless：纯后端模式，不显示系统托盘（适合 NAS/无界面环境）
	headless := flag.Bool("headless", false, "以纯后端模式运行，不显示系统托盘（适合 NAS/无界面环境）")
	flag.Parse()

	// 0. 切换到可执行文件所在目录，保证 manga.db / config.json / data 与 exe 同目录存放
	if exe, err := os.Executable(); err == nil {
		_ = os.Chdir(filepath.Dir(exe))
	}

	// 1. 初始化数据库
	database.InitDB()

	// 1.1 打印数据库实际路径：服务已 chdir 到可执行文件所在目录，manga.db 与 exe 同目录存放。
	//     ⚠️ 用 `go run` 启动时 exe 在 %TEMP% 临时目录，DB 也落在那里——看到与项目内 backend/manga.db
	//     不一致属正常现象；如需固定位置，请用打包后的 SakuHentai.exe 启动（见 build-release.bat）。
	if dbPath, err := filepath.Abs("manga.db"); err == nil {
		log.Printf("[DB] 数据库路径: %s", dbPath)
	}

	// 2. 启动时加载 config.json 中的代理配置
	services.InitProxyConfig()

	// 3. 启动标签引擎：加载本地翻译/热度数据，若缺失或非最新则自动下载（含 24 小时自动更新周期）
	services.InitTagEngine()

	// 4. 确保初始管理员存在（users 表为空时创建 admin/admin123，E 站凭证置空，日志打印账密）
	if err := services.EnsureInitialAdmin(database.DB); err != nil {
		panic("创建初始管理员失败: " + err.Error())
	}

	// 5. 初始化 Router 并挂载中间件
	r := gin.Default()
	r.Use(Cors())

	// 6. 初始化 E-Hentai 抓取服务，并注册全部 API 路由（路由配置见 internal/router）
	ehService := services.NewEHService()
	router.RegisterRoutes(r, database.DB, ehService)

	// 7. 内嵌前端静态文件 + SPA 回退（单 exe 打包，见 webui/embed.go）
	webui.RegisterRoutes(r)

	// 8. 读取服务器配置（监听地址 + 端口），无记录则用默认值 0.0.0.0:8081
	var setting models.ServerSetting
	if err := database.DB.First(&setting, 1).Error; err != nil {
		setting = models.ServerSetting{ID: 1, BindHost: "0.0.0.0", Port: 8081, HistoryLimit: 200}
	}
	addr := fmt.Sprintf("%s:%d", setting.BindHost, setting.Port)

	// 9. 监听端口；若被占用则自动回退到随机空闲端口
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("监听 %s 失败（端口可能被占用），自动切换随机端口", addr)
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			log.Fatalf("监听端口失败: %v", err)
		}
	}

	srv := &http.Server{Handler: r}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP 服务异常退出: %v", err)
		}
	}()

	// 浏览器打开地址：监听 0.0.0.0 / 双栈地址时统一换成 127.0.0.1
	host, port := "127.0.0.1", "0"
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		host = tcpAddr.IP.String()
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		port = strconv.Itoa(tcpAddr.Port)
	}
	url := fmt.Sprintf("http://%s", net.JoinHostPort(host, port))
	log.Printf("SakuHentai 已启动: %s", url)

	// 10. 桌面模式显示系统托盘（右键：打开界面/退出程序）；headless 模式等待退出信号
	if *headless {
		waitForSignal()
	} else {
		tray.Run(url)
	}

	// 11. 优雅关闭 HTTP 服务
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// waitForSignal 阻塞等待退出信号（Ctrl+C / 系统关闭），供 headless 模式使用。
func waitForSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
}
