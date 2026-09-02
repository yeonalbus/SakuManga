package services

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"SakuManga/internal/version"
)

// ─────────────────────────────────────────────────────────────
// GitHub 最新 Release 版本检测（设置 → 关于软件 → 版本红点提醒）
//
// 通道选型（实测）：
//   - GitHub REST API（api.github.com）对匿名请求有 60 次/h/IP 限流，
//     本机出口 IP 实测 403（配额被共享出口耗尽），不适合低频检查。
//   - 改用 HTML 通道：GET <repo>/releases/latest 会 302 重定向到最新
//     release 的 tag 页（如 /releases/tag/SakuManga-2.0.1），无 API 限流，
//     一次请求即可从最终 URL 提取最新 tag。
//   - 网络客户端复用标签引擎 getHTTPClient()（手动代理 → 系统代理兜底），
//     与标签词典（GitHub）下载走同一条代理链路。
// ─────────────────────────────────────────────────────────────

// updateCheckRepoURL 被检测的仓库（与 About 页 GitHub 链接一致）
const updateCheckRepoURL = "https://github.com/yeonalbus/SakuManga"

// LatestReleaseURL 最新 release 页面（302 恒指向最新非 draft/pre-release）
const LatestReleaseURL = updateCheckRepoURL + "/releases/latest"

// ReleaseCheckResult 版本检测结果（GET /api/v1/system/check-update 返回）
type ReleaseCheckResult struct {
	Ok        bool   `json:"ok"`                  // 检测是否成功（GitHub 可达且 tag 解析成功）
	Current   string `json:"current,omitempty"`   // 当前版本（前端传入，缺省为后端 AppVersion）
	Latest    string `json:"latest,omitempty"`    // 最新 release 的三段版本号（如 2.0.1）
	Tag       string `json:"tag,omitempty"`       // 最新 release 的 git tag（如 SakuManga-2.0.1）
	URL       string `json:"url,omitempty"`       // 最新 release 页面（供前端打开）
	HasUpdate bool   `json:"hasUpdate"`           // 是否存在比当前更新的版本
	CheckedAt int64  `json:"checkedAt,omitempty"` // 检测完成时间戳(ms)
	Error     string `json:"error,omitempty"`     // 失败原因（ok=false 时）
}

// semverRe 从 tag 中提取三段数字版本号（兼容 SakuManga-2.0.1 / v2.0.1 / SakuHentai-1.4.0 等）
var semverRe = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// CheckLatestRelease 检测 GitHub 最新 release 版本并与当前版本比较。
// currentVersion 为三段版本号（如 2.0.1）；网络失败/解析失败时 Ok=false，
// 由前端静默处理（不打扰用户，不显示红点）。
func CheckLatestRelease(currentVersion string) *ReleaseCheckResult {
	res := &ReleaseCheckResult{Current: currentVersion, CheckedAt: time.Now().UnixMilli()}

	tag, err := fetchLatestReleaseTag()
	if err != nil {
		res.Error = err.Error()
		log.Printf("[UPDATE-CHECK] 检测 GitHub 最新版本失败: %v", err)
		return res
	}
	ver := parseSemver(tag)
	if ver == "" {
		res.Error = "无法从最新 tag 解析版本号: " + tag
		log.Printf("[UPDATE-CHECK] %s", res.Error)
		return res
	}

	res.Ok = true
	res.Tag = tag
	res.Latest = ver
	res.URL = LatestReleaseURL
	if currentVersion != "" {
		res.HasUpdate = compareSemver(ver, currentVersion) > 0
	}
	log.Printf("[UPDATE-CHECK] 当前 %s → 最新 %s（tag=%s）hasUpdate=%v",
		currentVersion, ver, tag, res.HasUpdate)
	return res
}

// fetchLatestReleaseTag 请求 releases/latest 并返回最终重定向后的 tag 名。
// Go http.Client 默认自动跟随重定向（≤10 跳），最终响应的 Request.URL 即最新 release 页。
func fetchLatestReleaseTag() (string, error) {
	client := getHTTPClient() // 复用标签引擎网络客户端（手动/系统代理一致）
	client.Timeout = 8 * time.Second

	req, err := http.NewRequest(http.MethodGet, LatestReleaseURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "SakuManga/"+appSemver())
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub 返回异常状态码 %d", resp.StatusCode)
	}
	// 丢弃响应体（仅需 URL 中的 tag）
	_, _ = io.Copy(io.Discard, resp.Body)

	finalURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return tagFromReleaseURL(finalURL), nil
}

// tagFromReleaseURL 从 release 页 URL 提取 tag：取 /releases/tag/<tag> 段。
func tagFromReleaseURL(u string) string {
	if idx := strings.Index(u, "/releases/tag/"); idx >= 0 {
		seg := u[idx+len("/releases/tag/"):]
		if i := strings.IndexAny(seg, "/?#"); i >= 0 {
			seg = seg[:i]
		}
		if seg != "" {
			return seg
		}
	}
	// 兜底：URL 最后一段（末尾带 / 时先剥除）
	trimmed := strings.TrimRight(u, "/")
	if i := strings.LastIndex(trimmed, "/"); i >= 0 && i+1 < len(trimmed) {
		return trimmed[i+1:]
	}
	return ""
}

// parseSemver 从字符串提取第一处三段数字版本号；无匹配返回空串。
func parseSemver(s string) string {
	m := semverRe.FindStringSubmatch(s)
	if len(m) < 4 {
		return ""
	}
	return m[1] + "." + m[2] + "." + m[3]
}

// compareSemver 比较两个三段版本号：a>b 返回 1，a==b 返回 0，a<b 返回 -1。
// 输入应为合法三段版本；非法时按 0 段兜底（调用侧均已 parseSemver 校验）。
func compareSemver(a, b string) int {
	as := versionParts(a)
	bs := versionParts(b)
	for i := 0; i < 3; i++ {
		if as[i] > bs[i] {
			return 1
		}
		if as[i] < bs[i] {
			return -1
		}
	}
	return 0
}

// versionParts 解析三段版本号为数字数组（非法段按 0）
func versionParts(v string) [3]int64 {
	parts := strings.Split(v, ".")
	var out [3]int64
	for i := 0; i < 3 && i < len(parts); i++ {
		if n, err := strconv.ParseInt(parts[i], 10, 64); err == nil {
			out[i] = n
		}
	}
	return out
}

// appSemver 返回后端 AppVersion 的三段版本（供 UA 展示；非法时回退 0.0.0）
func appSemver() string {
	if v := parseSemver(version.AppVersion); v != "" {
		return v
	}
	return "0.0.0"
}
