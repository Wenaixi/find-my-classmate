package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// 响应直接序列化 SearchResponse：Student 的 json tag 保证隐私红线（只输出 name/grade/class）。
// 契约由类型声明单点保证（toResponse 双实现已移除）。

func resolveDataDir() string {
	if value := os.Getenv("FMC_DATA_DIR"); value != "" {
		return value
	}
	current, err := os.Getwd()
	if err != nil {
		return "./data"
	}
	if filepath.Base(current) == "server" {
		return filepath.Join(current, "..", "data")
	}
	return filepath.Join(current, "data")
}

// version 由发布流水线 ldflags 注入（-X main.version=<git tag>）；本地构建默认为 dev。
var version = "dev"

var logLevel = parseLogLevel(os.Getenv("FMC_LOG_LEVEL"))

type level int

const (
	levelError level = iota
	levelWarn
	levelInfo
)

func parseLogLevel(value string) level {
	switch value {
	case "error":
		return levelError
	case "warn":
		return levelWarn
	default:
		return levelInfo
	}
}

func logf(min level, format string, args ...any) {
	if logLevel < min {
		return
	}
	log.Printf(format, args...)
}

func logInfof(format string, args ...any)  { logf(levelInfo, format, args...) }
func logWarnf(format string, args ...any)  { logf(levelWarn, format, args...) }
func logErrorf(format string, args ...any) { logf(levelError, format, args...) }

// resolveLogDir 优先使用 FMC_LOG_DIR 环境变量；未设置时沿用数据目录下的 log 子目录。
// 容器场景数据目录通常只读挂载，日志必须写到独立可写位置。
func resolveLogDir(dataDir string) string {
	if value := os.Getenv("FMC_LOG_DIR"); value != "" {
		return value
	}
	return filepath.Join(dataDir, "log")
}

func openLog(dataDir string) (io.Writer, *os.File, error) {
	logDir := resolveLogDir(dataDir)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(filepath.Join(logDir, "server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	// 文件与 stdout 双写：容器场景由 docker 收集 stdout，本地场景保留文件
	return io.MultiWriter(file, os.Stderr), file, nil
}

func main() {
	dataDir := resolveDataDir()
	// 启动自举：幂等创建数据目录，缺失数据文件时给出清晰指引而非静默空跑
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("创建数据目录失败 %s: %v", dataDir, err)
	}
	output, logFile, err := openLog(dataDir)
	if err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(output)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	store, err := newStudentStore(dataDir, time.Now)
	if err != nil {
		log.Fatal(err)
	}
	logInfof("loaded %d students", store.Size())

	mux := buildMux(store, version)
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}
	logInfof("FindMyClassmate %s listening on :%s", version, port)
	server := buildServer(":"+port, newHandlerChain(mux))
	log.Fatal(server.ListenAndServe())
}

// newHandlerChain 组装 production 请求链，是中间件顺序的唯一事实源。
// 顺序（由外到内）：securityHeaders → rateLimit → accessLog → mux。
//
// 安全头居最外是刻意的：全站响应头契约必须覆盖所有响应，包括限流拒绝
// （429）与被访问日志跳过的健康检查；限流在 accessLog 之外短路，
// 被拒绝的请求不写访问日志，避免攻击流量放大日志磁盘写入。
// 429 因位于 securityHeaders 内侧，响应头由外层统一设置，writeRateLimited
// 无需手工重放。
func newHandlerChain(mux http.Handler) http.Handler {
	return newHandlerChainWith(mux, newRateLimiter(rateCapacity, rateInterval, time.Now))
}

// newHandlerChainWith 是链装配的唯一实现：只替换限流器本身，链的顺序与
// 其余中间件行为全部走 production 代码路径，调用方（含测试）无需复制嵌套。
// 委托关系取代此前测试侧重写一遍嵌套的做法——链顺序改错时测试会跟着错，
// 平行实现会让四个组合政策测试在测另一条链却依然全绿。
func newHandlerChainWith(mux http.Handler, limiter *rateLimiter) http.Handler {
	return securityHeaders(rateLimitWith(limiter, accessLog(mux)))
}

// buildServer 组装带超时配置的 http.Server：防止慢速攻击挂起连接。
func buildServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
