package models

import "time"

// AccountSetting 存储 E 站账号 Cookie、权限及站点级偏好设置
type AccountSetting struct {
	ID          uint      `gorm:"primaryKey;default:1" json:"id"` // 固定 ID=1 保持单条记录
	IPBMemberID string    `gorm:"column:ipb_member_id;not null" json:"ipb_member_id"`
	IPBPassHash string    `gorm:"column:ipb_pass_hash;not null" json:"ipb_pass_hash"`
	Igneous     string    `gorm:"column:igneous" json:"igneous"`
	SK          string    `gorm:"column:sk" json:"sk,omitempty"`  // 存储 E 站偏好与排序 sk Cookie
	IsEx        bool      `gorm:"default:false" json:"isEx"`      // 是否具备 ExHentai 访问权限
	
	// 🟢 补全：对应前端 EHSetting 的站点偏好
	Site           string `gorm:"default:'e-hentai'" json:"site"`           // e-hentai | exhentai
	PreferRedirect bool   `gorm:"default:true" json:"preferRedirect"`       // 访问 ex 失败时是否自动降级/重定向
	SelectedProfile string `gorm:"default:''" json:"selectedProfile,omitempty"` // E 站配额/设置檔 Profile

	UpdatedAt time.Time `json:"updatedAt"`
}

// EHUserStatus E 站动态配额与资产信息（由后端解析 HTML 头部或 API 后返回，不必全部落库）
type EHUserStatus struct {
	CurrentQuota int    `json:"currentQuota"` // 当前用量
	MaxQuota     int    `json:"maxQuota"`     // 最大配额
	AssetGP      string `json:"assetGP"`      // 算力/GP 点数
	AssetCredits string `json:"assetCredits"` // Credits 积分
}

// EHFavoriteCategory E 站自定义收藏夹结构（0 ~ 9 槽位）
type EHFavoriteCategory struct {
	Index int    `json:"index"` // 0 - 9
	Name  string `json:"name"`  // 自定义名称（如：Favorites 0 或 "必看精品"）
	Count int    `json:"count"` // 收藏数量
}

// UserPreferences 系统/界面层面的通用用户设置
type UserPreferences struct {
	ID                 uint   `gorm:"primaryKey;default:1" json:"id"`
	Theme              string `gorm:"default:'system'" json:"theme"`              // light | dark | system
	CardViewMode       string `gorm:"default:'card'" json:"cardViewMode"`         // card | compact
	ImgResolution      string `gorm:"default:'auto'" json:"imgResolution"`         // res_auto | res_1280 | res_1600
	EnableImgProxy     bool   `gorm:"default:false" json:"enableImgProxy"`        // 是否启用图片加载代理
	ImgProxyServer     string `json:"imgProxyServer,omitempty"`                   // 自定义代理服务器
	AutoMarkReadOnEnd  bool   `gorm:"default:true" json:"autoMarkReadOnEnd"`      // 翻到最后一页自动标记已读
}