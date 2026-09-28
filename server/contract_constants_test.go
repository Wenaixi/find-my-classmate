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

	pairs := []struct {
		frontend, backend string
		want              int
	}{
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

// parseFrontendPattern 抽出 src/lib/query.ts 中某条正则声明的值。
//
// 它与 TestContractConstantsMatchFrontend 的 extract 策略一致：锁的是值，
// 不是 TypeScript 的书写形式——容忍类型标注、行尾注释、尾随分号与排版。
//
// 两处与 extract 不同，都是这份文件里的实际形态逼出来的：
//
//  1. 必须还原 TypeScript 的转义。源码里的 "\\d+" 是一个反斜杠加 d，
//     读到的是源码文本而非字符串值，直接与后端的 "[0-9]+" 比较必然不等。
//
//  2. 声明可以是拼接式。classNumberPattern 写作
//     const classNumberPattern = "\\d+" + chineseNumberPattern;
//     右端不是字面量而是另一条常量，因此右段按常量名递归解析。
//     递归只沿本文件的常量声明进行，遇不到就报错而非静默跳过——
//     对拍跟丢了声明形态必须显式失败，那正是它该报告的漂移。
func parseFrontendPattern(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("../src/lib/query.ts")
	if err != nil {
		t.Fatalf("读取前端解析文件失败: %v", err)
	}
	src := string(raw)
	seen := make(map[string]bool)
	var resolve func(string) string
	resolve = func(constName string) string {
		if seen[constName] {
			t.Fatalf("前端声明 %s 存在循环引用，对拍无法确定其值", constName)
		}
		seen[constName] = true
		defer delete(seen, constName)

		// 匹配到行尾分号为止：右端可能是 "字面量 + 常量名" 的拼接链。
		pattern := regexp.MustCompile(`(?m)^\s*const\s+` + regexp.QuoteMeta(constName) + `\s*=\s*(.+?)\s*;\s*$`)
		match := pattern.FindStringSubmatch(src)
		if match == nil {
			t.Fatalf("前端声明 %s 未在 src/lib/query.ts 中找到（对拍读取的是该常量声明行的右端表达式）", constName)
		}

		// 只切分引号外的顶层加号。字符串字面量里的加号是正则量词
		// （"\\d+" 的尾随 +），不是拼接符——按裸 + 切会把字面量从中间截断。
		var builder strings.Builder
		for _, part := range splitConcatenation(match[1]) {
			part = strings.TrimSpace(part)
			switch {
			case strings.HasPrefix(part, `"`) && strings.HasSuffix(part, `"`):
				// strconv.Unquote 还原 TypeScript 字符串字面量：它接受的转义
				// （\" \\ \n \t \r）与 Go 字符串字面量一致，足以处理本文件的声明。
				value, err := strconv.Unquote(part)
				if err != nil {
					t.Fatalf("前端声明 %s 的字面量无法解析: %q: %v", constName, part, err)
				}
				builder.WriteString(value)
			case regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`).MatchString(part):
				builder.WriteString(resolve(part))
			default:
				t.Fatalf("前端声明 %s 的右端片段 %q 既不是字符串字面量也不是常量名（对拍只接受这两种拼接形态）", constName, part)
			}
		}
		return builder.String()
	}
	return resolve(name)
}

// splitConcatenation 按引号外的加号切分一条常量声明的右端表达式。
// 状态机扫字符并跟踪是否处于双引号字符串内（反斜杠转义使引号可以成对出现），
// 只有在引号外的加号才是拼接符。返回至少一个片段：空表达式返回单元素切片，
// 由调用方按「既不是字面量也不是常量名」报错，不在此处静默通过。
func splitConcatenation(expr string) []string {
	var parts []string
	start, inString, escaped := 0, false, false
	for i := range expr {
		c := expr[i]
		switch {
		case escaped:
			escaped = false
		case c == '\\' && inString:
			escaped = true
		case c == '"':
			inString = !inString
		case c == '+' && !inString:
			parts = append(parts, expr[start:i])
			start = i + 1
		}
	}
	return append(parts, expr[start:])
}

// TestChineseNumberPatternMatchesFrontend 对拍汉字数字的合法形态声明。
//
// chineseNumberPattern 编码班号的合法形态（单字 / 十开头 / 第二字为十）三种，
// 两端各持一份。它此前是纯手抄：解析层对拍只能看见「同一条查询在两端的
// 结果」，而这条声明的接受域一旦在某端放宽，两端在语料未覆盖的形态上
// 会一致地错，逐条对拍恒通过——第十三轮修复的静默截断就是这样长期
// 无前端对拍保护的。声明层对拍补的是这一层。
//
// 归一化说明：前端写 "\\d+|"、后端写 "[0-9]+|"，字面量文本不同但语义相同
// （JS 无 u 标志的 \d 与 Go 的 [0-9] 在 ASCII 数字上等价）。本对拍比较
// 归一化后的语义形式，因此把 \d 展开为 [0-9]。
//
// 表达力上限：只锁这两条声明。chineseNumberToInt、classDigits、
// splitGradeClass 是算法而非字面量，正则抽不出来，归契约语料覆盖；
// goSpaceChars 一侧是枚举而 Go 端用 unicode.IsSpace，根本没有第二份声明，
// 无法对拍——这三类都不在本对拍范围，不要因「已对拍」而以为已覆盖。
func TestChineseNumberPatternMatchesFrontend(t *testing.T) {
	got := normalizeDigitClass(parseFrontendPattern(t, "chineseNumberPattern"))
	if got != chineseNumberPattern {
		t.Errorf("汉字数字形态声明两端不一致:\n  前端 = %q\n  后端 = %q", got, chineseNumberPattern)
	}
}

// TestClassNumberPatternMatchesFrontend 对拍完整班号的形态声明。
// 它由 chineseNumberPattern 与数字部分拼接而成，因此单独锁一条拼接结果：
// 只锁 chineseNumberPattern 时，两端都把数字部分从 "+" 改成 "*" 这类改动
// 不会被发现。
func TestClassNumberPatternMatchesFrontend(t *testing.T) {
	got := normalizeDigitClass(parseFrontendPattern(t, "classNumberPattern"))
	want := normalizeDigitClass(classNumberPattern)
	if got != want {
		t.Errorf("班号形态声明两端不一致:\n  前端 = %q\n  后端 = %q", got, want)
	}
}

// normalizeDigitClass 把正则里的 \d 展开为 [0-9]，使书写形式不同而
// 语义相同的两端声明可比。它只做这一件事：字符类 \d 的替换，不触及
// 其他转义，因此不会把真正的差异抹平。
func normalizeDigitClass(pattern string) string {
	return strings.ReplaceAll(pattern, `\d`, "[0-9]")
}
