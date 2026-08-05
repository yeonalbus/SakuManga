package models

import "time"

// ReadingList 阅读清单（每用户每来源一个队列，复合主键 user_id + source）
type ReadingList struct {
	UserID    uint      `gorm:"primaryKey" json:"userId"`
	Source    string    `gorm:"primaryKey" json:"source"` // online | offline
	Items     string    `gorm:"type:text" json:"items"`   // JSON 数组（ComicItem 快照）
	UpdatedAt time.Time `json:"updatedAt"`
}
