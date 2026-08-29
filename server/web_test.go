package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func testFrontendFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":       {Data: []byte("<!doctype html><html></html>")},
		"assets/app.js":    {Data: []byte("console.log(1)")},
		"fonts/mona.woff2": {Data: []byte("font-data")},
	}
}

func TestFrontendIndex(t *testing.T) {
	rec := httptest.NewRecorder()
	frontendHandlerWithFS(testFrontendFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / 状态 = %d，期望 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q，期望 text/html; charset=utf-8", got)
	}
	// 首页不设缓存头：no-store 由 securityHeaders 提供，本 handler 不覆盖
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Errorf("GET / 不应设置 Cache-Control，实际 %q", got)
	}
}

func TestFrontendAssetsImmutableCache(t *testing.T) {
	rec := httptest.NewRecorder()
	frontendHandlerWithFS(testFrontendFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /assets/app.js 状态 = %d，期望 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q，期望 public, max-age=31536000, immutable", got)
	}
}

func TestFrontendFontsImmutableCache(t *testing.T) {
	rec := httptest.NewRecorder()
	frontendHandlerWithFS(testFrontendFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/fonts/mona.woff2", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /fonts/mona.woff2 状态 = %d，期望 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q，期望 public, max-age=31536000, immutable", got)
	}
}

func TestFrontendMissingFileNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	frontendHandlerWithFS(testFrontendFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/favicon.png", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /favicon.png 状态 = %d，期望 404", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Errorf("404 响应不应设置 Cache-Control，实际 %q", got)
	}
}

func TestFrontendPathTraversalRejected(t *testing.T) {
	rec := httptest.NewRecorder()
	frontendHandlerWithFS(testFrontendFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/../server/main.go", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("路径穿越请求状态 = %d，期望 404", rec.Code)
	}
}
