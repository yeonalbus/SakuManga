package models

import "time"

// FavoriteState 记录本地缓存的在线画廊收藏状态
type FavoriteState struct {
	GID       string    `gorm:"primaryKey" json:"gid"`
	Token     string    `json:"token"`
	FavCat    int       `gorm:"not null;default:0" json:"favCat"` // 0 ~ 9 表示对应收藏夹；-1 表示未收藏/已移除
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}