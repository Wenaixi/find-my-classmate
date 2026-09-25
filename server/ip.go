package main

import (
	"net"
	"strings"
)

// clientIP 从 RemoteAddr 提取客户端裸 IP（去端口与 zone）。
// 这是客户端 IP 解析的唯一入口：日志脱敏（maskedIP）与限流（rateLimit）必须共用它，
// 保证同一 RemoteAddr 两侧得到一致的 host。反代场景的 XFF 解析属反代层职责（README 已声明），此处不处理。
func clientIP(remote string) string {
	host := remote
	// 方括号 IPv6（带端口）：[addr]:port → addr
	if strings.HasPrefix(host, "[") {
		if end := strings.Index(host, "]"); end >= 0 {
			host = host[1:end]
		}
	} else if i := strings.LastIndex(host, ":"); i >= 0 && strings.Count(host, ":") == 1 {
		// 形如 host:port 的 IPv4：最后一次冒号后是端口
		host = host[:i]
	}
	// 去掉 IPv6 zone（%eth0 / %3）
	if i := strings.Index(host, "%"); i >= 0 {
		host = host[:i]
	}
	// 规范化：IPv6 压缩形式归一（仅当是合法 IP 时）
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return host
}

// maskedIP 脱敏客户端 IP 用于访问日志（隐私红线：只保留前缀，不记录完整地址）。
// IPv4 保留前两段；IPv6 保留前两组（行为改善，原实现将 IPv6 判为 unknown）。
func maskedIP(remote string) string {
	host := clientIP(remote)
	if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
		parts := strings.Split(host, ".")
		return parts[0] + "." + parts[1] + ".*.*"
	}
	// IPv6（或无法解析）：保留前两组 + ::*
	groups := strings.Split(host, ":")
	if len(groups) >= 2 && groups[0] != "" {
		return groups[0] + ":" + groups[1] + ":::*"
	}
	return "unknown"
}
