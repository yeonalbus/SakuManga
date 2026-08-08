package services

import (
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrAlreadyLatest = errors.New("已经是最新版本")

// 标签数据自动更新周期：每 24 小时检查一次远端是否有新版本
const TagUpdateIntervalHours = 24
const tagUpdateInterval = TagUpdateIntervalHours * time.Hour

type DownloadProgress struct {
	Status     string  `json:"status"`     // "idle", "downloading", "success", "error"
	Progress   float64 `json:"progress"`   // 0.0 ~ 100.0
	Downloaded int64   `json:"downloaded"` // 已下载 Byte
	Total      int64   `json:"total"`      // 总大小 Byte
	ErrorMsg   string  `json:"errorMsg,omitempty"`
}

type WriteCounter struct {
	Total      int64
	Downloaded int64
	OnProgress func(downloaded, total int64)
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Downloaded += int64(n)
	if wc.OnProgress != nil {
		wc.OnProgress(wc.Downloaded, wc.Total)
	}
	return n, nil
}

type TagItem struct {
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	Intro     string `json:"intro"`
	Count     int    `json:"count"`
}

type TagEngine struct {
	mu          sync.RWMutex
	tags        map[string]*TagItem
	tagList     []*TagItem
	cnVersion   string
	sortVersion string
	EnableCN    bool
	EnableSort  bool
	dataDir     string

	TransProgress DownloadProgress `json:"transProgress"`
	SortProgress  DownloadProgress `json:"sortProgress"`
}

var (
	globalProxyMu sync.RWMutex
	globalProxy   string = ""
)

var GlobalTagEngine = &TagEngine{
	tags:          make(map[string]*TagItem),
	dataDir:       "./data",
	EnableCN:      true,
	EnableSort:    true,
	TransProgress: DownloadProgress{Status: "idle"},
	SortProgress:  DownloadProgress{Status: "idle"},
}

func SetGlobalProxy(proxyStr string) error {
	proxyStr = strings.TrimSpace(proxyStr)
	if proxyStr != "" {
		if _, err := url.Parse(proxyStr); err != nil {
			return err
		}
	}
	globalProxyMu.Lock()
	globalProxy = proxyStr
	globalProxyMu.Unlock()
	return nil
}

func GetGlobalProxy() string {
	globalProxyMu.RLock()
	defer globalProxyMu.RUnlock()
	return globalProxy
}

func getHTTPClient() *http.Client {
	// 优先使用标签引擎专用代理；未设置时回退到全局网络代理（proxy.go config.json），
	// 保证标签数据（GitHub）下载同样走用户配置的代理。
	proxyStr := GetGlobalProxy()
	if proxyStr == "" {
		proxyStr = GetProxyURL()
	}
	client := &http.Client{Timeout: 30 * time.Minute}

	if proxyStr != "" {
		if u, err := url.Parse(proxyStr); err == nil {
			client.Transport = &http.Transport{
				Proxy: http.ProxyURL(u),
			}
		}
	}
	return client
}

func InitTagEngine() {
	os.MkdirAll(GlobalTagEngine.dataDir, 0755)
	GlobalTagEngine.LoadFromDisk()

	go func() {
		// 1. 启动时立即检查一次：自动查找本地文件；若缺失或非最新（远端 ETag 变化）则自行下载
		if GlobalTagEngine.EnableCN {
			GlobalTagEngine.UpdateTranslation()
		}
		if GlobalTagEngine.EnableSort {
			GlobalTagEngine.UpdateCountData()
		}

		// 2. 每 24 小时自动检查更新（更新周期对外可见，见 /tags/status）
		ticker := time.NewTicker(tagUpdateInterval)
		for range ticker.C {
			if GlobalTagEngine.EnableCN {
				GlobalTagEngine.UpdateTranslation()
			}
			if GlobalTagEngine.EnableSort {
				GlobalTagEngine.UpdateCountData()
			}
		}
	}()
}

// 🎯 辅助解析：兼容 db.raw.json 中 name/intro 为字符串或 {"raw":"","text":""} 对象的不同格式
func parseStringOrObject(v interface{}) string {
	if v == nil {
		return ""
	}
	if str, ok := v.(string); ok {
		return str
	}
	if m, ok := v.(map[string]interface{}); ok {
		if text, ok := m["text"].(string); ok && text != "" {
			return text
		}
		if raw, ok := m["raw"].(string); ok && raw != "" {
			return raw
		}
	}
	return fmt.Sprintf("%v", v)
}

// LoadFromDisk 从本地磁盘加载 JSON 与 CSV.GZ 到内存
func (e *TagEngine) LoadFromDisk() {
	e.mu.Lock()
	defer e.mu.Unlock()

	os.MkdirAll(e.dataDir, 0755)

	e.tags = make(map[string]*TagItem)
	e.tagList = make([]*TagItem, 0)

	cnLoadedCount := 0

	// 1. 读取翻译库 db.raw.json
	transPath := filepath.Join(e.dataDir, "db.raw.json")
	if data, err := os.ReadFile(transPath); err == nil {
		var rawData struct {
			Data []struct {
				Namespace string `json:"namespace"`
				Data      map[string]struct {
					Name  interface{} `json:"name"`
					Intro interface{} `json:"intro"`
				} `json:"data"`
			} `json:"data"`
		}

		if err := json.Unmarshal(data, &rawData); err == nil {
			for _, nsData := range rawData.Data {
				ns := strings.ToLower(nsData.Namespace)
				for key, item := range nsData.Data {
					keyClean := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(key)), "_", " ")
					fullKey := ns + ":" + keyClean
					nameStr := parseStringOrObject(item.Name)
					introStr := parseStringOrObject(item.Intro)

					if nameStr == "" {
						nameStr = key
					} else {
						cnLoadedCount++
					}

					tag := &TagItem{
						Namespace: ns,
						Key:       key,
						Name:      nameStr,
						Intro:     introStr,
					}
					e.tags[fullKey] = tag
					e.tagList = append(e.tagList, tag)
				}
			}
			fi, _ := os.Stat(transPath)
			e.cnVersion = fi.ModTime().Format("2006-01-02T15:04:05.000Z")
		} else {
			log.Printf("[TagEngine] db.raw.json 解析失败 (%v)，清除无效缓存文件\n", err)
			os.Remove(transPath)
			os.Remove(transPath + ".etag")
		}
	}

	// 2. 读取热度排序库 tagname_count.csv.gz
	countPath := filepath.Join(e.dataDir, "tagname_count.csv.gz")
	if file, err := os.Open(countPath); err == nil {
		defer file.Close()
		if gzReader, err := gzip.NewReader(file); err == nil {
			defer gzReader.Close()
			csvReader := csv.NewReader(gzReader)
			csvReader.FieldsPerRecord = -1
			csvReader.LazyQuotes = true

			for {
				record, err := csvReader.Read()
				if err == io.EOF {
					break
				}
				if err != nil || len(record) < 2 {
					continue
				}

				rawKey := strings.ToLower(strings.TrimSpace(record[0]))
				count, _ := strconv.Atoi(strings.TrimSpace(record[1]))

				parts := strings.SplitN(rawKey, ":", 2)
				ns, k := "other", rawKey
				if len(parts) == 2 {
					ns, k = parts[0], parts[1]
				}

				keyClean := strings.ReplaceAll(k, "_", " ")
				fullKey := ns + ":" + keyClean

				if tag, exists := e.tags[fullKey]; exists {
					tag.Count = count
				} else {
					tag := &TagItem{Namespace: ns, Key: k, Name: k, Count: count}
					e.tags[fullKey] = tag
					e.tagList = append(e.tagList, tag)
				}
			}
			fi, _ := os.Stat(countPath)
			e.sortVersion = "v" + fi.ModTime().Format("2006.01.02-15")
		}
	}

	log.Printf("[TagEngine] 成功装载标签库！内存总计 %d 条标签，其中包含中文翻译 %d 条\n", len(e.tags), cnLoadedCount)
}

func downloadFileWithProgress(destPath string, urlStr string, onProgress func(downloaded, total int64)) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}

	etagPath := destPath + ".etag"
	tmpPath := destPath + ".tmp"

	client := getHTTPClient()

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return err
	}

	if fi, statErr := os.Stat(destPath); statErr == nil && fi.Size() > 0 {
		if localETag, err := os.ReadFile(etagPath); err == nil && len(localETag) > 0 {
			req.Header.Set("If-None-Match", string(localETag))
		}
	} else {
		os.Remove(etagPath)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return ErrAlreadyLatest
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP 响应错误: %d", resp.StatusCode)
	}

	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	counter := &WriteCounter{
		Total:      resp.ContentLength,
		OnProgress: onProgress,
	}

	mw := io.MultiWriter(out, counter)
	_, copyErr := io.Copy(mw, resp.Body)
	out.Close()

	if copyErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("传输中断: %w", copyErr)
	}

	if newETag := resp.Header.Get("ETag"); newETag != "" {
		os.WriteFile(etagPath, []byte(newETag), 0644)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("替换文件失败: %w", err)
	}

	return nil
}

func (e *TagEngine) UpdateTranslation() {
	e.mu.Lock()
	e.TransProgress = DownloadProgress{Status: "downloading", Progress: 0}
	e.mu.Unlock()

	url := "https://github.com/EhTagTranslation/Database/releases/latest/download/db.raw.json"
	dest := filepath.Join(e.dataDir, "db.raw.json")

	err := downloadFileWithProgress(dest, url, func(downloaded, total int64) {
		e.mu.Lock()
		defer e.mu.Unlock()
		var p float64
		if total > 0 {
			p = float64(downloaded) / float64(total) * 100
		}
		e.TransProgress = DownloadProgress{
			Status:     "downloading",
			Progress:   p,
			Downloaded: downloaded,
			Total:      total,
		}
	})

	e.mu.Lock()
	if errors.Is(err, ErrAlreadyLatest) {
		e.TransProgress = DownloadProgress{Status: "success", Progress: 100}
	} else if err != nil {
		e.TransProgress = DownloadProgress{Status: "error", ErrorMsg: err.Error()}
	} else {
		e.TransProgress = DownloadProgress{Status: "success", Progress: 100}
	}
	e.mu.Unlock()

	if err == nil || errors.Is(err, ErrAlreadyLatest) {
		e.LoadFromDisk()
	}
}

func (e *TagEngine) UpdateCountData() {
	e.mu.Lock()
	e.SortProgress = DownloadProgress{Status: "downloading", Progress: 0}
	e.mu.Unlock()

	url := "https://github.com/mokurin000/e-hentai-tag-count/releases/latest/download/tagname_count.csv.gz"
	dest := filepath.Join(e.dataDir, "tagname_count.csv.gz")

	err := downloadFileWithProgress(dest, url, func(downloaded, total int64) {
		e.mu.Lock()
		defer e.mu.Unlock()
		var p float64
		if total > 0 {
			p = float64(downloaded) / float64(total) * 100
		}
		e.SortProgress = DownloadProgress{
			Status:     "downloading",
			Progress:   p,
			Downloaded: downloaded,
			Total:      total,
		}
	})

	e.mu.Lock()
	if errors.Is(err, ErrAlreadyLatest) {
		e.SortProgress = DownloadProgress{Status: "success", Progress: 100}
	} else if err != nil {
		e.SortProgress = DownloadProgress{Status: "error", ErrorMsg: err.Error()}
	} else {
		e.SortProgress = DownloadProgress{Status: "success", Progress: 100}
	}
	e.mu.Unlock()

	if err == nil || errors.Is(err, ErrAlreadyLatest) {
		e.LoadFromDisk()
	}
}

func (e *TagEngine) TranslateTags(rawTags []string) []*TagItem {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*TagItem, 0, len(rawTags))
	for _, raw := range rawTags {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		parts := strings.SplitN(raw, ":", 2)
		ns := "other"
		key := raw
		if len(parts) == 2 {
			ns = strings.ToLower(strings.TrimSpace(parts[0]))
			key = strings.TrimSpace(parts[1])
		}

		keyClean := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(key)), "_", " ")
		fullKey := ns + ":" + keyClean

		if tag, exists := e.tags[fullKey]; exists && e.EnableCN {
			result = append(result, &TagItem{
				Namespace: tag.Namespace,
				Key:       key,
				Name:      tag.Name,
				Intro:     tag.Intro,
				Count:     tag.Count,
			})
		} else {
			result = append(result, &TagItem{
				Namespace: ns,
				Key:       key,
				Name:      key,
			})
		}
	}
	return result
}

// matchLevelFor 计算 q 对 (key, name) 的匹配层级（key 已统一为小写空格形式）：
//
//	4 = key 完全等于 q
//	3 = key 以 q 开头（前缀）/ 中文名前缀
//	2 = 多词 key 中某个完整单词 == q（如 "penis" → "huge penis"）★ / 中文名子串命中
//	1 = 多词 key 中某个完整单词以 q 开头（跨词前缀，如 "peni" → "huge penis"）
//	0 = 纯字符子串（如 "la" → "glasses" 的 g-la-sses）
//	-1 = 不匹配
func matchLevelFor(key, name, q string, enableCN bool) int {
	if key == q {
		return 4
	}
	if strings.HasPrefix(key, q) {
		return 3
	}
	if q != "" {
		// 完整单词命中：多词 key 中某个完整单词 == q
		for _, w := range strings.Fields(key) {
			if w == q {
				return 2
			}
		}
		// 跨词前缀：某个完整单词以 q 开头
		for _, w := range strings.Fields(key) {
			if strings.HasPrefix(w, q) {
				return 1
			}
		}
	}
	if strings.Contains(key, q) {
		return 0
	}
	// 中文名：前缀视为 3；子串命中视为完整词命中（中文无空格分词，整串即词）→ 2
	if enableCN && name != "" {
		if strings.HasPrefix(name, q) {
			return 3
		}
		if strings.Contains(name, q) {
			return 2
		}
	}
	return -1
}

func (e *TagEngine) Suggest(query string, limit int) []*TagItem {
	e.mu.RLock()
	defer e.mu.RUnlock()

	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []*TagItem{}
	}

	// 显式命名空间：q 含冒号（如 female:sto）时要求 namespace 一致，再按 key 匹配，
	// 避免裸词 q 被「namespace:key 子串匹配」误命中（如 "la" 命中所有 female/language 标签，
	// 导致 female:stockings 之类高热度乱联想霸榜）。
	var ns string
	keyQ := q
	if idx := strings.Index(q, ":"); idx != -1 {
		ns = q[:idx]
		keyQ = q[idx+1:]
	}

	type scored struct {
		tag   *TagItem
		level int
		heat  float64
	}
	matched := make([]scored, 0, 32)

	for _, tag := range e.tagList {
		// 用户显式输入命名空间时，namespace 必须一致（裸词查询不做 namespace 子串匹配）
		if ns != "" && !strings.EqualFold(tag.Namespace, ns) {
			continue
		}
		lowerKey := strings.ToLower(tag.Key)
		lowerName := strings.ToLower(tag.Name)

		var level int
		if keyQ == "" {
			// 显式命名空间后为空（如 "female:"）：列出该命名空间全部标签，按热度排
			level = 3
		} else {
			level = matchLevelFor(lowerKey, lowerName, keyQ, e.EnableCN)
			if level < 0 {
				continue
			}
		}
		matched = append(matched, scored{
			tag:   tag,
			level: level,
			heat:  math.Log10(float64(tag.Count) + 1),
		})
	}

	// 热度协同排序：热度（log10 Count）主导 + 匹配层级次级（权重 6:1）。
	// 例：huge penis(36080, 完整词 level2) ≈ 2+6×4.557=29.34 >
	//     penis enlargement(21051, 前缀 level3) ≈ 3+6×4.323=28.94，完整词命中+高热度胜出；
	// 同时 3a 不回归：la 时 lactation(前缀,700) ≈ 3+6×2.846=20.08 >
	//     glasses(子串,900) ≈ 0+6×2.955=17.73。
	// 关闭热度排序（EnableSort=false）时仅按匹配层级排。
	sort.Slice(matched, func(i, j int) bool {
		if e.EnableSort {
			ti := float64(matched[i].level) + 6*matched[i].heat
			tj := float64(matched[j].level) + 6*matched[j].heat
			if ti != tj {
				return ti > tj
			}
		} else if matched[i].level != matched[j].level {
			return matched[i].level > matched[j].level
		}
		return len(matched[i].tag.Key) < len(matched[j].tag.Key)
	})

	if len(matched) > limit {
		matched = matched[:limit]
	}
	out := make([]*TagItem, 0, len(matched))
	for _, m := range matched {
		out = append(out, m.tag)
	}
	return out
}

func (e *TagEngine) GetVersions() (string, string) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cnVersion, e.sortVersion
}

func (e *TagEngine) GetProgress() (DownloadProgress, DownloadProgress) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.TransProgress, e.SortProgress
}

// GetTagList 导出词典列表（线程安全）
func (e *TagEngine) GetTagList() []*TagItem {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.tagList
}