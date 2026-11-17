package main

import (
	"net"
	"net/http"
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
// 统一用 net.ParseIP 归一化后的 ip.String() 做分割：
//   - IPv4-mapped（::ffff:1.2.3.4）经 clientIP 归一已是纯 IPv4，不会把映射前缀混进脱敏串
//   - 畸形输入（ParseIP 失败，如 "1.2.3.4:5678:999"）保守降级为 unknown，绝不泄露疑似端口/字段前缀
func maskedIP(remote string) string {
	host := clientIP(remote)
	ip := net.ParseIP(host)
	if ip == nil {
		return "unknown"
	}
	if v4 := ip.To4(); v4 != nil {
		parts := strings.Split(ip.String(), ".")
		return parts[0] + "." + parts[1] + ".*.*"
	}
	// IPv6：保留前两组 + :::*（::1 回环首位组为空，保守降级 unknown，与旧行为一致）
	groups := strings.Split(ip.String(), ":")
	if len(groups) >= 2 && groups[0] != "" {
		return groups[0] + ":" + groups[1] + ":::*"
	}
	return "unknown"
}

// clientIPFromRequest 从请求解析客户端 IP，供限流键与访问日志共用。
//
// 默认只取 RemoteAddr（与 clientIP 一致，直连场景行为不变）。当
// FMC_TRUST_PROXY_HEADERS=true 且连接来自回环地址（本地反代是唯一入口）时，
// 按 CF-Connecting-IP → X-Real-IP → X-Forwarded-For 首值 的顺序取真实客户端 IP，
// 每个候选都校验为合法 IP，非法或缺失即回退下一个；全部无效时回退 RemoteAddr。
//
// 安全边界：非回环来源一律忽略转发头回退 RemoteAddr。直连来源无法让 RemoteAddr
// 成为回环，回环门把「可信反代」与「直连伪造」区分开；转发头本身的可信度由部署层
// 保证（反代用 real_ip 等机制改写并覆盖 CF-Connecting-IP / X-Real-IP），应用层
// 回环门是第二道校验，部署层改写与应用层校验叠加才构成完整防线。
func clientIPFromRequest(r *http.Request) string {
	if !trustProxyHeaders || !isLoopbackHost(r.RemoteAddr) {
		return clientIP(r.RemoteAddr)
	}
	for _, header := range []string{"CF-Connecting-IP", "X-Real-IP"} {
		if v := strings.TrimSpace(r.Header.Get(header)); v != "" {
			if ip := net.ParseIP(v); ip != nil {
				return ip.String()
			}
		}
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := strings.TrimSpace(strings.Split(xff, ",")[0])
		if ip := net.ParseIP(first); ip != nil {
			return ip.String()
		}
	}
	return clientIP(r.RemoteAddr)
}

// isLoopbackHost 判断 RemoteAddr 解析出的 host 是否为回环地址（127.0.0.0/8 或 ::1）。
// 回环意味着连接来自本机（Nginx 等反代转发），是信任转发头的先决条件。
func isLoopbackHost(remote string) bool {
	ip := net.ParseIP(clientIP(remote))
	return ip != nil && ip.IsLoopback()
}
