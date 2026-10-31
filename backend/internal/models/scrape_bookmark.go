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
	ID      uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID  uint   `gorm:"index;not null" json:"userId"`
	Name    string `gorm:"not null" json:"name"` // 自定义名称
	Type    string `gorm:"not null" json:"type"` // 'home' | 'search'
	Keyword string `json:"keyword"`              // 冗余展示字段（搜索词）
	Config  string `gorm:"type:text" json:"-"`   // JSON：SearchConfig 快照
	Anchor  string `gorm:"type:text" json:"-"`   // JSON：{gid,token?,title?} 或 "null"
	// Round37：侧栏拖动排序的 LexoRank 浮点权值（单点移动只更新该项）。
	// 0 = 升级前的老数据未赋权，读取时按 id 升序兜底（等价于原创建顺序，升级前后顺序一致）。
	// Round49：书签页改为自动排序（名称序 + 有更新置前），该字段保留但不再参与排序。
	SortKey float64 `gorm:"default:0" json:"sortKey"`

	// ─── Round49：书签项级配置（每条书签独立，无跨书签联动）───
	Role            string `gorm:"default:interest" json:"role"`     // newest | oldest | interest
	Note            string `gorm:"type:text" json:"note"`            // 备注（前端限制 40 中文字符）
	Pinned          bool   `gorm:"default:false" json:"pinned"`      // 侧栏常驻（上限 5，仅用户手动设置）
	UpdateWatch     bool   `gorm:"default:false" json:"updateWatch"` // 更新开关
	HasUpdate       bool   `gorm:"default:false" json:"hasUpdate"`   // 更新标记（红点）
	UpdateCheckedAt int64  `gorm:"default:0" json:"updateCheckedAt"` // 上次探测时间(ms)
	UpdateError     string `gorm:"type:text" json:"updateError"`     // 探测失败原因（成功时清空）
	BulkDone        bool   `gorm:"default:false" json:"bulkDone"`    // 已囤货（无差别下载→筛选已完成）
	BulkDoneAt      int64  `gorm:"default:0" json:"bulkDoneAt"`      // 已囤货确认时间(ms)
	// FieldsMigrated 内部标记：Round49 一次性字段回填（name → role/bulkDone）是否已完成。
	// 不对外暴露；迁移逻辑只处理该标记为 false 的记录，保证幂等。
	FieldsMigrated bool `gorm:"default:false" json:"-"`

	CreatedAt time.Time `json:"createdAt"`
}
