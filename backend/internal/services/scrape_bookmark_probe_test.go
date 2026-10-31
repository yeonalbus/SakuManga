package services

import (
	"encoding/json"
	"testing"
)

// Round49：书签更新探测判据单测（首条发布时间 vs 锚定时间）
func TestPostedAfter(t *testing.T) {
	cases := []struct {
		name         string
		head, anchor string
		want         bool
	}{
		{"首条更晚 → 有更新", "2026-10-30 09:00", "2026-10-29 12:00", true},
		{"时间相等 → 不报", "2026-10-29 12:00", "2026-10-29 12:00", false},
		{"首条更早 → 不报", "2026-10-28 12:00", "2026-10-29 12:00", false},
		{"首条缺失 → 不报", "", "2026-10-29 12:00", false},
		{"锚点缺失 → 不报", "2026-10-30 09:00", "", false},
		{"无法解析 → 不报", "未知时间", "2026-10-29 12:00", false},
	}
	for _, c := range cases {
		if got := postedAfter(c.head, c.anchor); got != c.want {
			t.Errorf("%s: postedAfter(%q, %q) = %v，期望 %v", c.name, c.head, c.anchor, got, c.want)
		}
	}
}

// Round49：迁移期从旧名字提取 role / 已囤货
func TestInferBookmarkRoleFromName(t *testing.T) {
	cases := []struct {
		name     string
		wantRole string
		wantBulk bool
	}{
		{"首页", "interest", false},
		{"artist:煌野一人 最新", "newest", false},
		{"artist:煌野一人 最旧", "oldest", false},
		{"已下载完", "newest", true},
		{"artist:xxx 最新 已下载完", "newest", true},
		{"删除页 最旧", "oldest", false},
		{"  最新  ", "newest", false},
	}
	for _, c := range cases {
		role, bulk := inferBookmarkRoleFromName(c.name)
		if role != c.wantRole || bulk != c.wantBulk {
			t.Errorf("inferBookmarkRoleFromName(%q) = (%s, %v)，期望 (%s, %v)",
				c.name, role, bulk, c.wantRole, c.wantBulk)
		}
	}
}

// Round49：迁移清理 anchor.invalid，其余键原样保留
func TestStripAnchorInvalid(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"null 原样", "null", "null"},
		{"空串原样", "", ""},
		{"无 invalid 原样", `{"gid":"123","postedAt":"2026-10-01 10:00"}`, `{"gid":"123","postedAt":"2026-10-01 10:00"}`},
		{"非法 JSON 原样", `{bad json`, `{bad json`},
	}
	for _, c := range cases {
		if got := stripAnchorInvalid(c.in); got != c.want {
			t.Errorf("%s: stripAnchorInvalid(%q) = %q，期望 %q", c.name, c.in, got, c.want)
		}
	}

	// 含 invalid 时应被删除（键序不固定，用重新解析做断言）
	cleaned := stripAnchorInvalid(`{"gid":"123","invalid":{"kind":"unreachable","at":1},"postedAt":"2026-10-01 10:00"}`)
	if cleaned == "" || cleaned == "null" {
		t.Fatalf("清理 invalid 后不应为空: %q", cleaned)
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(cleaned), &probe); err != nil {
		t.Fatalf("清理结果不是合法 JSON: %v", err)
	}
	if _, ok := probe["invalid"]; ok {
		t.Errorf("invalid 键应被删除: %v", probe)
	}
	if probe["gid"] != "123" {
		t.Errorf("其余键应保留，gid = %v", probe["gid"])
	}
}
