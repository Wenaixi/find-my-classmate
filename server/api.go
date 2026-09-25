package main

import (
	"net/http"
	"strconv"
	"strings"
)

// buildMux 组装 API 路由（可注入 store 与版本号，便于测试）。health 反映数据可用性：
// 数据损坏/缺失时返回 503 degraded，避免编排层误判健康。
//
// 本文件只做 HTTP 翻译：参数取值、错误码映射与 JSON 写出。
// 查询语义（解析/匹配/排序/分页）由 search.go 独占，分页前置约定也由
// Search 自守；此处不再复制这些规则，只把 wire 形态翻译成调用。
func buildMux(store *studentStore, buildVersion string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", frontendHandler())
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if _, err := store.view(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "reason": "data", "version": buildVersion})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": buildVersion})
	})
	mux.HandleFunc("/api/search", searchHandler(store))
	// 未知 /api/* 统一返回 JSON 404（not_found），与全站错误格式一致
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errCodeNotFound})
	})
	return mux
}

// searchHandler 翻译 GET /api/search 的 wire 契约：方法、limit、offset 与
// 查询串长度校验在此完成并映射为 400 契约错误码；数据不可用映射为 500。
// 成功路径直接委托 Search，由它独占匹配、排序与分页语义。
func searchHandler(store *studentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": errCodeMethodNotAllowed})
			return
		}
		limit := defaultLimit
		if value := r.URL.Query().Get("limit"); value != "" {
			parsed, parseErr := strconv.Atoi(value)
			if parseErr != nil || parsed < 1 || parsed > maxLimit {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": errCodeInvalidLimit})
				return
			}
			limit = parsed
		}
		offset := 0
		if value := r.URL.Query().Get("offset"); value != "" {
			parsed, parseErr := strconv.Atoi(value)
			if parseErr != nil || parsed < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": errCodeInvalidOffset})
				return
			}
			offset = parsed
		}
		queryText := r.URL.Query().Get("q")
		if len([]rune(strings.TrimSpace(queryText))) > maxQueryRunes {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errCodeInvalidQuery})
			return
		}
		students, loadErr := store.view()
		if loadErr != nil {
			logErrorf("data reload failed: %v", loadErr)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": errCodeDataUnavailable})
			return
		}
		response, _ := Search(students, queryText, limit, offset)
		writeJSON(w, http.StatusOK, response)
	}
}
