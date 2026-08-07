package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────
// 四类日志存储服务（Round4 任务六）
//
// 设计：低侵入接入——向标准 log 包安装一个自定义 writer（log.SetOutput），
// 每行日志同时写 stdout 与 backend/logs/<cat>_log-YYYY-MM-DD.log（按本地日期切分）。
// 分类依据行内标签自动判定，无需改动既有 ~200 处 log.Printf 调用：
//
//	[update]        → update   更新检测 / 自动更新
//	[maintain]      → maintain 维护查重 / 删除
//	下载类标签       → download 下载 / 归档 / 调度
//	其余（scan/tagm/offline/Toplist/TagEngine 等）→ other
//
// 另提供显式分类入口 LogStore.Printf(category, format, args...)，
// 供新代码或需要强制分类时使用（直接写 stdout + 对应文件，不经 tee 避免重复落盘）。
// ─────────────────────────────────────────────────────────────

// LogCategory 日志分类
type LogCategory string

const (
	LogUpdate   LogCategory = "update"
	LogMaintain LogCategory = "maintain"
	LogDownload LogCategory = "download"
	LogOther    LogCategory = "other"
)

// AllLogCategories 全部四类，供遍历使用
var AllLogCategories = []LogCategory{LogUpdate, LogMaintain, LogDownload, LogOther}

// logBaseDir 日志根目录（相对后端工作目录，如 backend/logs）
var logBaseDir = filepath.Join("logs")

// LogBaseDir 返回日志根目录路径（供 handlers 组装文件路径）
func LogBaseDir() string { return logBaseDir }

// LogFileName 返回某分类某天的日志文件名，如 update_log-2026-08-07.log
func LogFileName(cat LogCategory, date string) string {
	return fmt.Sprintf("%s_log-%s.log", cat, date)
}

// ── 按分类缓存当天打开的文件句柄，跨天自动切换 ──
var (
	logFileMu      sync.Mutex
	logFileHandles = map[string]*os.File{}
	logFileDates   = map[string]string{}
)

// logFileFor 获取（按需打开/跨天切换）某分类当天的日志文件句柄
func logFileFor(cat LogCategory) (*os.File, error) {
	now := time.Now()
	date := now.Format("2006-01-02")
	key := string(cat)
	logFileMu.Lock()
	defer logFileMu.Unlock()
	if f, ok := logFileHandles[key]; ok && logFileDates[key] == date {
		return f, nil
	}
	// 跨天切换：关闭旧句柄，打开新文件
	if f, ok := logFileHandles[key]; ok {
		_ = f.Close()
		delete(logFileHandles, key)
	}
	if err := os.MkdirAll(logBaseDir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(logBaseDir, LogFileName(cat, date)), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	logFileHandles[key] = f
	logFileDates[key] = date
	return f, nil
}

// ── 系统日志开关（Round4 任务七：「启用系统日志」控制四类日志落盘）──
var systemLogsEnabled = true

// GetSystemLogsEnabled 返回是否启用系统日志落盘
func GetSystemLogsEnabled() bool { return systemLogsEnabled }

// SetSystemLogsEnabled 设置系统日志落盘开关
func SetSystemLogsEnabled(enabled bool) { systemLogsEnabled = enabled }

// ── 显式分类写入 API ──

// logStoreAPI 显式分类日志入口的实现类型；包级单例导出为 LogStore
type logStoreAPI struct{}

// LogStore 显式分类日志单例：services.LogStore.Printf(services.LogUpdate, "...", args...)
var LogStore logStoreAPI

// Printf 按指定分类写一条日志：同时写 stdout 与对应日归档文件。
// 直接写文件与 stdout，不经 tee writer，避免同一行被重复落盘。
func (logStoreAPI) Printf(cat LogCategory, format string, args ...interface{}) {
	line := fmt.Sprintf("%s [%s] %s", time.Now().Format("2006/01/02 15:04:05"), cat, fmt.Sprintf(format, args...))
	_, _ = os.Stdout.WriteString(line + "\n")
	if systemLogsEnabled {
		if f, err := logFileFor(cat); err == nil {
			_, _ = f.WriteString(line + "\n")
		}
	}
}

// ── 标准 log 包分流 writer ──

// logStoreWriter 实现 io.Writer：stdout 原样输出 + 按行标签自动分类落盘。
type logStoreWriter struct {
	mu sync.Mutex // 串行化整行写入，避免多协程交错造成半行
}

var logStoreWriterInstance = &logStoreWriter{}

// Write 实现 io.Writer：先写 stdout，再按行拆分后分类写入对应日志文件
func (w *logStoreWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := os.Stdout.Write(p)
	if err != nil {
		return n, err
	}
	if systemLogsEnabled {
		for _, line := range strings.Split(string(p), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			cat := classifyLogLine(line)
			if f, ferr := logFileFor(cat); ferr == nil {
				_, _ = f.WriteString(line + "\n")
			}
		}
	}
	return len(p), nil
}

// classifyLogLine 依据行内标签将日志行映射到四类。
// 优先级：update > maintain > 明确归「其他」的内标签 > 下载类标签 > 默认其他。
func classifyLogLine(line string) LogCategory {
	switch {
	case strings.Contains(line, "[update]"):
		return LogUpdate
	case strings.Contains(line, "[maintain]"):
		return LogMaintain
	case strings.Contains(line, "[scan]") || strings.Contains(line, "[tagm]") ||
		strings.Contains(line, "[offline]") || strings.Contains(line, "[client-log]") ||
		strings.Contains(line, "[Toplist]") || strings.Contains(line, "[TagEngine]"):
		return LogOther
	case strings.Contains(line, "[DOWNLOAD]") || strings.Contains(line, "[DOWNLOAD-WARN]") ||
		strings.Contains(line, "[DOWNLOAD-ERROR]") || strings.Contains(line, "[ARCHIVER]") ||
		strings.Contains(line, "[gallery-engine]") || strings.Contains(line, "[archive-engine]") ||
		strings.Contains(line, "[sched]") || strings.Contains(line, "[worker]") ||
		strings.Contains(line, "[gallery]") || strings.Contains(line, "[archive]") ||
		strings.Contains(line, "[archive-thread]") || strings.Contains(line, "[extract]"):
		return LogDownload
	default:
		return LogOther
	}
}

// InitLogStore 初始化日志存储：清理过期归档 + 安装分流 writer。
// 应在 main 中 chdir 之后、其他初始化之前调用，以尽早捕获启动日志。
func InitLogStore() {
	CleanupLogs(90)
	log.SetOutput(logStoreWriterInstance)
}

// CleanupLogs 清理四类日志中早于保留期（天）的归档文件。
func CleanupLogs(retainDays int) {
	if retainDays <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -retainDays)
	for _, cat := range AllLogCategories {
		_ = cleanupCategoryLogs(cat, cutoff)
	}
}

// cleanupCategoryLogs 删除某分类下 date < cutoff 的归档文件，返回删除数量
func cleanupCategoryLogs(cat LogCategory, cutoff time.Time) int {
	entries, err := os.ReadDir(logBaseDir)
	if err != nil {
		return 0
	}
	prefix := string(cat) + "_log-"
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
		if d.Before(cutoff) {
			if os.Remove(filepath.Join(logBaseDir, e.Name())) == nil {
				n++
			}
		}
	}
	return n
}
