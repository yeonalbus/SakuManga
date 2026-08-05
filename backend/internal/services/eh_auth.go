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
	"time"

	"SakuHentai/internal/models"
)

// buildTransport 构造带有代理设置的 http.Transport
func buildTransport() *http.Transport {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // 避免部分代理软件截断/解密 Https 时报证书错误
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
		// 🟢 关键补充：注入 SK Cookie，传递用户的个性化/排序状态
		if setting.SK != "" {
			cookies = append(cookies, &http.Cookie{Name: "sk", Value: setting.SK, Path: "/"})
		}

		jar.SetCookies(u, cookies)
	}

	client := &http.Client{
		Jar:       jar,
		Timeout:   20 * time.Second,
		Transport: buildTransport(),
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
		Transport: buildTransport(),
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