//go:build !windows

package services

// detectRegistryProxy 非 Windows 平台无注册表系统代理，返回空串（环境变量探测仍生效）。
func detectRegistryProxy() string {
	return ""
}
