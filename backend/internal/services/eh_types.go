package services

// EHService E站核心服务
type EHService struct{}

func NewEHService() *EHService {
	return &EHService{}
}

// SearchParams 前端发来的搜索请求参数
type SearchParams struct {
	Keyword          string   `form:"keyword"`
	Page             int      `form:"page"` // 1-based，前端第 1 页对应 E 站 p=0
	Next             string   `form:"next"` // 支持传递 GID 游标 (可选)
	ActiveCategories []string `form:"categories"`
}

// OnlineComicResult 抓取结果与分页信息
type OnlineComicResult struct {
	Comics      []OnlineComicDTO `json:"comics"`
	TotalPages  int              `json:"totalPages"`
	CurrentPage int              `json:"currentPage"`
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

type GalleryDetailResult struct {
	ID           string           `json:"id"`
	Token        string           `json:"token"`
	Title        string           `json:"title"`    // 主标题 (日文/原名)
	SubTitle     string           `json:"subTitle"` // 副标题 (英文/译名)
	CoverURL     string           `json:"coverUrl"`
	Source       string           `json:"source"` // "online"
	Category     string           `json:"category"`
	Uploader     string           `json:"uploader"`
	Rating       float64          `json:"rating"`
	PageCount    int              `json:"pageCount"`
	UpdatedAt    string           `json:"updatedAt"`
	Tags         []string         `json:"tags"`         // 格式: ["female:big breasts", "artist:okuma"]
	PreviewPages []PreviewPageDTO `json:"previewPages"` // 预览切片列表
	Comments     []CommentDTO     `json:"comments"`     // 社区评论列表
	IsFavorite   bool             `json:"isFavorite"`
	FavIndex     *int             `json:"favIndex"`
	MaxPreviewPage int            `json:"maxPreviewPage"`
}

type PreviewPageDTO struct {
	PageIndex int    `json:"pageIndex"` // 第几页 (1-based)
	ImageURL  string `json:"url"`       // 代理后的缩略图地址
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