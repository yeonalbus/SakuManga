package services

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"SakuHentai/internal/models"

	"github.com/PuerkitoBio/goquery"
)

// ─────────────────────────────────────────────────────────────
// 归档下载引擎（计划第 3 步）
//
// 流程：GET archiver.php?gid=..&token=.. 解析归档表单
//       → 按「原图/压缩图」POST 创建归档（dltype=org/res）
//       → 重新 GET 拿 archiver_key 表单
//       → POST 提交 archiver_key 获取 H@H 下载链接（302 Location）
//       → Range 续传 .part 下载 zip 到 archivePath/archive - gid - 本子名.zip
//       → 解压落地（extract.go）→ 按设置删除压缩包
//
// 落地：archivePath/archive - {gid} - {本子名}.zip（压缩包）
//        extractPath/archive - {gid} - {本子名}/（解压后文件夹）
// ─────────────────────────────────────────────────────────────

// archiverForm archiver.php 页面中的一个 <form> 及其隐藏字段
type archiverForm struct {
	action     string            // form action
	inputs     map[string]string // 所有 input[name] = value
	submitText string            // 提交按钮文字
	position   int               // 表单在页面中的序号（0 起）
}

// archiveDownloader 单次归档下载的执行上下文
type archiveDownloader struct {
	m         *DownloadManager
	task      *models.DownloadTask
	account   *models.AccountSetting
	setting   *models.DownloadSetting
	ehSetting *models.EHSetting
	client    *http.Client

	zipPath         string // archivePath/archive - gid - 本子名.zip
	partPath        string // zipPath + .part
	extractDir      string // extractPath/archive - gid - 本子名/
	referer         string
	limiter         *rateLimiter
	lockFailed      bool
	lastArchiveInfo *models.ArchiveInfo // 最近一次解析到的归档报价（含 SizeBytes 预估）
	lastPlain       string              // 最近一次 archiver.php 页面去标签文本（用于识别「已解锁」变体）
	mu              sync.Mutex
	startedAt       time.Time
}

// runArchiveEngine 归档下载引擎入口（由任务管理器分派）
func (m *DownloadManager) runArchiveEngine(task *models.DownloadTask) {
	g := &archiveDownloader{m: m, task: task}
	g.run()
}

// run 执行完整归档下载流程
func (g *archiveDownloader) run() {
	log.Printf("%s [archive-engine] 任务 %s 开始归档下载 gid=%s title=%q archiveType=%s",
		dlLogTag, g.task.ID, g.task.GID, g.task.Title, g.task.ArchiveType)

	// 1. 账号 / 设置 / 客户端
	var account models.AccountSetting
	if err := g.m.db.First(&account, 1).Error; err != nil || account.IPBMemberID == "" {
		g.fail("未绑定 E 站账号凭证，无法下载")
		return
	}
	g.account = &account
	g.setting = g.m.GetSettings()
	g.ehSetting = loadEHSetting(g.m.db)

	client, err := g.m.ehService.BuildClient(&account)
	if err != nil {
		g.fail("构建下载客户端失败: " + err.Error())
		return
	}
	g.client = client
	g.referer = GetBaseURL(&account, g.ehSetting)
	g.limiter = newRateLimiter(g.setting.SpeedLimitImages, g.setting.SpeedLimitInterval)
	g.startedAt = time.Now()

	// 2. 路径：archive - gid - 本子名
	dirName := fmt.Sprintf("archive - %s - %s", g.task.GID, cleanFolderName(g.task.Title))
	g.zipPath = filepath.Join(g.task.ArchivePath, dirName+".zip")
	g.partPath = g.zipPath + ".part"
	g.extractDir = filepath.Join(g.task.ExtractPath, dirName)

	if err := os.MkdirAll(g.task.ArchivePath, 0o755); err != nil {
		g.fail("创建压缩包目录失败: " + err.Error())
		return
	}
	if err := os.MkdirAll(g.task.ExtractPath, 0o755); err != nil {
		g.fail("创建解压目录失败: " + err.Error())
		return
	}
	log.Printf("%s [archive-engine] 任务 %s 压缩包=%q 解压目录=%q",
		dlLogTag, g.task.ID, g.zipPath, g.extractDir)

	// 3. 断点续传：压缩包已完整存在 → 跳过下载直接解压
	if fi, err := os.Stat(g.zipPath); err == nil && fi.Size() > 0 {
		if isValidZip(g.zipPath) {
			log.Printf("%s [archive-engine] 任务 %s 压缩包已存在且完整，跳过下载直接解压（断点续传）", dlLogTag, g.task.ID)
			g.task.TotalBytes = fi.Size()
			g.task.DoneBytes = fi.Size()
			g.task.DoneFiles = 1
			g.task.TotalFiles = 1
			g.persist()
			g.extractAndFinish()
			return
		}
		log.Printf("%s [archive-engine] 任务 %s 压缩包存在但不完整，删除后重新下载: %q", dlWarnTag, g.task.ID, g.zipPath)
		_ = os.Remove(g.zipPath)
	}

	// 4. archiver.php 全流程：获取 H@H 下载链接
	downloadURL, arcInfo, err := g.resolveArchiveDownloadURL()
	if err != nil {
		// 仅支持 H@H Downloader 的画廊：无法直接 HTTP 下载 zip，自动降级为画廊逐图下载
		if errors.Is(err, errHathdlOnly) {
			if g.task.UpdateForComicID != "" && !g.m.GetSettings().AutoUpdateFallbackToGallery {
				// 更新任务且用户关闭「归档失败降级为画廊」：按失败处理
				g.fail(err.Error())
				return
			}
			if g.stopped() {
				log.Printf("%s [archive-engine] 任务 %s 已被取消/暂停，跳过降级", dlWarnTag, g.task.ID)
				return
			}
			g.m.fallbackArchiveToGallery(g.task, err.Error())
			return
		}
		g.fail(err.Error())
		return
	}
	if arcInfo != nil && arcInfo.SizeBytes > 0 {
		g.task.TotalBytes = arcInfo.SizeBytes
		g.persist()
	}
	log.Printf("%s [archive-engine] 任务 %s 已获取 H@H 下载链接: %s", dlLogTag, g.task.ID, truncateForLog(downloadURL, 200))

	// 5. Range 续传下载 zip
	if err := g.downloadZip(downloadURL); err != nil {
		g.failOrLock(err)
		return
	}

	// 6. 解压落地
	g.extractAndFinish()
}

// extractAndFinish 解压 + 扫描入库 + 置完成
func (g *archiveDownloader) extractAndFinish() {
	n, err := extractArchive(g.task, g.zipPath, g.extractDir, g.setting.DeleteZipAfterArchiveDownload, g.m.db, g.m.ehService, g.account, g.ehSetting)
	if err != nil {
		// 解压失败：保留压缩包供手动处理，任务仍标记错误（可重试）
		g.fail("解压失败: " + err.Error())
		return
	}
	log.Printf("%s [archive-engine] 任务 %s 解压完成，共 %d 个文件 -> %q", dlLogTag, g.task.ID, n, g.extractDir)

	// 扫描入库：让解压后的文件夹出现在离线书架
	if count, err := ScanAndSaveDirectory(g.extractDir, false); err == nil {
		log.Printf("%s [archive-engine] 任务 %s 解压目录已扫描入库 %d 个", dlLogTag, g.task.ID, count)
	} else {
		log.Printf("%s [archive-engine] 任务 %s 解压目录扫描入库失败: %v", dlWarnTag, g.task.ID, err)
	}

	if g.stopped() {
		log.Printf("%s [archive-engine] 任务 %s 已被取消/暂停，跳过完成收尾", dlWarnTag, g.task.ID)
		return
	}
	g.mu.Lock()
	g.task.Status = models.DownloadCompleted
	g.task.Error = ""
	g.task.DoneFiles = 1
	g.task.TotalFiles = 1
	g.task.UpdatedAt = time.Now()
	if err := g.m.db.Save(g.task).Error; err != nil {
		log.Printf("%s [archive-engine] 任务 %s 完成状态保存失败: %v", dlErrTag, g.task.ID, err)
	}
	g.mu.Unlock()

	log.Printf("%s [archive-engine] 任务 %s 归档下载完成：%.2f MiB，落地 %q",
		dlLogTag, g.task.ID, float64(g.task.DoneBytes)/1024/1024, g.extractDir)
}

// resolveArchiveDownloadURL 完成 archiver.php 的「创建归档 → 拿 H@H 链接」流程
func (g *archiveDownloader) resolveArchiveDownloadURL() (string, *models.ArchiveInfo, error) {
	base := "https://e-hentai.org/archiver.php"
	query := "?gid=" + g.task.GID + "&token=" + g.task.Token

	// ① 首次 GET 归档页
	forms, err := g.fetchArchiverForms(base + query)
	if err != nil {
		return "", nil, err
	}
	if len(forms) == 0 {
		return "", nil, fmt.Errorf("archiver.php 未解析到任何归档表单（页面结构可能变化，见 [ARCHIVER] 日志）")
	}

	// ② 挑选目标表单（原图 org / 压缩图 res）
	target := g.pickArchiveForm(forms)
	if target == nil {
		return "", nil, fmt.Errorf("未找到归档类型 %s 的表单（可用表单数=%d）", g.task.ArchiveType, len(forms))
	}
	log.Printf("%s [archive-engine] 任务 %s 选择表单 #%d submit=%q action=%q",
		dlLogTag, g.task.ID, target.position, target.submitText, target.action)

	// ③ 若表单已含 archiver_key → 归档已创建，直接走下载
	keyForm := target
	if keyForm.inputs["archiver_key"] == "" {
		// 先 POST 创建归档
		if err := g.createArchive(*target, base); err != nil {
			return "", nil, err
		}
		// 重新 GET，找到含 archiver_key 的表单
		forms2, err := g.fetchArchiverForms(base + query)
		if err != nil {
			return "", nil, err
		}
		keyForm = g.findKeyForm(forms2)
		if keyForm == nil {
			// 创建归档后仍无 archiver_key 表单：若页面出现 H@H Downloader 表单，或原画已解锁
			// （页面含 "You unlocked ..." 横幅、归档无需重新创建），走「H@H 下载页直链」流程
			// 直接 HTTP 下载 zip（JHentai 同款，其仅用 dltype/dlcheck + #continue > a 一条流程）。
			alreadyUnlocked := g.isAlreadyUnlockedPage()
			if g.isHathdlOnly(forms2) || alreadyUnlocked {
				dlURL, err := g.resolveHathdlDownloadURL()
				if err != nil {
					if alreadyUnlocked {
						// 已解锁变体解析失败：直接报错（非仅 H@H 画廊，不做画廊降级）
						return "", nil, err
					}
					return "", nil, fmt.Errorf("%w: %v", errHathdlOnly, err)
				}
				return dlURL, g.lastArchiveInfo, nil
			}
			return "", nil, fmt.Errorf("创建归档后未找到 archiver_key 表单（请查看 [ARCHIVER] 日志）")
		}
	}

	// ④ 提交 archiver_key 获取 H@H 下载链接
	dlURL, err := g.requestDownloadLink(*keyForm)
	if err != nil {
		return "", nil, err
	}
	return dlURL, g.lastArchiveInfo, nil
}

// fetchArchiverForms GET archiver.php 并解析全部表单
func (g *archiveDownloader) fetchArchiverForms(archiverURL string) ([]archiverForm, error) {
	req, err := http.NewRequest("GET", archiverURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ehReaderUserAgent)
	req.Header.Set("Referer", g.referer)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 archiver.php 失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("archiver.php 返回状态码 %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析 archiver.php HTML 失败: %v", err)
	}

	// 记录页面文本片段便于 debug，并缓存完整去标签文本供「已解锁」变体识别
	plain := strings.Join(strings.Fields(doc.Find("body").Text()), " ")
	g.lastPlain = plain
	if len(plain) > 300 {
		plain = plain[:300]
	}
	log.Printf("%s [archive-engine] archiver.php 页面片段: %q", dlArcTag, plain)

	forms := parseArchiverForms(doc)
	log.Printf("%s [archive-engine] 解析到 %d 个归档表单", dlArcTag, len(forms))
	for _, f := range forms {
		log.Printf("%s [archive-engine]   表单#%d inputs=%v submit=%q", dlArcTag, f.position, f.inputs, f.submitText)
	}

	// 尝试同时解析报价（复用 extractArchiveOptions，若页面含 Download Cost）
	if html, err := doc.Html(); err == nil {
		if opts := extractArchiveOptions(html); len(opts) > 0 {
			g.lastArchiveInfo = &models.ArchiveInfo{GID: g.task.GID, Token: g.task.Token, Options: opts}
			for _, opt := range opts {
				log.Printf("%s [archive-engine] 归档报价 %s: cost=%q size=%q", dlArcTag, opt.Label, opt.Cost, opt.Size)
			}
			// 根据任务归档类型预估 TotalBytes
			if sz := archiveInfoSizeBytes(opts, g.task.ArchiveType); sz > 0 {
				g.lastArchiveInfo.SizeBytes = sz
			}
		}
	}

	return forms, nil
}

// parseArchiverForms 从文档中解析所有 <form> 及其 input 字段
func parseArchiverForms(doc *goquery.Document) []archiverForm {
	var forms []archiverForm
	doc.Find("form").Each(func(i int, s *goquery.Selection) {
		f := archiverForm{
			inputs:   map[string]string{},
			position: i,
		}
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
		// 没有隐藏字段的表单忽略
		if len(f.inputs) > 0 {
			forms = append(forms, f)
		}
	})
	return forms
}

// pickArchiveForm 按 ArchiveType 选择目标表单（优先匹配 dltype，其次按顺序：第一 org 第二 res）
func (g *archiveDownloader) pickArchiveForm(forms []archiverForm) *archiverForm {
	want := "org"
	if g.task.ArchiveType == models.ArchiveTypeResample {
		want = "res"
	}
	for i := range forms {
		if forms[i].inputs["dltype"] == want {
			return &forms[i]
		}
	}
	// 无 dltype 标记：按位置（原图第一、压缩图第二）
	idx := 0
	if want == "res" {
		idx = 1
	}
	if idx < len(forms) {
		return &forms[idx]
	}
	return nil
}

// findKeyForm 在表单列表中找含 archiver_key 的表单
func (g *archiveDownloader) findKeyForm(forms []archiverForm) *archiverForm {
	for i := range forms {
		if forms[i].inputs["archiver_key"] != "" {
			return &forms[i]
		}
	}
	return nil
}

// errHathdlOnly 标记「仅支持 H@H Downloader」的画廊在尝试直链下载失败后的降级信号：
// 已优先尝试 H@H 下载页直链（#continue > a / #db > p > a），全部失败时降级为画廊逐图下载。
var errHathdlOnly = errors.New("该画廊仅支持 H@H Downloader 且直链解析失败，降级为画廊逐图下载")

// isHathdlOnly 判断页面是否属于「仅 H@H Downloader」画廊：
// 存在 hathdl_xres 隐藏表单即判定（hathdl_xres 为 H@H Downloader 表单的特征字段；
// 正常画廊创建归档后应返回 archiver_key 表单，不会走到此判断）
func (g *archiveDownloader) isHathdlOnly(forms []archiverForm) bool {
	for i := range forms {
		if _, ok := forms[i].inputs["hathdl_xres"]; ok {
			log.Printf("%s [archive-engine] 任务 %s 检测到 H@H Downloader 表单（hathdl_xres），判定为仅 H@H 下载画廊",
				dlWarnTag, g.task.ID)
			return true
		}
	}
	return false
}

// isAlreadyUnlockedPage 判断最近一次 archiver.php 页面是否为「原画已解锁」变体：
// 页面含 "You unlocked an original download of this archive on ... [cancel]" 横幅。
// 此时归档已存在，无需重新创建，直接走 H@H 下载页直链流程即可拿到下载页 URL。
func (g *archiveDownloader) isAlreadyUnlockedPage() bool {
	low := strings.ToLower(g.lastPlain)
	if strings.Contains(low, "you unlocked") {
		log.Printf("%s [archive-engine] 任务 %s 检测到原画已解锁横幅（You unlocked），走已解锁直链流程",
			dlWarnTag, g.task.ID)
		return true
	}
	return false
}

// resolveHathdlDownloadURL 通过「H@H 下载页直链」流程解析 zip 直链（与 JHentai 官方流程一致）：
//
//	① POST archiver.php(dltype/dlcheck) 创建/解锁归档
//	② 轮询响应中的 #continue > a（H@H 下载页 URL），归档仍在生成时 1s 后重试
//	③ GET 下载页，解析 #db > p > a 得到直链路径
//	④ 拼 https:// + 下载页 host，并确保 start=1（移除 autostart）
//
// 适用于无 archiver_key 表单的「仅 H@H Downloader」画廊——zip 仍可直接 HTTP 下载，无需本机 H@H 客户端。
func (g *archiveDownloader) resolveHathdlDownloadURL() (string, error) {
	base := "https://e-hentai.org/archiver.php"
	query := "?gid=" + g.task.GID + "&token=" + g.task.Token

	dltype := "org"
	dlcheck := "Download Original Archive"
	if g.task.ArchiveType == models.ArchiveTypeResample {
		dltype = "res"
		dlcheck = "Download Resample Archive"
	}

	// ①+② 提交创建/解锁归档，并轮询 #continue > a 下载页 URL
	var downloadPageURL string
	for attempt := 0; attempt < 6; attempt++ {
		if g.stopped() {
			return "", errTaskStopped
		}
		form := url.Values{}
		form.Set("gid", g.task.GID)
		form.Set("token", g.task.Token)
		form.Set("dltype", dltype)
		form.Set("dlcheck", dlcheck)

		log.Printf("%s [archive-engine] 提交 H@H 解锁 POST dltype=%s 字段=%v", dlArcTag, dltype, form.Encode())
		req, err := http.NewRequest("POST", base+query, strings.NewReader(form.Encode()))
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", ehReaderUserAgent)
		req.Header.Set("Referer", base+query)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := g.client.Do(req)
		if err != nil {
			return "", fmt.Errorf("H@H 解锁 POST 失败: %v", err)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		resp.Body.Close()

		// 资金不足提示
		low := strings.ToLower(string(body))
		if strings.Contains(low, "do not have enough funds") {
			return "", errors.New("GP/Credits 不足，无法创建归档（请先兑换或等待配额）")
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
		if err != nil {
			return "", fmt.Errorf("解析 H@H 解锁响应失败: %v", err)
		}
		if href, ok := doc.Find("#continue > a").Attr("href"); ok && href != "" {
			downloadPageURL = href
			log.Printf("%s [archive-engine] 已获取 H@H 下载页 URL: %s", dlArcTag, truncateForLog(href, 200))
			break
		}
		if attempt < 5 {
			log.Printf("%s [archive-engine] 归档仍在生成（第 %d 次未获下载页 URL），1s 后重试", dlWarnTag, attempt+1)
			time.Sleep(time.Second)
		}
	}
	if downloadPageURL == "" {
		return "", errors.New("多次提交后仍未获取 H@H 下载页 URL（归档生成超时，请稍后重试）")
	}

	// ③ GET 下载页解析 #db > p > a 直链路径
	downloadPath, err := g.fetchHathdlDownloadPath(downloadPageURL)
	if err != nil {
		return "", err
	}

	// ④ 拼完整直链：https:// + 下载页 host + 路径，确保 start=1（移除 autostart）
	pageURL, err := url.Parse(downloadPageURL)
	if err != nil || pageURL.Host == "" {
		return "", fmt.Errorf("解析下载页 URL 失败: %s", truncateForLog(downloadPageURL, 200))
	}
	dlURL, err := url.Parse(downloadPath)
	if err != nil {
		return "", fmt.Errorf("解析直链路径失败: %s", truncateForLog(downloadPath, 200))
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

// fetchHathdlDownloadPath GET H@H 下载页并解析 #db > p > a 的直链路径
func (g *archiveDownloader) fetchHathdlDownloadPath(pageURL string) (string, error) {
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", ehReaderUserAgent)
	req.Header.Set("Referer", g.referer)

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 H@H 下载页失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("H@H 下载页返回状态码 %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("解析 H@H 下载页失败: %v", err)
	}
	href, ok := doc.Find("#db > p > a").Attr("href")
	if ok && href != "" {
		log.Printf("%s [archive-engine] 已解析 H@H 直链: %s", dlArcTag, truncateForLog(href, 200))
		return href, nil
	}
	plain := strings.Join(strings.Fields(doc.Text()), " ")
	return "", fmt.Errorf("H@H 下载页未找到 #db > p > a 直链，页面片段=%q", truncateForLog(plain, 200))
}

// createArchive POST 提交创建归档（gid/token/dltype/dlkey/form_uid/form_token）
func (g *archiveDownloader) createArchive(f archiverForm, base string) error {
	form := url.Values{}
	for k, v := range f.inputs {
		form.Set(k, v)
	}
	// 兜底：确保 gid/token 存在
	form.Set("gid", g.task.GID)
	form.Set("token", g.task.Token)

	log.Printf("%s [archive-engine] 提交创建归档 POST %s 字段=%v", dlArcTag, base, f.inputs)
	req, err := http.NewRequest("POST", base, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", ehReaderUserAgent)
	req.Header.Set("Referer", base+"?gid="+g.task.GID+"&token="+g.task.Token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("创建归档 POST 失败: %v", err)
	}
	defer resp.Body.Close()

	// 创建成功通常 302 → 归档页
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		log.Printf("%s [archive-engine] 创建归档响应 %d（已创建归档）", dlArcTag, resp.StatusCode)
		return nil
	}
	body := truncateForLog(readBody(resp), 200)
	return fmt.Errorf("创建归档失败 HTTP %d: %s", resp.StatusCode, body)
}

// requestDownloadLink POST archiver_key 获取 H@H 下载链接
// 服务器通常 302 到 H@H 下载地址；个别情况下返回页面内嵌链接。
func (g *archiveDownloader) requestDownloadLink(f archiverForm) (string, error) {
	base := "https://e-hentai.org/archiver.php"
	form := url.Values{}
	for k, v := range f.inputs {
		form.Set(k, v)
	}

	log.Printf("%s [archive-engine] 提交下载归档 POST 字段=%v", dlArcTag, f.inputs)
	req, err := http.NewRequest("POST", base, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", ehReaderUserAgent)
	req.Header.Set("Referer", base+"?gid="+g.task.GID+"&token="+g.task.Token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// 不自动跟随重定向，以便捕获 Location（H@H 下载地址）
	noRedirect := &http.Client{
		Jar: g.client.Jar,
		Transport: buildTransport(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		return "", fmt.Errorf("提交下载归档 POST 失败: %v", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther ||
		resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusTemporaryRedirect:
		loc := resp.Header.Get("Location")
		if loc == "" {
			return "", fmt.Errorf("下载归档返回 %d 但无 Location", resp.StatusCode)
		}
		// 相对路径补全
		if !strings.HasPrefix(loc, "http") {
			if u, err := url.Parse(base); err == nil {
				ref, _ := url.Parse(loc)
				loc = u.ResolveReference(ref).String()
			}
		}
		log.Printf("%s [archive-engine] 下载归档 302 -> %s", dlArcTag, truncateForLog(loc, 200))
		return loc, nil

	case resp.StatusCode == http.StatusOK:
		// 返回页面：尝试解析 H@H 下载链接
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		text := string(body)
		if u := extractDownloadLink(text); u != "" {
			log.Printf("%s [archive-engine] 从返回页面解析到下载链接: %s", dlArcTag, truncateForLog(u, 200))
			return u, nil
		}
		return "", fmt.Errorf("下载归档返回 200 但未解析到下载链接，页面片段=%q",
			truncateForLog(strings.Join(strings.Fields(text), " "), 200))

	default:
		body := truncateForLog(readBody(resp), 200)
		return "", fmt.Errorf("下载归档失败 HTTP %d: %s", resp.StatusCode, body)
	}
}

// extractDownloadLink 从 HTML 文本中提取第一个非 e-hentai 域名的链接（H@H 下载地址）
func extractDownloadLink(text string) string {
	re := regexp.MustCompile(`(?i)href\s*=\s*["'](https?://[^"']+)["']`)
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		u := m[1]
		low := strings.ToLower(u)
		if strings.Contains(low, "e-hentai.org") || strings.Contains(low, "exhentai.org") || strings.Contains(low, "archiver.php") {
			continue
		}
		return u
	}
	return ""
}

// downloadZip 从 H@H 服务器下载 zip（支持 Range 断点续传）
func (g *archiveDownloader) downloadZip(downloadURL string) error {
	var startOffset int64
	if fi, err := os.Stat(g.partPath); err == nil {
		startOffset = fi.Size()
	}
	// 已有部分文件且为完整 zip → 直接 rename
	if startOffset > 0 && isValidZip(g.partPath) {
		if err := os.Rename(g.partPath, g.zipPath); err != nil {
			return err
		}
		log.Printf("%s [archive-engine] 任务 %s .part 已是完整 zip，直接落盘", dlLogTag, g.task.ID)
		g.recordBytes(fiSize(g.zipPath))
		return nil
	}

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", ehReaderUserAgent)
	req.Header.Set("Referer", g.referer)
	if startOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
		log.Printf("%s [archive-engine] 任务 %s 从 %d 字节续传 zip", dlLogTag, g.task.ID, startOffset)
	}

	// 下载专用客户端：复用 Cookie/代理，但不设全局 Timeout。
	// BuildClient 的 20s 超时对整个请求（含 body 读取）生效，大文件 zip（数十~上百 MiB）会中途被截断；
	// 中断由 Range 断点续传兜底（失败重试时从 .part 偏移续传），故此处仅保留头部响应超时防止头部阶段挂死。
	dlClient := &http.Client{Jar: g.client.Jar, Transport: g.client.Transport}
	if tr, ok := g.client.Transport.(*http.Transport); ok {
		cl := tr.Clone()
		cl.ResponseHeaderTimeout = 30 * time.Second
		dlClient.Transport = cl
	}

	resp, err := dlClient.Do(req)
	if err != nil {
		return fmt.Errorf("下载 H@H zip 失败: %v", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		if startOffset > 0 {
			_ = os.Remove(g.partPath)
			startOffset = 0
		}
	case http.StatusPartialContent:
		// 正常续传
	case http.StatusRequestedRangeNotSatisfiable:
		// .part 已完整 → rename
		if err := os.Rename(g.partPath, g.zipPath); err != nil {
			return err
		}
		g.recordBytes(fiSize(g.zipPath))
		return nil
	default:
		body := ""
		if data, err := io.ReadAll(io.LimitReader(resp.Body, 512)); err == nil {
			body = strings.ToLower(string(data))
		}
		lockHints := []string{"banned", "quota", "rate limit", "exceeded", "sadpanda", "too many", "temporarily", "panda"}
		for _, h := range lockHints {
			if strings.Contains(body, h) {
				g.mu.Lock()
				g.lockFailed = true
				g.mu.Unlock()
				return fmt.Errorf("HTTP %d（疑似配额/限流，需解锁后重试）", resp.StatusCode)
			}
		}
		return fmt.Errorf("HTTP %d（下载 H@H zip 失败）", resp.StatusCode)
	}

	f, err := os.OpenFile(g.partPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()

	buf := make([]byte, 256*1024)
	written := int64(0)
	readChunk := 0
	for {
		readChunk++
		if readChunk%4 == 0 && g.stopped() {
			return errTaskStopped
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			g.recordBytes(startOffset + written)
			if g.limiter != nil {
				g.limiter.wait(int64(n))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}

	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	closed = true

	if !isValidZip(g.partPath) {
		return fmt.Errorf("下载完成但 zip 校验失败（文件损坏，可重试）")
	}
	if err := os.Rename(g.partPath, g.zipPath); err != nil {
		return err
	}
	g.recordBytes(fiSize(g.zipPath))
	log.Printf("%s [archive-engine] 任务 %s zip 下载完成（%.2f MiB）", dlLogTag, g.task.ID, float64(fiSize(g.zipPath))/1024/1024)
	return nil
}

// recordBytes 更新下载进度并周期性写库
func (g *archiveDownloader) recordBytes(n int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.task.DoneBytes = n
	elapsed := time.Since(g.startedAt).Seconds()
	if elapsed > 0 {
		g.task.Speed = float64(n) / elapsed
	}
	g.task.DoneFiles = 1
	if n% (8*1024*1024) < 256*1024 { // 每约 8MiB 落盘一次
		g.persist()
	}
}

// stopped 任务是否已被取消/暂停（回查 DB，引擎内存状态不会更新）
func (g *archiveDownloader) stopped() bool {
	return g.m.taskStopped(g.task.ID)
}

// persist 将进度写库（调用方需持有 g.mu）
// 只更新进度字段、不改状态列，并限定 status ∈ (queued, downloading)，避免覆盖取消/暂停。
func (g *archiveDownloader) persist() {
	g.task.UpdatedAt = time.Now()
	if err := g.m.db.Model(&models.DownloadTask{}).
		Where("id = ? AND status IN ?", g.task.ID, []string{
			string(models.DownloadQueued), string(models.DownloadDownloading),
		}).
		Updates(map[string]interface{}{
			"done_files":  g.task.DoneFiles,
			"done_bytes":  g.task.DoneBytes,
			"total_bytes": g.task.TotalBytes,
			"speed":       g.task.Speed,
			"updated_at":  g.task.UpdatedAt,
		}).Error; err != nil {
		log.Printf("%s 任务 %s 进度保存失败: %v", dlErrTag, g.task.ID, err)
	}
}

// fail 普通失败（若任务已被取消/暂停则不再写终态）
func (g *archiveDownloader) fail(msg string) {
	if g.stopped() {
		log.Printf("%s [archive-engine] 任务 %s 已被取消/暂停，跳过失败收尾", dlWarnTag, g.task.ID)
		return
	}
	g.m.failTask(g.task, msg)
}

// failOrLock 根据是否检测到配额/限流决定失败状态（任务被取消/暂停时不再写终态）
func (g *archiveDownloader) failOrLock(err error) {
	if g.stopped() {
		log.Printf("%s [archive-engine] 任务 %s 已被取消/暂停，跳过失败收尾: %v", dlWarnTag, g.task.ID, err)
		return
	}
	g.mu.Lock()
	locked := g.lockFailed
	g.mu.Unlock()
	if locked {
		log.Printf("%s [archive-engine] 任务 %s 因配额/限流进入锁定: %v", dlErrTag, g.task.ID, err)
		g.mu.Lock()
		g.task.Status = models.DownloadErrorLock
		g.task.Error = err.Error() + "；疑似 E 站配额/限流，请在 E 站处理配额后点击解锁重试"
		g.task.UpdatedAt = time.Now()
		g.m.db.Save(g.task)
		g.mu.Unlock()
		return
	}
	g.fail(err.Error())
}

// ─────────────────────────────────────────────────────────────
// 工具函数
// ─────────────────────────────────────────────────────────────

// isValidZip 校验文件是否为合法 zip（读取目录头）
func isValidZip(path string) bool {
	if fi, err := os.Stat(path); err != nil || fi.Size() == 0 {
		return false
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer zr.Close()
	return len(zr.File) > 0
}

// archiveInfoSizeBytes 从归档方案列表中挑出对应类型的 Size 估算字节数
func archiveInfoSizeBytes(opts []models.ArchiveDownloadOption, archiveType models.ArchiveType) int64 {
	want := string(archiveType)
	if want == "" {
		want = "original"
	}
	for _, opt := range opts {
		label := strings.ToLower(opt.Label)
		matched := false
		if want == "resample" {
			matched = label == "resample" || label == "compressed" || label == "低画质"
		} else {
			matched = label == "original" || label == "原图" || label == "高画质"
		}
		if matched {
			if sz := parseSizeToBytes(opt.Size); sz > 0 {
				return sz
			}
		}
	}
	// 兜底：任取第一个有 Size 的方案
	for _, opt := range opts {
		if sz := parseSizeToBytes(opt.Size); sz > 0 {
			return sz
		}
	}
	return 0
}

// fiSize 返回文件大小（不存在返回 0）
func fiSize(path string) int64 {
	if fi, err := os.Stat(path); err == nil {
		return fi.Size()
	}
	return 0
}

// readBody 读取响应体（上限 8KB，用于错误信息）
func readBody(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
	return string(data)
}

// parseSizeToBytes 解析 "18.56 MiB" / "1.72 GB" / "12345678" 为字节数
func parseSizeToBytes(s string) int64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0
	}
	num, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	unit := ""
	if len(fields) > 1 {
		unit = strings.ToUpper(fields[1])
	}
	switch {
	case strings.HasPrefix(unit, "K"):
		return int64(num * 1024)
	case strings.HasPrefix(unit, "M"):
		return int64(num * 1024 * 1024)
	case strings.HasPrefix(unit, "G"):
		return int64(num * 1024 * 1024 * 1024)
	default:
		return int64(num)
	}
}
