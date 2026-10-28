package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// clashLogTag 高级代理联动日志前缀（与 [DOWNLOAD]/[ARCHIVER] 对齐）
const clashLogTag = "[CLASH]"

var (
	// ErrClashUnauthorized 控制面鉴权失败（密钥不对）
	ErrClashUnauthorized = errors.New("clash 控制面返回 401：密钥不正确")
	// ErrClashUnreachable 控制面不可达（端口错 / 未启动 / 防火墙）
	ErrClashUnreachable = errors.New("无法连接 clash 控制面（请确认地址与端口，注意是外部控制端口而非代理端口）")
	// ErrClashGroupNotFound 代理组不存在
	ErrClashGroupNotFound = errors.New("clash 代理组不存在")
	// ErrClashGlobalForbidden 禁止把 GLOBAL 组作为联动目标（会影响全机流量）
	ErrClashGlobalForbidden = errors.New("不允许以 GLOBAL 组作为联动目标（会影响本机所有程序的流量）")
)

// clashExpectedStatus delay 测试接受的目标状态码区间：E 站无 cookie 时可能是 200/302，
// 被限流时是 503/509，因此只接受 2xx/3xx。
const clashExpectedStatus = "200-399"

// clashProxyInfo /proxies 返回的单个代理或策略组对象（只取联动用得上的字段）
type clashProxyInfo struct {
	Type string   `json:"type"`
	Now  string   `json:"now"`
	All  []string `json:"all"`
}

// ClashGroup 联动用到的策略组信息
type ClashGroup struct {
	Name string   // 组名
	Type string   // Selector / URLTest / Fallback / LoadBalance
	Now  string   // 当前选中项
	All  []string // 可选项（叶子节点或子组名）
}

// IsSelector 是否为手动选择组（只有这类组的手动切换是稳定的）
func (g *ClashGroup) IsSelector() bool { return g.Type == "Selector" }

// ClashClient mihomo 外部控制面（external-controller）的最小客户端。
//
// ⚠️ 必须使用**独立的不带代理的 Transport**：控制面监听 127.0.0.1，
// 若复用 buildTransport()（显式设置了代理），请求会被塞进代理端口绕一圈，
// 既可能被规则拦截，也可能在代理故障时一起失联。
type ClashClient struct {
	base   string // 已规范化为 http://host:port
	secret string
	client *http.Client
}

// NormalizeClashController 规范化控制面地址：允许用户只填 host:port 或带 http(s):// 前缀。
func NormalizeClashController(controller string) string {
	s := strings.TrimSpace(controller)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	return strings.TrimSuffix(s, "/")
}

// NewClashClient 构造控制面客户端。
func NewClashClient(controller, secret string) *ClashClient {
	transport := &http.Transport{
		MaxIdleConns:        4,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     30 * time.Second,
	}
	return &ClashClient{
		base:   NormalizeClashController(controller),
		secret: strings.TrimSpace(secret),
		// 不设 client.Timeout：普通请求用 5s 上下文，delay 测试单独给 60s 上下文
		client: &http.Client{Transport: transport},
	}
}

// BaseURL 返回规范化后的控制面地址（供日志/展示）
func (c *ClashClient) BaseURL() string { return c.base }

// do 统一发起请求：注入鉴权、限读响应体、分类错误。
// 返回 HTTP 状态码；网络层错误包装为 ErrClashUnreachable，401 包装为 ErrClashUnauthorized。
func (c *ClashClient) do(ctx context.Context, method, path string, body []byte, out interface{}) (int, error) {
	if c.base == "" {
		return 0, fmt.Errorf("%w: 控制面地址为空", ErrClashUnreachable)
	}

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrClashUnreachable, err)
	}
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		// 上下文超时单独提示，便于区分"连不上"与"太慢"
		if errors.Is(err, context.DeadlineExceeded) {
			return 0, fmt.Errorf("%w: 请求超时", ErrClashUnreachable)
		}
		return 0, fmt.Errorf("%w: %v", ErrClashUnreachable, err)
	}
	defer resp.Body.Close()

	// 控制面响应都很小，限制 2MB 防御异常响应
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

	if resp.StatusCode == http.StatusUnauthorized {
		return resp.StatusCode, ErrClashUnauthorized
	}
	if out != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && len(payload) > 0 {
		if err := json.Unmarshal(payload, out); err != nil {
			return resp.StatusCode, fmt.Errorf("解析控制面响应失败: %v", err)
		}
	}
	return resp.StatusCode, nil
}

// Version 读取内核版本（同时作为连通性探针）。
func (c *ClashClient) Version(ctx context.Context) (version string, meta bool, err error) {
	var out struct {
		Version string `json:"version"`
		Meta    bool   `json:"meta"`
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	status, err := c.do(ctx, http.MethodGet, "/version", nil, &out)
	if err != nil {
		return "", false, err
	}
	if status != http.StatusOK {
		return "", false, fmt.Errorf("控制面返回状态码 %d", status)
	}
	return out.Version, out.Meta, nil
}

// Group 读取指定策略组的类型 / 当前选中 / 可选项。
func (c *ClashClient) Group(ctx context.Context, name string) (*ClashGroup, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%w: 组名为空", ErrClashGroupNotFound)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var out clashProxyInfo
	status, err := c.do(ctx, http.MethodGet, "/proxies/"+url.PathEscape(name), nil, &out)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrClashGroupNotFound, name)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("读取代理组失败：HTTP %d", status)
	}
	return &ClashGroup{Name: name, Type: out.Type, Now: out.Now, All: out.All}, nil
}

// ListGroups 枚举全部策略组（Selector / URLTest / Fallback / LoadBalance）。
// GLOBAL 也会被列出（前端可展示但保存时会被拒绝，见 SetNode 保护）。
func (c *ClashClient) ListGroups(ctx context.Context) ([]ClashGroup, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var out struct {
		Proxies map[string]clashProxyInfo `json:"proxies"`
	}
	status, err := c.do(ctx, http.MethodGet, "/proxies", nil, &out)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("读取代理列表失败：HTTP %d", status)
	}

	groups := make([]ClashGroup, 0, len(out.Proxies))
	for name, info := range out.Proxies {
		switch info.Type {
		case "Selector", "URLTest", "Fallback", "LoadBalance":
			groups = append(groups, ClashGroup{Name: name, Type: info.Type, Now: info.Now, All: info.All})
		}
	}
	// 稳定排序，便于前端展示
	sortClashGroups(groups)
	return groups, nil
}

// GroupDelay 对组内全部节点做一次延迟测试，返回「节点名 → 延迟(ms)」。
//
// 关键用途：① 选"延迟最低且可用"的节点；② 测不出的节点（假节点/信息节点/死节点）自动被排除。
// target 传 E 站首页（不要用默认的 generate_204 探测地址，那只证明"能上谷歌"）。
func (c *ClashClient) GroupDelay(ctx context.Context, name, target string, timeoutMs int) (map[string]int, error) {
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	if strings.TrimSpace(target) == "" {
		target = "https://e-hentai.org/"
	}

	// 整体超时：单节点 timeoutMs + 余量；mihomo 内部并发测所有节点
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond+30*time.Second)
	defer cancel()

	path := "/group/" + url.PathEscape(name) + "/delay?url=" + url.QueryEscape(target) +
		"&timeout=" + fmt.Sprintf("%d", timeoutMs) + "&expected=" + url.QueryEscape(clashExpectedStatus)

	var out map[string]int
	status, err := c.do(ctx, http.MethodGet, path, nil, &out)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrClashGroupNotFound, name)
	}
	// 504 = 全部节点超时；503 = 有节点失败。两者都可能是"部分可用"，但 mihomo 此时不返回结果，
	// 统一按"没有可用节点"处理，由调用方决定降级。
	if status != http.StatusOK {
		return map[string]int{}, nil
	}
	if out == nil {
		out = map[string]int{}
	}
	return out, nil
}

// SetNode 把策略组的当前选中项切到 node（PUT /proxies/{group} → 204）。
//
// 注意：切换只影响**新建连接**，调用方必须在切换后重置 HTTP 连接池，
// 否则 keep-alive 连接仍走旧节点、出口 IP 没有变化。
func (c *ClashClient) SetNode(ctx context.Context, group, node string) error {
	if strings.EqualFold(group, "GLOBAL") {
		return ErrClashGlobalForbidden
	}
	if strings.TrimSpace(group) == "" || strings.TrimSpace(node) == "" {
		return errors.New("组名与节点名都不能为空")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	body, _ := json.Marshal(map[string]string{"name": node})
	status, err := c.do(ctx, http.MethodPut, "/proxies/"+url.PathEscape(group), body, nil)
	if err != nil {
		return err
	}
	switch {
	case status == http.StatusNoContent || status == http.StatusOK:
		return nil
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrClashGroupNotFound, group)
	case status == http.StatusBadRequest:
		return fmt.Errorf("切换失败：%s 不是可选择策略组（可能是自动选择组）", group)
	default:
		return fmt.Errorf("切换节点失败：HTTP %d", status)
	}
}

// sortClashGroups 稳定排序：组名字典序（GLOBAL 置后，便于前端把它排在末尾提醒用户别选）。
func sortClashGroups(groups []ClashGroup) {
	for i := 1; i < len(groups); i++ {
		for j := i; j > 0; j-- {
			a, b := groups[j-1], groups[j]
			if clashGroupLess(a, b) {
				break
			}
			groups[j-1], groups[j] = groups[j], groups[j-1]
		}
	}
}

// clashGroupLess 判断 a 是否应排在 b 之前。
func clashGroupLess(a, b ClashGroup) bool {
	ag := strings.EqualFold(a.Name, "GLOBAL")
	bg := strings.EqualFold(b.Name, "GLOBAL")
	if ag != bg {
		return bg // GLOBAL 排后
	}
	return a.Name < b.Name
}
