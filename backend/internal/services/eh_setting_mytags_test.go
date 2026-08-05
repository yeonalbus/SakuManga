package services

import (
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
