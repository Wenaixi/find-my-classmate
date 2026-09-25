package main

// API 错误码常量表：全站 JSON 错误体的唯一事实源。
// api.go 与 ratelimit.go 的发射点引用这些常量，杜绝字面量散落；
// 前端 src/lib/api.ts / src/lib/searchReducer.ts 按这些 code 分类文案。
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
