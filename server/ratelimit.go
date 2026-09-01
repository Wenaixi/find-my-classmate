package main

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// rateBucket 单个客户端的令牌桶：容量 rate 个令牌，每 interval 补充 1 个。
type rateBucket struct {
	tokens    float64
	lastFill  time.Time
	lastSeen  time.Time
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
		bucket = &rateBucket{tokens: l.capacity, lastFill: now, lastSeen: now}
		l.buckets[key] = bucket
	}
	// 时钟回拨防护：单调时钟在容器休眠恢复/CRIU 迁移/虚拟化 TSC 校正时可能倒退，
	// 负 elapsed 会把 tokens 拖成负数导致该 IP 被拒 B-capacity+1 秒。钳制到 0。
	if elapsed := now.Sub(bucket.lastFill); elapsed > 0 {
		bucket.tokens = min(l.capacity, bucket.tokens+elapsed.Seconds()/l.interval.Seconds())
	}
	bucket.lastFill = now
	bucket.lastSeen = now
	if bucket.tokens >= 1 {
		bucket.tokens--
		return true, 0
	}
	wait = time.Duration((1 - bucket.tokens) * l.interval.Seconds() * float64(time.Second))
	return false, wait
}

// sweep 清理超过 idleTTL 未访问的空闲桶，防止多源扫描导致内存无限增长。
func (l *rateLimiter) sweep(now time.Time, idleTTL time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, bucket := range l.buckets {
		if now.Sub(bucket.lastSeen) > idleTTL {
			delete(l.buckets, key)
		}
	}
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
// 429 响应为 JSON（与全站错误格式一致），Retry-After 输出整数秒（RFC 9110）。
func rateLimit(next http.Handler, capacity float64, interval time.Duration) http.Handler {
	limiter := newRateLimiter(capacity, interval)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, wait := limiter.allow(clientIP(r.RemoteAddr))
		if !allowed {
			// 整数秒，至少 1（"0s" 语义自相矛盾）
			seconds := int(math.Ceil(wait.Seconds()))
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
