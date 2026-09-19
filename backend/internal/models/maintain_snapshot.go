package models

// MaintainDedupSnapshot 维护查重结果快照（Round42 决策 D6：结果持久化）
//
// 背景：维护查重结果此前仅存于进程内存（offlineMaintainRes），重启即丢、必须重扫
// （数千本联网核对，数十分钟）。本表把结果落库，使进维护页可秒开上次结果。
//
// 设计取舍：单行快照（固定 ID=1，覆盖写）。前端读取的是整份结果（items + clusters），
// 不需要按簇查询，故不做规范化拆表，直接存 JSON —— 改动面最小、事务上最安全。
type MaintainDedupSnapshot struct {
	ID uint `gorm:"primaryKey" json:"id"`

	// Payload DedupResult 的 JSON（items + clusters + finishedAt + stale）
	Payload string `gorm:"type:text" json:"-"`

	// ForceFull 本次结果是否来自「全量在线核对」。
	// DedupResult.forceFull 是未导出字段（不参与 JSON），故单独存列以便还原。
	ForceFull bool `json:"forceFull"`

	// FinishedAt 结果生成时间戳(ms)，冗余一列便于排查与排序
	FinishedAt int64 `gorm:"index" json:"finishedAt"`

	// GeneratedAt 本行写入时间戳(ms)
	GeneratedAt int64 `json:"generatedAt"`
}
