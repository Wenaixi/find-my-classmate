package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

//go:embed all:web
var frontendFiles embed.FS

func frontendHandler() http.Handler {
	staticFiles, err := fs.Sub(frontendFiles, "web")
	if err != nil {
		panic(err)
	}
	return frontendHandlerWithFS(staticFiles)
}

// staticCache 缓存 assets/fonts 的压缩与 ETag 结果，避免每请求重复 gzip。
// ponytail: 构建产物为哈希命名且 immutable，数量固定（约 15 个），全量缓存内存代价可忽略。
//
// 缓存随所服务的 FS 来源而存在（见 frontendHandlerWithFS），不放在包级：
// 缓存条目绝不能跨来源复用另一个来源的内容或 ETag，否则同一个
// frontendHandlerWithFS 契约下的两个来源会互相污染。
type staticCache struct {
	assets sync.Map // path -> *cachedAsset
}

type cachedAsset struct {
	etag    string
	raw     []byte
	gzipped []byte
}

// frontendHandlerWithFS 服务页面与静态资源。
// /assets/ 与 /fonts/ 为构建产物（哈希命名或构建后不再变更）：
//   - Cache-Control: public, max-age=31536000, immutable（长缓存）
//   - ETag + If-None-Match → 304（让 immutable 缓存真正生效）
//   - Accept-Encoding: gzip → gzip 压缩传输（首屏体积约减 70%）
//
// 首页与其他文件保持不设缓存头，由 securityHeaders 的 no-store 兜底。
func frontendHandlerWithFS(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	// 缓存与本 handler 所服务的来源绑定：不同来源不会互相复用条目
	cache := &staticCache{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			page, readErr := fs.ReadFile(fsys, "index.html")
			if readErr != nil {
				http.Error(w, "frontend unavailable", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(page)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") || strings.HasPrefix(r.URL.Path, "/fonts/") {
			// 存在性判定与内容读取合为一次来源访问：缓存命中即证明资源存在
			// （未命中时才读盘，读不到即 404），不再每次请求都预检。
			// immutable 只在资源确实存在时设置：缺失资源不得继承一年长缓存，
			// 否则补上同名文件后客户端仍会长期命中旧缓存。
			serveCachedStatic(w, r, fsys, cache, r.URL.Path)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// serveCachedStatic 提供带 ETag / gzip / 304 的静态资源响应。
//
// 存在性判定与内容读取合为一次来源访问：缓存命中即证明资源存在（不再预检），
// 缓存未命中时读盘一次，读不到即 404。immutable 缓存头只在此处确认资源存在后设置，
// 缺失资源因此不会继承一年长缓存。

// acceptsGzip 判定客户端是否接受 gzip 表示。
//
// 响应已声明 Vary: Accept-Encoding（serveCachedStatic 内的 Set("Vary", …)），即向共享缓存声明了「本响应随
// Accept-Encoding 变化」；协商本身却曾用子串匹配实现，把 gzip;q=0（客户端明确
// 拒绝压缩）判为接受。声明了协商维度却不严格协商，是接入 CDN 或共享缓存后最难
// 排查的一类问题——缓存会忠实地复用错误的表示。
//
// 规则：逐个候选编码解析 q 值，gzip 出现且 q>0 才算接受；q=0 显式拒绝。
// 缺省 q 为 1。不解析整串（deflate;q=0.5, gzip;q=0 这类组合是合法的）。
func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		coding := strings.TrimSpace(fields[0])
		if !strings.EqualFold(coding, "gzip") {
			continue
		}
		quality := 1.0
		for _, param := range fields[1:] {
			key, value, found := strings.Cut(param, "=")
			if !found || !strings.EqualFold(strings.TrimSpace(key), "q") {
				continue
			}
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil {
				// q 值不可解析时按拒绝处理：宁可少压缩，不可把明确拒绝的客户端
				// 喂给它无法解码的表示。
				return false
			}
			quality = parsed
		}
		return quality > 0
	}
	return false
}

// 协商事实一致性：同一资源可能以 raw 或 gzip 返回，因此 200 与 304 都必须
// 声明 Vary: Accept-Encoding——否则共享缓存会把某一种表示复用到另一种请求上。
// 304 不声明 Content-Length / Content-Encoding，避免与实际表示不符的长度。
func serveCachedStatic(w http.ResponseWriter, r *http.Request, fsys fs.FS, cache *staticCache, path string) {
	cached, ok := cache.assets.Load(path)
	if !ok {
		content, err := fs.ReadFile(fsys, strings.TrimPrefix(path, "/"))
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		sum := sha256.Sum256(content)
		etag := fmt.Sprintf("%q", fmt.Sprintf("%x-%d", sum[:8], len(content)))
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		_, _ = gz.Write(content)
		_ = gz.Close()
		cached = &cachedAsset{etag: etag, raw: content, gzipped: buf.Bytes()}
		cache.assets.Store(path, cached)
	}
	// 走到这里即证明资源存在（命中缓存，或刚读盘成功）：
	// immutable 只给确认存在的资源，缺失资源已在上面的 404 分支返回。
	w.Header().Set("Cache-Control", assetCacheMaxAge)
	asset := cached.(*cachedAsset)
	ext := filepath.Ext(path)
	var contentType string
	switch ext {
	case ".js", ".mjs":
		contentType = "application/javascript"
	case ".woff2":
		contentType = "font/woff2"
	default:
		contentType = mime.TypeByExtension(ext)
	}
	if contentType == "" {
		contentType = http.DetectContentType(asset.raw)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", asset.etag)
	// 无论本次返回哪一种表示，都声明协商维度：raw 与 gzip 共用同一资源
	w.Header().Set("Vary", "Accept-Encoding")
	if r.Header.Get("If-None-Match") == asset.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if acceptsGzip(r.Header.Get("Accept-Encoding")) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", fmt.Sprint(len(asset.gzipped)))
		_, _ = io.Copy(w, bytes.NewReader(asset.gzipped))
		return
	}
	w.Header().Set("Content-Length", fmt.Sprint(len(asset.raw)))
	_, _ = io.Copy(w, bytes.NewReader(asset.raw))
}
