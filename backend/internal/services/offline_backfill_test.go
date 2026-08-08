package services

import (
	"os"
	"path/filepath"
	"testing"
)

// ─────────────────────────────────────────────────────────────
// S6/D5-B：跨文件夹查重——在线回填 GID（纯函数）与 .ehdata 元数据兼容
//
// 覆盖：
//   - buildSearchTitle  从本地标题构造在线搜索关键词（去 [Artist] 前缀/折叠空白/过短放弃）
//   - normalizeTitle    标题归一化（小写、丢弃标点、保留 CJK/日文、折叠空白）
//   - isTitleMatch      标题匹配判定（完全相等，或长度 ≥6 时互为子串）
//   - pickConfidentMatch 从搜索结果挑选高置信度唯一命中（多结果同名 → nil 防误写）
//   - ParseDirMetadata  额外路径无标准 sidecar 时，.ehdata 内嵌 JSON 亦可回填 GID/Token
// ─────────────────────────────────────────────────────────────

// TestBuildSearchTitle 验证搜索关键词构造：去方括号标签前缀、折叠空白、过短放弃。
func TestBuildSearchTitle(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`[milky] Cinderella`, `Cinderella`}, // 标准 "[Artist] Title"（带空格）
		{`[milky]Cinderella`, `Cinderella`},  // 无空格方括号前缀
		{`  Hello   World  `, `Hello World`}, // 折叠多余空白
		{`简体中文标题`, `简体中文标题`},          // CJK 保留（rune 计数 ≥4）
		{``, ``},                            // 空标题
		{`   `, ``},                         // 纯空白
		{`abc`, ``},                         // 过短（<4 字符）放弃
		{`[ab] xy`, ``},                     // 去前缀后过短 → 放弃
	}
	for _, c := range cases {
		if got := buildSearchTitle(c.in); got != c.want {
			t.Errorf("buildSearchTitle(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

// TestNormalizeTitle 验证标题归一化：小写、丢弃标点、保留 CJK、折叠空白。
func TestNormalizeTitle(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`Hello, World! (Part 2)`, `hello world part 2`},
		{`TEST-123_Case`, `test123case`},
		{`  多  空白  `, `多 空白`},
		{`Cinderella`, `cinderella`},
	}
	for _, c := range cases {
		if got := normalizeTitle(c.in); got != c.want {
			t.Errorf("normalizeTitle(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

// TestIsTitleMatch 验证匹配判定：完全相等或长度 ≥6 时互为子串。
func TestIsTitleMatch(t *testing.T) {
	eq := func(a, b string) bool {
		return isTitleMatch(a, b)
	}
	if !eq(`cinderella`, `cinderella`) {
		t.Errorf("完全相同标题应匹配")
	}
	// 互为子串且长度足够
	if !eq(`my awesome doujin collection`, `awesome doujin collection`) {
		t.Errorf("长度足够的包含关系应匹配（a 含 b）")
	}
	if !eq(`awesome doujin collection`, `my awesome doujin collection`) {
		t.Errorf("长度足够的包含关系应匹配（b 含 a）")
	}
	// 过短子串（<6）不应误判
	if eq(`abc`, `abcdef`) {
		t.Errorf("长度不足的子串不应匹配")
	}
	// 完全不同
	if eq(`completely different`, `totally unrelated`) {
		t.Errorf("不同标题不应匹配")
	}
}

// TestPickConfidentMatch 验证高置信度唯一命中；多结果同名 → nil 防误写。
func TestPickConfidentMatch(t *testing.T) {
	title := `[milky] Cinderella`

	// 1. 唯一命中 → 返回该 DTO，并回填 ID/Token
	unique := []OnlineComicDTO{
		{ID: `111`, Token: `tok1`, Title: `Cinderella`},
		{ID: `222`, Token: `tok2`, Title: `Some Other Gallery`},
	}
	got := pickConfidentMatch(title, unique)
	if got == nil {
		t.Fatalf("唯一命中时不应返回 nil")
	}
	if got.ID != `111` || got.Token != `tok1` {
		t.Errorf("应命中 ID=111/Token=tok1，实际 ID=%s/Token=%s", got.ID, got.Token)
	}

	// 2. 多个结果标题接近（归一化一致）→ 同名不同画，置信度不足
	multi := []OnlineComicDTO{
		{ID: `333`, Token: `tok3`, Title: `Cinderella`},
		{ID: `444`, Token: `tok4`, Title: `cinderella`}, // 归一化后相同
	}
	if got := pickConfidentMatch(title, multi); got != nil {
		t.Errorf("多结果同名应返回 nil（防误写），实际命中 ID=%s", got.ID)
	}

	// 3. 无匹配 → nil
	none := []OnlineComicDTO{
		{ID: `555`, Token: `tok5`, Title: `Unrelated Gallery`},
	}
	if got := pickConfidentMatch(title, none); got != nil {
		t.Errorf("无匹配应返回 nil，实际命中 ID=%s", got.ID)
	}

	// 4. 目标标题归一化为空 → nil
	empty := []OnlineComicDTO{{ID: `666`, Token: `tok6`, Title: `Cinderella`}}
	if got := pickConfidentMatch(`   `, empty); got != nil {
		t.Errorf("空目标标题应返回 nil，实际命中 ID=%s", got.ID)
	}

	// 5. 空结果集 → nil
	if got := pickConfidentMatch(title, nil); got != nil {
		t.Errorf("空结果集应返回 nil，实际命中 ID=%s", got.ID)
	}
}

// TestParseDirMetadataEhdata 验证额外路径无标准 sidecar 时，.ehdata 内嵌 JSON
// 可被识别并提取 GID/Token（D5-B 根因：无元数据文件夹 GID==''，查重无法跨路径识别）。
func TestParseDirMetadataEhdata(t *testing.T) {
	dir := t.TempDir()
	content := `{"gid":1509130,"token":"ehdata-token","title":"Test Ehdata Gallery","tags":["artist:a","language:english"]}`
	if err := os.WriteFile(filepath.Join(dir, ".ehdata"), []byte(content), 0o644); err != nil {
		t.Fatalf("写入 .ehdata 失败: %v", err)
	}

	meta := ParseDirMetadata(dir)
	if meta.GID != "1509130" {
		t.Errorf("GID 应为 1509130，实际 %q", meta.GID)
	}
	if meta.Token != "ehdata-token" {
		t.Errorf("Token 应为 ehdata-token，实际 %q", meta.Token)
	}
	if meta.Title != "Test Ehdata Gallery" {
		t.Errorf("Title 应为 Test Ehdata Gallery，实际 %q", meta.Title)
	}
	if len(meta.Tags) != 2 {
		t.Errorf("Tags 应解析出 2 个，实际 %d: %v", len(meta.Tags), meta.Tags)
	}
}

// TestParseDirMetadataEhdataNumericGID 验证 .ehdata 中 gid 为数字时可转字符串。
func TestParseDirMetadataEhdataNumericGID(t *testing.T) {
	dir := t.TempDir()
	content := `{"gid":42,"token":"num-token"}`
	if err := os.WriteFile(filepath.Join(dir, ".ehdata"), []byte(content), 0o644); err != nil {
		t.Fatalf("写入 .ehdata 失败: %v", err)
	}
	meta := ParseDirMetadata(dir)
	if meta.GID != "42" {
		t.Errorf("数字 gid 应转为字符串 \"42\"，实际 %q", meta.GID)
	}
	if meta.Token != "num-token" {
		t.Errorf("Token 应为 num-token，实际 %q", meta.Token)
	}
}
