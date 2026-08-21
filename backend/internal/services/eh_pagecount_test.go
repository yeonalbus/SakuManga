package services

import (
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// Round11-Bug3：列表页页数必须从「叶子节点」整串提取，禁止对整行文本拼接匹配。
//
// 根因：行文本是各子节点文本的拼接，上传者名字末尾的数字会与紧随其后的
// "N pages" 接成更大的数。实测 gid=4136008（上传者 "Nid135" + "29 pages"
// → "Nid13529 pages"），旧实现误取为 13529。
func TestExtractListPageCountLeaf(t *testing.T) {
	cases := []struct {
		name string
		html string
		want int
	}{
		{
			// 真实复现样本：上传者 Nid135 + 29 pages 拼接成 Nid13529 pages
			"uploader digits then pages",
			`<table class="itg"><tr><td class="gl2e"><div><div class="gl3e"><div class="cn ct2">Doujinshi</div><div onclick="popUp('x')" id="posted_4136008">2026-08-21 13:03</div><div class="ir" style="background-position:0px -1px;opacity:1"></div><div><a href="https://e-hentai.org/uploader/Nid135">Nid135</a></div><div>29 pages</div><div class="gldown"><a href="#">T</a></div></div></div></td></tr></table>`,
			29,
		},
		{
			"single page",
			`<table class="itg"><tr><td><div>1 page</div></td></tr></table>`,
			1,
		},
		{
			"uppercase P suffix",
			`<table class="itg"><tr><td><div>39P</div></td></tr></table>`,
			39,
		},
		{
			"chinese unit",
			`<table class="itg"><tr><td><div>39页</div></td></tr></table>`,
			39,
		},
		{
			"no page node",
			`<table class="itg"><tr><td><div>Nid135</div></td></tr></table>`,
			0,
		},
		{
			"posted date only",
			`<table class="itg"><tr><td><div>2026-08-21 13:03</div></td></tr></table>`,
			0,
		},
		{
			// 收藏页出现 "Favorited:" 段落，不得干扰
			"favorites extra paragraph",
			`<table class="itg"><tr><td><div><div>438 pages</div><div><p>Favorited:</p><p>2026-08-08 09:17</p></div></div></td></tr></table>`,
			438,
		},
		{
			// 超大数防御仍生效
			"oversized guard",
			`<table class="itg"><tr><td><div>123456789 pages</div></td></tr></table>`,
			0,
		},
		{
			// 标题里的 "Piece" 与标签文本不得被当作页数
			"piece in title not page",
			`<table class="itg"><tr><td><div class="glink">(223 Piece 223枚)</div><div>44 pages</div></td></tr></table>`,
			44,
		},
	}
	for _, c := range cases {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(c.html))
		if err != nil {
			t.Fatalf("%s: 解析 HTML 失败: %v", c.name, err)
		}
		sel := doc.Find("table.itg tr").First()
		if got := extractListPageCount(sel); got != c.want {
			t.Errorf("%s: extractListPageCount = %d，期望 %d", c.name, got, c.want)
		}
	}
}

// TestExtractListPageCountRealHome 用真实抓取的 e-hentai.org 首页样本做回归：
// gid=4136008 必须解析为 29 页（旧实现为 13529）。
// 样本文件 testdata_eh/eh_home_4136008.html（2026-08-21 抓取）。
func TestExtractListPageCountRealHome(t *testing.T) {
	for _, sample := range []string{
		"../../../testdata_eh/eh_home_4136008.html",  // e-hentai 首页
		"../../../testdata_eh/ex_home_4136008.html",  // exhentai 首页（账号 Site=exhentai 时）
	} {
		f, err := os.Open(sample)
		if err != nil {
			t.Skipf("真实样本缺失，跳过: %v", err)
		}
		defer f.Close()

		doc, err := goquery.NewDocumentFromReader(f)
		if err != nil {
			t.Fatal(err)
		}

		want := map[string]int{
			"4136008": 29,
		}
		matched := 0
		doc.Find("table.itg tr, div.gl1t, div.gl2t, div.gl1e, div.gl2e").Each(func(_ int, s *goquery.Selection) {
			href, ok := s.Find("a[href*='/g/']").First().Attr("href")
			if !ok {
				return
			}
			parts := strings.Split(strings.Trim(href, "/"), "/")
			if len(parts) < 2 {
				return
			}
			gid := parts[len(parts)-2]
			if w, exists := want[gid]; exists {
				matched++
				if got := extractListPageCount(s); got != w {
					t.Errorf("%s: gid=%s extractListPageCount = %d，期望 %d", sample, gid, got, w)
				}
			}
		})
		if matched == 0 {
			t.Skipf("%s: 样本中未找到目标 gid，跳过断言", sample)
		}
	}
}
