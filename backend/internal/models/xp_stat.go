package models

import "time"

// ─────────────────────────────────────────────────────────────
// Round30 阶段一：XP 词云统计（本地库 tag 偏好画像）
//
// 三张表分工：
//   XpComicStat  单本漫画的贡献快照（增量差分的基准，改一本只动这一本）
//   XpTagStat    tag 聚合权重（按 namespace+key，词云展示与偏好推荐的数据源）
//   XpMeta       统计元信息（公式版本 / 全量重建时间 / 脏标记）
//
// 口径（详见 plans/round30-xp-cloud-recommend-plan.md）：
//   有效 tag = MergeTags(onlineTags, offlineAddTags, offlineRemoveTags)（三态全空回退 Tags）
//   库藏侧   lib(c) = 1
//   阅读侧   r(c)   = 0.60·log1p(readCount) + 0.25·exp(-Δdays/90) + 0.15·(rating-1)/9
//   tag 权重 libWeight(t)/readWeight(t) = Σ_c signal(c) / |tags(c)|（稀释：防泛化标签淹没核心 tag）
//
// 阅读侧按 user_id 隔离（历史/评分本就是用户级数据），库藏侧在各单位用户下天然一致。
// ─────────────────────────────────────────────────────────────

// XpComicStat 单本漫画的 XP 贡献快照（复合主键：user_id + comic_id）
//
// LibWeight/ReadWeight 存「该本信号原始值」，tag 侧的稀释分摊在写入聚合表时完成；
// TagsJSON 存排序后的有效 tag 数组：既用于跳过未变化的重复重算，
// 也是差分减法取「旧 tag 集合」的唯一依据（只存指纹无法反推旧集合）。
type XpComicStat struct {
	UserID     uint      `gorm:"primaryKey" json:"userId"`
	ComicID    string    `gorm:"primaryKey" json:"comicId"`
	LibWeight  float64   `json:"libWeight"`                 // 库藏侧信号（当前恒为 1）
	ReadWeight float64   `json:"readWeight"`                // 阅读侧信号（多信号融合后原始值，未归一）
	TagCount   int       `json:"tagCount"`                  // 参与统计的有效 tag 数（稀释分母）
	TagsJSON   string    `gorm:"type:text" json:"tagsJson"` // 有效 tag 集合（排序后 JSON 数组）
	UpdatedAt  time.Time `json:"updatedAt"`
}

// TableName 固定表名，避免 GORM 命名策略对 Xp 前缀的歧义
func (XpComicStat) TableName() string { return "xp_comic_stats" }

// XpTagStat tag 聚合权重（复合主键：user_id + namespace + tag_key）
//
// ComicCount 含该 tag 的本数，用于泛化抑制（IDF）与词云 tooltip 展示。
type XpTagStat struct {
	UserID     uint      `gorm:"primaryKey" json:"userId"`
	Namespace  string    `gorm:"primaryKey" json:"namespace"`
	TagKey     string    `gorm:"primaryKey" json:"tagKey"`
	LibWeight  float64   `json:"libWeight"`  // Σ lib(c)/|tags(c)|
	ReadWeight float64   `json:"readWeight"` // Σ r(c)/|tags(c)|
	ComicCount int       `json:"comicCount"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// TableName 固定表名
func (XpTagStat) TableName() string { return "xp_tag_stats" }

// XpMeta XP 统计元信息（单例 ID=1）
//
// FormulaVersion 与代码常量不一致（公式调整/数据结构变更）时，下次查询自动全量重建；
// Dirty 用于增量写入失败或数据迁移后的兜底重建。
type XpMeta struct {
	ID             uint  `gorm:"primaryKey;default:1" json:"id"`
	SchemaVersion  int   `json:"schemaVersion"`
	FormulaVersion int   `json:"formulaVersion"`
	LastRebuildAt  int64 `json:"lastRebuildAt"` // Unix 时间戳(ms)
	Dirty          bool  `json:"dirty"`
}

// TableName 固定表名
func (XpMeta) TableName() string { return "xp_meta" }
