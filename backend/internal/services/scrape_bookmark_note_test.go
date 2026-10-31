package services

import (
	"strings"
	"testing"
)

// Round49：备注长度校验单测（与前端 src/utils/noteLimit.ts 同一规则）
//
// 规则：总宽 ≤ 110 半角单位；显示行数 ≤ 3（每行 38 半角折算，含自动折行）。
func TestValidateScrapeBookmarkNote(t *testing.T) {
	cases := []struct {
		name    string
		note    string
		wantErr bool
	}{
		{"空串放行", "", false},
		// 38 宽 → 1 行
		{"单行 19 中文（1 行）", strings.Repeat("字", 19), false},
		// 40 宽 → ceil(40/38)=2 行 ≤ 3
		{"单行 20 中文（折 2 行）", strings.Repeat("字", 20), false},
		// 110 宽 → ceil(110/38)=3 行，宽度恰好到上限
		{"单行 55 中文（3 行）", strings.Repeat("字", 55), false},
		// 112 宽 > 110 → 超宽
		{"单行 56 中文超宽", strings.Repeat("字", 56), true},
		// 两段各 20 中文：2+2=4 行 → 拒绝
		{"两段各 20 中文（折 4 行）", strings.Repeat("字", 20) + "\n" + strings.Repeat("字", 20), true},
		// 三段 19+19+17 = 55 中文 = 110 宽，各占 1 行 → 放行
		{"三段 19+19+17（3 行 55 字）",
			strings.Repeat("字", 19) + "\n" + strings.Repeat("字", 19) + "\n" + strings.Repeat("字", 17), false},
		// 四段各 1 字 → 4 行 → 拒绝
		{"四段各 1 字", "a\nb\nc\nd", true},
		{"三行放行", "a\nb\nc", false},
	}
	for _, c := range cases {
		err := validateScrapeBookmarkNote(c.note)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v，期望 wantErr=%v", c.name, err, c.wantErr)
		}
	}
}
