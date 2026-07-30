package models

import "time"

// AccountSetting 存储 E 站账号 Cookie 及状态
type AccountSetting struct {
	ID          uint      `gorm:"primaryKey;default:1" json:"id"` // 固定 ID=1 保持单条记录
	IPBMemberID string    `gorm:"column:ipb_member_id;not null" json:"ipb_member_id"`
	IPBPassHash string    `gorm:"column:ipb_pass_hash;not null" json:"ipb_pass_hash"`
	Igneous     string    `gorm:"column:igneous" json:"igneous"`
	IsEx        bool      `gorm:"default:false" json:"isEx"` // 是否具备 ExHentai 访问权限
	UpdatedAt   time.Time `json:"updatedAt"`
}