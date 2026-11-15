package main

import (
	"testing"
	"time"
)

// rateLimitFromEnv 从环境变量读取限流参数。本用例锁四条路径：
// 未设置 → 默认值；合法值 → 生效；非正数或解析失败 → 回退默认；只设一个 → 另一个保持默认。
// t.Setenv 在用例结束后自动还原环境变量，不污染其他用例。
func TestRateLimitFromEnv(t *testing.T) {
	cases := []struct {
		name       string
		capacity   string
		intervalMS string
		wantCap    float64
		wantInt    time.Duration
	}{
		{"默认值", "", "", 60, time.Second},
		{"合法值放大", "600", "100", 600, 100 * time.Millisecond},
		{"容量非数字回退", "abc", "100", 60, 100 * time.Millisecond},
		{"容量为0回退", "0", "100", 60, 100 * time.Millisecond},
		{"容量为负回退", "-10", "100", 60, 100 * time.Millisecond},
		{"间隔非数字回退", "600", "xyz", 600, time.Second},
		{"间隔为0回退", "600", "0", 600, time.Second},
		{"间隔为负回退", "600", "-1", 600, time.Second},
		{"只设容量", "300", "", 300, time.Second},
		{"只设间隔", "", "250", 60, 250 * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FMC_RATE_LIMIT_CAPACITY", tc.capacity)
			t.Setenv("FMC_RATE_LIMIT_INTERVAL_MS", tc.intervalMS)
			gotCap, gotInt := rateLimitFromEnv()
			if gotCap != tc.wantCap {
				t.Errorf("capacity = %v，期望 %v", gotCap, tc.wantCap)
			}
			if gotInt != tc.wantInt {
				t.Errorf("interval = %v，期望 %v", gotInt, tc.wantInt)
			}
		})
	}
}
