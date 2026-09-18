package handlers

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"SakuManga/internal/database"
	"SakuManga/internal/models"
	"SakuManga/internal/services"

	"github.com/gin-gonic/gin"
)

// ─────────────────────────────────────────────────────────────
// 四类日志查询 / 监控 / 管理接口（Round4 任务六）
//
// 数据来源：backend/logs/<cat>_log-YYYY-MM-DD.log（由 services.LogStore 写入）
//   - GET    /logs/categories          各类别可用日期与文件大小
//   - GET    /logs/query              按分类/日期/关键词分页查询
//   - GET    /logs/tail               实时监控：返回 since(ms) 之后的新行（前端 1s 轮询）
//   - DELETE /logs                    按类别 + 日期范围清理（Round4 任务七）
//   - GET/POST /logs/settings         系统日志开关（Round4 任务七）
// ─────────────────────────────────────────────────────────────

// logCategoryLabels 分类中文名（含前端错误日志 client）
var logCategoryLabels = map[string]string{
	"update":   "更新",
	"maintain": "维护",
	"download": "下载",
	"other":    "其他",
	"client":   "前端错误",
}

// LogFileInfo 单日归档信息
type LogFileInfo struct {
	Date string `json:"date"`
	Size int64  `json:"size"`
}

// LogCategoryInfo 单个分类概览
type LogCategoryInfo struct {
	Category string        `json:"category"`
	Label    string        `json:"label"`
	Files    []LogFileInfo `json:"files"`
}

// GetLogCategories 返回四类日志的可用日期与文件大小 + 前端错误日志大小
// GET /logs/categories
func GetLogCategories(c *gin.Context) {
	categories := make([]LogCategoryInfo, 0, 4)
	for _, cat := range []string{"update", "maintain", "download", "other"} {
		categories = append(categories, LogCategoryInfo{
			Category: cat,
			Label:    logCategoryLabels[cat],
			Files:    listLogFiles(cat),
		})
	}
	clientSize := int64(0)
	if info, err := os.Stat(clientLogPath); err == nil {
		clientSize = info.Size()
	}
	c.JSON(http.StatusOK, gin.H{
		"categories": categories,
		"client": gin.H{
			"category": "client",
			"label":    logCategoryLabels["client"],
			"size":     clientSize,
		},
	})
}

// listLogFiles 列出某分类全部 <cat>_log-YYYY-MM-DD.log 的日期与大小（日期倒序）
func listLogFiles(cat string) []LogFileInfo {
	entries, err := os.ReadDir(filepath.Join("logs"))
	if err != nil {
		return nil
	}
	prefix := cat + "_log-"
	var files []LogFileInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		date := strings.TrimSuffix(strings.TrimPrefix(e.Name(), prefix), ".log")
		if _, err := time.Parse("2006-01-02", date); err != nil {
			continue
		}
		if info, err := e.Info(); err == nil {
			files = append(files, LogFileInfo{Date: date, Size: info.Size()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Date > files[j].Date })
	return files
}

// QueryLogs 按分类/日期/关键词分页查询日志
// GET /logs/query?category=update&date=2026-08-07&keyword=失败&offset=0&limit=50
func QueryLogs(c *gin.Context) {
	cat := c.Query("category")
	if cat == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 category 参数"})
		return
	}
	date := c.Query("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	keyword := strings.ToLower(c.Query("keyword"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	path := filepath.Join("logs", services.LogFileName(services.LogCategory(cat), date))
	if cat == "client" {
		path = clientLogPath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		// 文件不存在视为空结果
		c.JSON(http.StatusOK, gin.H{"total": 0, "offset": offset, "limit": limit, "lines": []string{}})
		return
	}
	var filtered []string
	for _, ln := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		if keyword == "" || strings.Contains(strings.ToLower(ln), keyword) {
			filtered = append(filtered, ln)
		}
	}
	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	c.JSON(http.StatusOK, gin.H{
		"total":  total,
		"offset": offset,
		"limit":  limit,
		"lines":  filtered[offset:end],
	})
}

// LogTailLine 实时监控单行
type LogTailLine struct {
	Ts   int64  `json:"ts"`
	Text string `json:"text"`
}

// 实时监控反向读取参数（Round41）
//
// 原实现 os.ReadFile 全量读 + 逐行 time.ParseInLocation：成本 O(文件行数)，
// 前端 1s 轮询一次且随当天日志增长线性恶化（16905 行 ≈ 6ms，响应体可达 2MB）。
const (
	tailReadChunk    = 64 * 1024 // 反向读取块大小
	tailMaxScanBytes = 8 << 20   // 单次请求反向扫描上限（异常巨大文件保护）
	tailMaxLines     = 5000      // 单次返回行数硬上限
)

// TailLogs 返回某分类 since（毫秒）之后的新日志行，供前端 1s 轮询做实时监控
// GET /logs/tail?category=update&since=1750000000000&limit=300
//
//	limit > 0  → 最多返回最后 limit 行（前端首屏与增量轮询都用它做安全阀）
//	limit <= 0 → 兼容旧调用方：返回全部命中行（受 tailMaxLines 硬上限约束）
func TailLogs(c *gin.Context) {
	cat := c.Query("category")
	if cat == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 category 参数"})
		return
	}
	since, _ := strconv.ParseInt(c.DefaultQuery("since", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "0"))

	var path string
	if cat == "client" {
		path = clientLogPath
	} else {
		path = filepath.Join("logs", services.LogFileName(services.LogCategory(cat), time.Now().Format("2006-01-02")))
	}
	c.JSON(http.StatusOK, gin.H{"lines": tailFileLines(path, since, limit)})
}

// tailFileLines 返回 ts > since 的日志行（since <= 0 时返回文件尾部行），按时间正序。
//
// Round41：从文件尾部反向分块读取，只解析将要返回的行，满足任一条件即停止：
//   - 已收集满 maxLines 行；
//   - 遇到 ts <= since 的行（日志按时间追加，更早的行必然也不满足）；
//   - 反向扫描达到 tailMaxScanBytes。
//
// 取舍：停止依据是"时间戳单调递增"。若日志中途发生系统时间回退，回退点之前的行将不再返回
// （监控场景只关心最新动态，可接受）；无时间戳的行（多行堆栈等 ts=0）不作为停止依据。
// 文件末尾若没有换行符（正在写入的半行）会被丢弃，等待下一轮以完整行出现。
func tailFileLines(path string, since int64, limit int) []LogTailLine {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.Size() <= 0 {
		return nil
	}
	size := info.Size()

	maxLines := tailMaxLines
	if limit > 0 && limit < maxLines {
		maxLines = limit
	}

	// 文件末尾是否存在"尚未写完"的半行（日志写入恒以 \n 结尾，无换行即为写入中）
	dropTailPartial := false
	lastByte := make([]byte, 1)
	if _, rerr := f.ReadAt(lastByte, size-1); rerr == nil {
		dropTailPartial = lastByte[0] != '\n'
	}

	var (
		pos       = size
		pending   []byte        // 已切出但开头可能被截断的首行，留待与更早内容拼接
		collected []LogTailLine // 逆序收集（最新在前）
		scanned   int64
		firstRead = true
		stop      bool
	)

	for pos > 0 && !stop && len(collected) < maxLines && scanned < tailMaxScanBytes {
		readSize := int64(tailReadChunk)
		if pos < readSize {
			readSize = pos
		}
		pos -= readSize
		scanned += readSize

		buf := make([]byte, readSize)
		if _, rerr := f.ReadAt(buf, pos); rerr != nil && rerr != io.EOF {
			break
		}

		// 更早的 buf 在前、上一轮遗留的 pending 在后 → 拼接出跨块的完整行
		chunk := buf
		if len(pending) > 0 {
			chunk = append(buf, pending...)
		}

		lines := bytes.Split(chunk, []byte("\n"))
		if pos > 0 {
			// 未到文件开头：首行开头被截断，留给下一轮拼接（不含分隔符）
			pending = append([]byte(nil), lines[0]...)
			lines = lines[1:]
		} else {
			pending = nil
		}
		if firstRead && dropTailPartial && len(lines) > 0 {
			lines = lines[:len(lines)-1] // 丢弃正在写入的半行
		}
		firstRead = false

		for i := len(lines) - 1; i >= 0; i-- {
			text := strings.TrimSpace(string(lines[i]))
			if text == "" {
				continue
			}
			ts := parseLogTs(text)
			if since > 0 {
				if ts == 0 {
					continue // 无时间戳的行：过滤，但不作为"更早的行都不满足"的依据
				}
				if ts <= since {
					stop = true
					break
				}
			}
			collected = append(collected, LogTailLine{Ts: ts, Text: text})
			if len(collected) >= maxLines {
				stop = true
				break
			}
		}
	}

	// 逆序 → 正序
	for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
		collected[i], collected[j] = collected[j], collected[i]
	}
	return collected
}

// parseLogTs 解析 Go 默认日志前缀 "2006/01/02 15:04:05 ..."，失败返回 0
func parseLogTs(line string) int64 {
	if len(line) < 19 {
		return 0
	}
	if t, err := time.ParseInLocation("2006/01/02 15:04:05", line[:19], time.Local); err == nil {
		return t.UnixMilli()
	}
	return 0
}

// DeleteLogs 按类别 + 日期范围清理日志（Round4 任务七「清除日志」精细管理）
//   DELETE /logs?category=update&before=2026-08-01  → 删除该分类早于该日期的归档
//   DELETE /logs?category=update                    → 删除该分类全部归档
//   DELETE /logs                                    → 删除全部四类归档
//   DELETE /logs?category=client                    → 清空前端错误日志
func DeleteLogs(c *gin.Context) {
	cat := c.Query("category")
	before := c.Query("before")

	var beforeDate time.Time
	hasBefore := before != ""
	if hasBefore {
		var err error
		beforeDate, err = time.Parse("2006-01-02", before)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "before 参数格式错误，应为 YYYY-MM-DD"})
			return
		}
	}

	deleted := 0
	switch {
	case cat == "client":
		if err := os.Remove(clientLogPath); err == nil || os.IsNotExist(err) {
			deleted = 1
		}
	case cat != "":
		deleted = deleteCategoryLogs(cat, beforeDate, hasBefore)
	default:
		for _, cc := range []string{"update", "maintain", "download", "other"} {
			deleted += deleteCategoryLogs(cc, beforeDate, hasBefore)
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": deleted})
}

// deleteCategoryLogs 删除某分类下符合日期条件的归档文件，返回删除数量
func deleteCategoryLogs(cat string, before time.Time, hasBefore bool) int {
	entries, err := os.ReadDir(filepath.Join("logs"))
	if err != nil {
		return 0
	}
	prefix := cat + "_log-"
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		date := strings.TrimSuffix(strings.TrimPrefix(e.Name(), prefix), ".log")
		d, err := time.Parse("2006-01-02", date)
		if err != nil {
			continue
		}
		if hasBefore && !d.Before(before) {
			continue
		}
		if os.Remove(filepath.Join("logs", e.Name())) == nil {
			n++
		}
	}
	return n
}

// GetLogSettings 返回系统日志设置
// GET /logs/settings
func GetLogSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"systemLogsEnabled": services.GetSystemLogsEnabled()})
}

// SaveLogSettings 保存系统日志设置（持久化到 ServerSetting 单例，Round4 任务七）
// POST /logs/settings  body: {"systemLogsEnabled": true}
func SaveLogSettings(c *gin.Context) {
	var req struct {
		SystemLogsEnabled bool `json:"systemLogsEnabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}
	services.SetSystemLogsEnabled(req.SystemLogsEnabled)

	var setting models.ServerSetting
	if err := database.DB.First(&setting, 1).Error; err != nil {
		setting = models.ServerSetting{ID: 1}
	}
	setting.SystemLogsEnabled = req.SystemLogsEnabled
	if err := database.DB.Save(&setting).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存日志设置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "systemLogsEnabled": req.SystemLogsEnabled})
}
