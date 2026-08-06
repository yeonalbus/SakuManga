package services

import (
	"encoding/json"
	"fmt"

	"net/url"
	"os"
	"sync"
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

// GetProxyURL 获取当前配置的代理 URL
func GetProxyURL() string {
	proxyLock.RLock()
	defer proxyLock.RUnlock()
	return currentProxy
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