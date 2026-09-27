package main

import (
	"io"
	"net/http"
	"time"
)

// HTTP 中间件是横切关注点：访问日志与安全响应头对链上每个请求都生效，
// 与「服务怎么启动、怎么装配」无关。此前它们定义在 main.go，改访问日志政策
// 或安全头要去一个声称承担自举/装配的启动文件里找。搬到此处后「每个响应带什么头、
// 记不记访问日志」与「服务怎么装配」各自独立。
//
// 链的顺序不由本文件决定：securityHeaders / rateLimit / accessLog 的相对位置
// 收敛在 main.go 的 newHandlerChainWith 一处，那是链顺序的唯一实现。

// accessLog 记录请求访问日志：方法、路径、状态、耗时、脱敏客户端 IP。
// 隐私红线：不记录查询参数与响应内容，IP 只保留前两段。
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// healthcheck 探针每 30s 一次，不产生访问日志（避免 2880 条/天噪音）
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logInfof("access %s %s %d %s %s",
			r.Method, r.URL.Path, recorder.status, time.Since(started).Round(time.Millisecond), maskedIP(r.RemoteAddr))
	})
}

// statusRecorder 包装 ResponseWriter 记录实际发出的状态码，供 accessLog 使用。
// 它必须透明转发：Flush 与 ReadFrom 是接口断言，缺一个则流式响应与大文件的
// sendfile 优化路径会被静默降级为逐块写出。
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// WriteHeader 只记录第一次显式状态；二次调用与"Write 后调用"不覆盖已记录值，
// 保证 accessLog 反映实际发出的状态码。
func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Write 在未显式 WriteHeader 时以 200 落账（与 net/http 隐式 200 语义一致）。
func (r *statusRecorder) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(p)
}

// Flush 透传（流式响应/SSE 场景）。
func (r *statusRecorder) Flush() {
	r.WriteHeader(http.StatusOK)
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ReadFrom 透传（FileServer 大文件的 sendfile 优化路径）。
func (r *statusRecorder) ReadFrom(src io.Reader) (int64, error) {
	r.WriteHeader(http.StatusOK)
	if rf, ok := r.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}
	return io.Copy(struct{ io.Writer }{r.ResponseWriter}, src)
}

// securityHeaders 为进入下游之前的响应设置安全头。
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w.Header())
		next.ServeHTTP(w, r)
	})
}

// setSecurityHeaders 是全站安全响应头的唯一事实源。
// 正常路径与限流拒绝路径都必须经过它，避免 429 成为头部例外。
func setSecurityHeaders(h http.Header) {
	h.Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; font-src 'self'; style-src 'self' 'unsafe-inline'")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
}
