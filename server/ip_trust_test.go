package main

import (
	"net/http/httptest"
	"testing"
)

// clientIPFromRequest 的信任边界：
//   - FMC_TRUST_PROXY_HEADERS=true 且 RemoteAddr 为回环地址（反代唯一入口）时，
//     按 CF-Connecting-IP → X-Real-IP → X-Forwarded-For 首值 的顺序取真实客户端 IP；
//   - 未开启或非回环来源一律忽略转发头，回退 RemoteAddr（防直连伪造绕过限流）。
//
// 本用例锁定这条边界：伪造头在「未开启」或「非回环」时必须无效。
// 「默认不信任」用例用回环 RemoteAddr，才能真正锁「开关关闭」这一条独立路径——
// 用非回环 RemoteAddr 会先被回环门拦下，测不到开关本身。
func TestClientIPFromRequestDefaultNoTrust(t *testing.T) {
	// 默认不信任反代头：即使带了 CF-Connecting-IP / XFF，也应回退 RemoteAddr。
	cases := []struct {
		name   string
		remote string
		cf     string
		xff    string
		want   string
	}{
		{"无任何头", "1.2.3.4:5678", "", "", "1.2.3.4"},
		{"回环 + 带 CF 头但开关关闭", "127.0.0.1:3078", "9.9.9.9", "", "127.0.0.1"},
		{"回环 + 带 XFF 但开关关闭", "127.0.0.1:3078", "", "9.9.9.9", "127.0.0.1"},
		{"回环 + 带 X-Real-IP 但开关关闭", "127.0.0.1:3078", "", "", "127.0.0.1"},
		{"非回环 RemoteAddr 与伪造 CF 头", "203.0.113.7:5678", "9.9.9.9", "", "203.0.113.7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prev := trustProxyHeaders
			trustProxyHeaders = false
			defer func() { trustProxyHeaders = prev }()
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = c.remote
			if c.cf != "" {
				req.Header.Set("CF-Connecting-IP", c.cf)
			}
			if c.xff != "" {
				req.Header.Set("X-Forwarded-For", c.xff)
			}
			if got := clientIPFromRequest(req); got != c.want {
				t.Errorf("clientIPFromRequest = %q，期望 %q", got, c.want)
			}
		})
	}
}

func TestClientIPFromRequestTrustLoopback(t *testing.T) {
	prev := trustProxyHeaders
	trustProxyHeaders = true
	defer func() { trustProxyHeaders = prev }()

	cases := []struct {
		name   string
		remote string
		cf     string
		real   string
		xff    string
		want   string
	}{
		{"回环 + CF-Connecting-IP 优先", "127.0.0.1:3078", "59.56.40.4", "1.1.1.1", "", "59.56.40.4"},
		{"回环 + X-Real-IP（Nginx 反代配置）", "127.0.0.1:3078", "", "59.56.40.4", "", "59.56.40.4"},
		{"回环 + XFF 首值", "127.0.0.1:3078", "", "", "59.56.40.4, 10.0.0.1", "59.56.40.4"},
		{"回环 + 无任何转发头回退", "127.0.0.1:3078", "", "", "", "127.0.0.1"},
		{"回环 + 非法 CF 值回退 XFF", "127.0.0.1:3078", "not-an-ip", "", "59.56.40.4", "59.56.40.4"},
		{"IPv6 回环 + CF 头", "[::1]:3078", "2001:db8::1", "", "", "2001:db8::1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = c.remote
			if c.cf != "" {
				req.Header.Set("CF-Connecting-IP", c.cf)
			}
			if c.real != "" {
				req.Header.Set("X-Real-IP", c.real)
			}
			if c.xff != "" {
				req.Header.Set("X-Forwarded-For", c.xff)
			}
			if got := clientIPFromRequest(req); got != c.want {
				t.Errorf("clientIPFromRequest = %q，期望 %q", got, c.want)
			}
		})
	}
}

// 关键安全用例：开启信任后，非回环来源（直连攻击者场景）的伪造头必须被忽略。
func TestClientIPFromRequestNonLoopbackIgnoresForgedHeaders(t *testing.T) {
	prev := trustProxyHeaders
	trustProxyHeaders = true
	defer func() { trustProxyHeaders = prev }()

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.7:5678" // 直连攻击者，不在回环
	req.Header.Set("CF-Connecting-IP", "9.9.9.9")
	req.Header.Set("X-Forwarded-For", "123.123.123.123")
	if got := clientIPFromRequest(req); got != "203.0.113.7" {
		t.Errorf("直连场景伪造头应被忽略，实际 %q", got)
	}
}
