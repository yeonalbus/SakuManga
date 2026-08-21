package services

import "testing"

// Round11-Bug1：列表评分雪碧解析（与排行榜同一 X/Y 半星算法）
// 样本取自 testdata_eh/eh_popular_live.html（2026-08-21 真实抓取）
func TestParseRatingFromStyleHalfStar(t *testing.T) {
	cases := []struct {
		style string
		want  float64
	}{
		// 满星行（Y=-1px）
		{"background-position:0px -1px;opacity:1", 5.0},
		{"background-position:-16px -1px;opacity:1", 4.0},
		{"background-position:-32px -1px;opacity:1", 3.0},
		{"background-position:-48px -1px;opacity:1", 2.0},
		{"background-position:-64px -1px;opacity:1", 1.0},
		// 半星行（Y=-21px）：热门/首页列表大量使用
		{"background-position:0px -21px;opacity:1", 4.5},
		{"background-position:-16px -21px;opacity:1", 3.5},
		{"background-position:-32px -21px;opacity:1", 2.5},
		{"background-position:-48px -21px;opacity:1", 1.5},
		{"background-position:-64px -21px;opacity:1", 0.5},
		// 旧式无 Y / 纯 0px 0px
		{"background-position:0px 0px", 5.0},
		// 带 opacity 小数后缀（真实样本）
		{"background-position:-32px -21px;opacity:0.86666666666667", 2.5},
		{"background-position:-16px -1px;opacity:0.8", 4.0},
		// 无法解析
		{"", 0.0},
		{"background-image:url(x.png)", 0.0},
	}
	for _, c := range cases {
		got := parseRatingFromStyle(c.style)
		if got != c.want {
			t.Errorf("parseRatingFromStyle(%q) = %v，期望 %v", c.style, got, c.want)
		}
	}
}

// Round11-Bug2：页数正则必须排除「数字 + 大写P开头单词」（如 "223 Piece"），
// 仅匹配 pages/page/独立 P/页。
func TestPageCountRegexNoPWordMatch(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"Misc Misc 2026-08-05 03:11 223 pages 2026-08-05 03:11 title", "223"},
		{"2026-08-20 17:18 495 page 2026-08-20 17:18 title", "495"}, // 单数 page
		{"39P", "39"},                                                // 独立大写 P
		{"(223 Piece 223枚) [AI Generated]", ""},                     // 标题里的 Piece 不得误匹配
		{"959539 Piece", ""},                                         // 大数字 + Piece 不得误匹配
		{"(959539 页)", "959539"},                                    // 中文「页」仍应匹配
		{"39 Pages", "39"},                                           // Pages 大写（(?i:pages?)）仍匹配
		{"Holo_Hubuki(303p) [AI Generated]", ""},                       // 标题小写 p 后缀不得误匹配
		{"303 pages [AI Generated]", "303"},                            // 正常页数文本
	}
	for _, c := range cases {
		m := pageCountRegex.FindStringSubmatch(c.text)
		got := ""
		if len(m) > 1 {
			got = m[1]
		}
		if got != c.want {
			t.Errorf("pageCountRegex on %q = %q，期望 %q", c.text, got, c.want)
		}
	}
}