package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// TestStaticCacheDoesNotLeakAcrossSources 锁定缓存来源隔离不变式：
// 缓存条目绝不能跨 fs.FS 来源观察或复用另一个来源的内容与 ETag。
// 缓存键包含来源身份（staticCache 为 frontendHandlerWithFS 的实例级变量），
// 因此两个来源放同路径不同内容时各自命中自己的条目。
// 本测试在修复前是失败的：当时缓存以 URL path 为唯一键，第二个来源会拿到
// 第一个来源的内容与 ETag。断言保留为期望隔离，防止该缺陷回归。
func TestStaticCacheDoesNotLeakAcrossSources(t *testing.T) {
	first := fstest.MapFS{
		"assets/app.js": &fstest.MapFile{Data: []byte("FIRST-SOURCE-CONTENT")},
	}
	second := fstest.MapFS{
		"assets/app.js": &fstest.MapFile{Data: []byte("SECOND-SOURCE-CONTENT")},
	}

	recFirst := httptest.NewRecorder()
	frontendHandlerWithFS(first).ServeHTTP(recFirst, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if recFirst.Code != http.StatusOK {
		t.Fatalf("第一个来源应 200，实际 %d", recFirst.Code)
	}
	bodyFirst, _ := io.ReadAll(recFirst.Body)
	if string(bodyFirst) != "FIRST-SOURCE-CONTENT" {
		t.Fatalf("第一个来源内容 = %q", bodyFirst)
	}
	etagFirst := recFirst.Header().Get("ETag")

	recSecond := httptest.NewRecorder()
	frontendHandlerWithFS(second).ServeHTTP(recSecond, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if recSecond.Code != http.StatusOK {
		t.Fatalf("第二个来源应 200，实际 %d", recSecond.Code)
	}
	bodySecond, _ := io.ReadAll(recSecond.Body)
	if string(bodySecond) != "SECOND-SOURCE-CONTENT" {
		t.Errorf("第二个来源内容 = %q，期望 SECOND-SOURCE-CONTENT（缓存跨来源污染）", bodySecond)
	}
	if etagSecond := recSecond.Header().Get("ETag"); etagSecond == etagFirst {
		t.Errorf("第二个来源 ETag = %q，与第一个来源相同（缓存跨来源污染）", etagSecond)
	}
}

// TestMissingAssetDoesNotInheritImmutable 锁定缺失资源策略：
// immutable 只在资源确实存在时设置。缺失的 assets/fonts 不得获得一年长缓存，
// 否则补上同名文件后客户端仍会长期命中旧缓存。
func TestMissingAssetDoesNotInheritImmutable(t *testing.T) {
	empty := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}}
	handler := frontendHandlerWithFS(empty)

	for _, path := range []string{"/assets/missing.js", "/fonts/missing.woff2"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s 缺失资源应为 404，实际 %d", path, rec.Code)
		}
		if cc := rec.Header().Get("Cache-Control"); cc == assetCacheMaxAge {
			t.Errorf("%s 缺失资源不应带 immutable，实际 Cache-Control = %q", path, cc)
		}
	}
}

// TestStaticResponseNegotiationIsConsistent 锁定内容协商事实一致性：
// raw 200、gzip 200 与 304 都必须声明 Vary: Accept-Encoding，
// 否则共享缓存会把某一种表示复用到另一种请求上。304 不声明表示长度。
func TestStaticResponseNegotiationIsConsistent(t *testing.T) {
	source := fstest.MapFS{"assets/app.js": &fstest.MapFile{Data: []byte("console.log(1)")}}
	handler := frontendHandlerWithFS(source)

	// raw 200
	recRaw := httptest.NewRecorder()
	handler.ServeHTTP(recRaw, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if vary := recRaw.Header().Get("Vary"); vary != "Accept-Encoding" {
		t.Errorf("raw 200 Vary = %q，期望 Accept-Encoding", vary)
	}
	etag := recRaw.Header().Get("ETag")
	if etag == "" {
		t.Fatal("raw 200 应带 ETag")
	}

	// gzip 200
	reqGzip := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	reqGzip.Header.Set("Accept-Encoding", "gzip")
	recGzip := httptest.NewRecorder()
	frontendHandlerWithFS(source).ServeHTTP(recGzip, reqGzip)
	if vary := recGzip.Header().Get("Vary"); vary != "Accept-Encoding" {
		t.Errorf("gzip 200 Vary = %q，期望 Accept-Encoding", vary)
	}

	// 304
	req304 := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req304.Header.Set("If-None-Match", etag)
	rec304 := httptest.NewRecorder()
	handler.ServeHTTP(rec304, req304)
	if rec304.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match 命中应为 304，实际 %d", rec304.Code)
	}
	if vary := rec304.Header().Get("Vary"); vary != "Accept-Encoding" {
		t.Errorf("304 Vary = %q，期望 Accept-Encoding", vary)
	}
	if cl := rec304.Header().Get("Content-Length"); cl != "" {
		t.Errorf("304 不应声明 Content-Length，实际 %q", cl)
	}
}
