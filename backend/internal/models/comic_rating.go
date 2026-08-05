package models

import "time"

// ComicRating 个人评分（按用户隔离，复合主键 user_id + comic_id）
type ComicRating struct {
	UserID    uint      `gorm:"primaryKey" json:"userId"`
	ComicID   string    `gorm:"primaryKey" json:"comicId"`
	Score     int       `json:"score"` // 1-10
	UpdatedAt time.Time `json:"updatedAt"`
}
