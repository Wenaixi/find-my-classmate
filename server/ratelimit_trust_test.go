package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 集成行为：开启 FMC_TRUST_PROXY_HEADERS 后，经回环反代到达的两个不同真实 IP
// 必须各自独立限流桶（这才是「真 IP 独立桶」：两人互不拖累）。
// 关闭信任时（直连/未配置反代），相同 RemoteAddr 的请求共享一个桶（现状语义）。
func TestRateLimitSeparatesRealIPsViaProxyHeaders(t *testing.T) {
	prevTrust := trustProxyHeaders
	trustProxyHeaders = true
	defer func() { trustProxyHeaders = prevTrust }()

	clock := &fakeClock{current: time.Unix(0, 0)}
	limiter := newRateLimiter(2, time.Second, clock.Now)
	handler := rateLimitWith(limiter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	// 同一回环 RemoteAddr 下的两个真实客户端
	reqA := httptest.NewRequest(http.MethodGet, "/", nil)
	reqA.RemoteAddr = "127.0.0.1:3078"
	reqA.Header.Set("CF-Connecting-IP", "59.56.40.4")

	reqB := httptest.NewRequest(http.MethodGet, "/", nil)
	reqB.RemoteAddr = "127.0.0.1:3078"
	reqB.Header.Set("CF-Connecting-IP", "1.2.3.4")

	// 各发 2 次：每个桶容量 2，都能放行；若共享桶则第 3 次开始 429
	for _, req := range []*http.Request{reqA, reqA, reqB, reqB} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("独立桶下每次（各自前 2 次）都应放行，实际 %d", rec.Code)
		}
	}
	// 任一 IP 再发一次应 429（各自桶满）
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, reqA)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("桶满后应 429，实际 %d", rec.Code)
	}
}

// 关闭信任时同一 RemoteAddr 共享一个桶：第三个请求即 429。
func TestRateLimitSharedBucketWhenHeadersNotTrusted(t *testing.T) {
	prevTrust := trustProxyHeaders
	trustProxyHeaders = false
	defer func() { trustProxyHeaders = prevTrust }()

	clock := &fakeClock{current: time.Unix(0, 0)}
	limiter := newRateLimiter(2, time.Second, clock.Now)
	handler := rateLimitWith(limiter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:3078"
		req.Header.Set("CF-Connecting-IP", "59.56.40.4")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("第 %d 次应放行，实际 %d", i+1, rec.Code)
		}
	}
	// 第三个请求：共享桶已满（CF 头不生效因为未信任）→ 429
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:3078"
	req.Header.Set("CF-Connecting-IP", "1.2.3.4")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("未信任头时共享桶第 3 次应 429，实际 %d", rec.Code)
	}
}
