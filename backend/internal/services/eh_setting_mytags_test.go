package services

import (
	"SakuHentai/internal/models"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// 验证新版 mytags 布局解析，使用用户提供的 testdata_eh/eh_mytags.html 快照。
//
// 快照中共 8 个标签：
//   - Watched：usertag_3437 language:chinese、usertag_491694 other:ai generated
//   - Hidden：usertag_304301 male:males only、usertag_1069 male:yaoi、
//     usertag_63722 female:skinsuit、usertag_6772 other:non-nude、
//     usertag_1722 other:novel、usertag_388669 other:nudity only
//
// 另含 usertag_0（新增输入行）与 usertags_mass（批量操作区），应被跳过。
func TestExtractMyTagsFromSnapshot(t *testing.T) {
	data, err := os.ReadFile("../../../testdata_eh/eh_mytags.html")
	if err != nil {
		t.Fatalf("读取快照失败: %v", err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("解析快照失败: %v", err)
	}

	watched, hidden := extractMyTags(doc)

	wantWatched := []string{"language:chinese", "other:ai generated"}
	if !reflect.DeepEqual(watched, wantWatched) {
		t.Errorf("watched 不匹配\n  got:  %v\n  want: %v", watched, wantWatched)
	}

	wantHidden := []string{
		"male:males only",
		"male:yaoi",
		"female:skinsuit",
		"other:non-nude",
		"other:novel",
		"other:nudity only",
	}
	if !reflect.DeepEqual(hidden, wantHidden) {
		t.Errorf("hidden 不匹配\n  got:  %v\n  want: %v", hidden, wantHidden)
	}
}

// 验证 Tagset 下拉解析：快照中 #tagset_form select 只有一个
// <option value="1" selected>Tagset #1 (8)</option>，当前选中为 1，
// 名称 "Tagset #1"、数量 8。
func TestExtractTagsetsFromSnapshot(t *testing.T) {
	data, err := os.ReadFile("../../../testdata_eh/eh_mytags.html")
	if err != nil {
		t.Fatalf("读取快照失败: %v", err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("解析快照失败: %v", err)
	}

	tagsets, current := extractTagsets(doc)

	want := []models.EHTagset{{ID: 1, Name: "Tagset #1", Count: 8}}
	if !reflect.DeepEqual(tagsets, want) {
		t.Errorf("tagsets 不匹配\n  got:  %v\n  want: %v", tagsets, want)
	}
	if current != 1 {
		t.Errorf("currentTagset 不匹配: got %d, want 1", current)
	}

	// 单独验证文本解析辅助函数
	if name, count := parseTagsetText("Tagset #1 (8)"); name != "Tagset #1" || count != 8 {
		t.Errorf("parseTagsetText 解析失败: (%q, %d)", name, count)
	}
	if name, count := parseTagsetText("Tagset #2"); name != "Tagset #2" || count != 0 {
		t.Errorf("parseTagsetText 无数量时解析失败: (%q, %d)", name, count)
	}
}

// 验证 mytagsURL 的 tagset 参数拼接逻辑（与原站 change_tagset JS 一致：
// tagset<=1 不带参数，tagset>1 追加 ?tagset=N）。
func TestMyTagsURLTagset(t *testing.T) {
	cases := []struct {
		tagset int
		want   string
	}{
		{0, "https://exhentai.org/mytags"},
		{1, "https://exhentai.org/mytags"},
		{2, "https://exhentai.org/mytags?tagset=2"},
		{7, "https://exhentai.org/mytags?tagset=7"},
	}
	for _, c := range cases {
		if got := mytagsURL("https://exhentai.org/", c.tagset); got != c.want {
			t.Errorf("mytagsURL(%d) = %q, want %q", c.tagset, got, c.want)
		}
	}
}
