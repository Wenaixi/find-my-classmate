package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestChain 组装可注入时钟的 production 请求链。
// 顺序与 newHandlerChain 同构（securityHeaders → rateLimit → accessLog → mux）：
// 安全头居最外覆盖所有响应（含 429），限流在 accessLog 之外短路。
func newTestChain(clock *fakeClock, capacity float64, mux http.Handler) http.Handler {
	limiter := newRateLimiter(capacity, time.Second)
	limiter.now = clock.Now
	return securityHeaders(rateLimitWith(limiter, accessLog(mux)))
}

// captureLogs 把标准日志输出重定向到缓冲区，测试结束后恢复。
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	oldOut := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(oldOut)
		log.SetFlags(oldFlags)
	})
	return buf
}

// TestHandlerChainRateLimitedResponseCarriesSecurityHeaders 锁定组合政策：
// 限流在 securityHeaders 之外短路返回，但 429 仍必须携带全站安全响应头，
// 使安全头契约对被拒绝请求同样成立。
func TestHandlerChainRateLimitedResponseCarriesSecurityHeaders(t *testing.T) {
	store := newTestStore(t, map[string]string{"高一.json": validGradeOne, "高二.json": validGradeTwo})
	clock := &fakeClock{current: time.Now()}
	captureLogs(t)

	handler := newTestChain(clock, 1, buildMux(store))

	// 第一次放行
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=王", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("首次请求应放行，状态 = %d", rec.Code)
	}

	// 第二次同 IP 超限
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=王", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("超限请求应为 429，实际 = %d", rec.Code)
	}

	// 429 必须携带安全响应头
	for _, header := range []string{
		"Content-Security-Policy",
		"X-Frame-Options",
		"Referrer-Policy",
		"Permissions-Policy",
		"X-Content-Type-Options",
		"Cache-Control",
	} {
		if rec.Header().Get(header) == "" {
			t.Errorf("429 响应缺少安全头 %s", header)
		}
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("429 X-Frame-Options = %q，期望 DENY", got)
	}

	// 429 的 Retry-After 与 JSON 错误体保持不变
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 应带 Retry-After")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("429 Content-Type = %q，期望 application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("429 响应体应为 JSON，实际 %q", rec.Body.String())
	}
	if body["error"] != "rate_limited" {
		t.Errorf("429 error 码 = %q，期望 rate_limited", body["error"])
	}
}

// TestHandlerChainRateLimitedRequestIsNotAccessLogged 锁定访问日志政策：
// 限流位于 accessLog 之外，被拒绝的请求不写访问日志，避免攻击流量放大日志写入。
func TestHandlerChainRateLimitedRequestIsNotAccessLogged(t *testing.T) {
	store := newTestStore(t, map[string]string{"高一.json": validGradeOne, "高二.json": validGradeTwo})
	clock := &fakeClock{current: time.Now()}
	buf := captureLogs(t)

	handler := newTestChain(clock, 1, buildMux(store))

	// 放行一次，产生一条 access 日志
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=王", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("首次请求应放行，状态 = %d", rec.Code)
	}
	afterAllowed := buf.String()

	// 超限：不应新增 access 日志
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=王", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("超限请求应为 429，实际 = %d", rec.Code)
	}
	if extra := strings.Count(buf.String(), "access") - strings.Count(afterAllowed, "access"); extra != 0 {
		t.Errorf("429 不应新增访问日志，实际新增 %d 条: %s", extra, buf.String())
	}
}

// TestHandlerChainServesHealthWithoutAccessLog 锁定 health 语义：
// 请求穿过完整链返回 200，且不产生访问日志噪音。
func TestHandlerChainServesHealthWithoutAccessLog(t *testing.T) {
	store := newTestStore(t, map[string]string{"高一.json": validGradeOne, "高二.json": validGradeTwo})
	clock := &fakeClock{current: time.Now()}
	buf := captureLogs(t)

	handler := newTestChain(clock, 10, buildMux(store))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health 应为 200，实际 = %d", rec.Code)
	}
	if strings.Contains(buf.String(), "access") {
		t.Errorf("health 探针不应记录访问日志，实际: %s", buf.String())
	}
}

// TestHandlerChainConcurrentRequestsUnderOneLimit 锁定限流并发不变量：
// 高并发下放行数不超过容量，且响应不撕裂。
func TestHandlerChainConcurrentRequestsUnderOneLimit(t *testing.T) {
	store := newTestStore(t, map[string]string{"高一.json": validGradeOne, "高二.json": validGradeTwo})
	clock := &fakeClock{current: time.Now()}
	captureLogs(t)

	const capacity = 3
	handler := newTestChain(clock, capacity, buildMux(store))

	var mu sync.Mutex
	allowed := 0
	rejected := 0
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=王", nil))
			mu.Lock()
			defer mu.Unlock()
			switch rec.Code {
			case http.StatusOK:
				allowed++
			case http.StatusTooManyRequests:
				rejected++
			}
		}()
	}
	wg.Wait()

	if allowed > capacity {
		t.Errorf("并发下放行数 %d 超过容量 %d", allowed, capacity)
	}
	if allowed+rejected != 20 {
		t.Errorf("放行 %d + 拒绝 %d 应等于 20", allowed, rejected)
	}
}
