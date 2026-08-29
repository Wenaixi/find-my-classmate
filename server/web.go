package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:web
var frontendFiles embed.FS

func frontendHandler() http.Handler {
	staticFiles, err := fs.Sub(frontendFiles, "web")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(staticFiles))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			page, readErr := fs.ReadFile(staticFiles, "index.html")
			if readErr != nil {
				http.Error(w, "frontend unavailable", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(page)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
