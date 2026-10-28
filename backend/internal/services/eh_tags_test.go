package services

import (
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func newTagDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("解析测试 HTML 失败: %v", err)
	}
	return doc
}

// 真实页面结构（见 DSPlans/plans/round46）：显示文本可带 " | 别名/wiki 提示"，
// 规范 tag 身份在 <a id="ta_ns:key"> / href / onclick 中，必须取 "|" 左侧。
func TestParseGalleryTagsUsesCanonicalKey(t *testing.T) {
	const page = `<html><body><div id="taglist"><table>
<tr><td class="tc">language:</td><td>
<div id="td_language:japanese" class="gt"><a id="ta_language:japanese" href="https://e-hentai.org/tag/language:japanese" onclick="return toggle_tagmenu(1223,'language:japanese',this)">japanese | definition revised please read the wiki</a></div>
</td></tr>
<tr><td class="tc">character:</td><td>
<div id="td_character:cure_answer" class="gt"><a id="ta_character:cure_answer" href="https://e-hentai.org/tag/character:cure+answer" onclick="return toggle_tagmenu(517626,'character:cure answer',this)">cure answer | anna akechi</a></div>
<div id="td_character:cure_mystique" class="gtl"><a id="ta_character:cure_mystique" href="https://e-hentai.org/tag/character:cure+mystique" onclick="return toggle_tagmenu(517761,'character:cure mystique',this)">cure mystique | mikuru kobayashi</a></div>
</td></tr>
<tr><td class="tc">artist:</td><td>
<div id="td_artist:tiler" class="gt"><a id="ta_artist:tiler" href="https://e-hentai.org/tag/artist:tiler" onclick="return toggle_tagmenu(513452,'artist:tiler',this)">tiler</a></div>
<div id="td_artist:ere_2_earo" class="gt"><a id="ta_artist:ere_2_earo" href="https://e-hentai.org/tag/artist:ere+2+earo" onclick="return toggle_tagmenu(1,'artist:ere 2 earo',this)">ere 2 earo | kinokoya</a></div>
</td></tr>
</table></div></body></html>`

	got := parseGalleryTags(newTagDoc(t, page))
	want := []string{
		"language:japanese",
		"character:cure answer",
		"character:cure mystique",
		"artist:tiler",
		"artist:ere 2 earo",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseGalleryTags 结果不符\n got: %#v\nwant: %#v", got, want)
	}
}

// 无 id 的锚点走 href 兜底；无锚点时走 div 文本兜底（仍裁掉 "|" 右侧）。
func TestParseGalleryTagsFallbacks(t *testing.T) {
	const page = `<div id="taglist"><table>
<tr><td class="tc">parody:</td><td>
<a href="https://e-hentai.org/tag/parody:touhou+project">touhou project | 東方Project</a>
</td></tr>
<tr><td class="tc">female:</td><td>
<div class="gt">bunny girl | バニーガール</div>
</td></tr>
<tr><td class="tc">other:</td><td>
<a onclick="return toggle_tagmenu(9,'other:variant set',this)">variant set | 版本差异</a>
</td></tr>
</table></div>`

	got := parseGalleryTags(newTagDoc(t, page))
	want := []string{
		"parody:touhou project",
		"female:bunny girl",
		"other:variant set",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseGalleryTags 兜底结果不符\n got: %#v\nwant: %#v", got, want)
	}
}

// 同一 tag 重复出现只保留一次，且 namespace 归一为小写。
func TestParseGalleryTagsDedupAndLowerNamespace(t *testing.T) {
	const page = `<div id="taglist"><table>
<tr><td class="tc">Female:</td><td>
<div class="gt"><a id="ta_Female:bunny_girl" href="https://e-hentai.org/tag/Female:bunny+girl">bunny girl</a></div>
<div class="gt"><a id="ta_Female:bunny_girl" href="https://e-hentai.org/tag/Female:bunny+girl">bunny girl</a></div>
</td></tr>
</table></div>`

	got := parseGalleryTags(newTagDoc(t, page))
	want := []string{"female:bunny girl"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseGalleryTags 去重/小写结果不符: %#v", got)
	}
}

func TestNormalizeTagKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"language:japanese | definition revised please read the wiki", "language:japanese"},
		{"character:cure answer | anna akechi", "character:cure answer"},
		{"artist:ere_2_earo", "artist:ere 2 earo"},
		{"  Parody:Pokemon  ", "parody:Pokemon"},
		{"parody:pokemon", "parody:pokemon"},
		{"| leading pipe", ""},
		{"no meaning", ""},
		{"language:", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizeTagKey(c.in); got != c.want {
			t.Errorf("normalizeTagKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
