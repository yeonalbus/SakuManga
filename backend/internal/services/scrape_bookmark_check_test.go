package services

import (
	"errors"
	"testing"
)

// ─────────────────────────────────────────────────────────────
// Round35：搜刮书签失效判定（语义 = 书签级可见性）单测
//
// 语义：书签在「自己的搜索&筛选条件」下能否看到锚定画廊 = 有效/失效；
// 判据 = 按 config 复刻检索 + 动态越过判定（本页最旧一条已早于锚定时间即停止）。
// ─────────────────────────────────────────────────────────────

func bookmarkTestComics(pairs ...[2]string) []OnlineComicDTO {
	out := make([]OnlineComicDTO, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, OnlineComicDTO{ID: p[0], Token: "tok" + p[0], Title: "标题" + p[0], UpdatedAt: p[1]})
	}
	return out
}

// 记录每次请求参数的假 fetcher
type fakeFetcher struct {
	pages []*OnlineComicResult
	errs  []error
	calls []SearchParams
}

func (f *fakeFetcher) fetch(params SearchParams) (*OnlineComicResult, error) {
	f.calls = append(f.calls, params)
	i := len(f.calls) - 1
	if i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	if i >= len(f.pages) {
		// 超出预设：返回同一页（用于硬上限用例）
		if len(f.pages) == 0 {
			return &OnlineComicResult{}, nil
		}
		return f.pages[len(f.pages)-1], nil
	}
	return f.pages[i], nil
}

const testTarget = "2026-09-12 13:05"

// 命中：首屏未越过 → 第二页命中 → ok；且首屏带 seek、后续页带 next
func TestScanBookmarkVisibilityHitOnSecondPage(t *testing.T) {
	f := &fakeFetcher{pages: []*OnlineComicResult{
		{Comics: bookmarkTestComics([2]string{"4001", "2026-09-12 23:59"}, [2]string{"4002", "2026-09-12 20:00"}), Next: "4002"},
		{Comics: bookmarkTestComics([2]string{"4184523", "2026-09-12 13:05"}, [2]string{"4004", "2026-09-12 10:00"}), Next: "4004"},
	}}
	status, msg, refreshed := scanBookmarkVisibility(f.fetch, bookmarkSearchConfig{}, "4184523", testTarget)
	if status != BookmarkStatusOK {
		t.Fatalf("status=%s msg=%s，期望 ok", status, msg)
	}
	if refreshed == nil || refreshed.Title != "标题4184523" || refreshed.PostedAt != "2026-09-12 13:05" {
		t.Fatalf("refreshed 不符：%+v", refreshed)
	}
	if len(f.calls) != 2 {
		t.Fatalf("请求次数=%d，期望 2", len(f.calls))
	}
	if f.calls[0].Seek != "2026-09-12" || f.calls[0].Next != "" {
		t.Fatalf("首屏应带 seek 不带 next：%+v", f.calls[0])
	}
	if f.calls[1].Next != "4002" || f.calls[1].Seek != "" {
		t.Fatalf("第二页应带 next 不带 seek：%+v", f.calls[1])
	}
}

// 动态停止：首屏最旧一条已早于锚定时间 → 立即判失效（不依赖固定页数上限）
func TestScanBookmarkVisibilityStopsOnPassedTarget(t *testing.T) {
	f := &fakeFetcher{pages: []*OnlineComicResult{
		{Comics: bookmarkTestComics([2]string{"4001", "2026-09-12 23:00"}, [2]string{"4002", "2026-09-12 10:00"}), Next: "4002"},
	}}
	status, msg, _ := scanBookmarkVisibility(f.fetch, bookmarkSearchConfig{}, "4184523", testTarget)
	if status != BookmarkStatusUnreachable {
		t.Fatalf("status=%s msg=%s，期望 unreachable", status, msg)
	}
	if len(f.calls) != 1 {
		t.Fatalf("请求次数=%d，期望 1（越过即停）", len(f.calls))
	}
}

// 边界：锚定画廊恰好是「本页最旧一条」且时间等于锚定时间 → 仍应命中（先查命中，再判越过）
func TestScanBookmarkVisibilityHitOnBoundaryOldest(t *testing.T) {
	f := &fakeFetcher{pages: []*OnlineComicResult{
		{Comics: bookmarkTestComics([2]string{"4001", "2026-09-12 23:00"}, [2]string{"4184523", "2026-09-12 13:05"}), Next: "4184523"},
	}}
	status, _, _ := scanBookmarkVisibility(f.fetch, bookmarkSearchConfig{}, "4184523", testTarget)
	if status != BookmarkStatusOK {
		t.Fatalf("status=%s，期望 ok（边界命中最旧一条）", status)
	}
}

// 列表到底（无 next）且未越过 → 失效
func TestScanBookmarkVisibilityExhausted(t *testing.T) {
	f := &fakeFetcher{pages: []*OnlineComicResult{
		{Comics: bookmarkTestComics([2]string{"4001", "2026-09-12 23:00"}, [2]string{"4002", "2026-09-12 18:00"}), Next: ""},
	}}
	status, msg, _ := scanBookmarkVisibility(f.fetch, bookmarkSearchConfig{}, "4184523", testTarget)
	if status != BookmarkStatusUnreachable {
		t.Fatalf("status=%s msg=%s，期望 unreachable（列表到尽头）", status, msg)
	}
}

// 空结果页 → 失效
func TestScanBookmarkVisibilityEmptyPage(t *testing.T) {
	f := &fakeFetcher{pages: []*OnlineComicResult{{Comics: nil, Next: ""}}}
	status, _, _ := scanBookmarkVisibility(f.fetch, bookmarkSearchConfig{}, "4184523", testTarget)
	if status != BookmarkStatusUnreachable {
		t.Fatalf("status=%s，期望 unreachable（结果为空）", status)
	}
}

// 时间戳无法解析 → 不判越过，继续翻页；后续页越过 → 失效（不误判、不早停）
func TestScanBookmarkVisibilityUnparsableTimeKeepsScanning(t *testing.T) {
	f := &fakeFetcher{pages: []*OnlineComicResult{
		{Comics: bookmarkTestComics([2]string{"4001", "未知时间"}), Next: "4001"},
		{Comics: bookmarkTestComics([2]string{"4002", "2026-09-12 09:00"}), Next: "4002"},
	}}
	status, msg, _ := scanBookmarkVisibility(f.fetch, bookmarkSearchConfig{}, "4184523", testTarget)
	if status != BookmarkStatusUnreachable {
		t.Fatalf("status=%s msg=%s，期望 unreachable（第二页越过）", status, msg)
	}
	if len(f.calls) != 2 {
		t.Fatalf("请求次数=%d，期望 2（时间不可解析时继续扫）", len(f.calls))
	}
}

// 密集日：始终未越过 → 撞硬上限 → 未判定（error，不写失效标记）
func TestScanBookmarkVisibilityHardCapNotJudged(t *testing.T) {
	dense := &OnlineComicResult{Comics: bookmarkTestComics([2]string{"4001", "2026-09-12 23:59"}), Next: "4001"}
	f := &fakeFetcher{pages: []*OnlineComicResult{dense}}
	status, msg, _ := scanBookmarkVisibility(f.fetch, bookmarkSearchConfig{}, "4184523", testTarget)
	if status != BookmarkStatusError {
		t.Fatalf("status=%s msg=%s，期望 error（超硬上限）", status, msg)
	}
	if len(f.calls) != bookmarkScanHardCapPages {
		t.Fatalf("请求次数=%d，期望 %d", len(f.calls), bookmarkScanHardCapPages)
	}
	// 首屏 seek、后续全为 next 游标
	if f.calls[0].Seek != "2026-09-12" {
		t.Fatalf("首屏 seek 缺失：%+v", f.calls[0])
	}
	for i := 1; i < len(f.calls); i++ {
		if f.calls[i].Next != "4001" || f.calls[i].Seek != "" {
			t.Fatalf("第 %d 页参数不符：%+v", i+1, f.calls[i])
		}
	}
}

// 检索失败（网络/限流）→ 未判定，绝不写失效
func TestScanBookmarkVisibilityFetchError(t *testing.T) {
	f := &fakeFetcher{errs: []error{errors.New("E 站触发限流")}}
	status, msg, _ := scanBookmarkVisibility(f.fetch, bookmarkSearchConfig{}, "4184523", testTarget)
	if status != BookmarkStatusError {
		t.Fatalf("status=%s msg=%s，期望 error", status, msg)
	}
}

// 无 postedAt（老书签）→ 未判定
func TestScanBookmarkVisibilityMissingPostedAt(t *testing.T) {
	f := &fakeFetcher{}
	status, _, _ := scanBookmarkVisibility(f.fetch, bookmarkSearchConfig{}, "4184523", "")
	if status != BookmarkStatusError {
		t.Fatalf("status=%s，期望 error（缺锚定时间）", status)
	}
	if len(f.calls) != 0 {
		t.Fatalf("缺锚定时间不应发起检索，实际 %d 次", len(f.calls))
	}
}

// 关键回归：Expunged 画廊在 onlyRemoved=false（原条件）下不可见 → 失效；
// 同一 gid 在 onlyRemoved=true 的书签下可见 → 有效（书签级判定，与画廊状态无关）
func TestScanBookmarkVisibilityIsPerBookmarkNotPerGallery(t *testing.T) {
	gid := "4186957"
	// 普通条件：该条件下结果里没有它，且首屏已越过锚定时间
	plain := &fakeFetcher{pages: []*OnlineComicResult{
		{Comics: bookmarkTestComics([2]string{"5001", "2026-09-13 08:00"}, [2]string{"5002", "2026-09-13 03:00"}), Next: "5002"},
	}}
	statusPlain, _, _ := scanBookmarkVisibility(plain.fetch, bookmarkSearchConfig{OnlyRemoved: false}, gid, "2026-09-13 05:09")
	if statusPlain != BookmarkStatusUnreachable {
		t.Fatalf("普通条件 status=%s，期望 unreachable", statusPlain)
	}
	if plain.calls[0].OnlyRemoved {
		t.Fatalf("普通条件的请求不应带 onlyRemoved：%+v", plain.calls[0])
	}

	// 仅搜索移除了的画廊：首屏即命中
	removed := &fakeFetcher{pages: []*OnlineComicResult{
		{Comics: bookmarkTestComics([2]string{"5001", "2026-09-13 05:17"}, [2]string{gid, "2026-09-13 05:09"}), Next: "x"},
	}}
	statusRemoved, _, _ := scanBookmarkVisibility(removed.fetch, bookmarkSearchConfig{OnlyRemoved: true}, gid, "2026-09-13 05:09")
	if statusRemoved != BookmarkStatusOK {
		t.Fatalf("onlyRemoved 条件 status=%s，期望 ok", statusRemoved)
	}
	if !removed.calls[0].OnlyRemoved {
		t.Fatalf("onlyRemoved 书签的请求应带 onlyRemoved：%+v", removed.calls[0])
	}
}

// config → 检索参数：复刻前端 buildOnlineSearchParams
func TestBookmarkSearchConfigToSearchParams(t *testing.T) {
	cfg := bookmarkSearchConfig{
		Keyword:               "  我是顶栏词  ",
		Keywords:              []string{"artist:okuma$", "- female:yuri", "  ", "-3d", "language:chinese"},
		ActiveCategories:      []string{"Doujinshi", "Manga"},
		MinRating:             4.5,
		Language:              "Chinese",
		OnlyRemoved:           true,
		OnlyTorrents:          true,
		DisableLangFilter:     true,
		DisableUploaderFilter: true,
		DisableTagFilter:      true,
	}
	p := cfg.toSearchParams()
	want := "我是顶栏词 artist:okuma$ language:chinese"
	if p.Keyword != want {
		t.Fatalf("keyword=%q，期望 %q（负向项不下发）", p.Keyword, want)
	}
	if p.MinRating != "4.5" {
		t.Fatalf("minRating=%q，期望 4.5", p.MinRating)
	}
	if !p.OnlyRemoved || !p.OnlyTorrents || !p.DisableLangFilter || !p.DisableUploaderFilter || !p.DisableTagFilter {
		t.Fatalf("开关未透传：%+v", p)
	}
	if p.Language != "Chinese" {
		t.Fatalf("language=%q", p.Language)
	}
	if len(p.ActiveCategories) != 2 {
		t.Fatalf("分类未透传：%+v", p.ActiveCategories)
	}

	// 整数评分格式化：4 → "4"（与前端 String(4) 一致）
	intCfg := bookmarkSearchConfig{MinRating: 4}
	if got := intCfg.toSearchParams().MinRating; got != "4" {
		t.Fatalf("整数评分 minRating=%q，期望 4", got)
	}
	// 评分 0 → 不下发
	if got := (bookmarkSearchConfig{}).toSearchParams().MinRating; got != "" {
		t.Fatalf("0 分不应下发 f_srdd，实际 %q", got)
	}
}

// seek 日期取法（E 站 seek 只吃日粒度）
func TestSeekDateOfBookmark(t *testing.T) {
	cases := map[string]string{
		"2026-09-13 05:09":  "2026-09-13",
		"2026-09-13":        "2026-09-13",
		" 2026-09-13 05:09": "2026-09-13",
		"":                  "",
		"未知":                "",
	}
	for in, want := range cases {
		if got := seekDateOf(in); got != want {
			t.Fatalf("seekDateOf(%q)=%q，期望 %q", in, got, want)
		}
	}
}
