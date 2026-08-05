//go:build windows

// Package tray 提供 Windows 系统托盘支持：最小化到托盘，右键菜单「打开界面 / 退出程序」。
package tray

import (
	_ "embed"
	"os/exec"

	"github.com/getlantern/systray"
)

//go:embed icon.ico
var iconData []byte

// Run 启动系统托盘并阻塞，直到用户点击「退出程序」后返回。
// url 为点击「打开界面」时在默认浏览器中打开的地址。
func Run(url string) {
	systray.Run(onReady(url), onExit)
}

func onReady(url string) func() {
	return func() {
		systray.SetIcon(iconData)
		systray.SetTitle("SakuHentai")
		systray.SetTooltip("SakuHentai - 漫画管理")

		mOpen := systray.AddMenuItem("打开界面", "在默认浏览器中打开 SakuHentai 界面")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("退出程序", "退出 SakuHentai")

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					openBrowser(url)
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}
}

func onExit() {
	// 预留：程序退出前的清理逻辑（当前无需额外处理）
}

func openBrowser(url string) {
	// 调用系统默认浏览器（无需额外依赖）
	if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start(); err != nil {
		// 打开失败不阻塞主流程，静默忽略
	}
}
