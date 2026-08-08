package services

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// TestToplistParseAllSamples 校验 5 个真实榜单快照都能解析出整页 50 条、
// 排名从首页开始且严格连续递增（对齐 E 站 p 参数 0 基分页）。
func TestToplistParseAllSamples(t *testing.T) {
	cases := []struct {
		file     string
		wantSize int
		wantRank int // 该页首页全局排名
	}{
		{"eh_toplist_alltime_p1.html", 50, 51},
		{"eh_toplist_alltime_p2.html", 50, 101},
		{"eh_toplist_pastyear_p1.html", 50, 51},
		{"eh_toplist_pastmonth_p1.html", 50, 51},
		{"eh_toplist_yesterday_p1.html", 50, 51},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			list := parseToplistSnapshot(t, c.file)
			if len(list) != c.wantSize {
				t.Fatalf("期望 %d 条，实际 %d 条", c.wantSize, len(list))
			}
			for i, comic := range list {
				want := c.wantRank + i
				if comic.Rank != want {
					t.Fatalf("第 %d 条排名 = %d，期望 %d", i, comic.Rank, want)
				}
				if comic.ID == "" || comic.Token == "" {
					t.Fatalf("第 %d 条缺少 gid/token: %+v", i, comic)
				}
				if comic.Title == "" {
					t.Fatalf("第 %d 条标题为空", i)
				}
			}
		})
	}
}

// TestToplistParseAlltimeFirstRow 校验 alltime 第 1 页首两行的完整字段（对齐实测快照）。
func TestToplistParseAlltimeFirstRow(t *testing.T) {
	list := parseToplistSnapshot(t, "eh_toplist_alltime_p1.html")

	first := list[0]
	if first.ID != "596732" {
		t.Fatalf("首条 gid = %s，期望 596732", first.ID)
	}
	if first.Token != "5f196ab720" {
		t.Fatalf("首条 token = %s，期望 5f196ab720", first.Token)
	}
	if first.Rank != 51 {
		t.Fatalf("首条排名 = %d，期望 51", first.Rank)
	}
	if first.Score != 32801399 {
		t.Fatalf("首条浏览量 = %d，期望 32801399", first.Score)
	}
	if first.Category != "Artist CG" {
		t.Fatalf("首条分类 = %q，期望 Artist CG", first.Category)
	}
	// 快照中首条 .ir 为 background-position:0px -21px → 4.5
	if first.Rating != 4.5 {
		t.Fatalf("首条评分 = %v，期望 4.5", first.Rating)
	}
	if first.PageCount != 394 {
		t.Fatalf("首条页数 = %d，期望 394", first.PageCount)
	}
	if first.Uploader != "pocky00" {
		t.Fatalf("首条上传者 = %q，期望 pocky00", first.Uploader)
	}
	if len(first.Tags) == 0 {
		t.Fatalf("首条标签为空，期望解析出 .gt[title] 标签")
	}
	if first.CoverURL == "" {
		t.Fatalf("首条封面为空，期望解析出封面地址")
	}

	// 第二条 .ir 为 background-position:0px -1px → 5.0
	second := list[1]
	if second.ID != "2755054" {
		t.Fatalf("次条 gid = %s，期望 2755054", second.ID)
	}
	if second.Rating != 5.0 {
		t.Fatalf("次条评分 = %v，期望 5.0", second.Rating)
	}
}

// TestParseToplistRatingFromStyle 校验横向雪碧评分映射表（X 整星 + Y 半星）。
func TestParseToplistRatingFromStyle(t *testing.T) {
	cases := []struct {
		style string
		want  float64
	}{
		{"background-position:0px -1px;opacity:1", 5.0},
		{"background-position:0px -21px;opacity:1", 4.5},
		{"background-position:-16px -1px;opacity:1", 4.0},
		{"background-position:-16px -21px;opacity:1", 3.5},
		{"background-position:-32px -1px;opacity:1", 3.0},
		{"background-position:-32px -21px;opacity:1", 2.5},
		{"background-position:-48px -1px;opacity:1", 2.0},
		{"background-position:-48px -21px;opacity:1", 1.5},
		{"background-position:-64px -1px;opacity:1", 1.0},
		{"background-position:-64px -21px;opacity:1", 0.5},
		{"background-position:0px 0px", 5.0},
		{"", 0.0},
	}
	for _, c := range cases {
		got := parseToplistRatingFromStyle(c.style)
		if got != c.want {
			t.Errorf("parseToplistRatingFromStyle(%q) = %v，期望 %v", c.style, got, c.want)
		}
	}
}

// parseToplistSnapshot 读取测试快照并解析为榜单数据
func parseToplistSnapshot(t *testing.T, name string) []RankedOnlineComicDTO {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../../../testdata_eh", name))
	if err != nil {
		t.Fatalf("读取快照失败: %v", err)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("解析 HTML 失败: %v", err)
	}
	list, err := parseToplistDoc(doc)
	if err != nil {
		t.Fatalf("parseToplistDoc 失败: %v", err)
	}
	return list
}
