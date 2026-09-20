package models

import "time"

// DedupSetting 离线查重设置（Round42 决策 D2，单行配置，ID 固定为 1）
//
// 目前仅一项：联网复核（默认**关闭**）。
// 开启后，维护查重会对「近似层（Tier 2）」产出的、置信度非 high 的疑似重复簇，
// 用其归一标题去 E 站反查一次：若 E 站存在同标题的同作品条目 → 复核通过并升为 high；
// 否则标注「联网复核未确认」供人工优先确认。
type DedupSetting struct {
	ID uint `gorm:"primaryKey" json:"id"`

	// OnlineVerify 联网复核开关（默认 false）
	OnlineVerify bool `gorm:"default:false" json:"onlineVerify"`

	// Round44：移除时是否同时删除本地文件（维护页「含文件」勾选的记忆，默认 false）
	DeleteFileDefault bool `gorm:"default:false" json:"deleteFileDefault"`

	UpdatedAt time.Time `json:"updatedAt"`
}
