package main

import (
	"os"
	"strconv"
	"time"
)

// 后端契约常量：数值的唯一事实源。改动须同步前端 src/config.ts 的对应常量
// （80 字上限、limit 默认值等双端契约常量无法跨语言共享，两处各一份，docs/ARCHITECTURE.md 已注明）。

const (
	// defaultPort 服务监听端口（默认 3078；docker-compose/Dockerfile 的部署端口描述部署形态，不在此收敛）。
	defaultPort = "3078"
	// defaultLimit / maxLimit 分页默认与上限（前端 PAGE_SIZE 同步）。
	defaultLimit = 10
	maxLimit     = 50
	// maxQueryRunes 查询串 rune 上限（前端 MAX_QUERY_LENGTH 同步，App.tsx maxLength 同值）。
	maxQueryRunes = 80
	// rateCapacity / rateInterval 限流令牌桶：突发 capacity / 每秒回补 1 个。
	rateCapacity = 60
	rateInterval = time.Second
	// assetCacheMaxAge 静态资源长缓存头（构建产物哈希命名，immutable）。
	assetCacheMaxAge = "public, max-age=31536000, immutable"
)

// rateLimitFromEnv 从环境变量读取限流参数，供装配点注入：
// FMC_RATE_LIMIT_CAPACITY 令牌桶容量，默认 rateCapacity；
// FMC_RATE_LIMIT_INTERVAL_MS 补充一个令牌所需的毫秒数，默认 rateInterval。
// 环境变量化从 1.0.0-beta.2 起提供：反代共享桶场景（所有经 Nginx 进来的请求
// RemoteAddr 均为 127.0.0.1，全校共用一个桶）下，运维在 compose 设置环境变量
// 即可放宽，无需重新构建镜像。解析失败或非正数时回退默认值，保证部署误配时
// 服务不崩、行为可预期。
func rateLimitFromEnv() (float64, time.Duration) {
	capacity := float64(rateCapacity)
	if v := os.Getenv("FMC_RATE_LIMIT_CAPACITY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			capacity = float64(n)
		}
	}
	interval := rateInterval
	if v := os.Getenv("FMC_RATE_LIMIT_INTERVAL_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			interval = time.Duration(n) * time.Millisecond
		}
	}
	return capacity, interval
}
