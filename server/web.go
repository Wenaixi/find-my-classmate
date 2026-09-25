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

func newStaticCache() *staticCache { return &staticCache{} }

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
	cache := newStaticCache()
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
			// immutable 只在资源确实存在时设置：缺失资源不得继承一年长缓存，
			// 否则补上同名文件后客户端仍会长期命中旧缓存。
			if !assetExists(fsys, r.URL.Path) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Cache-Control", assetCacheMaxAge)
			serveCachedStatic(w, r, fsys, cache, r.URL.Path)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// assetExists 判定静态资源是否存在于给定来源。
// 用于在设置 immutable 之前确认资源真实存在。
func assetExists(fsys fs.FS, path string) bool {
	file, err := fsys.Open(strings.TrimPrefix(path, "/"))
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// serveCachedStatic 提供带 ETag / gzip / 304 的静态资源响应。
//
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
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", fmt.Sprint(len(asset.gzipped)))
		_, _ = io.Copy(w, bytes.NewReader(asset.gzipped))
		return
	}
	w.Header().Set("Content-Length", fmt.Sprint(len(asset.raw)))
	_, _ = io.Copy(w, bytes.NewReader(asset.raw))
}
