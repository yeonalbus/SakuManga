package models

import (
	"time"
)

// ComicSource 来源枚举
type ComicSource string

const (
	SourceOnline  ComicSource = "online"
	SourceOffline ComicSource = "offline"
)

// ExtraScanPath 额外的画廊扫描路径（对应你之前写的扫描设置）
type ExtraScanPath struct {
	ID                string    `gorm:"primaryKey" json:"id"`
	Path              string    `gorm:"uniqueIndex;not null" json:"path"`
	IncludeSubfolders bool      `gorm:"default:true" json:"includeSubfolders"`
	LastScanned       int64     `json:"lastScanned,omitempty"` // Unix 时间戳 (ms)
	ComicCount        int       `json:"comicCount,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
}

// OfflineComic 本地离线漫画模型 (整合了 BaseComic 与 OfflineComic)
type OfflineComic struct {
	ID           string      `gorm:"primaryKey" json:"id"`           // 本地 UUID
	Title        string      `gorm:"index;not null" json:"title"`     // 漫画标题
	CoverURL     string      `json:"coverUrl"`                        // 封面路由地址如 /api/v1/comics/:id/cover
	Source       ComicSource `gorm:"default:'offline'" json:"source"` // 标识来源
	Tags         string      `gorm:"type:text" json:"tags"`           // 标签列表 (在数据库中存 JSON 字符串，例如 ["TagA","TagB"])
	Rating       float64     `gorm:"default:0" json:"rating"`         // 评分
	PageCount    int         `json:"pageCount"`                       // 总页数
	UpdatedAt    time.Time   `json:"updatedAt"`                       // 扫描/更新时间
	IsDownloaded bool        `gorm:"default:true" json:"isDownloaded"`
	ClickCount   int         `gorm:"default:0" json:"clickCount"`     // 点击/阅读总次数

	// 本地特有字段
	Category    string `json:"category,omitempty"`    // 分类
	LocalPath   string `gorm:"uniqueIndex" json:"localPath"` // 本地存储绝对路径 (Z:\Comics\xxx)
	FileSize    int64  `json:"fileSize"`              // 文件大小 (Bytes)
	ReadCount   int    `gorm:"default:0" json:"readCount"`   // 本地阅读次数
	NeedsUpdate bool   `gorm:"default:false" json:"needsUpdate"` // 需要更新（检测到新版本）

	// ── E 站下载/更新关联字段（扫描/更新/查重用）──
	GID        string `gorm:"index" json:"gid,omitempty"`        // E 站画廊 GID（来自 metadata/ametadata）
	Token      string `json:"token,omitempty"`                   // E 站画廊 Token
	ParentGID  string `gorm:"index" json:"parentGID,omitempty"`  // 父画廊 GID（本画廊是某画廊的更新版）
	FileHash   string `gorm:"index" json:"fileHash,omitempty"`   // 归档文件 hash（完全相同查重）
	SourceMode string `json:"sourceMode,omitempty"`              // gallery | archive（下载来源）
	NewGID     string `json:"newGID,omitempty"`                  // 检测到的新版本 GID
	NewToken   string `json:"newToken,omitempty"`                // 检测到的新版本 Token
	UpdateNote string `json:"updateNote,omitempty"`              // 更新提示文案
}

// Bookshelf 本地书架
type Bookshelf struct {
	ID       string `gorm:"primaryKey" json:"id"`
	Name     string `gorm:"not null" json:"name"`
	Count    int    `gorm:"default:0" json:"count"`
	ComicIDs string `gorm:"type:text" json:"comicIds"` // JSON 数组存储: ["id1", "id2"]
}

// HistoryRecord 历史记录项
type HistoryRecord struct {
	ID               uint        `gorm:"primaryKey;autoIncrement" json:"id"`
	ComicID          string      `gorm:"index" json:"comicId"`
	Source           ComicSource `json:"source"`
	ComicTitle       string      `json:"comicTitle"`
	CoverURL         string      `json:"coverUrl"`
	LastChapterTitle string      `json:"lastChapterTitle,omitempty"`
	LastPageIndex    int         `json:"lastPageIndex"`    // 上次看到第几页
	TotalPageCount   int         `json:"totalPageCount"`   // 总页数
	LastReadAt       time.Time   `json:"lastReadAt"`       // 最后阅读时间
}