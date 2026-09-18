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

// ConfigFile 全局配置（config.json）：
//   - Proxy：系统级代理
//   - TagEngine：标签引擎开关（中文翻译 / 补全排序）。指针字段以区分「历史配置未写入」与「显式设为 false」，
//     缺失时按标签引擎默认值（均开启）处理，避免旧配置文件被当成「用户手动关闭」。
type ConfigFile struct {
	Proxy     string         `json:"proxy"`
	TagEngine *TagEngineConf `json:"tagEngine,omitempty"`
}

// TagEngineConf 标签引擎开关（持久化到 config.json，重启后保持）
type TagEngineConf struct {
	EnableCN   *bool `json:"enableCN,omitempty"`
	EnableSort *bool `json:"enableSort,omitempty"`
}

// loadConfigFile 读取 config.json（不存在 / 解析失败时返回零值配置，不报错）
func loadConfigFile() ConfigFile {
	var cfg ConfigFile
	if data, err := os.ReadFile(configFilePath); err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	return cfg
}

// saveConfigFile 整体写回 config.json。
// ⚠️ 必须走「读-改-写」：config.json 承载多个模块的设置（代理 + 标签引擎），
// 整体覆盖式写入会清掉其它模块刚保存的字段。
func saveConfigFile(cfg ConfigFile) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFilePath, data, 0644)
}

// GetTagEngineConf 返回持久化的标签引擎开关（字段可能为 nil = 历史配置未写入，由调用方套用默认值）
func GetTagEngineConf() *TagEngineConf {
	return loadConfigFile().TagEngine
}

// PersistTagEngineSettings 把标签引擎开关写入 config.json（保留代理等其它字段）
func PersistTagEngineSettings(enableCN, enableSort bool) error {
	cfg := loadConfigFile()
	cfg.TagEngine = &TagEngineConf{EnableCN: &enableCN, EnableSort: &enableSort}
	return saveConfigFile(cfg)
}

// InitProxyConfig 初始化时从 config.json 读取代理设置
func InitProxyConfig() {
	proxyLock.Lock()
	defer proxyLock.Unlock()

	cfg := loadConfigFile()
	currentProxy = cfg.Proxy
	// 同步到标签引擎的 globalProxy，保证标签数据（GitHub）下载同样走代理
	_ = SetGlobalProxy(cfg.Proxy)
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

	// 写入 config.json（读-改-写：保留标签引擎开关等其它模块的字段）
	cfg := loadConfigFile()
	cfg.Proxy = p
	_ = saveConfigFile(cfg)

	return nil
}
