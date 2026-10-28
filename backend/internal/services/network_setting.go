package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// 高级代理设置（Clash 控制面 + 代理组）
//
//   - 存储：DB 单行表（NetworkSetting），AutoMigrate 自动加列，跨设备一致
//   - 与 config.json 的 proxy 是两个概念：proxy 是出网地址，本模块是"在同一端口内切节点"
//   - 开关关闭时全部逻辑短路，行为与旧版一致
// ─────────────────────────────────────────────────────────────

// GetNetworkSetting 读取高级代理设置（不存在则创建默认单行）。
func GetNetworkSetting(db *gorm.DB) *models.NetworkSetting {
	if db == nil {
		def := models.DefaultNetworkSetting()
		return &def
	}
	var setting models.NetworkSetting
	if err := db.First(&setting, 1).Error; err != nil {
		setting = models.DefaultNetworkSetting()
		if err := db.Create(&setting).Error; err != nil {
			log.Printf("%s 初始化高级代理设置失败: %v", clashLogTag, err)
		}
		return &setting
	}
	// 地址兜底：历史数据/空值统一为默认控制面地址
	if strings.TrimSpace(setting.ClashController) == "" {
		setting.ClashController = "127.0.0.1:9090"
	}
	return &setting
}

// SaveNetworkSetting 保存高级代理设置（单行 upsert）。
func SaveNetworkSetting(db *gorm.DB, s *models.NetworkSetting) error {
	if db == nil {
		return errors.New("数据库未初始化")
	}
	if s == nil {
		return errors.New("设置内容为空")
	}
	s.ID = 1
	s.UpdatedAt = time.Now()
	return db.Save(s).Error
}

// ValidateNetworkSetting 静态校验（不联网）：联动开启时必填项检查。
func ValidateNetworkSetting(s *models.NetworkSetting) error {
	if s == nil {
		return errors.New("设置内容为空")
	}
	if !s.AdvancedProxyEnabled {
		return nil // 关闭时不校验，允许保存空值
	}
	if strings.TrimSpace(s.ClashController) == "" {
		return errors.New("请填写 Clash 控制面地址（如 127.0.0.1:9090，注意不是代理端口 7897）")
	}
	if strings.TrimSpace(s.GroupEGallery) == "" {
		return errors.New("请填写 e-hentai.org 的代理组名称")
	}
	if strings.TrimSpace(s.GroupEGalleryEx) == "" {
		return errors.New("请填写 exhentai.org 的代理组名称")
	}
	return nil
}

// ── Clash 客户端单例（controller/secret 变化时重建） ──

var (
	clashClientMu  sync.Mutex
	clashCached    *ClashClient
	clashCacheKey  string
)

// ClashClientFor 返回与设置匹配的控制面客户端（按 controller+secret 缓存复用连接池）。
func ClashClientFor(s *models.NetworkSetting) *ClashClient {
	if s == nil {
		return nil
	}
	key := strings.TrimSpace(s.ClashController) + "|" + s.ClashSecret
	clashClientMu.Lock()
	defer clashClientMu.Unlock()
	if clashCached != nil && clashCacheKey == key {
		return clashCached
	}
	clashCached = NewClashClient(s.ClashController, s.ClashSecret)
	clashCacheKey = key
	return clashCached
}

// ActiveClash 读取设置并返回可用的控制面客户端。
// 返回 (client, setting, true) 表示联动生效；否则 (nil, setting, false)。
func ActiveClash(db *gorm.DB) (*ClashClient, *models.NetworkSetting, bool) {
	setting := GetNetworkSetting(db)
	if !setting.ClashEnabled() {
		return nil, setting, false
	}
	return ClashClientFor(setting), setting, true
}

// VerifyClashGroups 联网校验配置的代理组：存在 / 类型 / 选项非空。
// 返回 warning（非阻断性提示，如"该组为自动选择组"）；error 表示阻断性失败。
func VerifyClashGroups(ctx context.Context, client *ClashClient, s *models.NetworkSetting) (string, error) {
	if client == nil || s == nil {
		return "", nil
	}
	groups := make([]string, 0, 2)
	if g := strings.TrimSpace(s.GroupEGallery); g != "" {
		groups = append(groups, g)
	}
	if g := strings.TrimSpace(s.GroupEGalleryEx); g != "" && g != strings.TrimSpace(s.GroupEGallery) {
		groups = append(groups, g)
	}

	var warnings []string
	for _, name := range groups {
		if strings.EqualFold(name, "GLOBAL") {
			return "", ErrClashGlobalForbidden
		}
		group, err := client.Group(ctx, name)
		if err != nil {
			if errors.Is(err, ErrClashGroupNotFound) {
				return "", fmt.Errorf("代理组不存在：%s（请确认 Clash 里的组名完全一致）", name)
			}
			return "", err
		}
		if len(group.All) == 0 {
			return "", fmt.Errorf("代理组 %s 内没有可用节点", name)
		}
		if !group.IsSelector() {
			warnings = append(warnings,
				fmt.Sprintf("代理组 %s 的类型是 %s（自动选择组），手动切换可能被下次测速覆盖", name, group.Type))
		}
	}
	return strings.Join(warnings, "；"), nil
}

// ── 出口 IP 探测（可选增强：仅用于绑定的校验/审计，失败不影响主流程） ──

// exitIPProviders IP 回显服务候选（按顺序尝试）
var exitIPProviders = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://ip.sb",
}

// DetectExitIP 经 SakuManga 当前的代理链路请求 IP 回显服务，返回出口 IP。
//
// ⚠️ 前提：回显域名必须与 E 站走**同一个代理组**，否则测到的不是 E 站看到的 IP。
// 失败（超时/被拦截/解析不出 IP）返回空串，调用方应容忍。
func DetectExitIP(ctx context.Context) string {
	transport := getSharedTransport()
	if transport == nil {
		return ""
	}
	client := &http.Client{Transport: transport, Timeout: 6 * time.Second}
	for _, target := range exitIPProviders {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "SakuManga/1.0")
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		resp.Body.Close()
		if ip := parseExitIP(string(body)); ip != "" {
			return ip
		}
	}
	return ""
}

// parseExitIP 从回显响应中提取合法 IP（兼容纯文本与简单 JSON）。
func parseExitIP(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	// JSON 形式：{"ip":"1.2.3.4"}
	if idx := strings.Index(body, "\"ip\""); idx >= 0 {
		rest := body[idx+4:]
		if c := strings.Index(rest, ":"); c >= 0 {
			rest = rest[c+1:]
			if q1 := strings.Index(rest, "\""); q1 >= 0 {
				rest = rest[q1+1:]
				if q2 := strings.Index(rest, "\""); q2 >= 0 {
					body = rest[:q2]
				}
			}
		}
	}
	// 纯文本可能带换行/附加说明，取第一段
	if f := strings.Fields(body); len(f) > 0 {
		body = f[0]
	}
	if net.ParseIP(body) != nil {
		return body
	}
	return ""
}
