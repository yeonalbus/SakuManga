package services

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round28：搜刮书签服务（后端化，按用户隔离）
//
// 数据模型：config / anchor 以 JSON 字符串落库，对外统一以
// json.RawMessage 透传原始 JSON（前端自行解析 + 兜底），
// 避免「服务端强校验把脏数据拒之门外」造成书签丢失（BUG2 教训：
// 原 localStorage 恢复校验过严会静默丢弃整条书签）。
// ─────────────────────────────────────────────────────────────

// ErrBookmarkNotFound 书签不存在（或不属于当前用户）
var ErrBookmarkNotFound = errors.New("书签不存在")

// ScrapeBookmarkDTO 对外书签结构（config/anchor 原始 JSON 透传）
type ScrapeBookmarkDTO struct {
	ID        uint            `json:"id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Keyword   string          `json:"keyword"`
	Config    json.RawMessage `json:"config"`
	Anchor    json.RawMessage `json:"anchor"`
	CreatedAt time.Time       `json:"createdAt"`
}

// ScrapeBookmarkInput 创建入参
type ScrapeBookmarkInput struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Keyword string          `json:"keyword"`
	Config  json.RawMessage `json:"config"`
	Anchor  json.RawMessage `json:"anchor"`
}

// normalizeScrapeBookmark 宽松归一化：坏字段兜底，不拒绝（防丢数据）。
func normalizeScrapeBookmark(in ScrapeBookmarkInput) (name, typ string, config, anchor string) {
	name = strings.TrimSpace(in.Name)
	if name == "" {
		name = "未命名书签"
	}
	typ = in.Type
	if typ != "search" {
		typ = "home" // 仅 home/search 两种，其余归 home
	}
	// config 必须是合法 JSON 对象；解析失败回退最小合法值（不丢书签）
	config = string(in.Config)
	if len(config) > 0 {
		var probe map[string]any
		if err := json.Unmarshal(in.Config, &probe); err != nil || probe == nil {
			config = "{}"
		}
	} else {
		config = "{}"
	}
	// anchor 允许 null 或 {gid,...}；解析失败置 null
	anchor = string(in.Anchor)
	if len(anchor) > 0 && anchor != "null" {
		var probe map[string]any
		if err := json.Unmarshal(in.Anchor, &probe); err != nil || probe == nil {
			anchor = "null"
		}
	} else {
		anchor = "null"
	}
	return name, typ, config, anchor
}

// toDTO 模型 → 对外结构
func toScrapeBookmarkDTO(m *models.ScrapeBookmark) ScrapeBookmarkDTO {
	return ScrapeBookmarkDTO{
		ID:        m.ID,
		Name:      m.Name,
		Type:      m.Type,
		Keyword:   m.Keyword,
		Config:    json.RawMessage(m.Config),
		Anchor:    json.RawMessage(m.Anchor),
		CreatedAt: m.CreatedAt,
	}
}

// ListScrapeBookmarks 读取当前用户全部书签（按创建时间升序）
func ListScrapeBookmarks(db *gorm.DB, userID uint) ([]ScrapeBookmarkDTO, error) {
	var marks []models.ScrapeBookmark
	if err := db.Where("user_id = ?", userID).Order("id ASC").Find(&marks).Error; err != nil {
		return nil, err
	}
	out := make([]ScrapeBookmarkDTO, 0, len(marks))
	for i := range marks {
		out = append(out, toScrapeBookmarkDTO(&marks[i]))
	}
	return out, nil
}

// CreateScrapeBookmark 新增书签，返回带 id 的完整结构
func CreateScrapeBookmark(db *gorm.DB, userID uint, in ScrapeBookmarkInput) (*ScrapeBookmarkDTO, error) {
	name, typ, config, anchor := normalizeScrapeBookmark(in)
	m := models.ScrapeBookmark{
		UserID:    userID,
		Name:      name,
		Type:      typ,
		Keyword:   strings.TrimSpace(in.Keyword),
		Config:    config,
		Anchor:    anchor,
		CreatedAt: time.Now(),
	}
	if err := db.Create(&m).Error; err != nil {
		return nil, err
	}
	dto := toScrapeBookmarkDTO(&m)
	return &dto, nil
}

// RenameScrapeBookmark 重命名书签（空名忽略，保持原名）
func RenameScrapeBookmark(db *gorm.DB, userID uint, id uint, name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil
	}
	res := db.Model(&models.ScrapeBookmark{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("name", trimmed)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrBookmarkNotFound
	}
	return nil
}

// DeleteScrapeBookmark 删除书签（仅限本人）
func DeleteScrapeBookmark(db *gorm.DB, userID uint, id uint) error {
	res := db.Where("id = ? AND user_id = ?", id, userID).Delete(&models.ScrapeBookmark{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrBookmarkNotFound
	}
	return nil
}
