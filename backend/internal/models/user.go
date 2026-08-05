package models

import "time"

// User 用户（取代全局 AccountSetting，每个用户持有自己的 E 站凭证）
type User struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	Username      string `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash  string `gorm:"not null" json:"-"`
	Role          string `gorm:"default:'member'" json:"role"` // admin | member
	AllowDownload bool   `gorm:"default:false" json:"allowDownload"`

	// E 站凭证（并入，取代全局 AccountSetting）
	IPBMemberID string    `gorm:"column:ipb_member_id" json:"ipb_member_id"`
	IPBPassHash string    `gorm:"column:ipb_pass_hash" json:"ipb_pass_hash"`
	Igneous     string    `gorm:"column:igneous" json:"igneous"`
	SK          string    `gorm:"column:sk" json:"sk,omitempty"`
	IsEx        bool      `gorm:"default:false" json:"isEx"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// UserSession 登录会话（随机 token，存 DB；重启后需重新登录）
type UserSession struct {
	Token     string    `gorm:"primaryKey" json:"token"`
	UserID    uint      `gorm:"index" json:"userId"`
	CreatedAt time.Time `json:"createdAt"`
}
