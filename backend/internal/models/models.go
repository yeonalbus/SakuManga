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
	ID                  string    `gorm:"primaryKey" json:"id"`
	Path                string    `gorm:"uniqueIndex;not null" json:"path"`
	Name                string    `json:"name,omitempty"`     // 可配置的显示名称（问题3：来源栏目标签）
	IncludeSubfolders   bool      `gorm:"default:true" json:"includeSubfolders"`
	EnableOfflineUpdate bool      `gorm:"default:true" json:"enableOfflineUpdate"` // 是否参与离线更新检测/本地维护查重（问题4）
	LastScanned         int64     `json:"lastScanned,omitempty"`                   // Unix 时间戳 (ms)
	ComicCount          int       `json:"comicCount,omitempty"`
	CreatedAt           time.Time `json:"createdAt"`
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

	// ── 标题/时间/来源字段（问题1/2/3）──
	TitleJpn       string     `gorm:"type:text" json:"titleJpn,omitempty"`          // 日文原名（metadata title_jpn / ComicInfo AlternateSeries），问题2 优先显示
	AddedAt        time.Time  `json:"addedAt"`                                      // 首次入库时间（不同于 UpdatedAt 会被 CheckUpdates 覆盖，问题1 排序）
	FileModifiedAt time.Time  `json:"fileModifiedAt,omitempty"`                     // 本地文件夹/归档文件修改时间（问题1 排序）
	PublishedAt    *time.Time `json:"publishedAt,omitempty"`                        // 发布时间（metadata publishTime / ComicInfo 日期，问题1 排序）
	ScanPathID     string     `gorm:"index" json:"scanPathID,omitempty"`            // 来源额外路径 ID；空 = 下载导入（问题3 来源识别）

	// ── E 站下载/更新关联字段（扫描/更新/查重用）──
	GID             string `gorm:"index" json:"gid,omitempty"`             // E 站画廊 GID（来自 metadata/ametadata）
	Token           string `json:"token,omitempty"`                        // E 站画廊 Token
	ParentGID       string `gorm:"index" json:"parentGID,omitempty"`       // 父画廊 GID（本画廊是某画廊的更新版）
	ParentCheckedAt int64  `json:"parentCheckedAt,omitempty"`              // 父画廊关系在线核对时间戳(ms)；>0=已核对（需求1 增量兜底，避免每次维护查重重复联网）
	FileHash        string `gorm:"index" json:"fileHash,omitempty"`        // 归档文件 hash（完全相同查重）
	SourceMode string `json:"sourceMode,omitempty"`              // gallery | archive（下载来源）
	NewGID     string `json:"newGID,omitempty"`                  // 检测到的新版本 GID
	NewToken   string `json:"newToken,omitempty"`                // 检测到的新版本 Token
	UpdateNote string `json:"updateNote,omitempty"`              // 更新提示文案

	// ── Aged 老化状态（Round4 任务四：365 天老化规则）──
	// E 站规则：发布超 365 天的画廊无法再通过 Gallery Manager Update 生成子画廊，
	// 此类画廊只扫描一次；若无可更新新版（或最新版也超 365 天），标记本状态位并排除后续扫描。
	AgedStatus    bool  `gorm:"default:false" json:"agedStatus"`       // 已老化（发布超 365 天且无可更新新版）
	AgedCheckedAt int64 `json:"agedCheckedAt,omitempty"`               // 上次老化判定时间戳(ms)；>0=已判定（一次性，防止重复扫描）

	// ── 画廊可用性（需求 3(2)：区分「画廊被删」与「网络故障」）──
	RemovedStatus bool  `gorm:"default:false" json:"removedStatus"`    // 画廊被删除/版权移除（removed/copyright）；网络故障不得标记
	RemovedAt     int64 `json:"removedAt,omitempty"`                   // 标记时间戳(ms)

	// ── Tag 双轨维护字段（本地漫画 Tag 维护系统）──
	OnlineTags        string `gorm:"type:text" json:"onlineTags,omitempty"`        // E站官方 tag JSON 数组（每日刷新覆盖）
	OfflineAddTags    string `gorm:"type:text" json:"offlineAddTags,omitempty"`    // 本地新增 tag JSON 数组（用户客制化）
	OfflineRemoveTags string `gorm:"type:text" json:"offlineRemoveTags,omitempty"` // 本地删除的 online tag JSON 数组（刷新略过/写回剔除）
	LastTagRefreshAt  int64  `json:"lastTagRefreshAt,omitempty"`                   // 上次 Tag 刷新时间戳(ms)
	TagRefreshCount   int    `json:"tagRefreshCount,omitempty"`                    // 累计刷新次数
}

// TagMaintainSetting Tag 维护设置（单例 ID=1）
type TagMaintainSetting struct {
	ID                  uint      `gorm:"primaryKey;default:1" json:"id"`
	EnableDailyRefresh  bool      `gorm:"default:true" json:"enableDailyRefresh"`   // 开启每日 Tag 刷新
	EnableWeeklyWriteback bool    `gorm:"default:true" json:"enableWeeklyWriteback"` // 开启每周反向写回
	RefreshHour         int       `gorm:"default:6" json:"refreshHour"`             // 每日刷新小时（系统本地时区，默认 6）
	WritebackWeekday    int       `gorm:"default:0" json:"writebackWeekday"`        // 写回日（0=周日）
	WritebackHour       int       `gorm:"default:6" json:"writebackHour"`           // 写回小时（系统本地时区，默认 6）
	LastDailyRunAt      int64     `json:"lastDailyRunAt,omitempty"`                 // 上次每日刷新执行时间(ms)
	LastWeeklyRunAt     int64     `json:"lastWeeklyRunAt,omitempty"`                // 上次每周写回执行时间(ms)
	UpdatedAt           time.Time `json:"updatedAt"`
}

// UpdateScanSetting 每周自动更新扫描设置（单例 ID=1，Round4 任务四）
// 仿 TagMaintainSetting：提供「每周固定时刻自动更新扫描」+ Aged Status 老化判定开关。
type UpdateScanSetting struct {
	ID               uint      `gorm:"primaryKey;default:1" json:"id"`
	EnableWeeklyScan bool      `gorm:"default:false" json:"enableWeeklyScan"` // 开启每周自动更新扫描
	ScanWeekday      int       `gorm:"default:0" json:"scanWeekday"`          // 扫描日（0=周日）
	ScanHour         int       `gorm:"default:6" json:"scanHour"`             // 扫描时刻（系统本地时区，默认 6）
	LastWeeklyScanAt int64     `json:"lastWeeklyScanAt,omitempty"`            // 上次自动扫描执行时间(ms)
	UpdatedAt        time.Time `json:"updatedAt"`
}

// Bookshelf 本地书架（按用户隔离）
type Bookshelf struct {
	ID       string `gorm:"primaryKey" json:"id"`
	UserID   uint   `gorm:"index" json:"userId"`
	Name     string `gorm:"not null" json:"name"`
	Count    int    `gorm:"default:0" json:"count"`
	ComicIDs string `gorm:"type:text" json:"comicIds"` // JSON 数组存储: ["id1", "id2"]
}

// HistoryRecord 历史记录项（按用户隔离）
type HistoryRecord struct {
	ID               uint        `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID           uint        `gorm:"index" json:"userId"`
	ComicID          string      `gorm:"index" json:"comicId"`
	Source           ComicSource `json:"source"`
	ComicTitle       string      `json:"comicTitle"`
	CoverURL         string      `json:"coverUrl"`
	Token            string      `json:"token,omitempty"`   // 在线画廊 token（历史打开在线详情必需）
	LastChapterTitle string      `json:"lastChapterTitle,omitempty"`
	LastPageIndex    int         `json:"lastPageIndex"`    // 上次看到第几页
	TotalPageCount   int         `json:"totalPageCount"`   // 总页数
	LastReadAt       time.Time   `json:"lastReadAt"`       // 最后阅读时间
}