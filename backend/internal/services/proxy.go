package services

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"net/url"
)

var (
	proxyLock    sync.RWMutex
	currentProxy string
)

const configFilePath = "config.json"

type ConfigFile struct {
	Proxy string `json:"proxy"`
}

// InitProxyConfig 初始化时从 config.json 读取代理设置
func InitProxyConfig() {
	proxyLock.Lock()
	defer proxyLock.Unlock()

	data, err := os.ReadFile(configFilePath)
	if err == nil {
		var cfg ConfigFile
		if err := json.Unmarshal(data, &cfg); err == nil {
			currentProxy = cfg.Proxy
			// 同步到标签引擎的 globalProxy，保证标签数据（GitHub）下载同样走代理
			_ = SetGlobalProxy(cfg.Proxy)
		}
	}
}

// GetProxyURL 获取当前配置的代理 URL（仅手动配置，config.json 来源）
func GetProxyURL() string {
	proxyLock.RLock()
	defer proxyLock.RUnlock()
	return currentProxy
}

// GetEffectiveProxy 返回实际生效的代理地址：手动配置优先，未设置时自动兜底系统代理
// （Windows 注册表系统代理 / 环境变量 http(s)_proxy，覆盖 Clash 等「系统代理」模式用户零配置直连）
func GetEffectiveProxy() string {
	if p := GetProxyURL(); p != "" {
		return p
	}
	return DetectSystemProxy()
}

// GetProxySource 返回当前代理来源，供设置页展示：
//   - "manual"：用户手动配置（config.json）
//   - "system"：自动检测到的系统代理（注册表 / 环境变量）
//   - "none" ：直连模式
func GetProxySource() string {
	if GetProxyURL() != "" {
		return "manual"
	}
	if DetectSystemProxy() != "" {
		return "system"
	}
	return "none"
}

// DetectSystemProxy 探测系统级代理：
//  1. 环境变量（http_proxy / https_proxy / all_proxy，NAS/Linux 常见）
//  2. Windows 注册表系统代理（Clash / v2rayN 等「系统代理」模式写入，见 proxy_system_windows.go）
//
// 返回空串表示系统无代理（直连）。
func DetectSystemProxy() string {
	if p := detectEnvProxy(); p != "" {
		return p
	}
	return detectRegistryProxy()
}

// detectEnvProxy 读取环境变量代理，https 优先（与 Go 标准库 ProxyFromEnvironment 同序）
func detectEnvProxy() string {
	for _, env := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v
		}
	}
	return ""
}

// SetProxyURL 设置全局代理地址并保存到配置文件
func SetProxyURL(p string) error {
	if p != "" {
		u, err := url.Parse(p)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
			return fmt.Errorf("无效的代理地址格式，需包含协议头 (如 http:// 或 socks5://)")
		}
	}

	proxyLock.Lock()
	currentProxy = p
	proxyLock.Unlock()

	// 同步到标签引擎的 globalProxy，保证标签数据（GitHub）下载同样走代理
	_ = SetGlobalProxy(p)

	// 代理已变更：清空共享 Transport（主站 + 封面），下次请求按新代理重建连接池
	resetSharedTransports()

	// 写入 config.json
	cfg := ConfigFile{Proxy: p}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(configFilePath, data, 0644)

	return nil
}