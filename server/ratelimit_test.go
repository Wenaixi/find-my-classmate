package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestLimiter 构造可注入时钟的限流中间件。
// 429 响应、Retry-After 与放行逻辑全部走 production rateLimitWith，
// 测试只替换限流器的时钟，不复制任何 HTTP implementation。
func newTestLimiter(clock *fakeClock, capacity float64, interval time.Duration) http.Handler {
	limiter := newRateLimiter(capacity, interval, clock.Now)
	return rateLimitWith(limiter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
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
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("突发窗口满后应拒绝，实际 %d", rec.Code)
	}
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
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("不同 IP 应独立限流，实际 %d", rec.Code)
	}
}

func TestRateLimitClockRollbackDoesNotStarve(t *testing.T) {
	clock := &fakeClock{current: time.Unix(0, 0)}
	limiter := newRateLimiter(60, time.Second, clock.Now)

	if allowed, _ := limiter.allow("1.2.3.4"); !allowed {
		t.Fatal("首个请求应放行")
	}
	clock.current = clock.current.Add(-200 * time.Second)
	allowed, wait := limiter.allow("1.2.3.4")
	if !allowed {
		t.Fatalf("时钟回拨后应放行（回补被钳制为 0），实际被拒 wait=%v", wait)
	}
}

func TestRateLimitClockRollbackWaitBounded(t *testing.T) {
	clock := &fakeClock{current: time.Unix(0, 0)}
	limiter := newRateLimiter(1, time.Second, clock.Now)
	if allowed, _ := limiter.allow("1.2.3.4"); !allowed {
		t.Fatal("首个请求应放行")
	}
	if allowed, _ := limiter.allow("1.2.3.4"); allowed {
		t.Fatal("容量 1 第二个请求应被拒")
	}
	clock.current = clock.current.Add(-10 * time.Second)
	_, wait := limiter.allow("1.2.3.4")
	if wait > 2*time.Second {
		t.Fatalf("回拨后 wait 应被钳制在 1 个 interval 内，实际 %v", wait)
	}
}

func TestRateLimit429JSONAndRetryAfterSeconds(t *testing.T) {
	clock := &fakeClock{current: time.Unix(0, 0)}
	handler := newTestLimiter(clock, 1, time.Second)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("状态 = %d，期望 429", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q，期望 application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("429 响应体应为 JSON，实际 %q: %v", rec.Body.String(), err)
	}
	if body["error"] != "rate_limited" {
		t.Errorf("error 码 = %q，期望 rate_limited", body["error"])
	}
	if ra := rec.Header().Get("Retry-After"); ra != "1" {
		t.Errorf("Retry-After = %q，期望整数秒 1", ra)
	}
}

// fillTokens 纯函数测试：回补逻辑不再依赖可写时钟字段，直接测极端值。
func TestFillTokensRefillsToCapacity(t *testing.T) {
	bucket := &rateBucket{tokens: 0, lastFill: time.Unix(0, 0)}
	fillTokens(bucket, time.Unix(60, 0), 10, time.Second)
	if bucket.tokens != 10 {
		t.Fatalf("60s 后应回补到容量 10，实际 %v", bucket.tokens)
	}
	if !bucket.lastFill.Equal(time.Unix(60, 0)) {
		t.Fatalf("回补后 lastFill 应推进到 now，实际 %v", bucket.lastFill)
	}
}

func TestFillTokensClockRollbackDoesNotStarve(t *testing.T) {
	bucket := &rateBucket{tokens: 5, lastFill: time.Unix(100, 0)}
	fillTokens(bucket, time.Unix(50, 0), 10, time.Second)
	if bucket.tokens != 5 {
		t.Fatalf("时钟回拨后 tokens 应保持 5（负 elapsed 钳制为 0），实际 %v", bucket.tokens)
	}
}

func TestFillTokensClampsToCapacity(t *testing.T) {
	bucket := &rateBucket{tokens: 0, lastFill: time.Unix(0, 0)}
	fillTokens(bucket, time.Unix(3600, 0), 10, 10*time.Second)
	if bucket.tokens != 10 {
		t.Fatalf("回补不应超过容量 10，实际 %v", bucket.tokens)
	}
}

func TestFillTokensMultipleIntervals(t *testing.T) {
	bucket := &rateBucket{tokens: 2, lastFill: time.Unix(0, 0)}
	fillTokens(bucket, time.Unix(5, 0), 10, 2*time.Second)
	if bucket.tokens != 4.5 {
		// 令牌是连续量：5s / 2s = 2.5 个，回补不做取整
		t.Fatalf("5s 按 2s 间隔应回补 2.5 个到 4.5，实际 %v", bucket.tokens)
	}
}
