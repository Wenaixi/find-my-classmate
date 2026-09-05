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

// staticCache 内存缓存 assets/fonts 的压缩与 ETag 结果，避免每请求重复 gzip。
// ponytail: 构建产物为哈希命名且 immutable，数量固定（约 15 个），全量缓存内存代价可忽略。
var staticCache sync.Map // path -> *cachedAsset

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
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			serveCachedStatic(w, r, fsys, r.URL.Path)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// serveCachedStatic 提供带 ETag / gzip / 304 的静态资源响应。
func serveCachedStatic(w http.ResponseWriter, r *http.Request, fsys fs.FS, path string) {
	cached, ok := staticCache.Load(path)
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
		staticCache.Store(path, cached)
	}
	asset := cached.(*cachedAsset)
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = http.DetectContentType(asset.raw)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", asset.etag)
	if r.Header.Get("If-None-Match") == asset.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		w.Header().Set("Content-Length", fmt.Sprint(len(asset.gzipped)))
		_, _ = io.Copy(w, bytes.NewReader(asset.gzipped))
		return
	}
	w.Header().Set("Content-Length", fmt.Sprint(len(asset.raw)))
	_, _ = io.Copy(w, bytes.NewReader(asset.raw))
}
