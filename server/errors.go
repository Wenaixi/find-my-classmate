package main

import "net/http"

// API 错误码常量表：全站 JSON 错误体的唯一事实源。
const (
	// errCodeNotFound 未知 /api/* 路径。
	errCodeNotFound = "not_found"
	// errCodeMethodNotAllowed 非 GET 访问 /api/search（带 Allow 头）。
	errCodeMethodNotAllowed = "method_not_allowed"
	// errCodeInvalidLimit limit 非数字或越界（1-50）。
	errCodeInvalidLimit = "invalid_limit"
	// errCodeInvalidOffset offset 非数字或负数。
	errCodeInvalidOffset = "invalid_offset"
	// errCodeInvalidQuery 查询串超过 maxQueryRunes。
	errCodeInvalidQuery = "invalid_query"
	// errCodeDataUnavailable 数据文件缺失/损坏（500）。
	errCodeDataUnavailable = "data_unavailable"
	// errCodeRateLimited 限流拒绝（429，带 Retry-After）。
	errCodeRateLimited = "rate_limited"
)

// errStatus 错误码 → HTTP 状态的单一映射表（C6）。
// 发射点只传码，writeError 统一查表带出状态；新增错误码必须在此登记一行，
// 否则回落 500 并由 TestErrStatusMapping 的尺寸断言捕获遗漏。
var errStatus = map[string]int{
	errCodeNotFound:         http.StatusNotFound,
	errCodeMethodNotAllowed: http.StatusMethodNotAllowed,
	errCodeInvalidLimit:     http.StatusBadRequest,
	errCodeInvalidOffset:    http.StatusBadRequest,
	errCodeInvalidQuery:     http.StatusBadRequest,
	errCodeDataUnavailable:  http.StatusInternalServerError,
	errCodeRateLimited:      http.StatusTooManyRequests,
}

// writeError 按错误码查表写出 JSON 错误体；未知码回落 500。
// 发射点（api.go / ratelimit.go）不再手写 status+code 配对。
func writeError(w http.ResponseWriter, code string) {
	status, ok := errStatus[code]
	if !ok {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, map[string]string{"error": code})
}
