//go:build windows

package services

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// detectRegistryProxy 读取 Windows 系统代理设置（注册表 Internet Settings）。
// Clash / v2rayN 等工具开启「系统代理」时写入：
//   - ProxyEnable  = 1
//   - ProxyServer  = "127.0.0.1:7897" 或 "http=127.0.0.1:7897;https=127.0.0.1:7897"
//
// 返回空串表示未开启系统代理。
func detectRegistryProxy() string {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()

	enable, _, err := k.GetIntegerValue("ProxyEnable")
	if err != nil || enable == 0 {
		return ""
	}

	server, _, err := k.GetStringValue("ProxyServer")
	if err != nil || strings.TrimSpace(server) == "" {
		return ""
	}
	return normalizeProxyServer(server)
}

// normalizeProxyServer 将注册表 ProxyServer 值规范化为可用代理 URL：
//   - "127.0.0.1:7897"            → http://127.0.0.1:7897
//   - "http=...;https=..."        → 取 http/https 地址
//   - "socks=127.0.0.1:7890"      → socks5://127.0.0.1:7890
func normalizeProxyServer(server string) string {
	server = strings.TrimSpace(server)
	if server == "" {
		return ""
	}
	// 已带协议头，直接返回
	if strings.HasPrefix(server, "http://") || strings.HasPrefix(server, "https://") ||
		strings.HasPrefix(server, "socks5://") || strings.HasPrefix(server, "socks5h://") {
		return server
	}
	// 多协议形式：http=...;https=... 或 socks=...
	if strings.Contains(server, "=") {
		var httpProxy, socksProxy string
		for _, part := range strings.Split(server, ";") {
			part = strings.TrimSpace(part)
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				continue
			}
			scheme, addr := strings.ToLower(strings.TrimSpace(kv[0])), strings.TrimSpace(kv[1])
			switch scheme {
			case "http", "https":
				if httpProxy == "" {
					httpProxy = addr
				}
			case "socks":
				if socksProxy == "" {
					socksProxy = addr
				}
			}
		}
		if httpProxy != "" {
			return "http://" + httpProxy
		}
		if socksProxy != "" {
			return "socks5://" + socksProxy
		}
		return ""
	}
	// 纯 host:port 形式，默认 http 代理
	return "http://" + server
}
