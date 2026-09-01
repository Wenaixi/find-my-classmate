package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// F30：二次 WriteHeader 不应覆盖已记录的状态码
func TestStatusRecorderIgnoresSecondWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}
	sr.WriteHeader(http.StatusTeapot)
	sr.WriteHeader(http.StatusInternalServerError)
	if sr.status != http.StatusTeapot {
		t.Fatalf("二次 WriteHeader 不应覆盖状态，实际 %d", sr.status)
	}
}

// F30：裸 Write 后状态应记录为 200
func TestStatusRecorderWriteDefaults200(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}
	_, _ = sr.Write([]byte("ok"))
	if sr.status != http.StatusOK {
		t.Fatalf("裸 Write 状态应为 200，实际 %d", sr.status)
	}
}

// F30：statusRecorder 应实现 http.Flusher
func TestStatusRecorderImplementsFlusher(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}
	if _, ok := any(sr).(http.Flusher); !ok {
		t.Fatal("statusRecorder 应实现 http.Flusher")
	}
}

// F30：statusRecorder 应实现 io.ReaderFrom
func TestStatusRecorderImplementsReaderFrom(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}
	if _, ok := any(sr).(io.ReaderFrom); !ok {
		t.Fatal("statusRecorder 应实现 io.ReaderFrom")
	}
	n, err := sr.ReadFrom(strings.NewReader(""))
	if err != nil || n != 0 {
		t.Fatalf("ReadFrom 空 reader 应返回 0,nil，实际 %d,%v", n, err)
	}
}

// F30：accessLog 中间件在 404 时应记录 404
func TestAccessLogRecords404(t *testing.T) {
	handler := accessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("状态 = %d，期望 404", rec.Code)
	}
}
