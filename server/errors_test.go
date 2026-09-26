package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// C6：错误码→HTTP 状态的配对散落在各发射点手写（api.go 6 处 + ratelimit 1 处），
// errStatus 单表使其唯一化：发射点只传码，writeError 统一查表带出状态。
// 本测试锁住 7 个错误码与状态的绑定，防止增删码时静默失配（前端按 code 分文案）。

func TestErrStatusMapping(t *testing.T) {
	want := map[string]int{
		errCodeNotFound:         http.StatusNotFound,
		errCodeMethodNotAllowed: http.StatusMethodNotAllowed,
		errCodeInvalidLimit:     http.StatusBadRequest,
		errCodeInvalidOffset:    http.StatusBadRequest,
		errCodeInvalidQuery:     http.StatusBadRequest,
		errCodeDataUnavailable:  http.StatusInternalServerError,
		errCodeRateLimited:      http.StatusTooManyRequests,
	}
	for code, status := range want {
		if got := errStatus[code]; got != status {
			t.Errorf("%s 映射状态 = %d, 期望 %d", code, got, status)
		}
	}
	if len(errStatus) != len(want) {
		t.Errorf("errStatus 表大小 = %d, 期望 %d（每码一行）", len(errStatus), len(want))
	}
}

func TestWriteErrorUnknownCodeFallsBackTo500(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, "unknown_code")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("未知错误码状态 = %d, 期望 500", rec.Code)
	}
}

func TestWriteErrorWritesJSONBody(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, errCodeInvalidLimit)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid_limit 状态 = %d, 期望 400", rec.Code)
	}
	body := rec.Body.String()
	if body != "{\"error\":\"invalid_limit\"}\n" {
		t.Errorf("响应体 = %q, 期望 {\"error\":\"invalid_limit\"}\\n", body)
	}
}
