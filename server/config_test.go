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

// envBool 的取值域：1/true/yes/on 为真，其余（含空、0、false、大小写变体）为假。
func TestEnvBool(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"", false}, {"0", false}, {"false", false}, {"no", false}, {"off", false},
		{"1", true}, {"true", true}, {"TRUE", true}, {"yes", true}, {"on", true}, {"On", true},
	}
	for _, c := range cases {
		t.Setenv("FMC_TRUST_PROXY_HEADERS", c.value)
		if got := envBool("FMC_TRUST_PROXY_HEADERS"); got != c.want {
			t.Errorf("envBool(%q) = %v，期望 %v", c.value, got, c.want)
		}
	}
}

// trustProxyHeaders 装配默认值承重：包级变量在 init 时读环境变量一次性初始化，
// 测试进程默认不设置 FMC_TRUST_PROXY_HEADERS，故初始值必须为 false（安全关键：
// 默认关 = 不信任转发头，直连伪造无法生效）。变异把初始化替换为 true 时本测试翻红。
func TestTrustProxyHeadersAssemblyDefaultFalse(t *testing.T) {
	if trustProxyHeaders {
		t.Error("trustProxyHeaders 默认应为 false（不信任转发头），实际为 true")
	}
}
