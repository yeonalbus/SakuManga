//go:build !windows

// Package tray 提供系统托盘支持。非 Windows 平台（如 NAS/Linux headless）为无操作实现。
package tray

// Run 在非 Windows 平台不启用托盘，直接返回。
// url 为待打开的界面地址（本实现中忽略）。
func Run(url string) {
	_ = url
}
