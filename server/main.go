package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// 响应直接序列化 SearchResponse：Student 的 json tag 保证隐私红线（只输出 name/grade/class）。
// toResponse 已删除（双实现漂移源），契约由类型声明单点保证。

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

	store, err := newStudentStore(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	logInfof("loaded %d students", len(store.items))

	mux := buildMux(store)
	port := os.Getenv("PORT")
	if port == "" {
		port = "3078"
	}
	logInfof("FindMyClassmate %s listening on :%s", version, port)
	server := buildServer(":"+port, rateLimit(accessLog(securityHeaders(mux)), 60, time.Second))
	log.Fatal(server.ListenAndServe())
}

// buildMux 组装 API 路由（可注入 store，便于测试）。health 反映数据可用性：
// 数据损坏/缺失时返回 503 degraded，避免编排层误判健康。
func buildMux(store *studentStore) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", frontendHandler())
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if _, err := store.snapshot(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "reason": "data", "version": version})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version})
	})
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		limit := 10
		if value := r.URL.Query().Get("limit"); value != "" {
			parsed, parseErr := strconv.Atoi(value)
			if parseErr != nil || parsed < 1 || parsed > 50 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_limit"})
				return
			}
			limit = parsed
		}
		offset := 0
		if value := r.URL.Query().Get("offset"); value != "" {
			parsed, parseErr := strconv.Atoi(value)
			if parseErr != nil || parsed < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_offset"})
				return
			}
			offset = parsed
		}
		queryText := r.URL.Query().Get("q")
		if len([]rune(strings.TrimSpace(queryText))) > 80 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_query"})
			return
		}
		students, loadErr := store.snapshot()
		if loadErr != nil {
			logErrorf("data reload failed: %v", loadErr)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "data_unavailable"})
			return
		}
		response, _ := Search(students, queryText, limit, offset)
		writeJSON(w, http.StatusOK, response)
	})
	// 未知 /api/* 统一返回 JSON 404（not_found），与全站错误格式一致
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	})
	return mux
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
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logInfof("access %s %s %d %s %s",
			r.Method, r.URL.Path, recorder.status, time.Since(started).Round(time.Millisecond), maskedIP(r.RemoteAddr))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
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

func maskedIP(remote string) string {
	host := remote
	if idx := strings.LastIndex(remote, ":"); idx >= 0 && strings.Count(remote, ":") == 1 {
		host = remote[:idx]
	}
	parts := strings.Split(host, ".")
	if len(parts) == 4 {
		return parts[0] + "." + parts[1] + ".*.*"
	}
	return "unknown"
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; font-src 'self'; style-src 'self' 'unsafe-inline'")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
