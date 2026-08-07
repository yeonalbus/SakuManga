package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"SakuHentai/internal/services"

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

// TailLogs 返回某分类 since（毫秒）之后的新日志行，供前端 1s 轮询做实时监控
// GET /logs/tail?category=update&since=1750000000000
func TailLogs(c *gin.Context) {
	cat := c.Query("category")
	if cat == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 category 参数"})
		return
	}
	since, _ := strconv.ParseInt(c.DefaultQuery("since", "0"), 10, 64)

	var lines []LogTailLine
	if cat == "client" {
		lines = tailFileLines(clientLogPath, since)
	} else {
		path := filepath.Join("logs", services.LogFileName(services.LogCategory(cat), time.Now().Format("2006-01-02")))
		lines = tailFileLines(path, since)
	}
	c.JSON(http.StatusOK, gin.H{"lines": lines})
}

// tailFileLines 读取文件全部行，解析行首时间戳并返回 ts > since 的行
func tailFileLines(path string, since int64) []LogTailLine {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []LogTailLine
	for _, ln := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			continue
		}
		ts := parseLogTs(trimmed)
		if since > 0 && ts <= since {
			continue
		}
		out = append(out, LogTailLine{Ts: ts, Text: trimmed})
	}
	return out
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
