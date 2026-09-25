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

// PR 审查发现：IPv4-mapped 地址（::ffff:1.2.3.4，后端 IPv6-mapped RemoteAddr 可能出现）
// 旧实现用原始 host 串做 split，把"::ffff:"映射前缀混进脱敏串（::ffff:1.2.*.*）。
// 修复：ParseIP 成功后一律用 ip.String() 归一再脱敏——映射地址归一为纯 IPv4。
func TestMaskedIPNormalizesIPv4Mapped(t *testing.T) {
	cases := []struct{ remote, want string }{
		{"::ffff:192.168.1.1", "192.168.*.*"},
		{"[::ffff:192.168.1.1]:8080", "192.168.*.*"},
	}
	for _, c := range cases {
		if got := maskedIP(c.remote); got != c.want {
			t.Errorf("maskedIP(%q) = %q，期望 %q", c.remote, got, c.want)
		}
	}
}

// 畸形输入（多冒号含端口字段、ParseIP 失败）：脱敏必须保守降级为 unknown，
// 绝不把任何疑似端口/字段前缀泄露进日志。
func TestMaskedIPMalformedFallback(t *testing.T) {
	cases := []struct{ remote, want string }{
		{"1.2.3.4:5678:999", "unknown"},
		{"999.888.777.666", "unknown"},
	}
	for _, c := range cases {
		if got := maskedIP(c.remote); got != c.want {
			t.Errorf("maskedIP(%q) = %q，期望 %q", c.remote, got, c.want)
		}
	}
}
