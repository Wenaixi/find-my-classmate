package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// F48：/api/health 应携带版本号（ldflags 注入），便于运维溯源
func TestHealthIncludesVersion(t *testing.T) {
	store := newTestStore(t, map[string]string{"高一.json": validGradeOne, "高二.json": validGradeTwo})
	mux := buildMux(store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["version"] == "" {
		t.Error("health 应含 version 字段（ldflags 注入，dev 为默认）")
	}
}
