package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func toResponse(response SearchResponse) map[string]any {
	items := make([]map[string]any, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, map[string]any{"name": item.Name, "grade": item.Grade, "class": item.ClassName})
	}
	return map[string]any{
		"items":   items,
		"total":   response.Total,
		"limit":   response.Limit,
		"offset":  response.Offset,
		"hasMore": response.HasMore,
	}
}

func resolveDataDir() string {
	if value := os.Getenv("FMC_DATA_DIR"); value != "" {
		return value
	}
	current, err := os.Getwd()
	if err != nil {
		return "./data"
	}
	if filepath.Base(current) == "server" {
		return filepath.Join(current, "..", "data")
	}
	return filepath.Join(current, "data")
}

func openLogFile(dataDir string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "log"), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dataDir, "log", "server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

func main() {
	dataDir := resolveDataDir()
	logFile, err := openLogFile(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	store, err := newStudentStore(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("loaded %d students", len(store.items))

	mux := http.NewServeMux()
	mux.Handle("/", frontendHandler())
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		limit := 10
		if value := r.URL.Query().Get("limit"); value != "" {
			parsed, parseErr := strconv.Atoi(value)
			if parseErr != nil || parsed < 1 || parsed > 50 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_limit"})
				return
			}
			limit = parsed
		}
		offset := 0
		if value := r.URL.Query().Get("offset"); value != "" {
			parsed, parseErr := strconv.Atoi(value)
			if parseErr != nil || parsed < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_offset"})
				return
			}
			offset = parsed
		}
		queryText := r.URL.Query().Get("q")
		if len([]rune(strings.TrimSpace(queryText))) > 80 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_query"})
			return
		}
		students, loadErr := store.snapshot()
		if loadErr != nil {
			log.Printf("data reload failed: %v", loadErr)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "data_unavailable"})
			return
		}
		response, _ := Search(students, queryText, limit, offset)
		writeJSON(w, http.StatusOK, toResponse(response))
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "3078"
	}
	log.Printf("FindMyClassmate API listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, securityHeaders(mux)))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
