package services

import "testing"

// TestNormalizeProxyServer 覆盖 Windows 注册表 ProxyServer 的常见格式
func TestNormalizeProxyServer(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"127.0.0.1:7897", "http://127.0.0.1:7897"},
		{"http=127.0.0.1:7897;https=127.0.0.1:7897", "http://127.0.0.1:7897"},
		{"http=127.0.0.1:7890;https=127.0.0.1:7891", "http://127.0.0.1:7890"},
		{"socks=127.0.0.1:7890", "socks5://127.0.0.1:7890"},
		{"https=127.0.0.1:7891;socks=127.0.0.1:7890", "http://127.0.0.1:7891"},
		{"http://127.0.0.1:7897", "http://127.0.0.1:7897"},
		{"socks5://127.0.0.1:7890", "socks5://127.0.0.1:7890"},
		{"", ""},
		{"  ", ""},
		{"http=127.0.0.1:7890;invalid-entry", "http://127.0.0.1:7890"},
	}
	for _, c := range cases {
		if got := normalizeProxyServer(c.in); got != c.want {
			t.Errorf("normalizeProxyServer(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestGetEffectiveProxySource 验证来源判定（不依赖真实系统代理，直接断言三种来源）
func TestGetEffectiveProxySource(t *testing.T) {
	// manual 优先于 system/none
	proxyLock.Lock()
	old := currentProxy
	currentProxy = "http://127.0.0.1:9999"
	proxyLock.Unlock()
	defer func() {
		proxyLock.Lock()
		currentProxy = old
		proxyLock.Unlock()
	}()

	if src := GetProxySource(); src != "manual" {
		t.Errorf("手动配置时 GetProxySource() = %q, want manual", src)
	}
	if got := GetEffectiveProxy(); got != "http://127.0.0.1:9999" {
		t.Errorf("手动配置时 GetEffectiveProxy() = %q, want 手动地址", got)
	}

	// 清空手动配置 → 回退系统代理（本机可能开启，仅断言回退行为不 panic）
	proxyLock.Lock()
	currentProxy = ""
	proxyLock.Unlock()
	if src := GetProxySource(); src != "system" && src != "none" {
		t.Errorf("未配置时 GetProxySource() = %q, want system|none", src)
	}
}
