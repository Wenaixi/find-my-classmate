package main

import (
	"net/http"
	"testing"
	"time"
)

// 超时期望与实现共用同一组具名常量：断言「实现等于常量」不构成恒真——
// 常量改值时本用例随之移动、改的是期望值，而实现改为内联字面量时本用例立即
// 翻红。此前此处用 10s/15s/60s/5s 复述实现里的字面量，两处可以各改一处而不冲突，
// 断言看似承重却与实现无共同事实源。
func TestBuildServerConfiguresTimeouts(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	server := buildServer(":0", handler)

	if server.ReadTimeout != readTimeout {
		t.Errorf("ReadTimeout = %v，期望 %v", server.ReadTimeout, readTimeout)
	}
	if server.WriteTimeout != writeTimeout {
		t.Errorf("WriteTimeout = %v，期望 %v", server.WriteTimeout, writeTimeout)
	}
	if server.IdleTimeout != idleTimeout {
		t.Errorf("IdleTimeout = %v，期望 %v", server.IdleTimeout, idleTimeout)
	}
	if server.ReadHeaderTimeout != readHeaderTimeout {
		t.Errorf("ReadHeaderTimeout = %v，期望 %v", server.ReadHeaderTimeout, readHeaderTimeout)
	}
	// 反向断言：超时值必须为正且落在合理量级，否则「等于常量」可能两侧同为 0
	// 而恒真——那正是本仓反复出现的一类假保护。
	for name, value := range map[string]time.Duration{
		"ReadTimeout": server.ReadTimeout, "WriteTimeout": server.WriteTimeout,
		"IdleTimeout": server.IdleTimeout, "ReadHeaderTimeout": server.ReadHeaderTimeout,
	} {
		if value <= 0 {
			t.Errorf("%s = %v，期望为正值", name, value)
		}
	}
}
