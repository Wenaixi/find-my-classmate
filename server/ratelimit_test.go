package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestLimiter 构造带可控时钟的限流中间件，方便模拟时间推进。
// capacity 为突发窗口令牌数，interval 为补充 1 个令牌所需时间。
func newTestLimiter(clock *fakeClock, capacity float64, interval time.Duration) http.Handler {
	limiter := newRateLimiter(capacity, interval)
	limiter.now = clock.Now
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, wait := limiter.allow(clientIP(r.RemoteAddr))
		if !allowed {
			w.Header().Set("Retry-After", wait.Round(time.Second).String())
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

type fakeClock struct{ current time.Time }

func (f *fakeClock) Now() time.Time          { return f.current }
func (f *fakeClock) advance(d time.Duration) { f.current = f.current.Add(d) }

func TestRateLimitAllowsBurst(t *testing.T) {
	clock := &fakeClock{current: time.Unix(0, 0)}
	handler := newTestLimiter(clock, 3, time.Second)
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("第 %d 次请求应放行，实际 %d", i+1, rec.Code)
		}
	}
}

func TestRateLimitRejectsExcess(t *testing.T) {
	clock := &fakeClock{current: time.Unix(0, 0)}
	handler := newTestLimiter(clock, 3, time.Second)
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("超出突发窗口应返回 429，实际 %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 响应应带 Retry-After 头")
	}
}

func TestRateLimitRefillsOverTime(t *testing.T) {
	clock := &fakeClock{current: time.Unix(0, 0)}
	handler := newTestLimiter(clock, 2, time.Second)
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	}
	// 第三个请求应被拒绝（突发窗口已满）
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("突发窗口满后应拒绝，实际 %d", rec.Code)
	}
	// 时间前进 1.1 秒（interval=1s，应恢复约 1 个令牌），请求应放行
	clock.advance(1100 * time.Millisecond)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec2.Code != http.StatusNoContent {
		t.Errorf("窗口过后应恢复请求，实际 %d", rec2.Code)
	}
}

func TestRateLimitSeparatesIPs(t *testing.T) {
	clock := &fakeClock{current: time.Unix(0, 0)}
	handler := newTestLimiter(clock, 2, time.Second)
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	}
	// 第二个 IP 应不受第一个 IP 影响
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("不同 IP 应独立限流，实际 %d", rec.Code)
	}
}
