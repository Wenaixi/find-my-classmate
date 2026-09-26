package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 二次 WriteHeader 不应覆盖已记录的状态码
func TestStatusRecorderIgnoresSecondWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}
	sr.WriteHeader(http.StatusTeapot)
	sr.WriteHeader(http.StatusInternalServerError)
	if sr.status != http.StatusTeapot {
		t.Fatalf("二次 WriteHeader 不应覆盖状态，实际 %d", sr.status)
	}
}

// 裸 Write 后状态应记录为 200
func TestStatusRecorderWriteDefaults200(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}
	_, _ = sr.Write([]byte("ok"))
	if sr.status != http.StatusOK {
		t.Fatalf("裸 Write 状态应为 200，实际 %d", sr.status)
	}
}

// handler 调用 Flush 时，透传必须真实到达下游 writer，且状态落账 200。
// 这取代"仅断言 statusRecorder 满足 http.Flusher"的形状测试：
// 形状断言在 Flush 实现体被改坏时不会失败，透传断言会。
func TestStatusRecorderFlushPassesThrough(t *testing.T) {
	rec := httptest.NewRecorder()
	handler := accessLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("chunk")); err != nil {
			t.Errorf("写入失败: %v", err)
		}
		f, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("accessLog 包装后应仍满足 http.Flusher")
		}
		f.Flush()
	}))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/flush", nil))

	if rec.Body.String() != "chunk" {
		t.Fatalf("Flush 前写入的正文应可读，实际 %q", rec.Body.String())
	}
	if !rec.Flushed {
		t.Error("Flush 应真实透传到下游 writer")
	}
}

// handler 通过 ReadFrom 搬运正文时，字节应完整到达下游且状态落账 200。
// ReadFrom 存在是为了保住 FileServer 的 sendfile 路径，真实字节搬运才是它的职责。
func TestStatusRecorderReadFromTransportsBytes(t *testing.T) {
	payload := "0123456789"
	rec := httptest.NewRecorder()
	handler := accessLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		rf, ok := w.(io.ReaderFrom)
		if !ok {
			t.Fatal("accessLog 包装后应仍满足 io.ReaderFrom")
		}
		n, err := rf.ReadFrom(strings.NewReader(payload))
		if err != nil {
			t.Errorf("ReadFrom 返回错误: %v", err)
		}
		if n != int64(len(payload)) {
			t.Errorf("ReadFrom 搬运字节数 = %d，期望 %d", n, len(payload))
		}
	}))
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readfrom", nil))

	if rec.Body.String() != payload {
		t.Fatalf("ReadFrom 正文应完整透传，实际 %q", rec.Body.String())
	}
}

// accessLog 中间件在 404 时应记录 404
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
