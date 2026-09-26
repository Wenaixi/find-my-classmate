package main

import (
	"compress/gzip"
	"io"
	"io/fs"
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

// countingFS 包装源 FS 并统计打开次数，用于验证静态资源服务路径的读盘次数。
type countingFS struct {
	fs    fs.FS
	opens int
}

func (c *countingFS) Open(name string) (fs.File, error) {
	c.opens++
	return c.fs.Open(name)
}

func (c *countingFS) reset() { c.opens = 0 }

// 缓存命中后不得再触碰来源 FS：
// 修复前每次请求都先跑 assetExists（Open + Stat）确认存在性，
// 命中缓存时仍产生 2 次文件系统调用。
func TestStaticCacheHitDoesNotTouchFS(t *testing.T) {
	source := &countingFS{fs: testFrontendFS()}
	handler := frontendHandlerWithFS(source)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("首次请求状态 = %d，期望 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != assetCacheMaxAge {
		t.Errorf("Cache-Control = %q，期望 %q", got, assetCacheMaxAge)
	}
	if source.opens == 0 {
		t.Fatal("首次请求应至少打开一次来源")
	}

	// 二次请求命中缓存：来源不应再被打开
	source.reset()
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req2.Header.Set("If-None-Match", rec.Header().Get("ETag"))
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotModified {
		t.Fatalf("ETag 命中应返回 304，实际 %d", rec2.Code)
	}
	if source.opens != 0 {
		t.Fatalf("缓存命中后不应触碰来源 FS，实际打开 %d 次", source.opens)
	}
}

// 缺失资源不得继承 immutable 缓存头（补上文件后客户端不应长期命中旧缓存）。
func TestStaticMissingAssetHasNoImmutable(t *testing.T) {
	handler := frontendHandlerWithFS(testFrontendFS())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("缺失资源状态 = %d，期望 404", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got == assetCacheMaxAge {
		t.Errorf("缺失资源不应带 immutable 缓存头，实际 %q", got)
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
		{path: "/fonts/mona.woff2", want: "font/woff2"},
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
	if got := rec.Header().Get("Cache-Control"); got != assetCacheMaxAge {
		t.Errorf("Cache-Control = %q，期望 %s", got, assetCacheMaxAge)
	}
}

func TestFrontendFontsImmutableCache(t *testing.T) {
	rec := httptest.NewRecorder()
	frontendHandlerWithFS(testFrontendFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/fonts/mona.woff2", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /fonts/mona.woff2 状态 = %d，期望 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != assetCacheMaxAge {
		t.Errorf("Cache-Control = %q，期望 %s", got, assetCacheMaxAge)
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

// Accept-Encoding: gzip 时静态资源应返回 gzip 压缩（Content-Encoding: gzip）
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

// 静态资源应带 ETag，且 If-None-Match 命中时返回 304
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

// 首页（/）不应被 gzip（保持 no-store 语义，且小页面不值得压缩）
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

// 协商规则必须能区分「接受」与「明确拒绝」：响应已声明 Vary: Accept-Encoding，
// 共享缓存会按该维度复用表示，把 gzip;q=0 误判为接受会让明确拒绝压缩的客户端
// 收到它无法解码的表示。子串匹配实现下本组用例全部通过（变异实验：把判定改成
// == "gzip" 后 132 条后端用例无一翻红）。
func TestAcceptsGzipNegotiation(t *testing.T) {
	cases := []struct {
		header string
		want   bool
		why    string
	}{
		{"gzip", true, "裸 gzip 应接受"},
		{"gzip, deflate, br", true, "多候选中的 gzip 应接受"},
		{"deflate, gzip", true, "非首位候选也应识别"},
		{"gzip;q=0", false, "q=0 是明确拒绝"},
		{"gzip;q=0.0", false, "q=0.0 同样是拒绝"},
		{"deflate, gzip;q=0", false, "候选列表中的 q=0 必须被识别"},
		{"gzip;q=0, deflate;q=1", false, "gzip 被拒时不得因 deflate 可用而放行"},
		{"gzip;q=1.0", true, "显式 q=1 等价于缺省"},
		{"gzip;q=0.5", true, "q=0.5 表示可接受"},
		{"deflate", false, "不含 gzip 则不压缩"},
		{"", false, "无 Accept-Encoding 则不压缩"},
		{"GZIP", true, "编码名大小写不敏感"},
		{"gzip;q=abc", false, "q 值不可解析时按拒绝处理"},
	}
	for _, c := range cases {
		if got := acceptsGzip(c.header); got != c.want {
			t.Errorf("acceptsGzip(%q) = %v，期望 %v（%s）", c.header, got, c.want, c.why)
		}
	}
}

// q=0 的客户端必须拿到未压缩表示：这是用户可见行为，不只是纯函数返回值。
func TestFrontendAssetsGzipQZeroServesRaw(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip;q=0")
	rec := httptest.NewRecorder()
	frontendHandlerWithFS(testFrontendFS()).ServeHTTP(rec, req)

	if ce := rec.Header().Get("Content-Encoding"); ce != "" {
		t.Fatalf("明确拒绝 gzip 的客户端不应收到压缩表示，实际 Content-Encoding = %q", ce)
	}
	if rec.Body.String() != "console.log(1)" {
		t.Errorf("应返回原始内容，实际 = %q", rec.Body.String())
	}
	// 协商维度仍必须声明：本次是 raw，但同一资源也可能是 gzip 表示。
	if v := rec.Header().Get("Vary"); v != "Accept-Encoding" {
		t.Errorf("Vary = %q，期望 Accept-Encoding", v)
	}
}

// 多候选编码列表同样必须正确协商：裸相等判断会把 "gzip, deflate, br" 与
// "deflate, gzip" 一起判为不接受，而浏览器实际发送的正是后者这种形式。
// 断言走 HTTP 接口而非直接调用 acceptsGzip：直接测纯函数只能锁住实现，
// 实现被换掉时断言不会翻红，而用户可见行为才是真正要保护的东西。
func TestFrontendAssetsGzipInCandidateList(t *testing.T) {
	for _, header := range []string{"gzip, deflate, br", "deflate, gzip", "br, gzip;q=0.5"} {
		req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
		req.Header.Set("Accept-Encoding", header)
		rec := httptest.NewRecorder()
		frontendHandlerWithFS(testFrontendFS()).ServeHTTP(rec, req)

		if ce := rec.Header().Get("Content-Encoding"); ce != "gzip" {
			t.Errorf("Accept-Encoding %q 应协商为 gzip，实际 Content-Encoding = %q", header, ce)
		}
		reader, err := gzip.NewReader(rec.Body)
		if err != nil {
			t.Fatalf("Accept-Encoding %q：gzip 解压失败: %v", header, err)
		}
		body, _ := io.ReadAll(reader)
		if string(body) != "console.log(1)" {
			t.Errorf("Accept-Encoding %q：解压内容 = %q", header, string(body))
		}
	}
}
