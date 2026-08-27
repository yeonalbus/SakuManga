package services

import (
	"reflect"
	"testing"
)

// Round25-Bug1 回归：自然文件名排序。
//
// 旧实现拆块正则 `\d+|\D+` 中 `\D+` 贪婪匹配会吞掉扩展名——
// "img.jpg" 被拆成整块 ["img.jpg"]，与 "img0001.jpg" 的 ["img","0001",".jpg"]
// 比较第 0 块时短前缀 "img" < "img.jpg"，导致 img.jpg 被错误排到 img0029.jpg 之后。
// 修复后 `.` 单独成块（`\d+|[^\d.]+|\.`），".jpg" 与数字块比较时 '.'(46) < '0'(48)，
// 无数字扩展名的文件正确排到数字序列之前。
func TestSortFilenamesNatural(t *testing.T) {
	cases := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name: "Round25-Bug1 复现样本（无数字扩展名排在数字序列前）",
			input: []string{
				"yanxue2_solo.jpg",
				"img0029.jpg", "img.jpg", "img0001.jpg",
				"im.jpg", "i.jpg", "img0028.jpg",
			},
			want: []string{
				"i.jpg", "im.jpg", "img.jpg",
				"img0001.jpg", "img0028.jpg", "img0029.jpg",
				"yanxue2_solo.jpg",
			},
		},
		{
			name:  "数字跨位按数值（2 < 10）",
			input: []string{"page10.jpg", "page2.jpg", "page1.jpg"},
			want:  []string{"page1.jpg", "page2.jpg", "page10.jpg"},
		},
		{
			name:  "中文序数（第10话 > 第2话）",
			input: []string{"第10话.jpg", "第2话.jpg", "第1话.jpg"},
			want:  []string{"第1话.jpg", "第2话.jpg", "第10话.jpg"},
		},
		{
			name:  "版本号点分隔（v1.10 > v1.2）",
			input: []string{"v1.10.jpg", "v1.2.jpg", "v1.1.jpg"},
			want:  []string{"v1.1.jpg", "v1.2.jpg", "v1.10.jpg"},
		},
		{
			name:  "无扩展名短前缀在前（img < img.jpg）",
			input: []string{"img.jpg", "img", "img0001.jpg"},
			want:  []string{"img", "img.jpg", "img0001.jpg"},
		},
		{
			name:  "大小写不敏感（IMG == img）",
			input: []string{"IMG0002.jpg", "img0001.jpg", "IMG0003.jpg"},
			want:  []string{"img0001.jpg", "IMG0002.jpg", "IMG0003.jpg"},
		},
		{
			name:  "混合字母数字（a2b1 < a10b1）",
			input: []string{"a10b1.jpg", "a2b1.jpg"},
			want:  []string{"a2b1.jpg", "a10b1.jpg"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := append([]string(nil), tc.input...)
			sortFilenames(files)
			if !reflect.DeepEqual(files, tc.want) {
				t.Errorf("排序结果错误\n got: %v\nwant: %v", files, tc.want)
			}
		})
	}
}
