package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testFrontendFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":       {Data: []byte("<!doctype html><html></html>")},
		"assets/app.js":    {Data: []byte("console.log(1)")},
		"assets/app.mjs":   {Data: []byte("export default 1")},
		"assets/app.css":   {Data: []byte("body { color: red }")},
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
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Errorf("GET / 不应设置 Cache-Control，实际 %q", got)
	}
}

func TestFrontendAssetsServeExpectedContentTypes(t *testing.T) {
	for _, test := range []struct {
		path string
		want string
	}{
		{path: "/assets/app.js", want: "application/javascript"},
		{path: "/assets/app.mjs", want: "application/javascript"},
		{path: "/assets/app.css", want: "text/css; charset=utf-8"},
	} {
		t.Run(test.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			frontendHandlerWithFS(testFrontendFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, test.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s 状态 = %d，期望 200", test.path, rec.Code)
			}
			if got := rec.Header().Get("Content-Type"); got != test.want {
				t.Errorf("Content-Type = %q，期望 %q", got, test.want)
			}
		})
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

// F26：Accept-Encoding: gzip 时静态资源应返回 gzip 压缩（Content-Encoding: gzip）
func TestFrontendAssetsGzip(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	frontendHandlerWithFS(testFrontendFS()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态 = %d，期望 200", rec.Code)
	}
	if ce := rec.Header().Get("Content-Encoding"); ce != "gzip" {
		t.Fatalf("Content-Encoding = %q，期望 gzip", ce)
	}
	// 解压后内容应与原文一致
	reader, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip 解压失败: %v", err)
	}
	body, _ := io.ReadAll(reader)
	if string(body) != "console.log(1)" {
		t.Errorf("解压内容 = %q，期望 console.log(1)", string(body))
	}
}

// F55：静态资源应带 ETag，且 If-None-Match 命中时返回 304
func TestFrontendAssetsETagAnd304(t *testing.T) {
	rec := httptest.NewRecorder()
	frontendHandlerWithFS(testFrontendFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("静态资源应带 ETag")
	}

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	frontendHandlerWithFS(testFrontendFS()).ServeHTTP(rec2, req)
	if rec2.Code != http.StatusNotModified {
		t.Fatalf("ETag 命中应返回 304，实际 %d", rec2.Code)
	}
}

// F26 关联：首页（/）不应被 gzip（保持 no-store 语义，且小页面不值得压缩）
func TestFrontendIndexNotGzipped(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	frontendHandlerWithFS(testFrontendFS()).ServeHTTP(rec, req)
	if ce := rec.Header().Get("Content-Encoding"); ce != "" {
		t.Errorf("首页不应 gzip，实际 %q", ce)
	}
	if !strings.Contains(rec.Body.String(), "<html>") {
		t.Errorf("首页内容异常: %q", rec.Body.String())
	}
}
