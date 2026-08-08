package services

import "testing"

// NormalizeFSearch 简版幂等转义回归测试
func TestNormalizeFSearch_Empty(t *testing.T) {
	if got := NormalizeFSearch(""); got != "" {
		t.Fatalf("空输入应原样返回，got %q", got)
	}
	// 纯空白输入经 TrimSpace 后为空，调用方不会下发，返回空即可
	if got := NormalizeFSearch("   "); got != "" {
		t.Fatalf("空白输入应返回空，got %q", got)
	}
}

func TestNormalizeFSearch_PlainKeyword(t *testing.T) {
	// 纯裸词（无 namespace）原样透传
	cases := []string{"full color", "blue archive", "larger b"}
	for _, in := range cases {
		if got := NormalizeFSearch(in); got != in {
			t.Fatalf("裸词 %q 应原样返回，got %q", in, got)
		}
	}
}

func TestNormalizeFSearch_SingleWordTag(t *testing.T) {
	// 单词 tag → namespace:key$
	cases := map[string]string{
		"female:futanari":   "female:futanari$",
		"language:chinese":  "language:chinese$",
		"parody:blue_archive": "parody:blue_archive$",
	}
	for in, want := range cases {
		if got := NormalizeFSearch(in); got != want {
			t.Fatalf("%q 应规范化为 %q，got %q", in, want, got)
		}
	}
}

func TestNormalizeFSearch_TwoWordTag(t *testing.T) {
	// 两词 tag → namespace:"key1 key2$"
	cases := map[string]string{
		"group:da hootch":            `group:"da hootch$"`,
		"character:reimu hakurei":    `character:"reimu hakurei$"`,
		"parody:blue archive":        `parody:"blue archive$"`,
		"female:white stokings":      `female:"white stokings$"`,
	}
	for in, want := range cases {
		if got := NormalizeFSearch(in); got != want {
			t.Fatalf("%q 应规范化为 %q，got %q", in, want, got)
		}
	}
}

func TestNormalizeFSearch_ThreePlusWords(t *testing.T) {
	// 3 词及以上视为混合独立词输入，不处理（避免过度合并）
	cases := []string{
		"character:reimu hakurei large penis",
		"group:da hootch extra word",
	}
	for _, in := range cases {
		if got := NormalizeFSearch(in); got != in {
			t.Fatalf("混合输入 %q 应原样返回，got %q", in, got)
		}
	}
}

func TestNormalizeFSearch_MultiNamespace(t *testing.T) {
	// 多 tag 查询（多个 namespace）不处理，原样透传
	cases := []string{
		"group:da hootch artist:yy",
		"female:futanari character:reimu hakurei",
		"language:chinese full color",
	}
	for _, in := range cases {
		if got := NormalizeFSearch(in); got != in {
			t.Fatalf("多 namespace 输入 %q 应原样返回，got %q", in, got)
		}
	}
}

func TestNormalizeFSearch_AlreadyNormalized(t *testing.T) {
	// 幂等：已含引号或 $ 的手写标准语法不二次处理
	cases := []string{
		`group:"da hootch$"`,
		"female:futanari$",
		`parody:"blue archive$" full color`,
		"character:kashima$",
	}
	for _, in := range cases {
		if got := NormalizeFSearch(in); got != in {
			t.Fatalf("已标准语法 %q 应原样返回（幂等），got %q", in, got)
		}
	}
}

func TestNormalizeFSearch_NoColonOrInvalidNS(t *testing.T) {
	// 无冒号 / namespace 名非法 → 原样
	cases := []string{
		"foo bar:baz", // 首 token 无冒号
		"12345:foo",   // namespace 含数字（非法约定，不处理）
		"Foo:bar",     // namespace 大写（非法约定，不处理）
	}
	for _, in := range cases {
		if got := NormalizeFSearch(in); got != in {
			t.Fatalf("输入 %q 应原样返回，got %q", in, got)
		}
	}
}
