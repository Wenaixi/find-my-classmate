package main

import "testing"

// Step C：客户端 IP 解析统一为 ip.go 的单一实现。
// maskedIP 必须消费 clientIP 的输出：同一 RemoteAddr 在日志脱敏与限流两侧得到一致的 host。
func TestClientIP(t *testing.T) {
	cases := []struct{ remote, want string }{
		{"1.2.3.4:5678", "1.2.3.4"},           // IPv4 带端口
		{"1.2.3.4", "1.2.3.4"},                // IPv4 裸地址
		{"[2001:db8::1]:8080", "2001:db8::1"}, // IPv6 带端口
		{"[2001:db8::1]", "2001:db8::1"},      // IPv6 带方括号无端口
		{"2001:db8::1%eth0", "2001:db8::1"},   // IPv6 带 zone（去 %zone）
		{"", ""},                              // 空输入
	}
	for _, c := range cases {
		if got := clientIP(c.remote); got != c.want {
			t.Errorf("clientIP(%q) = %q，期望 %q", c.remote, got, c.want)
		}
	}
}

func TestMaskedIPConsumesClientIP(t *testing.T) {
	cases := []struct{ remote, want string }{
		{"1.2.3.4:5678", "1.2.*.*"},
		{"[2001:db8::1]:8080", "2001:db8:::*"}, // IPv6 从"unknown"改善为保留前两组
	}
	for _, c := range cases {
		if got := maskedIP(c.remote); got != c.want {
			t.Errorf("maskedIP(%q) = %q，期望 %q", c.remote, got, c.want)
		}
	}
}
