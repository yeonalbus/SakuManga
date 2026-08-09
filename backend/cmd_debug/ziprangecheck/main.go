// 实测工具：验证「带 start 参数的 H@H streaming 直链」在两种下载策略下的 zip 完整性
//
// 背景：SakuHentai 当前对 H@H 直链参与 Range 探测分块并发下载（默认 10 线程），
// 用户报告"下载完成但 zip 校验失败"的任务 90% 无法通过重新下载修复。
// 本工具对同一 H@H 直链分别实测：
//
//	A) 单线程顺序下载（不带 Range）  —— 候选修复路径（对齐 downloadZip 对 hathStream 的处理）
//	B) 并发 Range 分片下载（默认 10 线程）—— 当前 SakuHentai 行为
//
// 分别保存 zip 并校验，确认根因后再实施代码修复。
//
// 用法：cd backend && go run ./cmd_debug/ziprangecheck -gid 3707345 -token e766e9c75a [-threads 10] [-out ./ziprangecheck_out]
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// 测试账号 cookie（Ehentai测试用账号.md）
var accountCookies = map[string]string{
	"igneous":       "REDACTED_EH_IGNEOUS",
	"ipb_member_id": "3762315",
	"ipb_pass_hash": "REDACTED_EH_PASS_HASH",
	"sk":            "REDACTED_EH_SK",
}

// proxyURL 代理（与 backend/config.json 保持一致）
const proxyURL = "http://127.0.0.1:7897"

// siteReferer / siteBase 由 -site 参数决定（e-hentai / exhentai），复现用户环境 exhentai 差异
var siteReferer = "https://e-hentai.org/"
var siteBase = "https://e-hentai.org/archiver.php"

func buildClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	for _, host := range []string{"https://e-hentai.org", "https://exhentai.org"} {
		u, _ := url.Parse(host)
		cookies := []*http.Cookie{}
		for k, v := range accountCookies {
			cookies = append(cookies, &http.Cookie{Name: k, Value: v, Path: "/"})
		}
		jar.SetCookies(u, cookies)
	}
	pu, err := url.Parse(proxyURL)
	if err != nil {
		log.Fatalf("解析代理失败: %v", err)
	}
	transport := &http.Transport{
		Proxy:               http.ProxyURL(pu),
		MaxConnsPerHost:     16,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     90 * time.Second,
	}
	return &http.Client{Jar: jar, Transport: transport, Timeout: 60 * time.Second}
}

// ---------- archiver.php 流程（对齐 download_archive.go resolveArchiveDownloadURL + resolveHathdlDownloadURL） ----------

type archiverForm struct {
	inputs     map[string]string
	submitText string
	position   int
}

func getBody(client *http.Client, target, referer string) (int, []byte, error) {
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", referer)
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	return resp.StatusCode, body, err
}

func fetchForms(client *http.Client, archiverURL string) ([]archiverForm, string, error) {
	status, body, err := getBody(client, archiverURL, siteReferer)
	if err != nil {
		return nil, "", err
	}
	if status != http.StatusOK {
		return nil, "", fmt.Errorf("archiver.php 状态码 %d", status)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, "", err
	}
	plain := strings.Join(strings.Fields(doc.Find("body").Text()), " ")
	var forms []archiverForm
	doc.Find("form").Each(func(i int, s *goquery.Selection) {
		f := archiverForm{inputs: map[string]string{}, position: i}
		if action, ok := s.Attr("action"); ok {
			_ = action
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
	if len(plain) > 300 {
		plain = plain[:300]
	}
	return forms, plain, nil
}

func pickOrgForm(forms []archiverForm) *archiverForm {
	for i := range forms {
		if forms[i].inputs["dltype"] == "org" {
			return &forms[i]
		}
	}
	if len(forms) > 0 {
		return &forms[0]
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

func postForm(client *http.Client, target, referer, formData string, noRedirect bool) (*http.Response, error) {
	cli := client
	if noRedirect {
		cli = &http.Client{
			Jar: client.Jar,
			Transport: &http.Transport{
				Proxy: http.ProxyURL(mustURL(proxyURL)),
			},
			Timeout: 60 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	req, err := http.NewRequest("POST", target, strings.NewReader(formData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", referer)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return cli.Do(req)
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func createArchive(client *http.Client, f *archiverForm, base, gid, token string) error {
	vals := url.Values{}
	for k, v := range f.inputs {
		vals.Set(k, v)
	}
	vals.Set("gid", gid)
	vals.Set("token", token)
	resp, err := postForm(client, base, base+"?gid="+gid+"&token="+token, vals.Encode(), false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return nil
	}
	return fmt.Errorf("创建归档失败 HTTP %d", resp.StatusCode)
}

func requestDownloadLink(client *http.Client, f *archiverForm, base, gid, token string) (string, error) {
	vals := url.Values{}
	for k, v := range f.inputs {
		vals.Set(k, v)
	}
	resp, err := postForm(client, base, base+"?gid="+gid+"&token="+token, vals.Encode(), true)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := resp.Header.Get("Location")
		if loc == "" {
			return "", fmt.Errorf("302 无 Location")
		}
		if !strings.HasPrefix(loc, "http") {
			u, _ := url.Parse(base)
			ref, _ := url.Parse(loc)
			loc = u.ResolveReference(ref).String()
		}
		return loc, nil
	}
	if resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		if u := extractDownloadLink(string(body)); u != "" {
			return u, nil
		}
		return "", fmt.Errorf("下载归档返回 200 但无链接")
	}
	return "", fmt.Errorf("下载归档失败 HTTP %d", resp.StatusCode)
}

func extractDownloadLink(text string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(text))
	if err != nil {
		return ""
	}
	href := ""
	doc.Find("a").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		h, _ := s.Attr("href")
		low := strings.ToLower(h)
		if strings.Contains(low, "e-hentai.org") || strings.Contains(low, "exhentai.org") || strings.Contains(low, "archiver.php") {
			return true
		}
		href = h
		return false
	})
	return href
}

// resolveHathdl 走 JHentai 同款 H@H 下载页直链流程（dltype/dlcheck → #continue > a → #db > p > a → start=1）
func resolveHathdl(client *http.Client, base, gid, token string) (string, error) {
	query := "?gid=" + gid + "&token=" + token
	formData := url.Values{}
	formData.Set("gid", gid)
	formData.Set("token", token)
	formData.Set("dltype", "org")
	formData.Set("dlcheck", "Download Original Archive")

	var downloadPageURL string
	for attempt := 0; attempt < 6; attempt++ {
		resp, err := postForm(client, base+query, base+query, formData.Encode(), false)
		if err != nil {
			return "", err
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		resp.Body.Close()
		low := strings.ToLower(string(body))
		if strings.Contains(low, "do not have enough funds") {
			return "", fmt.Errorf("GP 不足")
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
		if err != nil {
			return "", err
		}
		if href, ok := doc.Find("#continue > a").Attr("href"); ok && href != "" {
			downloadPageURL = href
			break
		}
		time.Sleep(time.Second)
	}
	if downloadPageURL == "" {
		return "", fmt.Errorf("多次提交后未获 H@H 下载页 URL")
	}

	// GET 下载页解析 #db > p > a
	fmt.Printf("下载页 URL: %s\n", downloadPageURL)
	status, body, err := getBody(client, downloadPageURL, base+query)
	if err != nil {
		return "", err
	}
	fmt.Printf("下载页状态码: %d\n", status)
	if status != http.StatusOK {
		return "", fmt.Errorf("下载页状态码 %d", status)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	// 打印 #db 片段定位直链
	if dbSel := doc.Find("#db"); dbSel.Length() > 0 {
		dbPlain := strings.Join(strings.Fields(dbSel.Text()), " ")
		if len(dbPlain) > 300 {
			dbPlain = dbPlain[:300]
		}
		fmt.Printf("下载页 #db 文本: %q\n", dbPlain)
	}
	downloadPath, ok := doc.Find("#db > p > a").Attr("href")
	if !ok || downloadPath == "" {
		return "", fmt.Errorf("下载页未找到 #db > p > a 直链")
	}
	fmt.Printf("下载页直链 path: %s\n", downloadPath)

	pageURL, _ := url.Parse(downloadPageURL)
	dlURL, err := url.Parse(downloadPath)
	if err != nil {
		return "", err
	}
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

// resolveDownloadURL 获取原图归档 H@H 直链
func resolveDownloadURL(client *http.Client, base, gid, token string) (string, error) {
	query := "?gid=" + gid + "&token=" + token
	forms, plain, err := fetchForms(client, base+query)
	if err != nil {
		return "", err
	}
	fmt.Printf("archiver.php 页面片段: %q\n", plain)
	fmt.Printf("解析到 %d 个表单\n", len(forms))
	for _, f := range forms {
		fmt.Printf("  表单#%d inputs=%v submit=%q\n", f.position, f.inputs, f.submitText)
	}
	if len(forms) == 0 {
		return "", fmt.Errorf("无归档表单")
	}
	target := pickOrgForm(forms)
	if target == nil {
		return "", fmt.Errorf("无原图表单")
	}
	keyForm := target
	if keyForm.inputs["archiver_key"] == "" {
		if err := createArchive(client, target, base, gid, token); err != nil {
			return "", err
		}
		forms2, _, err := fetchForms(client, base+query)
		if err != nil {
			return "", err
		}
		keyForm = findKeyForm(forms2)
		if keyForm == nil {
			fmt.Printf("创建归档后无 archiver_key 表单，尝试 H@H 下载页直链流程\n")
			return resolveHathdl(client, base, gid, token)
		}
	}
	return requestDownloadLink(client, keyForm, base, gid, token)
}

// ---------- 两种下载策略 ----------

// downloadSingle 单线程顺序下载（不带 Range）
func downloadSingle(client *http.Client, downloadURL, savePath string) (int64, error) {
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", siteReferer)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("HTTP %d body=%q", resp.StatusCode, string(body))
	}
	f, err := os.Create(savePath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return io.Copy(f, resp.Body)
}

// probe 探测 Range 支持与总大小（对齐 probeArchiveDownload）
func probe(client *http.Client, downloadURL string) (int64, bool) {
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", siteReferer)
	req.Header.Set("Range", "bytes=0-1023")
	resp, err := client.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		fmt.Printf("  探测非 206: HTTP %d CL=%s CR=%s body=%q\n",
			resp.StatusCode, resp.Header.Get("Content-Length"), resp.Header.Get("Content-Range"), string(body))
		return 0, false
	}
	return parseContentRangeTotal(resp.Header.Get("Content-Range")), true
}

func parseContentRangeTotal(s string) int64 {
	idx := strings.LastIndexByte(s, '/')
	if idx < 0 {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s[idx+1:]), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// downloadChunked 并发 Range 分片下载（对齐 archiveChunkDownloader 核心行为）
// 增强诊断：打印每块响应头与实际读取字节数，校验块完整性（实际字节 != 期望长度视为块损坏）。
func downloadChunked(client *http.Client, downloadURL, savePath string, total int64, threads int) error {
	if threads < 1 {
		threads = 1
	}
	chunkSize := total / int64(threads)
	f, err := os.Create(savePath)
	if err != nil {
		return err
	}
	if err := f.Truncate(total); err != nil {
		f.Close()
		return err
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	errCh := make(chan error, threads)
	shortCh := make(chan string, threads)
	for i := 0; i < threads; i++ {
		start := int64(i) * chunkSize
		end := start + chunkSize
		if i == threads-1 {
			end = total
		}
		wg.Add(1)
		go func(idx int, s, e int64) {
			defer wg.Done()
			req, err := http.NewRequest("GET", downloadURL, nil)
			if err != nil {
				errCh <- err
				return
			}
			req.Header.Set("User-Agent", ua)
			req.Header.Set("Referer", siteReferer)
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", s, e-1))
			resp, err := client.Do(req)
			if err != nil {
				errCh <- err
				return
			}
			defer resp.Body.Close()
			want := e - s
			cl := resp.Header.Get("Content-Length")
			cr := resp.Header.Get("Content-Range")
			te := resp.Header.Get("Transfer-Encoding")
			if resp.StatusCode != http.StatusPartialContent {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
				mu.Lock()
				fmt.Printf("  块%d 请求 range=%d-%d 响应 HTTP %d CL=%s CR=%s TE=%s body=%q\n", idx, s, e-1, resp.StatusCode, cl, cr, te, string(body))
				mu.Unlock()
				errCh <- fmt.Errorf("块%d HTTP %d", idx, resp.StatusCode)
				return
			}
			mu.Lock()
			fmt.Printf("  块%d 请求 range=%d-%d 期望=%d 响应 206 CL=%s CR=%s TE=%s\n", idx, s, e-1, want, cl, cr, te)
			mu.Unlock()
			buf := make([]byte, 256*1024)
			off := s
			var got int64
			var lastErr error
			for {
				n, rerr := resp.Body.Read(buf)
				if n > 0 {
					if _, werr := f.WriteAt(buf[:n], off); werr != nil {
						errCh <- werr
						return
					}
					off += int64(n)
					got += int64(n)
				}
				if rerr == io.EOF {
					break
				}
				if rerr != nil {
					lastErr = rerr
					break
				}
			}
			// 块完整性校验：实际读取字节数必须等于期望长度，否则视为服务器截断/响应异常
			if got != want {
				shortCh <- fmt.Sprintf("块%d 请求 range=%d-%d 期望=%d 实际=%d lastErr=%v CL=%s CR=%s TE=%s",
					idx, s, e-1, want, got, lastErr, cl, cr, te)
				return
			}
		}(i, start, end)
	}
	wg.Wait()
	close(errCh)
	close(shortCh)
	for s := range shortCh {
		f.Close()
		return fmt.Errorf("块不完整: %s", s)
	}
	for e := range errCh {
		if e != nil {
			f.Close()
			return e
		}
	}
	return f.Close()
}

// ---------- zip 校验 ----------

func isValidZip(path string) bool {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer zr.Close()
	return len(zr.File) > 0
}

func main() {
	gid := flag.String("gid", "3707345", "画廊 gid")
	token := flag.String("token", "e766e9c75a", "画廊 token")
	threads := flag.Int("threads", 10, "并发 Range 分片线程数")
	out := flag.String("out", "./ziprangecheck_out", "输出目录")
	site := flag.String("site", "e-hentai", "站点: e-hentai / exhentai（决定 referer 与 archiver.php 基础 URL）")
	base := flag.String("base", "", "archiver.php 基础 URL（默认跟随 -site）")
	flag.Parse()

	// 依据 -site 设置 siteReferer / siteBase（复现用户 exhentai 环境差异）
	switch *site {
	case "exhentai":
		siteReferer = "https://exhentai.org/"
		siteBase = "https://exhentai.org/archiver.php"
	default:
		siteReferer = "https://e-hentai.org/"
		siteBase = "https://e-hentai.org/archiver.php"
	}
	if *base != "" {
		siteBase = *base
	}
	fmt.Printf("站点: %s  referer=%s base=%s\n", *site, siteReferer, siteBase)

	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}

	client := buildClient()

	fmt.Printf("======== 获取 H@H 直链 gid=%s token=%s ========\n", *gid, *token)
	dlURL, err := resolveDownloadURL(client, siteBase, *gid, *token)
	if err != nil {
		log.Fatalf("获取直链失败: %v", err)
	}
	fmt.Printf("H@H 直链: %s\n", dlURL)
	fmt.Printf("isHathStream(hath.network + start): %v\n", isHathStream(dlURL))

	// 探测
	total, rangeOK := probe(client, dlURL)
	fmt.Printf("探测: rangeOK=%v total=%d (%.2f MiB)\n", rangeOK, total, float64(total)/1048576)

	// A: 单线程顺序下载（不带 Range）
	fmt.Printf("\n======== A) 单线程顺序下载（不带 Range） ========\n")
	singlePath := filepath.Join(*out, "single.zip")
	t0 := time.Now()
	n1, err := downloadSingle(client, dlURL, singlePath)
	if err != nil {
		fmt.Printf("单线程下载失败: %v\n", err)
	} else {
		fmt.Printf("单线程下载完成: %d bytes (%.2f MiB) 用时 %v\n", n1, float64(n1)/1048576, time.Since(t0).Round(time.Millisecond))
		fmt.Printf("zip 校验: %v\n", isValidZip(singlePath))
	}

	// B: 并发 Range 分片下载
	fmt.Printf("\n======== B) 并发 Range 分片下载（%d 线程） ========\n", *threads)
	chunkPath := filepath.Join(*out, "chunked.zip")
	t0 = time.Now()
	err = downloadChunked(client, dlURL, chunkPath, total, *threads)
	if err != nil {
		fmt.Printf("分块下载失败: %v\n", err)
	} else {
		if fi, e := os.Stat(chunkPath); e == nil {
			fmt.Printf("分块下载完成: %d bytes (%.2f MiB) 用时 %v\n", fi.Size(), float64(fi.Size())/1048576, time.Since(t0).Round(time.Millisecond))
		}
		fmt.Printf("zip 校验: %v\n", isValidZip(chunkPath))
	}
}

// isHathStream 对齐 SakuHentai isHathStreamDownloadURL：host 含 hath.network 且 query 含 start
func isHathStream(downloadURL string) bool {
	u, err := url.Parse(downloadURL)
	if err != nil {
		return false
	}
	if !strings.Contains(strings.ToLower(u.Host), "hath.network") {
		return false
	}
	return u.Query().Get("start") != ""
}
