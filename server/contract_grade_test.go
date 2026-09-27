package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestGradeDomainMatchesFrontend 对拍年段值域：后端 knownGrades 与前端
// query.ts 的 gradeValues 必须是同一份年段清单。
//
// 为什么需要它：年段值域是跨语言手抄的（Go 与 TypeScript 无法共享声明），
// 而解析层已有的对拍（docs/query-contract.json）表达不了清单本身——
// 语料只在恰好含有该年段样本时才拦得住。两端一致地不认识新年段时，
// 逐条断言的语料同样通过。因此清单漂移需要一条独立的对拍。
//
// 与 docs/query-contract.json 的分工：那份对拍比「同一条查询在两端的解析结果」，
// 本条比「声明本身」。后者是前者的输入，前者对了不代表后者没漂移。
//
// 表达力上限（不可通过加断言消除）：两端同时缺少某个年段时，本条恒通过。
// 它拦得住的是「只改了一端」——那才是扩展年段时真实发生的形态。
// 两端一致地不认识仍是结构性盲区，由 types.ts 的编译期反向完整性检查
// 覆盖前端内部的一致性，但跨语言这一侧无解。
func TestGradeDomainMatchesFrontend(t *testing.T) {
	frontend := parseFrontendGradeValues(t)

	// 方向一：后端声明的每个年段，前端必须也已声明。
	// 扩展年段时先改 knownGrades 是最常见的形态（数据文件先到位），
	// 漏改前端则 parseQuery 不认识该年段，查询静默返回 0 条。
	for _, grade := range knownGrades {
		if _, ok := frontend[grade]; !ok {
			t.Errorf("后端 knownGrades 含 %q，前端 gradeValues 未声明（前端解析将不识别该年段）", grade)
		}
	}

	// 方向二：前端声明的每个年段，后端必须也已声明。
	// 漏改后端则名单文件循环探测不到该年段，文件放在目录里也不会被加载。
	for grade := range frontend {
		found := false
		for _, g := range knownGrades {
			if g == grade {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("前端 gradeValues 含 %q，后端 knownGrades 未声明（该年段名单文件不会被加载）", grade)
		}
	}
}

// parseFrontendGradeValues 从 src/lib/query.ts 抽出 gradeValues 的年段清单。
//
// 识别策略与 contract_constants_test.go 一致：锁定「值」而不是 TypeScript 的
// 书写形式，容忍纯排版改动。gradeValues 用 as const satisfies 收窄类型，
// 抽取只认字符串字面量数组本身，as const 与 satisfies 都不影响抽取结果。
// 因此前端换用别的类型收窄手法不会让本对拍失败——它只关心声明了哪些年段。
func parseFrontendGradeValues(t *testing.T) map[Grade]struct{} {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "src", "lib", "query.ts"))
	if err != nil {
		t.Fatalf("读取前端年段声明文件失败: %v", err)
	}
	// 数组可能跨行书写，因此匹配到第一个右方括号为止，不按行锚定尾部。
	pattern := regexp.MustCompile(`(?s)const\s+gradeValues\s*(?::[^=]+)?=\s*\[(.*?)\]`)
	match := pattern.FindSubmatch(raw)
	if match == nil {
		t.Fatal("前端 gradeValues 声明未在 src/lib/query.ts 中找到（对拍读取的是该声明的数组字面量）")
	}
	grades := make(map[Grade]struct{})
	// 逐个引号字面量切分。容忍空白、换行与尾随逗号：只取字面量本身。
	for _, item := range regexp.MustCompile(`"([^"]*)"`).FindAllSubmatch(match[1], -1) {
		name := Grade(strings.TrimSpace(string(item[1])))
		if name == "" {
			continue
		}
		grades[name] = struct{}{}
	}
	if len(grades) == 0 {
		t.Fatal("前端 gradeValues 存在但未抽出任何年段（对拍只认双引号字符串字面量）")
	}
	return grades
}
