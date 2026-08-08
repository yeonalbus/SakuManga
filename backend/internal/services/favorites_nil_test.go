package services

import (
	"encoding/json"
	"testing"
)

// 回归验证：在线列表搜索无结果（如 "group:da hootch"）时，
// Attach* 必须返回非 nil 空切片，避免 JSON 序列化为 null，
// 否则前端 filteredComics 对 comics 直接 .filter 会抛
// "Cannot read properties of null (reading 'filter')" 导致渲染区崩溃。
func TestAttachFavoriteStates_NilInput_ReturnsNonNil(t *testing.T) {
	out := AttachFavoriteStates(nil, 0, nil)
	if out == nil {
		t.Fatal("AttachFavoriteStates(nil) 返回了 nil 切片，JSON 会序列化为 null")
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if string(b) != "[]" {
		t.Fatalf("期望 JSON []，实际 %s", string(b))
	}
}

func TestAttachDownloadStates_NilInput_ReturnsNonNil(t *testing.T) {
	out := AttachDownloadStates(nil, nil)
	if out == nil {
		t.Fatal("AttachDownloadStates(nil) 返回了 nil 切片，JSON 会序列化为 null")
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if string(b) != "[]" {
		t.Fatalf("期望 JSON []，实际 %s", string(b))
	}
}
