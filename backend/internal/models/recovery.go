package models

import "time"

// ─────────────────────────────────────────────────────────────
// Round54：恢复与阵亡（数据恢复功能）
// 设计文档：DSPlans/plans/round54-recovery-fallen-plan.md
//
// 核心原则：g_id 是狗牌——恢复、阵亡、去重、剔除全部以 g_id 为准，
// comic_id 仅作关联辅助（更新任务会让 comic_id 变化）。
// ─────────────────────────────────────────────────────────────

// 阵亡原因（fallen_comics.reason），与设计文档 6.3 节保持一致
const (
	FallenReasonRemoved     = "removed"       // EH 版权/下架（removed_status=1 或恢复任务 404/copyright）
	FallenReasonUnlockFail  = "unlock-failed" // 密码压缩包无法解锁
	FallenReasonNoEhentai   = "no-ehentai"    // 非 E 站来源（无 g_id/token，无法重下）
	FallenReasonPathRemoved = "path-removed"  // 扫描路径被移除，未恢复候选转写
	FallenReasonAbandoned   = "abandoned"     // 用户手动清空候选，放弃恢复
)

// RecoveryCandidate 恢复候选（全量扫描失效对账的产物，自包含）。
//
// 主键采用 g_id：只有「失效 + g_id/token 非空 + 未标记 removed」的记录才进候选。
// 自包含 = 携带 token/标题/封面等全部恢复所需数据，即使 offline_comics
// 中对应记录被删（如移除扫描路径连带清理），候选仍可支撑恢复。
type RecoveryCandidate struct {
	GID        string    `gorm:"primaryKey" json:"gid"`
	Token      string    `json:"token"`
	Title      string    `gorm:"type:text" json:"title"`
	TitleJpn   string    `gorm:"type:text" json:"titleJpn,omitempty"`
	CoverURL   string    `json:"coverUrl,omitempty"`
	Category   string    `json:"category,omitempty"`
	PageCount  int       `json:"pageCount"`
	FileSize   int64     `json:"fileSize"`
	LocalPath  string    `json:"localPath"`                         // 失效原路径快照（展示/审计用）
	ScanPathID string    `gorm:"index" json:"scanPathId,omitempty"` // 原属额外扫描路径（扫描锁推导用）
	ComicID    string    `json:"comicId,omitempty"`                 // 原 offline_comics 记录 ID（删除时机关联）
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// FallenComic 阵亡名单条目（全局单一名单，快照化，不依赖 offline_comics 记录）。
//
// DedupeKey 为显式去重键：有 g_id → "gid:<gid>"；无 g_id（no-ehentai）→ "cid:<comic_id>"。
// 原因：SQLite 唯一索引对 NULL 不去重，而 no-ehentai 条目没有 g_id，
// 必须用显式键保证重复对账时 upsert 幂等。
type FallenComic struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	DedupeKey   string     `gorm:"uniqueIndex" json:"-"`
	GID         string     `gorm:"index" json:"gid,omitempty"`
	ComicID     string     `json:"comicId,omitempty"`
	Title       string     `gorm:"type:text" json:"title"`
	TitleJpn    string     `gorm:"type:text" json:"titleJpn,omitempty"`
	CoverURL    string     `json:"coverUrl,omitempty"`
	Category    string     `json:"category,omitempty"`
	FileSize    int64      `json:"fileSize"`
	PageCount   int        `json:"pageCount"`
	Reason      string     `gorm:"index" json:"reason"`
	SourcePath  string     `json:"sourcePath,omitempty"`           // 来源扫描路径快照
	Dismissed   bool       `gorm:"default:false" json:"dismissed"` // 用户手动 x：划线 + 置底，保留显示
	DismissedAt *time.Time `json:"dismissedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}
