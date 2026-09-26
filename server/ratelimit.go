package main

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// rateBucket 单个客户端的令牌桶：容量 rate 个令牌，每 interval 补充 1 个。
type rateBucket struct {
	tokens   float64
	lastFill time.Time
	lastSeen time.Time
}

// rateLimiter 按客户端 IP 限流的令牌桶，容量与补充间隔可配。
// ponytail: 内存 map 实现，单实例够用；多副本部署时需换共享存储（如 Redis）。
//
// now 由构造函数注入：时间推进的测试装置不暴露为可写字段，
// 避免生产代码或测试从外部改写限流器的时间源。
type rateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*rateBucket
	capacity  float64
	interval  time.Duration
	lastSweep time.Time
	now       func() time.Time
}

func newRateLimiter(capacity float64, interval time.Duration, now func() time.Time) *rateLimiter {
	return &rateLimiter{
		buckets:  make(map[string]*rateBucket),
		capacity: capacity,
		interval: interval,
		now:      now,
	}
}

// fillTokens 惰性回补令牌，是令牌推进的纯函数：无锁、无副作用、不读时间源，
// 时间以参数进入（now），因此时钟回拨、零间隔与容量钳制都能被直接单测。
// 时钟回拨防护：单调时钟在容器休眠恢复/CRIU 迁移/虚拟化 TSC 校正时可能倒退，
// 负 elapsed 会把 tokens 拖成负数导致该 IP 被拒 B-capacity+1 秒。负 elapsed 钳制为 0。
func fillTokens(bucket *rateBucket, now time.Time, capacity float64, interval time.Duration) {
	if elapsed := now.Sub(bucket.lastFill); elapsed > 0 {
		bucket.tokens = min(capacity, bucket.tokens+elapsed.Seconds()/interval.Seconds())
	}
	bucket.lastFill = now
}

// allow 检查并消耗一个令牌。allowed=false 时 wait 为建议重试等待时间。
func (l *rateLimiter) allow(key string) (allowed bool, wait time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()

	// 惰性自驱动淘汰：若超过 1 分钟未清理，顺带淘汰超过 10 分钟无请求的旧桶，防内存单调泄漏
	if l.lastSweep.IsZero() {
		l.lastSweep = now
	} else if now.Sub(l.lastSweep) >= time.Minute {
		l.sweepLocked(now, 10*time.Minute)
		l.lastSweep = now
	}

	bucket, exists := l.buckets[key]
	if !exists {
		bucket = &rateBucket{tokens: l.capacity, lastFill: now, lastSeen: now}
		l.buckets[key] = bucket
	}
	// 回补交给纯函数 fillTokens：时钟回拨防护与容量钳制收在该函数内
	fillTokens(bucket, now, l.capacity, l.interval)
	bucket.lastSeen = now
	if bucket.tokens >= 1 {
		bucket.tokens--
		return true, 0
	}
	wait = time.Duration((1 - bucket.tokens) * l.interval.Seconds() * float64(time.Second))
	return false, wait
}

func (l *rateLimiter) sweepLocked(now time.Time, idleTTL time.Duration) {
	for key, bucket := range l.buckets {
		if now.Sub(bucket.lastSeen) > idleTTL {
			delete(l.buckets, key)
		}
	}
}

// rateLimitWith 组装限流中间件：每 IP 每秒 capacity 个请求的突发窗口
// （capacity 即令牌容量）。429 响应为 JSON（与全站错误格式一致），
// Retry-After 输出整数秒（RFC 9110）。
//
// 限流器由调用方构造并注入，生产与测试共用这一个装配点——生产传真实时钟
// 构造的限流器，测试传可推进时钟的限流器，其余 HTTP 行为完全一致。
// 此前存在一个零调用点的 rateLimit 薄包装（只做 newRateLimiter + 转发），
// 它让读者误以为生产装配走该入口，实际生产与测试都直接调用本函数。
func rateLimitWith(limiter *rateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, wait := limiter.allow(clientIP(r.RemoteAddr))
		if !allowed {
			writeRateLimited(w, wait)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// writeRateLimited 输出 429 响应：整数秒 Retry-After（至少 1，"0s" 语义自相矛盾）
// 与全站统一的 JSON 错误体。独立成函数使限流响应只有一个事实源。
//
// 安全响应头不再在此重放：中间件链把 securityHeaders 置于 rateLimit 外侧
// （main.go newHandlerChain），429 短路响应自动继承全站安全头契约。
func writeRateLimited(w http.ResponseWriter, wait time.Duration) {
	seconds := int(math.Ceil(wait.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeError(w, errCodeRateLimited)
}
