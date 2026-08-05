package models

import "time"

// EHSetting E站站点偏好与交互配置（多用户下每用户一条，取代全局单例）
// 其中 Site / PreferRedirect 为“当前生效”的配置快照，始终与所选 Profile 保持同步
type EHSetting struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	UserID          uint      `gorm:"index" json:"userId"`                  // 归属用户
	Site            string    `gorm:"default:'e-hentai'" json:"site"`       // "e-hentai" | "exhentai"
	PreferRedirect  bool      `gorm:"default:true" json:"preferRedirect"`   // 优先重定向至表站
	SelectedProfile string    `gorm:"default:''" json:"selectedProfile"`    // 当前选中的 Profile ID（字符串形式）
	UpdatedAt       time.Time `json:"updatedAt"`
}

// EHProfile 合并后的“站点设置 Profile”：把 E 站浏览预设与站点配置组织为可切换/保存的预设档
type EHProfile struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"index" json:"userId"`                      // 归属用户
	Name           string    `gorm:"not null" json:"name"`                     // 预设名称
	IsDefault      bool      `gorm:"default:false" json:"isDefault"`           // 是否为默认预设（不可删除）
	Site           string    `gorm:"default:'e-hentai'" json:"site"`           // "e-hentai" | "exhentai"
	PreferRedirect bool      `gorm:"default:true" json:"preferRedirect"`       // 优先重定向至表站
	RowsPerPage    int       `gorm:"default:40" json:"rowsPerPage"`            // 画廊列表每页条数 20/40/50
	TopListSize    int       `gorm:"default:10" json:"topListSize"`            // Toplist 显示条数
	Resolution     string    `gorm:"default:'auto'" json:"resolution"`         // 默认原图分辨率 auto/780/980/1280/1600/2400
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// EHUserStatus E 站动态配额与资产信息（由后端解析首页 / My Home 后返回，不落库）
type EHUserStatus struct {
	CurrentQuota int    `json:"currentQuota"` // 当前已使用配额
	MaxQuota     int    `json:"maxQuota"`     // 配额上限
	AssetGP      string `json:"assetGP"`      // GP 点数
	AssetCredits string `json:"assetCredits"` // Credits 积分
	AssetHath    string `json:"assetHath"`    // Hath 点数
}

// EHTagset 一个 Tagset（E 站 mytags 页顶部下拉的一个选项）
type EHTagset struct {
	ID    int    `json:"id"`    // Tagset 数字 ID（1 为默认集）
	Name  string `json:"name"`  // 显示名称，如 "Tagset #1"
	Count int    `json:"count"` // 该 Tagset 内标签数量（来自下拉文本 "(8)"）
}

// EHMyTags 我的标签（关注/隐藏），从 E 站 mytags 页实时读取
type EHMyTags struct {
	Watched       []string   `json:"watched"`       // 关注的标签
	Hidden        []string   `json:"hidden"`        // 隐藏的标签
	Tagsets       []EHTagset `json:"tagsets"`       // 全部 Tagset 列表（用于前端下拉）
	CurrentTagset int        `json:"currentTagset"` // 当前选中的 Tagset ID（1 为默认集）
}