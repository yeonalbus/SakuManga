package services

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"SakuHentai/internal/models"
)

// 🟢 新增：根据账号权限与站点偏好动态决定请求根路径 (BaseURL)
// 🟢 改为 package 级别的通用函数（去掉 (s *EHService)）
func GetBaseURL(account *models.AccountSetting, ehSetting *models.EHSetting) string {
	if ehSetting != nil && ehSetting.Site == "exhentai" && account != nil && account.IsEx {
		return "https://exhentai.org/"
	}
	return "https://e-hentai.org/"
}

// 🟢 新增：获取画廊详情页 URL (支持“优先重定向至表站”配置)
// 🟢 同理，如果有 GetGalleryURL，也改为通用函数
func GetGalleryURL(account *models.AccountSetting, ehSetting *models.EHSetting, gid, token string) string {
	if ehSetting != nil && ehSetting.PreferRedirect {
		return fmt.Sprintf("https://e-hentai.org/g/%s/%s/", gid, token)
	}
	return fmt.Sprintf("%sg/%s/%s/", GetBaseURL(account, ehSetting), gid, token)
}

// buildTransport 构造带有代理设置的 http.Transport
func buildTransport() *http.Transport {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // 避免部分代理软件截断/解密 Https 时报证书错误
		// 连接池参数：同主机并发上限 + 空闲连接复用与回收。没有复用时每个请求都新建
		// TCP 连接打向代理隧道，并发一高就容易被隧道批量断开（EOF）。
		MaxConnsPerHost:     6,
		MaxIdleConnsPerHost: 4,
		MaxIdleConns:        32,
		IdleConnTimeout:     90 * time.Second,
	}

	proxyStr := GetProxyURL()
	if proxyStr != "" {
		if proxyURL, err := url.Parse(proxyStr); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		} else {
			log.Printf("[EH-NETWORK-ERROR] 解析代理地址失败: %v", err)
		}
	} else {
		log.Printf("[EH-NETWORK-WARN] 当前代理配置为空，正在使用【直连模式】")
	}

	return transport
}

// ---------------------------------------------------------------------------
// 共享 Transport 单例：把「每请求新建连接池」收敛为「进程级复用」。
//   - getSharedTransport：主站/交互请求共用（列表、详情、阅读、收藏、下载等）；
//   - getCoverTransport：封面代理专用，与主站隔离，避免下载/列表抢占封面连接配额；
//   - resetSharedTransports：代理配置变更后置空，下次请求按最新代理重建。
// ---------------------------------------------------------------------------

var (
	sharedTransportMu sync.RWMutex
	sharedTransport   *http.Transport
)

// getSharedTransport 返回进程级共享的主站 Transport（懒加载）。
func getSharedTransport() *http.Transport {
	sharedTransportMu.RLock()
	t := sharedTransport
	sharedTransportMu.RUnlock()
	if t != nil {
		return t
	}

	sharedTransportMu.Lock()
	defer sharedTransportMu.Unlock()
	if sharedTransport == nil {
		sharedTransport = buildTransport()
	}
	return sharedTransport
}

var (
	coverTransportMu sync.RWMutex
	coverTransport   *http.Transport
)

// getCoverTransport 返回封面代理专用的共享 Transport。
// MaxConnsPerHost 取正常并发上限，实际动态并发由 cover_health.go 的应用层闸门收敛。
func getCoverTransport() *http.Transport {
	coverTransportMu.RLock()
	t := coverTransport
	coverTransportMu.RUnlock()
	if t != nil {
		return t
	}

	coverTransportMu.Lock()
	defer coverTransportMu.Unlock()
	if coverTransport == nil {
		coverTransport = buildTransport()
		coverTransport.MaxConnsPerHost = coverConcurrencyNormal
	}
	return coverTransport
}

// resetSharedTransports 在代理配置变更后清空共享 Transport，下次请求重建。
func resetSharedTransports() {
	sharedTransportMu.Lock()
	sharedTransport = nil
	sharedTransportMu.Unlock()

	coverTransportMu.Lock()
	coverTransport = nil
	coverTransportMu.Unlock()
}

// BuildClient 根据凭证与当前代理配置构造 HTTP Client
func (s *EHService) BuildClient(setting *models.AccountSetting) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	domains := []string{"https://e-hentai.org", "https://exhentai.org"}
	for _, domainStr := range domains {
		u, _ := url.Parse(domainStr)
		cookies := []*http.Cookie{
			{Name: "ipb_member_id", Value: setting.IPBMemberID, Path: "/"},
			{Name: "ipb_pass_hash", Value: setting.IPBPassHash, Path: "/"},
			{Name: "inline_set", Value: "ts_l", Path: "/"},
		}
		if setting.Igneous != "" {
			cookies = append(cookies, &http.Cookie{Name: "igneous", Value: setting.Igneous, Path: "/"})
		}
		// 注入 SK Cookie，传递用户的个性化/排序状态
		if setting.SK != "" {
			cookies = append(cookies, &http.Cookie{Name: "sk", Value: setting.SK, Path: "/"})
		}

		jar.SetCookies(u, cookies)
	}

	client := &http.Client{
		Jar:       jar,
		Timeout:   20 * time.Second,
		Transport: getSharedTransport(),
	}

	return client, nil
}

// BuildCoverClient 构造用于封面代理的 HTTP Client（复用共享的封面 Transport）。
// 与主站客户端隔离：封面请求的并发由 cover_health.go 的动态降级闸门统一收敛，
// 避免隧道抖动时封面/下载相互抢占连接配额。
func (s *EHService) BuildCoverClient(setting *models.AccountSetting) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	domains := []string{"https://e-hentai.org", "https://exhentai.org"}
	for _, domainStr := range domains {
		u, _ := url.Parse(domainStr)
		cookies := []*http.Cookie{
			{Name: "ipb_member_id", Value: setting.IPBMemberID, Path: "/"},
			{Name: "ipb_pass_hash", Value: setting.IPBPassHash, Path: "/"},
			{Name: "inline_set", Value: "ts_l", Path: "/"},
		}
		if setting.Igneous != "" {
			cookies = append(cookies, &http.Cookie{Name: "igneous", Value: setting.Igneous, Path: "/"})
		}
		// 注入 SK Cookie，传递用户的个性化/排序状态
		if setting.SK != "" {
			cookies = append(cookies, &http.Cookie{Name: "sk", Value: setting.SK, Path: "/"})
		}

		jar.SetCookies(u, cookies)
	}

	client := &http.Client{
		Jar:       jar,
		Timeout:   20 * time.Second,
		Transport: getCoverTransport(),
	}

	return client, nil
}

// TryFetchIgneous 尝试自动请求 ExHentai 并提取下发的 igneous
func (s *EHService) TryFetchIgneous(setting *models.AccountSetting) string {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return ""
	}

	u, _ := url.Parse("https://exhentai.org")
	cookies := []*http.Cookie{
		{Name: "ipb_member_id", Value: setting.IPBMemberID, Path: "/", Domain: u.Host},
		{Name: "ipb_pass_hash", Value: setting.IPBPassHash, Path: "/", Domain: u.Host},
	}
	jar.SetCookies(u, cookies)

	client := &http.Client{
		Jar:       jar,
		Timeout:   20 * time.Second,
		Transport: getSharedTransport(),
	}

	req, err := http.NewRequest("GET", "https://exhentai.org/", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	for _, c := range jar.Cookies(u) {
		if c.Name == "igneous" && c.Value != "" && c.Value != "mystery" && c.Value != "deleted" {
			return c.Value
		}
	}

	return ""
}

// VerifyAccount 校验凭证并在必要时自动刷新/抓取 igneous
func (s *EHService) VerifyAccount(setting *models.AccountSetting) (isEx bool, err error) {
	// 1. 未填 igneous 时自动尝试抓取
	if setting.Igneous == "" {
		fetched := s.TryFetchIgneous(setting)
		if fetched != "" {
			setting.Igneous = fetched
		}
	}

	// 2. 尝试验证 ExHentai (里站)
	if setting.Igneous != "" {
		client, err := s.BuildClient(setting)
		if err == nil {
			req, _ := http.NewRequest("GET", "https://exhentai.org/", nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == 200 {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				bodyStr := string(body)
				if !strings.Contains(bodyStr, "sadpanda") && !strings.Contains(bodyStr, "Your IP address has been temporarily banned") {
					return true, nil
				}
			}
		}
	}

	// 3. 抓取/更新 igneous 重新校验里站
	fetched := s.TryFetchIgneous(setting)
	if fetched != "" && fetched != setting.Igneous {
		setting.Igneous = fetched
		client, _ := s.BuildClient(setting)
		req, _ := http.NewRequest("GET", "https://exhentai.org/", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == 200 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if !strings.Contains(string(body), "sadpanda") {
				return true, nil
			}
		}
	}

	// 4. 降级验证 E-Hentai (表站)
	client, err := s.BuildClient(setting)
	if err != nil {
		return false, err
	}

	req, _ := http.NewRequest("GET", "https://e-hentai.org/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("网络连接失败，请检查代理配置: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if strings.Contains(bodyStr, "act=Logout") || strings.Contains(bodyStr, setting.IPBMemberID) {
		return false, nil
	}

	return false, fmt.Errorf("Cookie 凭证无效或已过期")
}