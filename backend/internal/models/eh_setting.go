package models

import "time"

// EHSetting E站站点偏好与交互配置（对应数据库 eh_settings 表）
type EHSetting struct {
	ID              uint      `gorm:"primaryKey;default:1" json:"id"` // 固定 ID=1 保证单例
	Site            string    `gorm:"default:'e-hentai'" json:"site"` // "e-hentai" | "exhentai"
	PreferRedirect  bool      `gorm:"default:true" json:"preferRedirect"` // 优先重定向至表站
	SelectedProfile string    `gorm:"default:'default'" json:"selectedProfile"` // Profile 标识
	UpdatedAt       time.Time `json:"updatedAt"`
}