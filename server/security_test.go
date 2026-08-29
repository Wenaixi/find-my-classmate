package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	required := []string{
		"Content-Security-Policy",
		"X-Frame-Options",
		"Referrer-Policy",
		"Permissions-Policy",
		"X-Content-Type-Options",
		"Cache-Control",
	}
	for _, header := range required {
		if rec.Header().Get(header) == "" {
			t.Errorf("缺少响应头 %s", header)
		}
	}

	csp := rec.Header().Get("Content-Security-Policy")
	for _, directive := range []string{
		"default-src 'self'",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"font-src 'self' https://fonts.gstatic.com",
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
	} {
		if !strings.Contains(csp, directive) {
			t.Errorf("CSP 缺少指令 %q，实际: %s", directive, csp)
		}
	}

	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q，期望 DENY", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy = %q，期望 no-referrer", got)
	}
	if got := rec.Header().Get("Permissions-Policy"); !strings.Contains(got, "geolocation=()") {
		t.Errorf("Permissions-Policy 缺少 geolocation 禁用以防位置泄露: %q", got)
	}
}

func TestSecurityHeadersPassThrough(t *testing.T) {
	called := false
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Fatal("下游 handler 未被调用")
	}
	if rec.Code != http.StatusTeapot {
		t.Errorf("状态码 = %d，期望 418", rec.Code)
	}
}
