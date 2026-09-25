package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestContractConstantsMatchFrontend 跨语言常量对拍：前端 src/config.ts 的
// PAGE_SIZE / MAX_QUERY_LENGTH / MAX_LIMIT 必须与后端 config.go 的
// defaultLimit / maxQueryRunes / maxLimit 一致。双端常量无法跨语言共享，
// 本测试与 docs/query-contract.json 同一机制：任一侧漂移立即失败。
func TestContractConstantsMatchFrontend(t *testing.T) {
	configTS, err := os.ReadFile("../src/config.ts")
	if err != nil {
		t.Fatalf("读取前端常量文件失败: %v", err)
	}
	src := string(configTS)

	// 提取前端常量数值：逐行解析 export const NAME = value;
	extract := func(name string) int {
		for _, line := range strings.Split(src, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "export const "+name+" = ") {
				value := strings.TrimSuffix(strings.TrimPrefix(line, "export const "+name+" = "), ";")
				value = strings.ReplaceAll(value, "_", "")
				n, err := strconv.Atoi(value)
				if err != nil {
					t.Fatalf("解析 %s = %q 失败: %v", name, value, err)
				}
				return n
			}
		}
		t.Fatalf("前端常量 %s 未找到", name)
		return 0
	}

	pairs := []struct{ frontend, backend string; want int }{
		{"PAGE_SIZE", "defaultLimit", defaultLimit},
		{"MAX_QUERY_LENGTH", "maxQueryRunes", maxQueryRunes},
		{"MAX_LIMIT", "maxLimit", maxLimit},
	}
	for _, p := range pairs {
		if got := extract(p.frontend); got != p.want {
			t.Errorf("%s = %d，与后端 %s = %d 不一致（双端契约常量必须同步）", p.frontend, got, p.backend, p.want)
		}
	}
}
