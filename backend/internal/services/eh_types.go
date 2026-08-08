package services

// EHService E站核心服务
type EHService struct{}

func NewEHService() *EHService {
	return &EHService{}
}

// SearchParams 前端发来的搜索请求参数
type SearchParams struct {
	Keyword          string   `form:"keyword"`
	Page             int      `form:"page"`      // 仅线下/兼容保留 (1-based)
	Next             string   `form:"next"`      // 下一页 GID 游标 (loadMore)
	Prev             string   `form:"prev"`      // 上一页 GID 游标 (loadBefore)
	Seek             string   `form:"seek"`      // 按日期跳转 (格式如 "2023-05-20" 或 Unix 时间戳)
	ActiveCategories []string `form:"categories"`

	// ─── E-Hentai 高级筛选 (需配合 advsearch=1 生效) ───
	MinRating             string `form:"minRating"`               // f_srdd=星级 (如 4 / 4.5)
	Language              string `form:"language"`                // All | Chinese | Japanese | English → 并入 f_search
	OnlyRemoved           bool   `form:"onlyRemoved"`             // f_sh=on 仅搜索移除了的画廊
	OnlyTorrents          bool   `form:"onlyTorrents"`            // f_sto=on 只显示有种子的画廊
	DisableLangFilter     bool   `form:"disableLangFilter"`       // f_sfl=on 禁用语言过滤
	DisableUploaderFilter bool   `form:"disableUploaderFilter"`   // f_sfu=on 禁用上传者过滤
	DisableTagFilter      bool   `form:"disableTagFilter"`        // f_sft=on 禁用 Tag 过滤
}

// OnlineComicResult 抓取结果与分页信息
type OnlineComicResult struct {
	Comics      []OnlineComicDTO `json:"comics"`
	TotalPages  int              `json:"totalPages,omitempty"`  // 线上模式可不返回
	CurrentPage int              `json:"currentPage,omitempty"` // 线上模式可不返回
	Next        string           `json:"next,omitempty"`        // 下页游标锚点 GID
	Prev        string           `json:"prev,omitempty"`        // 上页游标锚点 GID
	HasMore     bool             `json:"hasMore"`               // 是否还能继续下滑加载
}

type OnlineComicDTO struct {
	ID           string   `json:"id"`
	Token        string   `json:"token,omitempty"`
	Title        string   `json:"title"`
	CoverURL     string   `json:"coverUrl"`
	Source       string   `json:"source"`
	Category     string   `json:"category,omitempty"`
	Rating       float64  `json:"rating,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	PageCount    int      `json:"pageCount,omitempty"`
	UpdatedAt    string   `json:"updatedAt"`
	Uploader     string   `json:"uploader,omitempty"`
	IsFavorite   bool     `json:"isFavorite"`
	FavIndex     *int     `json:"favIndex,omitempty"`
	IsDownloaded bool     `json:"isDownloaded"`
	ClickCount   int      `json:"clickCount,omitempty"`
}

// GalleryRelation 在线详情页中发现的关系画廊（父/子/新版）
type GalleryRelation struct {
	GID     string `json:"gid"`
	Token   string `json:"token"`
	AddedAt string `json:"addedAt,omitempty"` // "newer versions" 列表中的 added 发布时间（如 2026-07-30 12:13）
}

type GalleryDetailResult struct {
	ID               string            `json:"id"`
	Token            string            `json:"token"`
	ParentGID        string            `json:"parentGID,omitempty"` // 父画廊 gid（本画廊是某画廊的更新版）
	ParentToken      string            `json:"parentToken,omitempty"`
	NewVersionGID    string            `json:"newVersionGID,omitempty"` // #dms "newer version" 横幅指向的更新版 gid（本画廊已被取代）
	NewVersionToken  string            `json:"newVersionToken,omitempty"`
	Children         []GalleryRelation `json:"children,omitempty"` // 子画廊列表（#gnd / child_ 链接，即更新版）
	Title            string            `json:"title"`              // 主标题 (日文/原名)
	SubTitle         string            `json:"subTitle"`           // 副标题 (英文/译名)
	CoverURL         string            `json:"coverUrl"`
	Source           string            `json:"source"` // "online"
	Category         string            `json:"category"`
	Uploader         string            `json:"uploader"`
	Rating           float64           `json:"rating"`
	PageCount        int               `json:"pageCount"`
	UpdatedAt        string            `json:"updatedAt"`
	Tags             []string          `json:"tags"`         // 格式: ["female:big breasts", "artist:okuma"]
	PreviewPages     []PreviewPageDTO  `json:"previewPages"` // 预览切片列表
	Comments         []CommentDTO      `json:"comments"`     // 社区评论列表
	IsFavorite       bool              `json:"isFavorite"`
	FavIndex         *int              `json:"favIndex"`
	IsDownloaded     bool              `json:"isDownloaded"` // 本地离线库是否已存在同 GID（供下载去重提示）
	MaxPreviewPage   int               `json:"maxPreviewPage"`
	Local            *GalleryLocalInfo `json:"local,omitempty"` // S1 本地优先：本地库存在同 GID 画廊时附加
}

// GalleryLocalInfo 详情页「本地优先」（S1）附加信息
// 开启本地优先且按 GID 查到本地 OfflineComic 时返回；元数据与评论仍在线抓取，
// 前端据此将预览/阅读页图改走本地接口 /comics/:id/page/:index。
type GalleryLocalInfo struct {
	ComicID     string `json:"comicId"`     // 本地 OfflineComic.ID
	PageCount   int    `json:"pageCount"`   // 本地页数
	CoverURL    string `json:"coverUrl"`    // 本地封面路由 /api/v1/comics/:id/cover
	LocalPath   string `json:"localPath"`   // 本地存储绝对路径
	HasComments bool   `json:"hasComments"` // 在线详情是否解析出社区评论
}

type PreviewPageDTO struct {
	PageIndex int    `json:"pageIndex"` // 第几页 (1-based)
	ImageURL  string `json:"url"`       // 图片/雪碧图地址
	IsSprite  bool   `json:"isSprite"`  // 是否为 CSS 雪碧图
	OffsetX   int    `json:"offsetX"`   // X 轴偏移量 (px)
	OffsetY   int    `json:"offsetY"`   // Y 轴偏移量 (px)
	Width     int    `json:"width"`     // 单张切片宽度 (px)
	Height    int    `json:"height"`    // 单张切片高度 (px)
}

type CommentDTO struct {
	ID      int64  `json:"id"`
	User    string `json:"user"`
	Date    string `json:"date"`
	Content string `json:"content"`
}

// categoryBitmaskMap 分类掩码映射表
var categoryBitmaskMap = map[string]int{
	"Misc":       1,
	"Doujinshi":  2,
	"Manga":      4,
	"Artist CG":  8,
	"Game CG":    16,
	"Image Set":  32,
	"Cosplay":    64,
	"Asian Porn": 128,
	"Non-H":      256,
	"Western":    512,
}