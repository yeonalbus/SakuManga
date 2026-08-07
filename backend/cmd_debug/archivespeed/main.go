// 归档下载测速诊断工具
//
// 目的：回答「多线程归档下载慢 / 频繁 EOF」到底是 VPN/网络问题还是本程序并发代码问题。
// 方法：用指定 E 站账号完成 archiver.php 解锁拿到 H@H 下载链接后，对同一归档分别测量：
//   - 单连接 Range 下载吞吐（基线：若单连接本身很慢 → 网络/VPN/节点问题）
//   - 多连接并发 Range 下载吞吐（生产配置 MaxConnsPerHost=6 / 无限两种）
//   - 各连接的 EOF（连接被服务器提前中断）次数
//
// 用法：cd backend && go run ./cmd_debug/archivespeed -gid 4100369 -token 93df5bebde
//
// 说明：会消耗 E 站 GP/Credits 解锁归档；测速只下载归档前若干 MiB，不会下载完整文件。
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"SakuHentai/internal/models"
	"SakuHentai/internal/services"

	"github.com/PuerkitoBio/goquery"
)

const ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

var regexHref = regexp.MustCompile(`(?i)href\s*=\s*["'](https?://[^"']+)["']`)

// archiverForm archiver.php 页面中的一个 <form> 及其隐藏字段
type archiverForm struct {
	action     string
	inputs     map[string]string
	submitText string
	position   int
}

func main() {
	gid := flag.String("gid", "4100369", "画廊 gid")
	token := flag.String("token", "93df5bebde", "画廊 token")
	proxy := flag.String("proxy", "", "代理地址（空=从 config.json 读取）")
	member := flag.String("member", "3762315", "ipb_member_id")
	passHash := flag.String("pass", "REDACTED_EH_PASS_HASH", "ipb_pass_hash")
	igneous := flag.String("igneous", "REDACTED_EH_IGNEOUS", "igneous")
	sk := flag.String("sk", "REDACTED_EH_SK", "sk")
	refresh := flag.Bool("refresh", false, "先取消旧归档 Session 再重新解锁（消耗 GP）")
	flag.Parse()

	if *proxy != "" {
		if err := services.SetProxyURL(*proxy); err != nil {
			log.Fatalf("设置代理失败: %v", err)
		}
	} else {
		services.InitProxyConfig()
	}
	fmt.Printf("当前代理: %q（TUN 模式下可留空走系统隧道）\n", services.GetProxyURL())

	account := &models.AccountSetting{
		IPBMemberID: *member,
		IPBPassHash: *passHash,
		Igneous:     *igneous,
		SK:          *sk,
		IsEx:        true,
		Site:        "exhentai",
	}
	ehSetting := &models.EHSetting{Site: "exhentai"}

	svc := services.NewEHService()
	client, err := svc.BuildClient(account)
	if err != nil {
		log.Fatalf("构建客户端失败: %v", err)
	}
	referer := services.GetBaseURL(account, ehSetting)
	fmt.Printf("referer=%s\n", referer)

	fmt.Printf("\n===== 目标画廊 gid=%s token=%s =====\n", *gid, *token)

	// 可选：取消旧 Session，强制用当前 IP 重新解锁（消耗 GP）
	if *refresh {
		if err := cancelSession(client, referer, *gid, *token); err != nil {
			log.Fatalf("取消旧 Session 失败: %v", err)
		}
		fmt.Println("等待归档页刷新 3s ...")
		time.Sleep(3 * time.Second)
	}

	// 当前出口 IP（判断是否多 IP / IP 变更触发 H@H 封锁）
	checkEgressIP(client)

	// 1. 解锁归档并获取 H@H 下载链接
	dlURL, total, err := resolveDownloadURL(client, referer, *gid, *token)
	if err != nil {
		log.Fatalf("获取下载链接失败: %v", err)
	}
	fmt.Printf("H@H 下载 URL: %s\n", truncate(dlURL, 160))
	fmt.Printf("归档总大小: %.2f MiB (%d bytes)\n\n", float64(total)/1048576, total)

	if total <= 0 {
		log.Fatalf("无法确定归档总大小，退出")
	}

	// 诊断：直连 GET（不带 Range）状态码与响应体前 300 字节
	diagnoseDirect(client, referer, dlURL)

	// 2. 基线：单连接（无 Range 直连 GET，对应生产探测 404 回退路径）
	runDirectGET("直连GET单连接(基线) 8MiB", client, dlURL, 1, 8*1024*1024, -1)

	// 3. 无 Range 直连 3 并发（对比连接数影响）
	runDirectGET("直连GET 3并发(生产配置) 3×4MiB", client, dlURL, 3, 4*1024*1024, -1)
	runDirectGET("直连GET 3并发(无限连接) 3×4MiB", client, dlURL, 3, 4*1024*1024, 0)

	// 4. 带 Range 单连接测速（若 206 可用则对比）
	runSpeedTest("Range单连接(基线) 1×8MiB", client, dlURL, 1, 8*1024*1024, -1)

	// 5. 带 Range 10 并发（生产 transport = 受限6连接）
	runSpeedTest("Range 10并发(生产配置=受限6连接) 10×1.5MiB", client, dlURL, 10, 1536*1024, -1)

	// 6. 带 Range 10 并发（解除 MaxConnsPerHost 限制）
	runSpeedTest("Range 10并发(无限连接) 10×1.5MiB", client, dlURL, 10, 1536*1024, 0)
}

// ─────────────────────────────────────────────────────────────
// 解锁归档 + 获取 H@H 下载链接（复刻 download_archive.go 流程）
// ─────────────────────────────────────────────────────────────

// checkEgressIP 通过 ipify 获取当前出口 IP，用于判断是否多出口 IP 触发 H@H 封锁
func checkEgressIP(client *http.Client) {
	req, err := http.NewRequest("GET", "https://api.ipify.org?format=json", nil)
	if err != nil {
		fmt.Printf("出口 IP 检查失败(构造): %v\n", err)
		return
	}
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("出口 IP 检查失败: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	fmt.Printf("当前出口 IP: %s\n", strings.TrimSpace(string(body)))
}

// cancelSession 取消 E 站服务端归档 Session（POST invalidate_sessions=1）
func cancelSession(client *http.Client, referer, gid, token string) error {
	base := strings.TrimSuffix(referer, "/") + "/archiver.php"
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "https://e-hentai.org/archiver.php" // referer 异常兜底
	}
	target := base + "?gid=" + url.QueryEscape(gid) + "&token=" + url.QueryEscape(token)
	form := url.Values{}
	form.Set("invalidate_sessions", "1")

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequest("POST", target, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Referer", target)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			fmt.Println("已取消旧归档 Session（invalidate_sessions）")
			return nil
		}
		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
	}
	return fmt.Errorf("取消归档 Session 失败: %w", lastErr)
}

// resolveDownloadURL 完成 archiver.php「创建归档 → 拿 H@H 链接」并探测总大小
func resolveDownloadURL(client *http.Client, referer, gid, token string) (string, int64, error) {
	base := "https://exhentai.org/archiver.php"
	query := "?gid=" + url.QueryEscape(gid) + "&token=" + url.QueryEscape(token)

	forms, err := fetchForms(client, referer, base+query)
	if err != nil {
		return "", 0, err
	}
	if len(forms) == 0 {
		return "", 0, fmt.Errorf("archiver.php 未解析到任何表单")
	}

	// 选原图表单
	target := pickForm(forms, "org")
	if target == nil {
		return "", 0, fmt.Errorf("未找到原图表单")
	}
	fmt.Printf("选择表单 #%d submit=%q action=%q\n", target.position, target.submitText, truncate(target.action, 120))

	keyForm := target
	if keyForm.inputs["archiver_key"] == "" {
		if err := createArchive(client, referer, base, gid, token, *target); err != nil {
			return "", 0, err
		}
		forms2, err := fetchForms(client, referer, base+query)
		if err != nil {
			return "", 0, err
		}
		keyForm = findKeyForm(forms2)
		if keyForm == nil {
			// 已解锁 / 仅 H@H Downloader 画廊：走直链流程
			dlURL, err := resolveHathdl(client, referer, base+query, gid, token)
			if err != nil {
				return "", 0, err
			}
			return probeSize(client, referer, dlURL)
		}
	}

	// 提交 archiver_key 拿 302 Location
	dlURL, err := requestDownloadLink(client, referer, base, *keyForm)
	if err != nil {
		return "", 0, err
	}
	return probeSize(client, referer, dlURL)
}

// fetchForms GET archiver.php 并解析全部表单
func fetchForms(client *http.Client, referer, archiverURL string) ([]archiverForm, error) {
	req, err := http.NewRequest("GET", archiverURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", referer)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 archiver.php 失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("archiver.php 返回状态码 %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析 HTML 失败: %v", err)
	}
	plain := strings.Join(strings.Fields(doc.Find("body").Text()), " ")
	if len(plain) > 300 {
		plain = plain[:300]
	}
	fmt.Printf("archiver.php 页面片段: %q\n", plain)

	var forms []archiverForm
	doc.Find("form").Each(func(i int, s *goquery.Selection) {
		f := archiverForm{inputs: map[string]string{}, position: i}
		if action, ok := s.Attr("action"); ok {
			f.action = action
		}
		s.Find("input").Each(func(_ int, inp *goquery.Selection) {
			name, _ := inp.Attr("name")
			val, _ := inp.Attr("value")
			typ, _ := inp.Attr("type")
			if name == "" {
				return
			}
			if typ == "submit" {
				if f.submitText == "" && val != "" {
					f.submitText = val
				}
				return
			}
			f.inputs[name] = val
		})
		if len(f.inputs) > 0 {
			forms = append(forms, f)
		}
	})
	fmt.Printf("解析到 %d 个表单\n", len(forms))
	return forms, nil
}

func pickForm(forms []archiverForm, want string) *archiverForm {
	for i := range forms {
		if forms[i].inputs["dltype"] == want {
			return &forms[i]
		}
	}
	return nil
}

func findKeyForm(forms []archiverForm) *archiverForm {
	for i := range forms {
		if forms[i].inputs["archiver_key"] != "" {
			return &forms[i]
		}
	}
	return nil
}

func createArchive(client *http.Client, referer, base, gid, token string, f archiverForm) error {
	form := url.Values{}
	for k, v := range f.inputs {
		form.Set(k, v)
	}
	form.Set("gid", gid)
	form.Set("token", token)

	req, err := http.NewRequest("POST", base, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", base+"?gid="+gid+"&token="+token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("创建归档 POST 失败: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		fmt.Printf("创建归档响应 %d（已创建归档）\n", resp.StatusCode)
		return nil
	}
	return fmt.Errorf("创建归档失败 HTTP %d", resp.StatusCode)
}

func requestDownloadLink(client *http.Client, referer, base string, f archiverForm) (string, error) {
	form := url.Values{}
	for k, v := range f.inputs {
		form.Set(k, v)
	}
	req, err := http.NewRequest("POST", base, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", referer)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	noRedirect := &http.Client{
		Jar: client.Jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		return "", fmt.Errorf("提交下载归档 POST 失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := resp.Header.Get("Location")
		if loc == "" {
			return "", fmt.Errorf("返回 %d 但无 Location", resp.StatusCode)
		}
		if !strings.HasPrefix(loc, "http") {
			u, _ := url.Parse(base)
			ref, _ := url.Parse(loc)
			loc = u.ResolveReference(ref).String()
		}
		fmt.Printf("下载归档 302 -> %s\n", truncate(loc, 160))
		return loc, nil
	}
	if resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		if u := extractDownloadLink(string(body)); u != "" {
			fmt.Printf("从返回页面解析到下载链接: %s\n", truncate(u, 160))
			return u, nil
		}
		return "", fmt.Errorf("返回 200 但未解析到下载链接")
	}
	return "", fmt.Errorf("下载归档失败 HTTP %d", resp.StatusCode)
}

func extractDownloadLink(text string) string {
	for _, m := range regexHref.FindAllStringSubmatch(text, -1) {
		u := m[1]
		low := strings.ToLower(u)
		if strings.Contains(low, "e-hentai.org") || strings.Contains(low, "exhentai.org") || strings.Contains(low, "archiver.php") {
			continue
		}
		return u
	}
	return ""
}

// resolveHathdl 走 H@H 下载页直链流程（仅 H@H Downloader / 已解锁画廊）
func resolveHathdl(client *http.Client, referer, baseQuery, gid, token string) (string, error) {
	form := url.Values{}
	form.Set("gid", gid)
	form.Set("token", token)
	form.Set("dltype", "org")
	form.Set("dlcheck", "Download Original Archive")

	var downloadPageURL string
	for attempt := 0; attempt < 6; attempt++ {
		req, err := http.NewRequest("POST", baseQuery, strings.NewReader(form.Encode()))
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Referer", baseQuery)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("H@H 解锁 POST 失败: %v", err)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		resp.Body.Close()
		low := strings.ToLower(string(body))
		if strings.Contains(low, "do not have enough funds") {
			return "", fmt.Errorf("GP/Credits 不足，无法创建归档")
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
		if err != nil {
			return "", err
		}
		if href, ok := doc.Find("#continue > a").Attr("href"); ok && href != "" {
			downloadPageURL = href
			fmt.Printf("已获取 H@H 下载页 URL: %s\n", truncate(href, 160))
			break
		}
		if attempt < 5 {
			fmt.Printf("归档仍在生成（第 %d 次未获下载页 URL），1s 后重试\n", attempt+1)
			time.Sleep(time.Second)
		}
	}
	if downloadPageURL == "" {
		return "", fmt.Errorf("多次提交后仍未获取 H@H 下载页 URL")
	}

	req, err := http.NewRequest("GET", downloadPageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", referer)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求下载页失败: %v", err)
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}
	href, ok := doc.Find("#db > p > a").Attr("href")
	if !ok || href == "" {
		return "", fmt.Errorf("下载页未找到 #db > p > a 直链")
	}
	pageURL, _ := url.Parse(downloadPageURL)
	dlURL, _ := url.Parse(href)
	if !dlURL.IsAbs() {
		dlURL.Scheme = "https"
		dlURL.Host = pageURL.Host
	}
	q := dlURL.Query()
	q.Del("autostart")
	if q.Get("start") == "" {
		q.Set("start", "1")
	}
	dlURL.RawQuery = q.Encode()
	return dlURL.String(), nil
}

// probeSize 探测 Range 支持与总大小（206 时返回 total；非 206 返回 ContentLength）
func probeSize(client *http.Client, referer, dlURL string) (string, int64, error) {
	req, err := http.NewRequest("GET", dlURL, nil)
	if err != nil {
		return dlURL, 0, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", referer)
	req.Header.Set("Range", "bytes=0-1023")
	resp, err := client.Do(req)
	if err != nil {
		return dlURL, 0, fmt.Errorf("探测失败: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusPartialContent {
		total := parseContentRangeTotal(resp.Header.Get("Content-Range"))
		fmt.Printf("探测：支持 Range，总大小=%d bytes\n", total)
		return dlURL, total, nil
	}
	fmt.Printf("探测：status=%d（非 206），ContentLength=%d\n", resp.StatusCode, resp.ContentLength)
	return dlURL, resp.ContentLength, nil
}

func parseContentRangeTotal(s string) int64 {
	idx := strings.LastIndexByte(s, '/')
	if idx < 0 {
		return 0
	}
	var n int64
	_, err := fmt.Sscanf(strings.TrimSpace(s[idx+1:]), "%d", &n)
	if err != nil {
		return 0
	}
	return n
}

// ─────────────────────────────────────────────────────────────
// 测速
// ─────────────────────────────────────────────────────────────

// diagnoseDirect 不带 Range 直接 GET，打印状态码与响应体前 300 字节（用于诊断 404 原因）
func diagnoseDirect(client *http.Client, referer, dlURL string) {
	fmt.Printf("\n──────── 直连 GET 诊断（无 Range） ────────\n")
	req, err := http.NewRequest("GET", dlURL, nil)
	if err != nil {
		fmt.Printf("构造请求失败: %v\n", err)
		return
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", referer)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
	fmt.Printf("status=%d ContentLength=%d\n", resp.StatusCode, resp.ContentLength)
	fmt.Printf("响应体前 300 字节: %q\n", string(body))
}

// runDirectGET workers 个并发「无 Range 直连 GET」，各连接只读取 chunkBytes 后断开，统计吞吐与 EOF。
// maxConns: -1=沿用 base transport（生产配置，MaxConnsPerHost=6）；0=无限；>0=指定。
// 用于生产探测 404 回退到单线程 downloadZip 路径时，判断单/多连接直连的真实速度。
func runDirectGET(label string, base *http.Client, dlURL string, workers int, chunkBytes int64, maxConns int) {
	fmt.Printf("\n──────── %s ────────\n", label)
	dl := base
	if maxConns >= 0 {
		if t, ok := base.Transport.(*http.Transport); ok {
			c := t.Clone()
			c.MaxConnsPerHost = maxConns
			dl = &http.Client{Jar: base.Jar, Transport: c}
		}
	}

	start := time.Now()
	var mu sync.Mutex
	var totalBytes int64
	var errCount int
	var eofCount int
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			n, d, isEOF, err := directRead(dl, dlURL, chunkBytes)
			mu.Lock()
			defer mu.Unlock()
			totalBytes += n
			if err != nil {
				errCount++
				if isEOF {
					eofCount++
				}
				fmt.Printf("  worker%d 失败: %v\n", i, err)
				return
			}
			sp := float64(n) / d.Seconds() / 1048576
			fmt.Printf("  worker%d 下载 %.2f MiB 耗时 %.1fs 速度 %.2f MiB/s\n", i, float64(n)/1048576, d.Seconds(), sp)
		}(i)
	}
	wg.Wait()
	el := time.Since(start)
	agg := float64(totalBytes) / el.Seconds() / 1048576
	fmt.Printf("  合计: 下载 %.2f MiB 耗时 %.1fs 聚合速度 %.2f MiB/s\n", float64(totalBytes)/1048576, el.Seconds(), agg)
	fmt.Printf("  结果: 成功=%d 失败=%d EOF=%d\n", workers-errCount, errCount, eofCount)
}

// directRead 无 Range 直接 GET，最多读取 maxBytes 字节，返回字节数/耗时/是否 EOF
func directRead(dl *http.Client, dlURL string, maxBytes int64) (int64, time.Duration, bool, error) {
	req, err := http.NewRequest("GET", dlURL, nil)
	if err != nil {
		return 0, 0, false, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", "https://exhentai.org/")

	startT := time.Now()
	resp, err := dl.Do(req)
	if err != nil {
		return 0, 0, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return 0, 0, false, fmt.Errorf("status=%d body=%q", resp.StatusCode, string(body))
	}
	reader := io.Reader(resp.Body)
	if maxBytes > 0 {
		reader = io.LimitReader(resp.Body, maxBytes)
	}
	buf := make([]byte, 256*1024)
	var n int64
	for {
		c, rerr := reader.Read(buf)
		n += int64(c)
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return n, time.Since(startT), rerr == io.ErrUnexpectedEOF, rerr
		}
	}
	return n, time.Since(startT), false, nil
}

// runSpeedTest workers 个并发 Range 下载，各下载 chunkBytes，统计吞吐与 EOF。
// maxConns: -1=沿用 base transport（生产配置，MaxConnsPerHost=6）；0=解除限制（无限）；>0=指定。
func runSpeedTest(label string, base *http.Client, dlURL string, workers int, chunkBytes int64, maxConns int) {
	fmt.Printf("\n──────── %s ────────\n", label)
	dl := base
	if maxConns >= 0 {
		if t, ok := base.Transport.(*http.Transport); ok {
			c := t.Clone()
			c.MaxConnsPerHost = maxConns
			dl = &http.Client{Jar: base.Jar, Transport: c}
		}
	}

	start := time.Now()
	var mu sync.Mutex
	var totalBytes int64
	var errCount int
	var eofCount int
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := int64(i) * chunkBytes
			e := s + chunkBytes
			n, d, isEOF, err := downloadRange(dl, dlURL, s, e)
			mu.Lock()
			defer mu.Unlock()
			totalBytes += n
			if err != nil {
				errCount++
				if isEOF {
					eofCount++
				}
				fmt.Printf("  worker%d 失败: %v\n", i, err)
				return
			}
			sp := float64(n) / d.Seconds() / 1048576
			fmt.Printf("  worker%d 下载 %.2f MiB 耗时 %.1fs 速度 %.2f MiB/s\n", i, float64(n)/1048576, d.Seconds(), sp)
		}(i)
	}
	wg.Wait()
	el := time.Since(start)
	agg := float64(totalBytes) / el.Seconds() / 1048576
	fmt.Printf("  合计: 下载 %.2f MiB 耗时 %.1fs 聚合速度 %.2f MiB/s\n", float64(totalBytes)/1048576, el.Seconds(), agg)
	fmt.Printf("  结果: 成功=%d 失败=%d EOF=%d\n", workers-errCount, errCount, eofCount)
}

// downloadRange Range 下载 [start,end) 并返回字节数/耗时/是否 EOF
func downloadRange(dl *http.Client, dlURL string, start, end int64) (int64, time.Duration, bool, error) {
	req, err := http.NewRequest("GET", dlURL, nil)
	if err != nil {
		return 0, 0, false, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", "https://exhentai.org/")
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end-1))

	startT := time.Now()
	resp, err := dl.Do(req)
	if err != nil {
		return 0, 0, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return 0, 0, false, fmt.Errorf("status=%d", resp.StatusCode)
	}
	buf := make([]byte, 256*1024)
	var n int64
	for {
		c, rerr := resp.Body.Read(buf)
		n += int64(c)
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return n, time.Since(startT), rerr == io.ErrUnexpectedEOF, rerr
		}
	}
	return n, time.Since(startT), false, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
