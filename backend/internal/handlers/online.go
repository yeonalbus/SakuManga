package handlers

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"SakuHentai/internal/middleware"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
)

type OnlineComicHandler struct {
	db        *gorm.DB
	ehService *services.EHService
}

func NewOnlineComicHandler(db *gorm.DB, ehService *services.EHService) *OnlineComicHandler {
	return &OnlineComicHandler{db: db, ehService: ehService}
}

// 获取或初始化 EHSetting 配置
func getEHSetting(db *gorm.DB, userID uint) *models.EHSetting {
	var setting models.EHSetting
	// 1. 按用户 ID 查找（多用户隔离）
	if err := db.Where("user_id = ?", userID).First(&setting).Error; err != nil {
		// 2. 只有查不到记录时，才插入默认配置
		setting = models.EHSetting{
			UserID:         userID,
			Site:           "e-hentai",
			PreferRedirect: true,
		}
		db.Create(&setting)
	}
	return &setting
}

// writeEHServiceError 统一把 E 站服务错误映射为 HTTP 响应：
// 画廊不可用（已删除/版权下架）返回明确状态码（410/403）与友好提示，其余保持 500
func writeEHServiceError(c *gin.Context, err error) {
	var gErr *services.ErrGalleryUnavailable
	if errors.As(err, &gErr) {
		status := http.StatusInternalServerError
		switch gErr.Kind {
		case "removed":
			status = http.StatusGone // 410 Gone：画廊已删除/不可用
		case "copyright":
			status = http.StatusForbidden // 403：版权下架
		}
		c.JSON(status, gin.H{"error": gErr.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

// GetOnlineComics 抓取线上画廊列表 (首页 /)
func (h *OnlineComicHandler) GetOnlineComics(c *gin.Context) {
	account := middleware.CurrentAccount(c)
	if account == nil || account.IPBMemberID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	var params services.SearchParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "非法请求参数"})
		return
	}

	if params.Page <= 0 {
		params.Page = 1
	}

	ehSetting := getEHSetting(h.db, account.ID)
	result, err := h.ehService.FetchGalleryList(account, params, ehSetting)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 🟢 解构 result.Comics 并挂载本地 SQLite 收藏状态 + 本地已下载状态
	comics := services.AttachFavoriteStates(h.db, account.ID, result.Comics)
	comics = services.AttachDownloadStates(h.db, comics)

	c.JSON(http.StatusOK, gin.H{
		"comics":      comics,
		"totalPages":  result.TotalPages,
		"currentPage": result.CurrentPage,
		"next":        result.Next,    // 向下游标
		"prev":        result.Prev,    // 向上游标
		"hasMore":     result.HasMore, // 是否有更多标记
	})
}

// 🟢 新增：GetWatchedComics 抓取线上订阅列表 (/watched)
func (h *OnlineComicHandler) GetWatchedComics(c *gin.Context) {
	account := middleware.CurrentAccount(c)
	if account == nil || account.IPBMemberID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	var params services.SearchParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "非法请求参数"})
		return
	}

	if params.Page <= 0 {
		params.Page = 1
	}

	ehSetting := getEHSetting(h.db, account.ID)
	// 调用 services/eh_sub.go 的 FetchWatchedList
	result, err := h.ehService.FetchWatchedList(account, params, ehSetting)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 挂载本地 SQLite 收藏状态 + 本地已下载状态
	comics := services.AttachFavoriteStates(h.db, account.ID, result.Comics)
	comics = services.AttachDownloadStates(h.db, comics)

	c.JSON(http.StatusOK, gin.H{
		"comics":      comics,
		"totalPages":  result.TotalPages,
		"currentPage": result.CurrentPage,
		"next":        result.Next,    // 向下游标
		"prev":        result.Prev,    // 向上游标
		"hasMore":     result.HasMore, // 是否有更多标记
	})
}

// ehImageHosts E 站图片 / CDN 域名白名单（后缀匹配），用于封面代理防 SSRF
var ehImageHosts = []string{
	"s.exhentai.org",
	"exhentai.org",
	"e-hentai.org",
	"ehgt.org",
	"hentai-cdn.com",
	"hath.network",
}

// isEHImageHost 校验目标 url 的 host 是否属于 E 站图片域名白名单（防止代理任意内网/外部地址造成 SSRF）
func isEHImageHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return false
	}
	for _, allowed := range ehImageHosts {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

// resolveProxyAccount 封面代理的可选认证解析：
//  1. 优先解析请求携带的 token（Authorization: Bearer / query token）定位当前登录用户，
//     使用该用户绑定的 E 站凭证（并注入 context 供后续复用）；
//  2. 未登录 / 用户未绑定凭证时，回退使用 admin 账号的 E 站凭证；
//  3. 两者都无凭证（IPBMemberID 为空）时返回 nil，由调用方判定 401。
func (h *OnlineComicHandler) resolveProxyAccount(c *gin.Context) *models.AccountSetting {
	auth := c.GetHeader("Authorization")
	token := ""
	if strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimPrefix(auth, "Bearer ")
	}
	if token == "" {
		token = c.Query("token")
	}
	if token != "" {
		var session models.UserSession
		if err := h.db.Where("token = ?", token).First(&session).Error; err == nil {
			var user models.User
			if err := h.db.First(&user, session.UserID).Error; err == nil {
				c.Set(middleware.ContextTokenKey, token)
				c.Set(middleware.ContextUserKey, &user)
				return &models.AccountSetting{
					ID:          user.ID,
					IPBMemberID: user.IPBMemberID,
					IPBPassHash: user.IPBPassHash,
					Igneous:     user.Igneous,
					SK:          user.SK,
					IsEx:        user.IsEx,
				}
			}
		}
	}
	// 回退：admin 账号凭证（后台固定维护账号）
	return services.LoadAdminAccount(h.db)
}

// ProxyCover 代理转发 ExHentai / E-Hentai 的封面图片（可选认证，兼容 <img> 媒体加载）
func (h *OnlineComicHandler) ProxyCover(c *gin.Context) {
	targetURL := c.Query("url")
	if targetURL == "" {
		c.Status(http.StatusBadRequest)
		return
	}

	// 域名白名单校验，防止代理任意内网/外部地址（SSRF）
	if !isEHImageHost(targetURL) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "仅允许代理 E 站图片域名"})
		return
	}

	account := h.resolveProxyAccount(c)
	if account == nil || account.IPBMemberID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	client, err := h.ehService.BuildCoverClient(account)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	// E 站图片 CDN 偶发超时/限流（网络错误、5xx、429、403），单次失败直接 502 会让列表封面
	// 大面积空白且无法自愈。这里对临时性失败做有限次退避重试：
	//   - 网络层错误 / 5xx / 429 视为临时性，最多重试 3 次，并计入健康统计；
	//   - 403 可能为凭证失效（永久）或风控限流（临时），仅额外重试 1 次观察，不计入健康；
	//   - 404/410（图不存在）等确定性 4xx 不重试，不计入健康；
	// 退避间隔递增并叠加随机抖动错峰；隧道抖动（健康降级）时自动拉长退避并收敛并发，
	// 避免把隧道连接数打爆（EOF）。
	const coverMaxRetries = 3

	var (
		resp    *http.Response
		lastErr error
	)
	for attempt := 0; attempt <= coverMaxRetries; attempt++ {
		if attempt > 0 {
			delay := services.CoverBackoffFor(attempt) + time.Duration(rand.IntN(400))*time.Millisecond
			select {
			case <-time.After(delay):
			case <-c.Request.Context().Done():
				return // 客户端已断开，终止重试
			}
		}

		req, reqErr := http.NewRequestWithContext(c.Request.Context(), "GET", targetURL, nil)
		if reqErr != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Referer", "https://exhentai.org/")

		// 并发闸门：超过当前并发上限的请求在此排队（降级时自然错峰，避免打爆隧道）
		releaseCover := services.AcquireCoverSlot()
		resp, err = client.Do(req)
		releaseCover()

		if err != nil {
			lastErr = err
			services.RecordCoverResult(false) // 网络层错误（超时/连接重置/EOF）→ 计入健康
			continue                           // 网络层错误 → 重试
		}

		// 成功：立即透传
		if resp.StatusCode == http.StatusOK {
			services.RecordCoverResult(true)
			break
		}

		// 图不存在等确定性 4xx：不重试，且与隧道健康无关，不计入统计
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
			resp.Body.Close()
			c.Status(http.StatusNotFound)
			return
		}
		if resp.StatusCode == http.StatusForbidden {
			resp.Body.Close()
			lastErr = fmt.Errorf("E 站图片返回 403")
			if attempt >= 1 {
				c.Status(http.StatusForbidden)
				return
			}
			continue // 403（凭证/风控）非隧道问题，不计入健康统计
		}

		// 其余（5xx / 429 等）视为临时性 → 计入健康并重试
		resp.Body.Close()
		lastErr = fmt.Errorf("E 站图片返回状态码 %d", resp.StatusCode)
		services.RecordCoverResult(false)
	}

	if resp == nil || resp.StatusCode != http.StatusOK {
		log.Printf("[COVER-PROXY] 代理封面失败 url=%s err=%v", targetURL, lastErr)
		c.Status(http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	c.Header("Cache-Control", "public, max-age=86400")

	_, _ = io.Copy(c.Writer, resp.Body)
}

// GetOnlineComicDetail 获取画廊详情
func (h *OnlineComicHandler) GetOnlineComicDetail(c *gin.Context) {
	gid := c.Query("id")
	token := c.Query("token")

	if gid == "" || token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失，必需传递 id (GID) 和 token"})
		return
	}

	account := middleware.CurrentAccount(c)
	if account == nil || account.IPBMemberID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	ehSetting := getEHSetting(h.db, account.ID)
	detail, err := h.ehService.FetchGalleryDetail(account, gid, token, ehSetting)
	if err != nil {
		writeEHServiceError(c, err)
		return
	}

	// 1. 如果 E 站网页端成功解析出了收藏状态，同步刷新到 SQLite 本地库
	if detail.IsFavorite && detail.FavIndex != nil {
		favState := models.FavoriteState{
			UserID: account.ID,
			GID:    gid,
			Token:  token,
			FavCat: *detail.FavIndex,
		}
		h.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "g_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"fav_cat", "token", "updated_at"}),
		}).Create(&favState)
	} else {
		// 2. 如果网页端未解析出，使用 SQLite 中的本地数据兜底
		detail = services.AttachDetailFavoriteState(h.db, account.ID, detail)
	}

	// 3. 本地优先（S1）：开启本地优先且按 GID 查到本地 OfflineComic 时附加 local 信息
	//    元数据与评论仍在线抓取；前端据此将预览/阅读页图改走本地接口 /comics/:id/page/:index
	preferLocal := c.Query("preferLocal") == "1" || c.Query("preferLocal") == "true"
	if preferLocal && detail != nil {
		var local models.OfflineComic
		if err := h.db.Where("g_id = ?", gid).First(&local).Error; err == nil {
			detail.Local = &services.GalleryLocalInfo{
				ComicID:     local.ID,
				PageCount:   local.PageCount,
				CoverURL:    local.CoverURL,
				LocalPath:   local.LocalPath,
				HasComments: len(detail.Comments) > 0,
			}
		}
	}

	c.JSON(http.StatusOK, detail)
}

// ResolveOnlineToken 按 gid 解析在线画廊 token GET /comics/online/resolve-token?id=<gid>
// 用途：历史旧记录未持久化 token 时兜底。优先查本地已存 token（收藏表），否则抓 /g/<gid>/ 页面解析。
func (h *OnlineComicHandler) ResolveOnlineToken(c *gin.Context) {
	account := middleware.CurrentAccount(c)
	if account == nil || account.IPBMemberID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}
	gid := strings.TrimSpace(c.Query("id"))
	if gid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 id (GID) 参数"})
		return
	}

	// 1. 本地兜底：收藏表可能已持久化该 gid 的 token
	var fav models.FavoriteState
	if err := h.db.Where("user_id = ? AND g_id = ?", account.ID, gid).First(&fav).Error; err == nil && fav.Token != "" {
		c.JSON(http.StatusOK, gin.H{"gid": gid, "token": fav.Token})
		return
	}

	// 2. 在线解析：抓取 /g/<gid>/ 页面从 missing-key 链接提取 token
	ehSetting := getEHSetting(h.db, account.ID)
	token, err := h.ehService.ResolveGalleryToken(account, gid, ehSetting)
	if err != nil {
		writeEHServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"gid": gid, "token": token})
}

// GetOnlinePopular 获取线上热门画廊列表
func (h *OnlineComicHandler) GetOnlinePopular(c *gin.Context) {
	account := middleware.CurrentAccount(c)
	if account == nil || account.IPBMemberID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	ehSetting := getEHSetting(h.db, account.ID)
	comics, err := h.ehService.FetchPopularList(account, ehSetting)
	if err != nil {
		writeEHServiceError(c, err)
		return
	}

	comics = services.AttachFavoriteStates(h.db, account.ID, comics)
	comics = services.AttachDownloadStates(h.db, comics)

	c.JSON(http.StatusOK, gin.H{
		"comics": comics,
	})
}

// GetOnlineComicPreviews 独立获取指定页码的预览图列表
func (h *OnlineComicHandler) GetOnlineComicPreviews(c *gin.Context) {
	gid := c.Query("id")
	token := c.Query("token")
	pageStr := c.DefaultQuery("page", "1")

	if gid == "" || token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失，必需传递 id (GID) 和 token"})
		return
	}

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "页码格式不正确"})
		return
	}

	account := middleware.CurrentAccount(c)
	if account == nil || account.IPBMemberID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	ehSetting := getEHSetting(h.db, account.ID)
	previews, err := h.ehService.FetchGalleryPreviews(account, gid, token, page, ehSetting)
	if err != nil {
		writeEHServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, previews)
}

// GetOnlineComicPages 获取在线画廊每页原图 URL 列表（供阅读器使用）
func (h *OnlineComicHandler) GetOnlineComicPages(c *gin.Context) {
	gid := c.Query("id")
	token := c.Query("token")

	if gid == "" || token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失，必需传递 id (GID) 和 token"})
		return
	}

	account := middleware.CurrentAccount(c)
	if account == nil || account.IPBMemberID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	ehSetting := getEHSetting(h.db, account.ID)
	result, err := h.ehService.FetchOnlinePageUrls(account, gid, token, ehSetting)
	if err != nil {
		writeEHServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetOnlinePageByIndex 就近解析在线画廊指定页（1-based）的原图 URL（供阅读器懒加载补全）
func (h *OnlineComicHandler) GetOnlinePageByIndex(c *gin.Context) {
	gid := c.Query("id")
	token := c.Query("token")
	pageStr := c.DefaultQuery("index", "1")

	if gid == "" || token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失，必需传递 id (GID) 和 token"})
		return
	}

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "页码格式不正确"})
		return
	}

	account := middleware.CurrentAccount(c)
	if account == nil || account.IPBMemberID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	ehSetting := getEHSetting(h.db, account.ID)
	url, total, err := h.ehService.FetchOnlinePageURL(account, gid, token, ehSetting, page)
	if err != nil {
		writeEHServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"index": page, "url": url, "total": total})
}


