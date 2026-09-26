package main

import (
	"encoding/json"
	"net/http"
)

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

// writeJSON 写出 JSON 响应：设置 Content-Type 后编码输出。
//
// 此前定义在 main.go（启动装配文件）而全部消费点都在 API 侧（api.go 的三处
// 成功响应、errors.go 的错误体、ratelimit.go 间接经 writeError）。改响应写出
// 格式要去一个只声称承担「启动自举 / 日志 / 中间件 / 装配」四个薄角色的文件里
// 找，而那个文件自身零调用它。搬到此处与 writeError 同址：写出格式与错误码映射
// 是同一件事的两面，main.go 只留装配。
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
