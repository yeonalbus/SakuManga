package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"SakuManga/internal/database"
	"SakuManga/internal/models"
	"SakuManga/internal/services"

	"github.com/gin-gonic/gin"
)

// GetProxyHandler GET /api/v1/network/proxy
// 返回手动配置（proxy）、实际生效代理（effective，手动优先、系统兜底）与来源（manual/system/none）
func GetProxyHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"proxy":     services.GetProxyURL(),
		"effective": services.GetEffectiveProxy(),
		"source":    services.GetProxySource(),
	})
}

type SetProxyReq struct {
	Proxy string `json:"proxy"`
}

// SetProxyHandler POST /api/v1/network/proxy
func SetProxyHandler(c *gin.Context) {
	var req SetProxyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败"})
		return
	}

	if err := services.SetProxyURL(req.Proxy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "代理设置已更新",
		"proxy":   req.Proxy,
	})
}

// ─────────────────────────────────────────────────────────────
// Round48：高级代理设置（Clash 控制面 + 代理组）
// ─────────────────────────────────────────────────────────────

// advancedProxyDTO 读取响应（密钥只回传 hasSecret，不回传明文）
type advancedProxyDTO struct {
	Enabled         bool   `json:"enabled"`
	Controller      string `json:"controller"`
	HasSecret       bool   `json:"hasSecret"`
	GroupEGallery   string `json:"groupEGallery"`
	GroupEGalleryEx string `json:"groupEGalleryEx"`
	GroupHath       string `json:"groupHath"`
}

// GetAdvancedProxyHandler GET /api/v1/network/advanced
func GetAdvancedProxyHandler(c *gin.Context) {
	setting := services.GetNetworkSetting(database.DB)
	c.JSON(http.StatusOK, advancedProxyDTO{
		Enabled:         setting.AdvancedProxyEnabled,
		Controller:      setting.ClashController,
		HasSecret:       strings.TrimSpace(setting.ClashSecret) != "",
		GroupEGallery:   setting.GroupEGallery,
		GroupEGalleryEx: setting.GroupEGalleryEx,
		GroupHath:       setting.GroupHath,
	})
}

// advancedProxyReq 保存请求。KeepSecret=true 时忽略 Secret（前端未修改密钥）。
type advancedProxyReq struct {
	Enabled         bool   `json:"enabled"`
	Controller      string `json:"controller"`
	Secret          string `json:"secret"`
	KeepSecret      bool   `json:"keepSecret"`
	GroupEGallery   string `json:"groupEGallery"`
	GroupEGalleryEx string `json:"groupEGalleryEx"`
	GroupHath       string `json:"groupHath"`
}

// SaveAdvancedProxyHandler POST /api/v1/network/advanced
//
// 校验策略：
//   - 关闭开关：只做保存，允许空值
//   - 打开开关：静态必填校验 → 尝试联网校验组
//   - 控制面不可达：仍保存，但返回 warning（用户可能稍后才启动 Clash）
//   - 组不存在 / 组内无节点 / 选了 GLOBAL：拒绝保存（明确错误）
func SaveAdvancedProxyHandler(c *gin.Context) {
	var req advancedProxyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败"})
		return
	}

	old := services.GetNetworkSetting(database.DB)
	setting := &models.NetworkSetting{
		AdvancedProxyEnabled: req.Enabled,
		ClashController:      strings.TrimSpace(req.Controller),
		GroupEGallery:        strings.TrimSpace(req.GroupEGallery),
		GroupEGalleryEx:      strings.TrimSpace(req.GroupEGalleryEx),
		GroupHath:            strings.TrimSpace(req.GroupHath),
	}
	if req.KeepSecret {
		setting.ClashSecret = old.ClashSecret
	} else {
		setting.ClashSecret = strings.TrimSpace(req.Secret)
	}

	if err := services.ValidateNetworkSetting(setting); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	warning := ""
	if setting.ClashEnabled() {
		client := services.NewClashClient(setting.ClashController, setting.ClashSecret)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		if _, _, err := client.Version(ctx); err != nil {
			// 控制面暂时不可达：仍然保存，提示用户稍后自测
			warning = "配置已保存，但当前无法连接 Clash 控制面（" + err.Error() + "），请稍后用「连通性测试」确认"
		} else {
			w, err := services.VerifyClashGroups(ctx, client, setting)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			warning = w
		}
	}

	if err := services.SaveNetworkSetting(database.DB, setting); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败：" + err.Error()})
		return
	}
	// 配置变更后重置熔断状态（用户可能刚换到可用组）
	services.ArchiveCircuit().Reset()

	c.JSON(http.StatusOK, gin.H{"message": "已保存", "warning": warning})
}

// clashTestReq 连通性测试请求（字段为空时回退到已保存配置）
type clashTestReq struct {
	Controller string `json:"controller"`
	Secret     string `json:"secret"`
}

// TestClashHandler POST /api/v1/network/clash/test
//
// 一次完成三件事：GET /version 可达 → 读 e-hentai 组 → 跑一次 delay 测试。
func TestClashHandler(c *gin.Context) {
	var req clashTestReq
	_ = c.ShouldBindJSON(&req) // 允许空 body：直接用已保存配置

	saved := services.GetNetworkSetting(database.DB)
	controller := strings.TrimSpace(req.Controller)
	secret := strings.TrimSpace(req.Secret)

	// 未提供密钥时，只要地址没变就沿用已保存密钥：
	// 前端保存后会保留掩码，但刷新页面/重新进入时输入框为空，
	// 此时点「测试连接」也应能直接工作，不必让用户重输一遍。
	if secret == "" && (controller == "" || controller == strings.TrimSpace(saved.ClashController)) {
		secret = saved.ClashSecret
	}
	if controller == "" {
		controller = saved.ClashController
	}
	client := services.NewClashClient(controller, secret)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	version, _, err := client.Version(ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": err.Error()})
		return
	}

	groupName := strings.TrimSpace(saved.GroupEGallery)
	if groupName == "" {
		groupName = strings.TrimSpace(saved.GroupEGalleryEx)
	}
	if groupName == "" {
		c.JSON(http.StatusOK, gin.H{
			"ok": true, "version": version, "message": "连接正常（尚未配置代理组）",
		})
		return
	}

	group, err := client.Group(ctx, groupName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok": false, "version": version, "message": err.Error(),
		})
		return
	}

	best, usable := 0, 0
	if delayMap, err := client.GroupDelay(ctx, groupName, "https://e-hentai.org/", 5000); err == nil {
		usable = len(delayMap)
		for _, ms := range delayMap {
			if best == 0 || ms < best {
				best = ms
			}
		}
	}

	message := "连接正常"
	if usable > 0 {
		message = "连接正常，组内有可用节点"
	} else {
		message = "连接正常，但组内节点均未能连上 E 站（请检查节点或规则）"
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":         true,
		"version":    version,
		"group":      group.Name,
		"groupType":  group.Type,
		"now":        group.Now,
		"options":    len(group.All),
		"usable":     usable,
		"bestDelayMs": best,
		"message":    message,
	})
}

// clashGroupDTO 候选组下拉项
type clashGroupDTO struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Now     string `json:"now"`
	Options int    `json:"options"`
}

// ListClashGroupsHandler GET /api/v1/network/clash/groups
// 枚举控制面里的全部策略组，供用户从下拉里选（避免手填错名）。
func ListClashGroupsHandler(c *gin.Context) {
	setting := services.GetNetworkSetting(database.DB)
	client := services.NewClashClient(setting.ClashController, setting.ClashSecret)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	groups, err := client.ListGroups(ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	out := make([]clashGroupDTO, 0, len(groups))
	for _, g := range groups {
		out = append(out, clashGroupDTO{Name: g.Name, Type: g.Type, Now: g.Now, Options: len(g.All)})
	}
	c.JSON(http.StatusOK, gin.H{"groups": out})
}
