package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
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

// frontendHandlerWithFS 服务页面与静态资源。
// /assets/ 与 /fonts/ 为构建产物（哈希命名或构建后不再变更），
// 设置 immutable 长缓存；本 handler 后于 securityHeaders 执行，
// 因此会覆盖其全局 no-store。首页与其他文件保持不设缓存头，
// 由 securityHeaders 的 no-store 兜底，避免缓存旧版无哈希资源。
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
		}
		fileServer.ServeHTTP(w, r)
	})
}
