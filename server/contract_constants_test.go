package main

import (
	"os"
	"regexp"
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

	// 提取前端常量数值。契约要锁定的是「值」，不是 TypeScript 的书写形式：
	// 识别时容忍空白、类型标注与尾随逗号，纯排版或格式器改动不应让对拍失败。
	// 数字分隔符（10_000）属于字面量本身，解析时按 Go 侧习惯去掉下划线。
	extract := func(name string) int {
		pattern := regexp.MustCompile(`(?m)^\s*export\s+const\s+` + regexp.QuoteMeta(name) + `\s*(?::[^=]+)?=\s*(.+?)\s*,?\s*$`)
		match := pattern.FindStringSubmatch(src)
		if match == nil {
			t.Fatalf("前端常量 %s 未在 src/config.ts 中找到（对拍读取的是常量声明行）", name)
		}
		// 去掉行尾注释、尾随分号/逗号与数字分隔符，剩下裸字面量再解析。
		literal := strings.TrimSpace(strings.SplitN(match[1], "//", 2)[0])
		literal = strings.TrimRight(literal, " \t,;")
		literal = strings.ReplaceAll(literal, "_", "")
		n, err := strconv.Atoi(literal)
		if err != nil {
			t.Fatalf("前端常量 %s 存在但字面量无法解析: %q（对拍只接受十进制整数字面量）", name, literal)
		}
		return n
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
