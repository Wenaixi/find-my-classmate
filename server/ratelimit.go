package main

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateBucket 单个客户端的令牌桶：容量 rate 个令牌，每 interval 补充 1 个。
type rateBucket struct {
	tokens   float64
	lastFill time.Time
}

// rateLimiter 按客户端 IP 限流的令牌桶，容量与补充间隔可配。
// ponytail: 内存 map 实现，单实例够用；多副本部署时需换共享存储（如 Redis）。
type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*rateBucket
	capacity float64
	interval time.Duration
	now      func() time.Time
}

func newRateLimiter(capacity float64, interval time.Duration) *rateLimiter {
	return &rateLimiter{
		buckets:  make(map[string]*rateBucket),
		capacity: capacity,
		interval: interval,
		now:      time.Now,
	}
}

// allow 检查并消耗一个令牌。allowed=false 时 wait 为建议重试等待时间。
func (l *rateLimiter) allow(key string) (allowed bool, wait time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	bucket, exists := l.buckets[key]
	if !exists {
		bucket = &rateBucket{tokens: l.capacity, lastFill: now}
		l.buckets[key] = bucket
	}
	elapsed := now.Sub(bucket.lastFill)
	bucket.tokens = min(l.capacity, bucket.tokens+elapsed.Seconds()/l.interval.Seconds())
	bucket.lastFill = now
	if bucket.tokens >= 1 {
		bucket.tokens--
		return true, 0
	}
	wait = time.Duration((1 - bucket.tokens) * l.interval.Seconds() * float64(time.Second))
	return false, wait
}

// clientIP 提取客户端 IP（IPv6 去掉端口与 zone）。
func clientIP(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		return ip.String()
	}
	return host
}

// rateLimit 中间件：每 IP 每秒 capacity 个请求的突发窗口（capacity 即令牌容量）。
func rateLimit(next http.Handler, capacity float64, interval time.Duration) http.Handler {
	limiter := newRateLimiter(capacity, interval)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, wait := limiter.allow(clientIP(r.RemoteAddr))
		if !allowed {
			w.Header().Set("Retry-After", wait.Round(time.Second).String())
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
