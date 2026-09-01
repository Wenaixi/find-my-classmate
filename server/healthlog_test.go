package main

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// F45：healthcheck 探针请求不应产生 access 日志行（减少 2880 条/天噪音）
func TestAccessLogSkipsHealthProbe(t *testing.T) {
	var buf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(oldOut)

	handler := accessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("health 请求应透传，状态 = %d", rec.Code)
	}
	if strings.Contains(buf.String(), "access") {
		t.Fatalf("health 探针不应记录访问日志，实际: %s", buf.String())
	}

	// 普通请求仍应记录
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/search", nil))
	if !strings.Contains(buf.String(), "access") {
		t.Fatal("普通请求应记录访问日志")
	}
}
