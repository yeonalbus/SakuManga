package services

import "testing"

// Round20-Bug3：f_search tag 解析器与本地匹配语义（决策 D5=A：$ 精确、无 $ 前缀）

func TestParseFSearchTag(t *testing.T) {
	cases := []struct {
		raw       string
		ns, key   string
		anchored  bool
		isTag     bool
	}{
		{raw: `female:"magical girl$"`, ns: "female", key: "magical girl", anchored: true, isTag: true},
		{raw: `female:yuri$`, ns: "female", key: "yuri", anchored: true, isTag: true},
		{raw: `female:yuri`, ns: "female", key: "yuri", anchored: false, isTag: true},
		{raw: `parody:genshin impact`, ns: "parody", key: "genshin impact", anchored: false, isTag: true},
		{raw: `- female:magical_girl$`, ns: "female", key: "magical girl", anchored: true, isTag: true},
		{raw: `Female:"Big Breasts"`, ns: "female", key: "big breasts", anchored: false, isTag: true},
		{raw: `3d`, isTag: false},
		{raw: `  `, isTag: false},
		{raw: `:no-ns`, isTag: false},
	}
	for _, c := range cases {
		got := ParseFSearchTag(c.raw)
		if got.IsTag != c.isTag {
			t.Errorf("ParseFSearchTag(%q) IsTag=%v，期望 %v", c.raw, got.IsTag, c.isTag)
			continue
		}
		if !c.isTag {
			continue
		}
		if got.Namespace != c.ns || got.Key != c.key || got.Anchored != c.anchored {
			t.Errorf("ParseFSearchTag(%q) = {%s,%s,anchored=%v}，期望 {%s,%s,anchored=%v}",
				c.raw, got.Namespace, got.Key, got.Anchored, c.ns, c.key, c.anchored)
		}
	}
}

func TestFSearchTagMatchTag(t *testing.T) {
	cases := []struct {
		raw   string
		tag   string // 本地已归一化 tagRaws（ns:key，小写、_→空格）
		want  bool
	}{
		{raw: `female:"magical girl$"`, tag: "female:magical girl", want: true},
		{raw: `female:"magical girl$"`, tag: "female:magical girl 2", want: false}, // $ 精确
		{raw: `female:magical`, tag: "female:magical girl", want: true},             // 无 $ 前缀
		{raw: `female:magical$`, tag: "female:magical girl", want: false},
		{raw: `female:sole$`, tag: "female:sole female", want: false},
		{raw: `female:sole`, tag: "female:sole male", want: true},
		{raw: `parody:genshin impact`, tag: "parody:genshin impact", want: true},
		{raw: `3d`, tag: "female:3d", want: false}, // 裸词不是 tag
	}
	for _, c := range cases {
		got := ParseFSearchTag(c.raw).MatchTag(c.tag)
		if got != c.want {
			t.Errorf("MatchTag(%q, %q) = %v，期望 %v", c.raw, c.tag, got, c.want)
		}
	}
}

func TestFSearchTagJSONPatterns(t *testing.T) {
	tag := ParseFSearchTag(`female:"magical girl$"`)
	patterns := tag.TagJSONMatchPatterns()
	if len(patterns) != 2 {
		t.Fatalf("应生成空格+下划线两种模式，得到 %d: %v", len(patterns), patterns)
	}
	// 模式应能命中 JSON 数组元素（前导 % 匹配开引号，尾 % 匹配闭引号），且转义了 LIKE 通配符
	if patterns[0] != `%"female:magical girl%` {
		t.Errorf("空格模式 = %q", patterns[0])
	}
	if patterns[1] != `%"female:magical\_girl%` {
		t.Errorf("下划线模式 = %q", patterns[1])
	}

	// 含下划线通配符的 key 必须被转义，避免 SQL LIKE 误匹配
	tag2 := ParseFSearchTag(`female:big_breasts`)
	for _, p := range tag2.TagJSONMatchPatterns() {
		if containsUnescapedUnderscore(p) {
			t.Errorf("模式 %q 含未转义下划线", p)
		}
	}
}

func containsUnescapedUnderscore(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '_' {
			if i == 0 || s[i-1] != '\\' {
				return true
			}
		}
	}
	return false
}
