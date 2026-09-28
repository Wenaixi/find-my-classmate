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

// parseFrontendGradeAliases 抽出前端 gradeAliases 的「别名 → 规范名」映射。
//
// 与 gradeValues 的抽取同策略：只认双引号字符串字面量，按出现顺序两两配对，
// 容忍空白、换行与尾随逗号——对拍锁的是声明的值，不是 TypeScript 的书写形式，
// 因此前端换用别的类型收窄手法不会让对拍失败。
//
// 形如 ["高1", "高一"] 的二元组数组，抽出的引号序列按偶数位配对。
// 若将来别名声明改为对象形态或引入非字面量，本函数会因抽不出条目而失败——
// 那是对拍跟丢了声明形态的信号，不是静默通过。
func parseFrontendGradeAliases(t *testing.T) map[string]Grade {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "src", "lib", "query.ts"))
	if err != nil {
		t.Fatalf("读取前端年段声明文件失败: %v", err)
	}
	pattern := regexp.MustCompile(`(?s)const\s+gradeAliases\s*(?::[^=]+)?=\s*\[(.*?)\n\]`)
	match := pattern.FindSubmatch(raw)
	if match == nil {
		t.Fatal("前端 gradeAliases 声明未在 src/lib/query.ts 中找到（对拍读取的是该声明的数组字面量）")
	}
	literals := regexp.MustCompile(`"([^"]*)"`).FindAllSubmatch(match[1], -1)
	if len(literals)%2 != 0 {
		t.Fatalf("前端 gradeAliases 抽出 %d 个字符串字面量，不是偶数，无法按二元组配对", len(literals))
	}
	aliases := make(map[string]Grade, len(literals)/2)
	for i := 0; i+1 < len(literals); i += 2 {
		alias := strings.TrimSpace(string(literals[i][1]))
		canonical := Grade(strings.TrimSpace(string(literals[i+1][1])))
		if alias == "" || canonical == "" {
			continue
		}
		aliases[alias] = canonical
	}
	if len(aliases) == 0 {
		t.Fatal("前端 gradeAliases 存在但未抽出任何别名（对拍只认双引号字符串字面量）")
	}
	return aliases
}

// TestGradeAliasesMatchFrontend 对拍年段别名映射：后端 gradeAliases 与前端
// query.ts 的 gradeAliases 必须是同一份声明。
//
// 为什么它与 TestGradeDomainMatchesFrontend 并存而不合并：规范名清单与别名映射
// 是两份独立声明，声明形态不同（集合 vs 映射），漏改的形态也不同——前者漏改会让
// 该年段被两端同时忽略，后者漏改只影响前端对别名的识别。
//
// 它拦得住的具体形态：扩展年段时只在一端追加别名。这是真实发生的形态，
// 因为别名表是「加别名时才想起补」的那类声明。
//
// 表达力上限（不可通过加断言消除）：
//   - 两端同时漏加同一条别名时本条恒通过（同 TestGradeDomainMatchesFrontend）。
//   - 本条只锁声明集合，不锁行为。行为由 docs/query-contract.json 的别名样本
//     （「高1」「高2」「高3」）覆盖，但那些样本是手写的：两端同时删掉同一条别名
//     而语料未同步，对拍恒通过。
func TestGradeAliasesMatchFrontend(t *testing.T) {
	frontend := parseFrontendGradeAliases(t)

	// 方向一：后端声明的每个别名，前端必须也已声明。
	for alias, canonical := range backendGradeAliases() {
		got, ok := frontend[alias]
		if !ok {
			t.Errorf("后端 gradeAliases 含 %q → %q，前端 gradeAliases 未声明（前端会把该别名当姓名条件）",
				alias, canonical)
			continue
		}
		if got != canonical {
			t.Errorf("别名 %q 的规范名两端不一致：后端 %q，前端 %q", alias, canonical, got)
		}
	}

	// 方向二：前端声明的每个别名，后端必须也已声明。
	// 漏改后端则别名被当姓名条件，全年级查询退化为精确姓名查询。
	for alias, canonical := range frontend {
		if _, ok := backendGradeAliases()[alias]; !ok {
			t.Errorf("前端 gradeAliases 含 %q → %q，后端 gradeAliases 未声明（该别名不参与年段解析）",
				alias, canonical)
		}
	}
}

// TestGradeAliasTargetsAreDeclaredYears 别名的规范名目标必须落在规范名清单内。
// 这一条不必跨语言：两端各自的 aliases 表若指向一个未声明的规范名，
// 该别名就指向了一个不存在的年段。TypeScript 侧由 as const satisfies 强制，
// Go 侧没有等价机制，因此在此补上——它不比较两端，只校验后端声明自身自洽。
func TestGradeAliasTargetsAreDeclaredYears(t *testing.T) {
	declared := make(map[Grade]struct{}, len(knownGrades))
	for _, g := range knownGrades {
		declared[g] = struct{}{}
	}
	for alias, canonical := range backendGradeAliases() {
		if _, ok := declared[canonical]; !ok {
			t.Errorf("别名 %q 指向规范名 %q，但该年段不在 knownGrades 中（别名指向不存在的年段）", alias, canonical)
		}
	}
}

// backendGradeAliases 把后端的匿名结构体切片转成映射，供对拍与自洽校验复用。
// 转换一次而非在两处重复遍历：映射形态让「某别名缺失」成为一次存在性判定。
func backendGradeAliases() map[string]Grade {
	aliases := make(map[string]Grade, len(gradeAliases))
	for _, a := range gradeAliases {
		aliases[a.name] = a.grade
	}
	return aliases
}
