package services

import (
	"testing"
)

// TestClassifyLogLine 验证行内标签 → 四类日志的映射与优先级。
// 注意：实际日志行形如 "2026/08/07 10:00:00 [DOWNLOAD] [update] ..."，
// 即 [update]/[maintain]/[scan]/[tagm] 等内标签位于 [DOWNLOAD] 前缀之后，
// 分类必须先匹配内标签，再匹配下载类标签。
func TestClassifyLogLine(t *testing.T) {
	cases := []struct {
		line string
		want LogCategory
	}{
		{"2026/08/07 10:00:00 [DOWNLOAD] [update] 开始更新检测：共 10 个离线漫画", LogUpdate},
		{"2026/08/07 10:00:00 [DOWNLOAD-WARN] [update] 漫画详情拉取失败（跳过）", LogUpdate},
		{"2026/08/07 10:00:00 [DOWNLOAD] [maintain] 维护查重完成：建议保留 5 项", LogMaintain},
		{"2026/08/07 10:00:00 [DOWNLOAD] [maintain] 已删除本地文件", LogMaintain},
		{"2026/08/07 10:00:00 [DOWNLOAD] [scan] 路径扫描完成：发现 3 本", LogOther},
		{"2026/08/07 10:00:00 [DOWNLOAD-ERROR] [scan] 扫描失败", LogOther},
		{"2026/08/07 10:00:00 [DOWNLOAD] [tagm] 开始 Tag 刷新", LogOther},
		{"2026/08/07 10:00:00 [DOWNLOAD] [offline] 读取额外路径配置失败", LogOther},
		{"2026/08/07 10:00:00 [DOWNLOAD] 任务 1 开始下载（gid=123）", LogDownload},
		{"2026/08/07 10:00:00 [DOWNLOAD-WARN] 网络请求重试", LogDownload},
		{"2026/08/07 10:00:00 [DOWNLOAD-ERROR] 归档下载失败", LogDownload},
		{"2026/08/07 10:00:00 [ARCHIVER] archiver 请求成功", LogDownload},
		{"2026/08/07 10:00:00 [sched] 调度检查下载队列", LogDownload},
		{"2026/08/07 10:00:00 [Toplist] 成功更新全站热度榜单", LogOther},
		{"2026/08/07 10:00:00 [TagEngine] 成功装载标签库", LogOther},
		{"2026/08/07 10:00:00 [EH-READER] 画廊抓取成功", LogOther},
		{"2026/08/07 10:00:00 [client-log] 写入失败", LogOther},
		{"2026/08/07 10:00:00 [DB] 数据库路径", LogOther},
		{"2026/08/07 10:00:00 普通无标签日志", LogOther},
	}
	for _, c := range cases {
		if got := classifyLogLine(c.line); got != c.want {
			t.Errorf("classifyLogLine(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

// TestLogFileName 验证按日归档文件名格式 <cat>_log-YYYY-MM-DD.log
func TestLogFileName(t *testing.T) {
	cases := map[LogCategory]string{
		LogUpdate:   "update_log-2026-08-07.log",
		LogMaintain: "maintain_log-2026-08-07.log",
		LogDownload: "download_log-2026-08-07.log",
		LogOther:    "other_log-2026-08-07.log",
	}
	for cat, want := range cases {
		if got := LogFileName(cat, "2026-08-07"); got != want {
			t.Errorf("LogFileName(%s, 2026-08-07) = %q, want %q", cat, got, want)
		}
	}
}
