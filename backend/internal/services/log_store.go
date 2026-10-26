package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
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

// classifyLogLine 依据**行首标签**将日志行映射到四类（Round45：不再按全文子串匹配）。
//
// 行首标签解析规则：
//
//	`2026/10/26 02:50:01 [DOWNLOAD-WARN] [update] 任务 ...`
//	 → 首个标签是「级别」（[DOWNLOAD] / [DOWNLOAD-WARN] / [ARCHIVER] ...），
//	   之后的连续标签是「内标签」；只有内标签才参与 update/maintain/other 判定。
//
// 正文里出现的方括号（如 "页面含 [update] 标记"）不会被当作标签，避免误分类。
// 优先级：显式分类标签 > 内标签白名单 > 下载类标签 > 默认其他。
func classifyLogLine(line string) LogCategory {
	tags := logPrefixTags(line)
	if len(tags) == 0 {
		return LogOther
	}
	// ① 显式分类行（LogStore.Printf）：首标签即分类名（update / maintain / download / other）
	if c, ok := categoryTagOf(tags[0]); ok {
		return c
	}
	// ② 标准 log 行：首标签是级别，内标签从第二个开始判定
	inner := tags[1:]
	if len(tags) == 1 {
		inner = tags
	}
	for _, t := range inner {
		if c, ok := innerTagCategory(t); ok {
			return c
		}
	}
	// ③ 级别或引擎标签属于下载类
	for _, t := range tags {
		if isDownloadLogTag(t) {
			return LogDownload
		}
	}
	return LogOther
}

// logTimestampPrefixRe 匹配行首标准 log 时间戳（time 包默认格式，可能带 ~H 后缀）。
var logTimestampPrefixRe = regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(?:~H)?\s+`)

// logTagRunRe 匹配紧贴行首（去时间戳后）的一段连续标签串。
var logTagRunRe = regexp.MustCompile(`^(?:\[[^\]]+\]\s*)+`)

// logSingleTagRe 从标签串中切出单个 [xxx]。
var logSingleTagRe = regexp.MustCompile(`\[[^\]]+\]`)

// logPrefixTags 提取行首时间戳之后的连续标签（去掉方括号）。
// 无时间戳时按行首连续标签解析；正文中的方括号不参与（只取行首连续段）。
func logPrefixTags(line string) []string {
	rest := logTimestampPrefixRe.ReplaceAllString(line, "")
	run := logTagRunRe.FindString(rest)
	if run == "" {
		return nil
	}
	found := logSingleTagRe.FindAllString(run, -1)
	tags := make([]string, 0, len(found))
	for _, t := range found {
		tags = append(tags, strings.Trim(t, "[]"))
	}
	return tags
}

// categoryTagOf 判断标签本身是否为显式分类名（LogStore.Printf 的行首标签）。
func categoryTagOf(tag string) (LogCategory, bool) {
	switch tag {
	case string(LogUpdate):
		return LogUpdate, true
	case string(LogMaintain):
		return LogMaintain, true
	case string(LogDownload):
		return LogDownload, true
	case string(LogOther):
		return LogOther, true
	}
	return "", false
}

// innerTagCategory 内标签白名单（精确匹配，不做子串判定）。
func innerTagCategory(tag string) (LogCategory, bool) {
	switch tag {
	case "update":
		return LogUpdate, true
	case "maintain":
		return LogMaintain, true
	case "scan", "tagm", "offline", "client-log", "Toplist", "TagEngine":
		return LogOther, true
	}
	return "", false
}

// isDownloadLogTag 下载/归档/调度相关标签（级别标签或引擎标签）。
func isDownloadLogTag(tag string) bool {
	switch tag {
	case "DOWNLOAD", "DOWNLOAD-WARN", "DOWNLOAD-ERROR", "ARCHIVER",
		"gallery-engine", "archive-engine", "sched", "worker",
		"gallery", "archive", "archive-thread", "extract":
		return true
	}
	return false
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
