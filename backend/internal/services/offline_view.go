package services

import (
	"time"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round50 E：维护结果瘦身——结果里不再内嵌完整的 models.OfflineComic
//
// 背景：DedupItem / ClusterMember 此前直接持有 models.OfflineComic（34 个非空字段），
// 单次 `/offline/maintain/result` 实测 923 KB（线上 209 组 / 444 成员），其中约 56%
// 是页面从不读取、或只用于后端判定的字段（首次入库标题备份、8 个时间戳、fileHash、
// downloadScheme、hiddenPages、token、parentGID 等）。前端每渲染一次都要传输 + 解析 +
// 参与响应式代理，是"刷新一次卡一下"的固定成本。
//
// 本视图只保留维护页 / 对比页真正渲染用到的字段（实测约 −39% 体积）。
// 后端自己需要的判定信息（如「忽略后新增」依赖 addedAt）放在结构体的非导出字段里，
// 不进 JSON——新增字段请先确认前端确实消费，避免 DTO 再次膨胀。
// ─────────────────────────────────────────────────────────────

// MaintainComicView 维护结果中的漫画视图（页面渲染必需字段）
type MaintainComicView struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	CoverURL   string    `json:"coverUrl"`
	LocalPath  string    `json:"localPath"`
	FileSize   int64     `json:"fileSize"`
	PageCount  int       `json:"pageCount"`
	SourceMode string    `json:"sourceMode,omitempty"` // gallery | archive
	GID        string    `json:"gid,omitempty"`        // 规则「忽略此提示」需要
	UpdatedAt  time.Time `json:"updatedAt"`
	Tags       string    `json:"tags"`                 // 决策标签判定（与 onlineTags 取并集）
	OnlineTags string    `json:"onlineTags,omitempty"` // E 站官方 tag JSON（决策标签主来源）
	// Round52：社区评分（维护卡第三行展示；0=未知，前端显示 ★ —）
	CommunityRating float64 `json:"communityRating"`
}

// toMaintainComicView 由完整模型投影视图（唯一投影入口，避免字段口径漂移）
func toMaintainComicView(c models.OfflineComic) MaintainComicView {
	return MaintainComicView{
		ID:         c.ID,
		Title:      c.Title,
		CoverURL:   c.CoverURL,
		LocalPath:  c.LocalPath,
		FileSize:   c.FileSize,
		PageCount:  c.PageCount,
		SourceMode: c.SourceMode,
		GID:        c.GID,
		UpdatedAt:  c.UpdatedAt,
		Tags:       c.Tags,
		OnlineTags: c.OnlineTags,
		CommunityRating: c.CommunityRating,
	}
}

// toMaintainComicViewPtr 指针版本（成对对象用）
func toMaintainComicViewPtr(c models.OfflineComic) *MaintainComicView {
	v := toMaintainComicView(c)
	return &v
}
