package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Round41：/logs/tail 反向分块读取的回归用例。
// 覆盖：文件缺失/空、尾部 limit、since 过滤、跨块边界、末行半行、超长行、
// 无时间戳行、硬上限，以及与「全量朴素实现」的一致性对照。

var tailTestBase = time.Date(2026, 9, 18, 10, 0, 0, 0, time.Local)

func tailTestLine(sec int) string {
	at := tailTestBase.Add(time.Duration(sec) * time.Second)
	return fmt.Sprintf("%s [EH-DETAIL-DEBUG] 成功抓取画廊 [%d] 首页 | 初始预览图: 20 张 | 总预览页数: 2 | 社区评论: 13 条",
		at.Format("2006/01/02 15:04:05"), 4000000+sec)
}

func tailTestTs(sec int) int64 {
	return tailTestBase.Add(time.Duration(sec) * time.Second).UnixMilli()
}

// writeTailFile 写出日志文件；trailingNewline=false 时模拟"最后一行尚未写完"
func writeTailFile(t *testing.T, lines []string, trailingNewline bool) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "other_log-2026-09-18.log")
	content := strings.Join(lines, "\n")
	if trailingNewline && len(lines) > 0 {
		content += "\n"
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("写测试日志失败: %v", err)
	}
	return p
}

// naiveTail 旧实现的等价逻辑（全量读 + 逐行解析 + 取末尾 limit 行），用于一致性对照
func naiveTail(path string, since int64, limit int) []LogTailLine {
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
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func sameLines(a, b []LogTailLine) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Ts != b[i].Ts || a[i].Text != b[i].Text {
			return false
		}
	}
	return true
}

func TestTailFileLinesMissingAndEmpty(t *testing.T) {
	if got := tailFileLines(filepath.Join(t.TempDir(), "不存在.log"), 0, 10); got != nil {
		t.Fatalf("文件不存在应返回 nil，得到 %d 行", len(got))
	}
	empty := writeTailFile(t, nil, false)
	if got := tailFileLines(empty, 0, 10); got != nil {
		t.Fatalf("空文件应返回 nil，得到 %d 行", len(got))
	}
}

func TestTailFileLinesTailLimit(t *testing.T) {
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, tailTestLine(i))
	}
	p := writeTailFile(t, lines, true)

	got := tailFileLines(p, 0, 3)
	if len(got) != 3 {
		t.Fatalf("limit=3 应返回 3 行，得到 %d 行", len(got))
	}
	// 正序：应为最后 3 行（sec=17,18,19）
	for i, wantSec := range []int{17, 18, 19} {
		if got[i].Ts != tailTestTs(wantSec) {
			t.Fatalf("第 %d 行 ts=%d，期望 %d（应为末尾 3 行且正序）", i, got[i].Ts, tailTestTs(wantSec))
		}
		if !strings.Contains(got[i].Text, fmt.Sprintf("[%d]", 4000000+wantSec)) {
			t.Fatalf("第 %d 行内容不符: %q", i, got[i].Text)
		}
	}
}

func TestTailFileLinesSinceFilter(t *testing.T) {
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, tailTestLine(i))
	}
	p := writeTailFile(t, lines, true)

	got := tailFileLines(p, tailTestTs(9), 0)
	if len(got) != 10 {
		t.Fatalf("since=sec9 应返回 sec10..19 共 10 行，得到 %d 行", len(got))
	}
	if got[0].Ts != tailTestTs(10) || got[len(got)-1].Ts != tailTestTs(19) {
		t.Fatalf("返回区间不符: 首 %d 末 %d", got[0].Ts, got[len(got)-1].Ts)
	}
	for _, l := range got {
		if l.Ts <= tailTestTs(9) {
			t.Fatalf("返回了 ts<=since 的行: ts=%d", l.Ts)
		}
	}
}

// 跨块一致性：文件远超 64KB 单块，反向拼接不得丢行/串行
func TestTailFileLinesCrossChunkConsistency(t *testing.T) {
	var lines []string
	for i := 0; i < 4000; i++ {
		lines = append(lines, tailTestLine(i))
	}
	p := writeTailFile(t, lines, true)

	cases := []struct {
		since int64
		limit int
	}{
		{0, 250},          // 尾部窗口，跨块
		{0, 5000},         // 全量（< 硬上限）
		{tailTestTs(1000), 0},
		{tailTestTs(3500), 120},
	}
	for _, cse := range cases {
		got := tailFileLines(p, cse.since, cse.limit)
		want := naiveTail(p, cse.since, cse.limit)
		if !sameLines(got, want) {
			t.Fatalf("since=%d limit=%d 与朴素实现不一致：got %d 行 / want %d 行", cse.since, cse.limit, len(got), len(want))
		}
	}
}

// 文件末尾半行（写入中）应被丢弃，等下一轮以完整行出现
func TestTailFileLinesDropsPartialTail(t *testing.T) {
	lines := []string{tailTestLine(0), tailTestLine(1), tailTestLine(2) + " [半行未写完"}
	p := writeTailFile(t, lines, false) // 末尾无换行

	got := tailFileLines(p, 0, 10)
	if len(got) != 2 {
		t.Fatalf("末尾半行应被丢弃，期望 2 行，得到 %d 行", len(got))
	}
	if got[1].Ts != tailTestTs(1) {
		t.Fatalf("末行应为 sec=1，得到 ts=%d", got[1].Ts)
	}

	// 补全后（带换行）应能返回全部 3 行
	p2 := writeTailFile(t, lines, true)
	if got2 := tailFileLines(p2, 0, 10); len(got2) != 3 {
		t.Fatalf("完整文件应返回 3 行，得到 %d 行", len(got2))
	}
}

// 单行长度超过块大小（ARCHIVER 片段等）不得被截断或丢弃
func TestTailFileLinesLongLine(t *testing.T) {
	long := tailTestLine(0) + " " + strings.Repeat("X", 200*1024)
	lines := []string{long, tailTestLine(1)}
	p := writeTailFile(t, lines, true)

	got := tailFileLines(p, 0, 10)
	if len(got) != 2 {
		t.Fatalf("期望 2 行，得到 %d 行", len(got))
	}
	if len(got[0].Text) != len(long) || got[0].Text != long {
		t.Fatalf("超长行被破坏：期望 %d 字节，得到 %d 字节", len(long), len(got[0].Text))
	}
}

// 无时间戳的行（ts=0，多行堆栈等）：since>0 时过滤，且不阻断更早行的停止判定
func TestTailFileLinesZeroTsLines(t *testing.T) {
	lines := []string{
		tailTestLine(0),
		"\tgoroutine 1 [running]:",
		"main.main()",
		tailTestLine(1),
	}
	p := writeTailFile(t, lines, true)

	// since=sec0：只应返回 sec1 那行（无时间戳行被过滤）
	got := tailFileLines(p, tailTestTs(0), 0)
	if len(got) != 1 || got[0].Ts != tailTestTs(1) {
		t.Fatalf("期望仅返回 sec=1 一行，得到 %d 行: %+v", len(got), got)
	}

	// since=0：无时间戳行照常返回（与旧行为一致）
	all := tailFileLines(p, 0, 0)
	if len(all) != 4 {
		t.Fatalf("since=0 应返回全部 4 行，得到 %d 行", len(all))
	}
	if all[3].Text != tailTestLine(1) {
		t.Fatalf("末行应为 sec=1，得到 %q", all[3].Text)
	}
}

// limit<=0 时的硬上限：返回行数不得超过 tailMaxLines
func TestTailFileLinesHardLimit(t *testing.T) {
	var lines []string
	for i := 0; i < tailMaxLines+800; i++ {
		lines = append(lines, tailTestLine(i))
	}
	p := writeTailFile(t, lines, true)

	got := tailFileLines(p, 0, 0)
	if len(got) != tailMaxLines {
		t.Fatalf("硬上限应为 %d 行，得到 %d 行", tailMaxLines, len(got))
	}
	// 应保留最新的 tailMaxLines 行
	if got[len(got)-1].Ts != tailTestTs(tailMaxLines+800-1) {
		t.Fatalf("末行 ts=%d，期望 %d", got[len(got)-1].Ts, tailTestTs(tailMaxLines+800-1))
	}
}

// ── 性能基线（Round41）：新实现 vs 旧全量实现 ──
// 运行：cd backend && go test ./internal/handlers/ -bench BenchmarkTailFile -benchmem -run '^$'

func benchLogFile(b *testing.B, n int) string {
	b.Helper()
	lines := make([]string, 0, n)
	for i := 0; i < n; i++ {
		lines = append(lines, tailTestLine(i))
	}
	p := filepath.Join(b.TempDir(), "bench.log")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		b.Fatalf("写基准文件失败: %v", err)
	}
	return p
}

// BenchmarkTailFileLast300Round41 首屏/尾窗场景：只取最后 300 行
func BenchmarkTailFileLast300Round41(b *testing.B) {
	p := benchLogFile(b, 4000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := tailFileLines(p, 0, 300); len(got) != 300 {
			b.Fatalf("期望 300 行，得到 %d 行", len(got))
		}
	}
}

// BenchmarkTailFileLast300Naive 旧实现（全量读 + 逐行解析）同场景对照
func BenchmarkTailFileLast300Naive(b *testing.B) {
	p := benchLogFile(b, 4000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := naiveTail(p, 0, 300); len(got) != 300 {
			b.Fatalf("期望 300 行，得到 %d 行", len(got))
		}
	}
}

// BenchmarkTailFileIncrementalRound41 增量轮询场景：since = 最后一秒之前
func BenchmarkTailFileIncrementalRound41(b *testing.B) {
	p := benchLogFile(b, 4000)
	since := tailTestTs(3999) - 1000
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tailFileLines(p, since, 600)
	}
}

// BenchmarkTailFileIncrementalNaive 旧实现同场景对照
func BenchmarkTailFileIncrementalNaive(b *testing.B) {
	p := benchLogFile(b, 4000)
	since := tailTestTs(3999) - 1000
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = naiveTail(p, since, 600)
	}
}
