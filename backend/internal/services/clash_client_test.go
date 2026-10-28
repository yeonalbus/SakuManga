package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newFakeClash 构造一个最小可用的 mihomo 控制面假服务。
func newFakeClash(t *testing.T, secret string) (*httptest.Server, *[]string) {
	t.Helper()
	var switched []string

	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r, secret) {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"version": "v1.19.33", "meta": true})
	})
	mux.HandleFunc("/proxies", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r, secret) {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"proxies": map[string]interface{}{
				"Ehentai": map[string]interface{}{
					"type": "Selector", "now": "HK 01",
					"all": []string{"HK 01", "HK 02", "JP 01"},
				},
				"Auto": map[string]interface{}{
					"type": "URLTest", "now": "JP 01",
					"all": []string{"JP 01"},
				},
				"GLOBAL": map[string]interface{}{
					"type": "Selector", "now": "DIRECT",
					"all": []string{"DIRECT"},
				},
				"Leaf": map[string]interface{}{"type": "Shadowsocks"},
			},
		})
	})
	mux.HandleFunc("/proxies/", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r, secret) {
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/proxies/")
		if r.Method == http.MethodPut {
			var body struct {
				Name string `json:"name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			switch name {
			case "Ehentai":
				switched = append(switched, body.Name)
				w.WriteHeader(http.StatusNoContent)
			case "Auto":
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"message":"Must be a Selector"}`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
			return
		}
		switch name {
		case "Ehentai":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"type": "Selector", "now": "HK 01", "all": []string{"HK 01", "HK 02", "JP 01"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("/group/", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r, secret) {
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/delay") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]int{"HK 01": 280, "HK 02": 300, "JP 01": 350})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, &switched
}

func checkAuth(w http.ResponseWriter, r *http.Request, secret string) bool {
	if secret == "" {
		return true
	}
	if r.Header.Get("Authorization") != "Bearer "+secret {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}

func TestClashClientVersionAndAuth(t *testing.T) {
	server, _ := newFakeClash(t, "s3cret")
	// 去掉 http:// 前缀也应能工作（NormalizeClashController 自动补全）
	client := NewClashClient(strings.TrimPrefix(server.URL, "http://"), "s3cret")

	version, meta, err := client.Version(context.Background())
	if err != nil {
		t.Fatalf("Version 失败: %v", err)
	}
	if version != "v1.19.33" || !meta {
		t.Fatalf("版本解析错误: version=%q meta=%v", version, meta)
	}
}

func TestClashClientUnauthorized(t *testing.T) {
	server, _ := newFakeClash(t, "s3cret")
	client := NewClashClient(server.URL, "wrong")

	_, _, err := client.Version(context.Background())
	if !errors.Is(err, ErrClashUnauthorized) {
		t.Fatalf("期望 ErrClashUnauthorized，实际: %v", err)
	}
}

func TestClashClientGroupAndMissing(t *testing.T) {
	server, _ := newFakeClash(t, "")
	client := NewClashClient(server.URL, "")

	group, err := client.Group(context.Background(), "Ehentai")
	if err != nil {
		t.Fatalf("Group 失败: %v", err)
	}
	if group.Type != "Selector" || group.Now != "HK 01" || len(group.All) != 3 {
		t.Fatalf("组信息解析错误: %+v", group)
	}

	if _, err := client.Group(context.Background(), "NotExist"); !errors.Is(err, ErrClashGroupNotFound) {
		t.Fatalf("期望 ErrClashGroupNotFound，实际: %v", err)
	}
}

func TestClashClientListGroupsFiltersTypes(t *testing.T) {
	server, _ := newFakeClash(t, "")
	client := NewClashClient(server.URL, "")

	groups, err := client.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("ListGroups 失败: %v", err)
	}
	names := map[string]bool{}
	for _, g := range groups {
		names[g.Name] = true
	}
	if !names["Ehentai"] || !names["Auto"] || !names["GLOBAL"] {
		t.Fatalf("缺少期望的组: %v", names)
	}
	if names["Leaf"] {
		t.Fatalf("普通节点不应出现在策略组列表里")
	}
	// GLOBAL 必须排在最后
	if len(groups) > 0 && groups[len(groups)-1].Name != "GLOBAL" {
		t.Fatalf("GLOBAL 应排在最后，实际顺序: %v", groups)
	}
}

func TestClashClientGroupDelay(t *testing.T) {
	server, _ := newFakeClash(t, "")
	client := NewClashClient(server.URL, "")

	delayMap, err := client.GroupDelay(context.Background(), "Ehentai", "https://e-hentai.org/", 5000)
	if err != nil {
		t.Fatalf("GroupDelay 失败: %v", err)
	}
	if len(delayMap) != 3 || delayMap["HK 01"] != 280 {
		t.Fatalf("延迟解析错误: %v", delayMap)
	}
}

func TestClashClientSetNode(t *testing.T) {
	server, switched := newFakeClash(t, "")
	client := NewClashClient(server.URL, "")

	if err := client.SetNode(context.Background(), "Ehentai", "JP 01"); err != nil {
		t.Fatalf("SetNode 失败: %v", err)
	}
	if len(*switched) != 1 || (*switched)[0] != "JP 01" {
		t.Fatalf("切换记录错误: %v", *switched)
	}

	// 自动组返回 400 → 明确报错
	if err := client.SetNode(context.Background(), "Auto", "JP 01"); err == nil {
		t.Fatalf("自动选择组切换应失败")
	}

	// GLOBAL 保护
	if err := client.SetNode(context.Background(), "GLOBAL", "DIRECT"); !errors.Is(err, ErrClashGlobalForbidden) {
		t.Fatalf("期望 ErrClashGlobalForbidden，实际: %v", err)
	}
}

func TestTopNodesByDelay(t *testing.T) {
	delayMap := map[string]int{"A": 300, "B": 100, "C": 200}
	got := topNodesByDelay(delayMap, 2, "")
	if len(got) != 2 || got[0] != "B" || got[1] != "C" {
		t.Fatalf("排序/截断错误: %v", got)
	}

	// 排除当前节点
	got = topNodesByDelay(delayMap, 3, "B")
	if len(got) != 2 || got[0] != "C" {
		t.Fatalf("排除当前节点错误: %v", got)
	}
}

func TestIsArchiveNetworkFailure(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"请求 archiver.php 失败: Get \"https://exhentai.org/archiver.php\": EOF", true},
		{"请求 archiver.php 失败: context deadline exceeded", true},
		{"dial tcp: connection reset by peer", true},
		{"archiver.php 返回状态码 404", false},
		{"创建归档失败 HTTP 403", false},
		{"页面结构异常（未解析到归档表单）", false},
	}
	for _, c := range cases {
		got := isArchiveNetworkFailure(errors.New(c.msg))
		if got != c.want {
			t.Errorf("isArchiveNetworkFailure(%q)=%v，期望 %v", c.msg, got, c.want)
		}
	}
	if isArchiveNetworkFailure(nil) {
		t.Error("nil 不应判为网络失败")
	}
}
