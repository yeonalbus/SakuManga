package models

import (
	"strings"
	"time"
)

// NetworkSetting 高级代理设置（单例 ID=1）。
//
// 与 config.json 里的 proxy 是两个独立概念：
//   - proxy：SakuManga 出网用的代理地址（混合端口，如 http://127.0.0.1:7897）
//   - 本表：Clash 控制面地址 + 代理组，用于"在同一端口内切换出口节点"
//
// 存储放 DB（与 DownloadSetting 一致）：AutoMigrate 自动加列、并发安全、跨设备一致。
type NetworkSetting struct {
	ID uint `gorm:"primaryKey;default:1" json:"id"`

	// AdvancedProxyEnabled 高级代理总开关（默认关闭；关闭时所有联动逻辑短路，行为与旧版一致）
	AdvancedProxyEnabled bool `gorm:"default:false" json:"enabled"`

	// ClashController mihomo 控制面地址（host:port），注意是 external-controller 端口，
	// 不是代理端口（Verge 默认代理端口 7897、控制面默认 9097）。
	ClashController string `gorm:"default:'127.0.0.1:9090'" json:"controller"`

	// ClashSecret 控制面密钥（Bearer）；为空表示控制面未设置密钥。
	// json:"-" 避免明文回传前端，读取时只返回 hasSecret 布尔值。
	ClashSecret string `json:"-"`

	// GroupEGallery e-hentai.org 所走的代理组（必填）
	GroupEGallery string `json:"groupEGallery"`
	// GroupEGalleryEx exhentai.org 所走的代理组（必填）
	GroupEGalleryEx string `json:"groupEGalleryEx"`
	// GroupHath hath.network 所走的代理组（可空；留空 = H@H 请求不参与联动切换）
	GroupHath string `json:"groupHath"`

	UpdatedAt time.Time `json:"updatedAt"`
}

// DefaultNetworkSetting 默认高级代理设置
func DefaultNetworkSetting() NetworkSetting {
	return NetworkSetting{
		ID:              1,
		ClashController: "127.0.0.1:9090",
	}
}

// ClashEnabled 判定联动是否生效（开关打开且控制面地址非空）。
func (s *NetworkSetting) ClashEnabled() bool {
	return s != nil && s.AdvancedProxyEnabled && strings.TrimSpace(s.ClashController) != ""
}

// GroupForSite 返回该站点应使用的代理组名。
// isEx=true 表示里站（exhentai）；未配置对应组时回退到另一个，都没有则返回空串。
func (s *NetworkSetting) GroupForSite(isEx bool) string {
	if s == nil {
		return ""
	}
	if isEx {
		if g := strings.TrimSpace(s.GroupEGalleryEx); g != "" {
			return g
		}
		return strings.TrimSpace(s.GroupEGallery)
	}
	if g := strings.TrimSpace(s.GroupEGallery); g != "" {
		return g
	}
	return strings.TrimSpace(s.GroupEGalleryEx)
}

// ArchiveBinding 归档 ↔ 节点绑定。
//
// 记录"某个画廊是在哪个出口节点上完成解锁（扣 GP）的"，用于：
//   - 重试时报价为 GP（说明出口 IP 已变）→ 尝试切回原节点，恢复到免费解锁状态
//   - 审计：这个归档当时花了多少 GP、从哪个节点下的
//
// 主键是节点名而非 IP：Clash 只能按节点名切换，IP 仅作校验/审计值。
type ArchiveBinding struct {
	ID     uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	GID    string `gorm:"index" json:"gid"`    // 画廊 GID
	UserID uint   `gorm:"index" json:"userId"` // 发起者（解锁状态按账号 + IP）
	Group  string `json:"group"`               // 解锁时所在代理组
	Node   string `gorm:"index" json:"node"`   // 节点名（切换主键）
	IP     string `json:"ip,omitempty"`        // 出口 IP（可选；探测失败为空）
	GPCost string `json:"gpCost"`              // 当时报价文本（Free! / 1952 GP）

	CreatedAt time.Time `json:"createdAt"` // 扣费时刻
	ExpiresAt time.Time `gorm:"index" json:"expiresAt"`
	Consumed  bool      `gorm:"default:false" json:"consumed"` // 已成功下载，无需再切回
}
