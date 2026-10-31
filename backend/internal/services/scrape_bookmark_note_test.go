package services

import (
	"strings"
	"testing"
)

// Round49：备注长度校验单测（与前端 src/utils/noteLimit.ts 同一规则）
func TestValidateScrapeBookmarkNote(t *testing.T) {
	cases := []struct {
		name    string
		note    string
		wantErr bool
	}{
		{"空串放行", "", false},
		{"单行 19 中文放行", strings.Repeat("字", 19), false},
		{"单行 20 中文超限", strings.Repeat("字", 20), true},
		{"三行放行", "a\nb\nc", false},
		{"四行超限", "a\nb\nc\nd", true},
		// 19 + 19 + 17 = 55 个中文字符（110 半角单位）
		{"三行 55 中文放行", strings.Repeat("字", 19) + "\n" + strings.Repeat("字", 19) + "\n" + strings.Repeat("字", 17), false},
		// 19 + 19 + 18 = 56 个中文字符（112 半角单位）
		{"三行 56 中文超限", strings.Repeat("字", 19) + "\n" + strings.Repeat("字", 19) + "\n" + strings.Repeat("字", 18), true},
	}
	for _, c := range cases {
		err := validateScrapeBookmarkNote(c.note)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v，期望 wantErr=%v", c.name, err, c.wantErr)
		}
	}
}
