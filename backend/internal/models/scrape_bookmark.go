package models

import "time"

// ─────────────────────────────────────────────────────────────
// Round28：搜刮书签（后端化，多端同步）
//
// 原实现存前端 localStorage（多端不同步），现迁至后端按用户隔离，
// 与历史 / 书架 / 评分同一模式。config 与 anchor 为 JSON 字符串存储
// （仿 ReadingList.Items），由服务层解析为对象返回。
// ─────────────────────────────────────────────────────────────

// ScrapeBookmark 搜刮书签：某次在线浏览的位置快照 + 可选锚定画廊。
type ScrapeBookmark struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"userId"`
	Name      string    `gorm:"not null" json:"name"` // 自定义名称
	Type      string    `gorm:"not null" json:"type"` // 'home' | 'search'
	Keyword   string    `json:"keyword"`              // 冗余展示字段（搜索词）
	Config    string    `gorm:"type:text" json:"-"`   // JSON：SearchConfig 快照
	Anchor    string    `gorm:"type:text" json:"-"`   // JSON：{gid,token?,title?} 或 "null"
	CreatedAt time.Time `json:"createdAt"`
}
