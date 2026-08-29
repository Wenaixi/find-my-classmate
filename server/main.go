package main

import (
	"encoding/json"
	"io"
	"log"
	"time"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func toResponse(response SearchResponse) map[string]any {
	items := make([]map[string]any, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, map[string]any{"name": item.Name, "grade": item.Grade, "class": item.ClassName})
	}
	return map[string]any{
		"items":   items,
		"total":   response.Total,
		"limit":   response.Limit,
		"offset":  response.Offset,
		"hasMore": response.HasMore,
	}
}

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

func logInfof(format string, args ...any) { logf(levelInfo, format, args...) }
func logWarnf(format string, args ...any) { logf(levelWarn, format, args...) }
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

	mux := http.NewServeMux()
	mux.Handle("/", frontendHandler())
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
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
		writeJSON(w, http.StatusOK, toResponse(response))
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "3078"
	}
	logInfof("FindMyClassmate API listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, accessLog(securityHeaders(mux))))
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
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
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
