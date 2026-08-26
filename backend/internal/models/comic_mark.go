package models

import "time"

// ─────────────────────────────────────────────────────────────
// Round24：书签 / 章节标记（仅本地阅读器）
//
// 标记绑定「物理页索引」（0-based 原文件索引），与隐藏页（HiddenPages）
// 同一锚点体系：隐藏某物理页 → 该页的书签/章节标记联动清除。
// ─────────────────────────────────────────────────────────────

// ComicBookmark 书签标记：快速跳转某页，无名称。
type ComicBookmark struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ComicID   string    `gorm:"index;not null" json:"comicId"` // 本地漫画 id（md5）
	PageIndex int       `gorm:"not null" json:"pageIndex"`     // 物理页索引（0-based 原文件索引）
	CreatedAt time.Time `json:"createdAt"`
}

// ComicChapter 章节标记：命名目录项（≤3 级、允许跳级），绑定起始物理页。
type ComicChapter struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ComicID   string    `gorm:"index;not null" json:"comicId"` // 本地漫画 id（md5）
	ParentID  uint      `gorm:"default:0" json:"parentId"`     // 父章节 id；0 = 一级节点
	Level     int       `gorm:"not null" json:"level"`         // 1 / 2 / 3
	Title     string    `gorm:"not null" json:"title"`         // 自定义名称
	PageIndex int       `gorm:"not null" json:"pageIndex"`     // 起始物理页索引（0-based）
	OrderNo   int       `gorm:"default:0" json:"orderNo"`      // 同层排序号
	CreatedAt time.Time `json:"createdAt"`
}
