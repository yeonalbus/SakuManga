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
