package main

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

// sameStringData 判定两个字符串是否共享同一底层数组（用于零拷贝断言）。
func sameStringData(a, b string) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	return unsafe.StringData(a) == unsafe.StringData(b)
}

func testStudents() []Student {
	return []Student{
		newStudent("示例同学", GradeThree, "18班", parseClassName("18班")),
		newStudent("示 例 同 学", GradeOne, "6班", parseClassName("6班")),
		newStudent("EXAMPLE STUDENT", GradeOne, "11班", parseClassName("11班")),
	}
}

func TestSearchRules(t *testing.T) {
	tests := []struct {
		name, query string
		want        int
	}{
		{"姓名去空格", "示 例", 2},
		{"组合筛选", "高三, 示例同学", 1},
		{"加号组合筛选", "高三+示例同学+18班", 1},
		{"班级数字", "18", 1},
		{"中文班级", "六班", 1},
		{"年级数字别名", "高1", 2},
		{"混合分隔符", "高一，六班", 1},
		{"高一筛选", "高一", 2},
		{"高三筛选", "高三", 1},
		{"高三数字别名", "高3", 1},
		// 纯数字按班级解析而非姓名：契约语料声明「223」得到 classNumber=223，
		// 本 fixture 没有 223 班，因此命中 0 条。原用例名「纯数字按姓名处理」
		// 宣称走姓名路径，与实现相反——按名字读代码会误判班级降级策略。
		{"超出班级域的数字班级无匹配", "223", 0},
		{"空输入", "", 0},
		{"纯分隔符顿号", "、", 0},
		{"纯分隔符英文逗号", ",", 0},
		{"纯分隔符加号", "+", 0},
		{"纯分隔符中文逗号", "，", 0},
		{"纯分隔符连写", ",,,", 0},
		{"空白与分隔符混合", "  、  ", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Search(testStudents(), tt.query, 10, 0)
			if len(got.Items) != tt.want {
				t.Fatalf("got %d want %d", len(got.Items), tt.want)
			}
		})
	}
}

// 上一条表里「超出班级域的数字班级无匹配」断言的是结果条数，
// 而「223 究竟是班级还是姓名」由解析层决定。此处把它钉住，
// 使读者不必去翻契约语料才知道那个 0 条意味着什么。
func TestPureDigitsParseAsClassNumber(t *testing.T) {
	q := parseQuery("223")
	if q.ClassNo != 223 {
		t.Errorf("parseQuery(\"223\").ClassNo = %d，期望 223（纯数字按班级解析）", q.ClassNo)
	}
	if len(q.NameTokens) != 0 {
		t.Errorf("parseQuery(\"223\").NameTokens = %v，期望空（不应降级为姓名条件）", q.NameTokens)
	}
}

func TestSearchPagination(t *testing.T) {
	students := append(testStudents(), testStudents()...)
	first := Search(students, "示例", 2, 0)
	second := Search(students, "示例", 2, 2)
	if first.Total != 4 || len(first.Items) != 2 || !first.HasMore {
		t.Fatalf("unexpected first page: total=%d items=%d hasMore=%v", first.Total, len(first.Items), first.HasMore)
	}
	if second.Offset != 2 || len(second.Items) != 2 || second.HasMore {
		t.Fatalf("unexpected second page: offset=%d items=%d hasMore=%v", second.Offset, len(second.Items), second.HasMore)
	}
	if second.Items[0].Name == first.Items[0].Name || second.Items[1].Name == first.Items[1].Name {
		t.Fatal("pagination repeated an item")
	}
	last := Search(students, "示例", 2, 999999999999)
	if last.Offset != 4 || len(last.Items) != 0 || last.HasMore {
		t.Fatalf("unexpected out-of-range page: offset=%d items=%d hasMore=%v", last.Offset, len(last.Items), last.HasMore)
	}
}

// Search 必须自守分页前置约定：offset 早于列表开头时按第一页处理。
// 该约定此前只由 buildMux 的 HTTP 层校验兜底，Search 自身对负 offset 会
// panic（slice bounds out of range）。把不变量收进接口，调用者无需记忆。
func TestSearchNegativeOffsetTreatedAsFirstPage(t *testing.T) {
	students := append(testStudents(), testStudents()...)
	got := Search(students, "示例", 2, -3)
	if got.Offset != 0 {
		t.Fatalf("负 offset 应归一到 0，实际 %d", got.Offset)
	}
	first := Search(students, "示例", 2, 0)
	if len(got.Items) != len(first.Items) {
		t.Fatalf("负 offset 与 offset=0 结果条目数应一致：%d vs %d", len(got.Items), len(first.Items))
	}
	if got.Total != first.Total || got.HasMore != first.HasMore {
		t.Fatalf("负 offset 与 offset=0 的分页元数据应一致：total %d/%d hasMore %v/%v",
			got.Total, first.Total, got.HasMore, first.HasMore)
	}
}

// 与负 offset 对称：Search 声明自守分页前置约定，limit 侧必须同样自守。
// 负 limit 会让 end = offset + limit 变为负下界，matches[offset:end] 触发
// panic（slice bounds out of range [:-1]）。修复前只钳制了 offset，
// 承诺只兑现了一半——调用者必须记忆「limit 不能为负」才能安全使用该接口。
func TestSearchNegativeLimitDoesNotPanic(t *testing.T) {
	students := append(testStudents(), testStudents()...)
	got := Search(students, "示例", -1, 0)
	if got.Total == 0 {
		t.Fatalf("负 limit 不应清空结果集，total=%d", got.Total)
	}
	first := Search(students, "示例", defaultLimit, 0)
	if got.Limit < 0 {
		t.Fatalf("Limit 回显不应为负，实际 %d", got.Limit)
	}
	if len(got.Items) > len(first.Items) {
		t.Fatalf("负 limit 归一后不应返回超过默认页的条目数：%d vs %d",
			len(got.Items), len(first.Items))
	}
}

// 空查询路径此前在两个钳制之前提前返回，使「Search 自守分页前置约定」
// 只在非空路径成立：负 limit/offset 会原样回显到响应里。调用方读响应的
// offset 字段就必须知道这个保证是有条件的，接口因此不是自守的。
func TestSearchBlankQueryClampsPagination(t *testing.T) {
	got := Search(testStudents(), "", -3, -5)
	if got.Limit != 0 {
		t.Errorf("空查询的负 limit 应归一到 0，实际 %d", got.Limit)
	}
	if got.Offset != 0 {
		t.Errorf("空查询的负 offset 应归一到 0，实际 %d", got.Offset)
	}
}

// 汉字多位班级号（十一~九十九）应解析为数值
func TestClassNumberChineseMultiDigit(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"11班", 11},
		{"十一班", 11},
		{"十二班", 12},
		{"二十班", 20},
		{"二十一班", 21},
		{"三十班", 30},
		{"十八班", 18},
		{"十班", 10},
		{"六班", 6},
		{"1班", 1},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			parsed := parseClassName(c.in)
			if !parsed.Valid || parsed.Overflow {
				t.Fatalf("汉字多位数班级 %q 应解析为合法: %+v", c.in, parsed)
			}
			if parsed.ClassNo != c.want {
				t.Fatalf("parseClassName(%q).ClassNo = %d, want %d", c.in, parsed.ClassNo, c.want)
			}
		})
	}
}

// 查询 "十一班" 应命中 11 班而不是全校
func TestSearchChineseMultiDigitClass(t *testing.T) {
	// fixture 中 11 班有 EXAMPLE STUDENT（1 条）；18 班与 6 班不应命中
	got := Search(testStudents(), "十一班", 10, 0)
	if len(got.Items) != 1 {
		t.Fatalf("查询十一班应精确命中 1 条（11 班），实际 %d", len(got.Items))
	}
	if got.Items[0].ClassName != "11班" {
		t.Errorf("命中班级 = %s，期望 11班", got.Items[0].ClassName)
	}
}

// 超长数字班级串应被安全处理（不 panic、按姓名处理返回空）。
// 三态化后溢出由 parseClassName().Overflow 显式表达。
// 此用例此前还断言薄包装 classNumber 溢出返回 0——那是测试自身的行为
// （该薄包装生产零调用），已随薄包装一并删除。
func TestClassNumberOverflowSafe(t *testing.T) {
	parsed := parseClassName("99999999999999999999班")
	if parsed.Valid || !parsed.Overflow {
		t.Fatalf("超长数字应 Overflow=true, Valid=false，实际 %+v", parsed)
	}
	// 查询超长数字不应 panic；按姓名处理（姓名不含数字）应返回空结果
	got := Search(testStudents(), "99999999999999999999", 10, 0)
	if len(got.Items) != 0 {
		t.Fatalf("超长数字查询应返回空，实际 %d", len(got.Items))
	}
}

// 姓名包含"高"/"班"字不应被误判为年级/班级（回归保护）
func TestNameTokensNotMisparsed(t *testing.T) {
	students := []Student{
		newStudent("高翔", GradeOne, "1班", parseClassName("1班")),
		newStudent("班长", GradeTwo, "2班", parseClassName("2班")),
	}
	if got := Search(students, "高翔", 10, 0); len(got.Items) != 1 {
		t.Errorf("查询高翔应命中 1 条（作为姓名），实际 %d", len(got.Items))
	}
	if got := Search(students, "班长", 10, 0); len(got.Items) != 1 {
		t.Errorf("查询班长应命中 1 条（作为姓名），实际 %d", len(got.Items))
	}
}

// 年级+班级连写输入（"高三三班"）精确解析为年段+班级：
// 旧语义按年级子串处理返回全年级，现改为精确班级筛选（用户报告缺陷）。
func TestGradeSubstringBehavior(t *testing.T) {
	// C5：解析语义独立暴露——Search 不再返回 Query，断言直接走 parseQuery
	q := parseQuery("高三三班")
	if q.Grade != GradeThree {
		t.Errorf("高三三班 应解析为年级=高三，实际 %q", q.Grade)
	}
	if q.ClassNo != 3 {
		t.Errorf("高三三班 应解析出班级=3，实际 %d", q.ClassNo)
	}
	// fixture 中高三只有 18 班，三班应精确命中 0 条（不再返回整个高三年段）
	got := Search(testStudents(), "高三三班", 10, 0)
	if len(got.Items) != 0 {
		t.Errorf("高三三班 应精确命中 0 条（fixture 高三无三班），实际 %d", len(got.Items))
	}
}

// 年级+班级连写（"高二三班"/"高二1班"）精确筛选对应班级的人
func TestGradeClassCompoundPrecise(t *testing.T) {
	students := []Student{
		newStudent("甲", GradeTwo, "1班", parseClassName("1班")),
		newStudent("乙", GradeTwo, "2班", parseClassName("2班")),
		newStudent("丙", GradeOne, "1班", parseClassName("1班")),
		newStudent("丁", GradeTwo, "12班", parseClassName("12班")),
	}
	cases := []struct {
		query string
		want  string
	}{
		{"高二一班", "甲"},
		{"高二1班", "甲"},
		{"高一1班", "丙"},
		{"高二十二班", "丁"},
		{"高二，1班", "甲"},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			// C5：解析语义独立暴露——Search 不再返回 Query，断言直接走 parseQuery
			q := parseQuery(c.query)
			if c.query == "高一1班" && q.Grade != GradeOne {
				t.Fatalf("%s 年级 = %q，期望 高一", c.query, q.Grade)
			}
			if c.query != "高一1班" && q.Grade != GradeTwo {
				t.Fatalf("%s 年级 = %q，期望 高二", c.query, q.Grade)
			}
			got := Search(students, c.query, 10, 0)
			if len(got.Items) != 1 || got.Items[0].Name != c.want {
				t.Fatalf("%s 应精确命中 %s，实际 %+v", c.query, c.want, got.Items)
			}
		})
	}
}

// Search 必须对任意输入顺序产出规定顺序，不依赖调用方已排好序。
//
// 此前无任何断言覆盖排序的第三键（班级号升序）。原因不是第三键无足轻重，
// 而是所有既有 fixture 都恰好已按班号升序构造（loadStudents 在 data.go:95
// 排序班名后追加学生，knownGrades 按自然序循环加载年段文件），
// 于是 slices.SortStableFunc 对已序输入是恒等操作，第三键的缺失无从暴露。
// 但 Search 的调用方只有 api.go 一处、传的是 store.view()，
// 「输入已排序」这一前置性质从未作为契约写进任何地方——
// 一旦有人调整 loadStudents 的追加顺序（如改为并发收集、或按姓名去重后重排），
// 结果顺序会静默改变而全部测试仍然通过。
//
// 本用例刻意传入乱序输入，把「Search 自守排序」钉成接口事实。
func TestSearchSortsUnorderedInput(t *testing.T) {
	students := []Student{
		newStudent("张甲", GradeThree, "9班", parseClassName("9班")),
		newStudent("张乙", GradeOne, "3班", parseClassName("3班")),
		newStudent("张丙", GradeOne, "7班", parseClassName("7班")),
		newStudent("张丁", GradeOne, "1班", parseClassName("1班")),
	}
	got := Search(students, "张", 10, 0)
	if len(got.Items) != 4 {
		t.Fatalf("应返回 4 条，实际 %d", len(got.Items))
	}
	// 姓名单命中时全部同分，顺序由「年级声明序 → 班级号升序」决定。
	wantGrades := []Grade{GradeOne, GradeOne, GradeOne, GradeThree}
	wantClasses := []int{1, 3, 7, 9}
	for i, s := range got.Items {
		if s.Grade != wantGrades[i] || s.ClassNo != wantClasses[i] {
			t.Fatalf("位置 %d 期望 %s/%d班，实际 %s/%s", i,
				wantGrades[i], wantClasses[i], s.Grade, s.ClassName)
		}
	}
}

// 排序第一键（匹配度）此前零判别力断言：全仓四条顺序断言的取样点全部落在同分格。
//
// 取样点分析：TestSearchSortsUnorderedInput 查询「张」命中「张甲/乙/丙/丁」，
// 四者同为前缀匹配（全 1 分）；TestGradeOrderAcrossGrades 用三份同名「林宇」，
// 三者同为完全相等（全 0 分）。因此把排序比较器的第一键置为恒返 0 时，
// 全量 Go 测试零翻红——但行为确实会变（探针实测顺序翻转）。
// 这与第九轮撤销的「第三键」必须分开记账：那次根因是 loadStudents 已使输入全局有序
// （真冗余）；本例第一键活跃，只是在两个不同分值共存时无人取样（真缺口）。
//
// 取样点的设计约束（第一版探针踩过这个坑）：必须让匹配度键与班级号键给出**相反**顺序，
// 否则一个键会掩盖另一个的失效。「伟张」得 2 分（包含匹配）却坐 1 班，
// 「张伟」得 1 分（前缀匹配）却坐 2 班——第一键生效则张伟在前，
// 第二键生效则伟张在前，两者绝无可能同时成立。
func TestSearchSortsByMatchScoreBeforeClass(t *testing.T) {
	students := []Student{
		newStudent("伟张", GradeOne, "1班", parseClassName("1班")),
		newStudent("张伟", GradeOne, "2班", parseClassName("2班")),
	}
	got := Search(students, "张", 10, 0)
	if len(got.Items) != 2 {
		t.Fatalf("应返回 2 条，实际 %d", len(got.Items))
	}
	// 契约：完整匹配 < 前缀匹配 < 包含匹配。与班级号升序相反，故此断言
	// 只能由第一键满足——删掉第一键时顺序翻转为「伟张、张伟」，本条立即翻红。
	if got.Items[0].Name != "张伟" || got.Items[1].Name != "伟张" {
		t.Fatalf("匹配度优先：前缀匹配的张伟应排在包含匹配的伟张之前，实际 %s、%s",
			got.Items[0].Name, got.Items[1].Name)
	}
	// 双向：同分时第二键（班级号升序）接续。本 fixture 的两名不同分，
	// 故另取一组同分样本，确保第一键与第二键的职责各自被覆盖。
	sameScore := []Student{
		newStudent("张丙", GradeOne, "3班", parseClassName("3班")),
		newStudent("张甲", GradeOne, "1班", parseClassName("1班")),
	}
	byClass := Search(sameScore, "张", 10, 0)
	if len(byClass.Items) != 2 {
		t.Fatalf("应返回 2 条，实际 %d", len(byClass.Items))
	}
	if byClass.Items[0].Name != "张甲" || byClass.Items[1].Name != "张丙" {
		t.Fatalf("同分时按班级号升序：1班的张甲应在前，实际 %s、%s",
			byClass.Items[0].Name, byClass.Items[1].Name)
	}
}

// HasMore 必须等价于「本页取完后仍有剩余」，这是前端唯一的翻页依据。
//
// App.tsx 把 hasMore 直接传给 ResultList 的「继续加载」按钮：
// hasMore 为真则按钮显示，点击后按 offset+limit 再取一页。
// 契约一旦破坏，用户看到的就是「还有更多」但点出来是空列表——
// 而 total 与 items 都正确，肉眼无法分辨，只会以为服务坏了。
//
// 此前无断言覆盖该契约。穷举 43188 个 (limit, offset) 组合确认
// 边界符号 limit < len(matches)-offset 与 <= 完全等价（违反数恒为 0），
// 因此「修边界」是伪命题；承重点在 HasMore 的判据本身。
//
// 取样点选在 end 恰为 total-1 的那一格：这是 HasMore 为真的临界位置，
// 判据一旦写成 end < len(matches)-1 就在此处翻红。末页（end == total）
// 反而抓不到——那里两个写法同为 false。
func TestSearchHasMoreMatchesRemainingItems(t *testing.T) {
	students := benchStudents()
	total := Search(students, "高一", 1, 0).Total
	if total < defaultLimit+2 {
		t.Fatalf("fixture 规模不足，无法覆盖末页临界格，total=%d", total)
	}
	// 每组两处：临界格（end == total-1，应为 true）与整页（end << total，true），
	// 另加末页（end == total，false）。跨 limit 取样，避免只锁住单一页宽。
	for _, limit := range []int{1, 3, defaultLimit} {
		for _, back := range []int{1, defaultLimit} {
			offset := total - limit - back
			if offset < 0 {
				continue
			}
			got := Search(students, "高一", limit, offset)
			wantMore := offset+len(got.Items) < got.Total
			if got.HasMore != wantMore {
				t.Errorf("limit=%d offset=%d：取回 %d 条、合计 %d 条，HasMore=%v，"+
					"按「本页之后仍有剩余」应为 %v",
					limit, offset, len(got.Items), got.Total, got.HasMore, wantMore)
			}
		}
	}
	// 末页单独断言：取完后无剩余，HasMore 必须为假。
	last := Search(students, "高一", defaultLimit, total)
	if last.HasMore {
		t.Errorf("offset 越过末页时 HasMore 应为 false，实际 true（total=%d）", total)
	}
}

// 高三/高二的排序权重与声明序一致
func TestGradeOrderAcrossGrades(t *testing.T) {
	students := []Student{
		newStudent("林宇", GradeThree, "1班", parseClassName("1班")),
		newStudent("林宇", GradeTwo, "1班", parseClassName("1班")),
		newStudent("林宇", GradeOne, "1班", parseClassName("1班")),
	}
	got := Search(students, "林宇", 10, 0)
	if len(got.Items) != 3 {
		t.Fatalf("应 3 条，实际 %d", len(got.Items))
	}
	if got.Items[0].Grade != GradeOne || got.Items[1].Grade != GradeTwo || got.Items[2].Grade != GradeThree {
		t.Fatalf("排序应为高一/高二/高三，实际 %s/%s/%s",
			got.Items[0].Grade, got.Items[1].Grade, got.Items[2].Grade)
	}
}

// newStudent 必须填充派生字段——排序比较直接读这些字段，零值会导致排序静默错乱
func TestNewStudentFillsDerivedFields(t *testing.T) {
	s := newStudent("张三", GradeTwo, "18班", parseClassName("18班"))
	if s.Name != "张三" || s.NameKey != "张三" || s.Grade != GradeTwo || s.ClassName != "18班" {
		t.Fatalf("基础字段错误: %+v", s)
	}
	if s.ClassNo != 18 {
		t.Errorf("ClassNo = %d，期望 18", s.ClassNo)
	}
	if s.GradeIdx != 1 {
		t.Errorf("GradeIdx = %d，期望 1（高二在 knownGrades 中下标为 1）", s.GradeIdx)
	}
	// 姓名归一化必须与 normalizeName 一致
	spaced := newStudent("张 三", GradeOne, "6班", parseClassName("6班"))
	if spaced.NameKey != "张三" {
		t.Errorf("NameKey = %q，期望 张三（去空白）", spaced.NameKey)
	}
	if spaced.ClassNo != 6 {
		t.Errorf("中文班级 ClassNo = %d，期望 6", spaced.ClassNo)
	}
	// 非法班级格式：ClassNo 为 0（与 classNumber 对非班级串的返回值一致）
	if bad := newStudent("李四", GradeOne, "未知", parseClassName("未知")); bad.ClassNo != 0 {
		t.Errorf("非法班级 ClassNo = %d，期望 0", bad.ClassNo)
	}
}

// normalizeName 快路径必须与原语义完全一致（含全角空格与大小写）
func TestNormalizeNameFastPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"张三", "张三"},
		{"张 三", "张三"},
		{"张　三", "张三"}, // 全角空格 U+3000
		{"张\t三", "张三"},
		{"abc", "ABC"},
		{"AbC", "ABC"},
		{"EXAMPLE STUDENT", "EXAMPLESTUDENT"},
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizeName(c.in); got != c.want {
			t.Errorf("normalizeName(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// 已归一化的姓名必须零拷贝复用原串（Name 与 NameKey 共享底层数组），
// 这是内存占用的关键优化：2000 条名单可省约一半姓名字符串内存
func TestNormalizeNameReusesCleanInput(t *testing.T) {
	clean := "张三"
	if got := normalizeName(clean); got != clean {
		t.Fatalf("normalizeName(%q) = %q", clean, got)
	}
	// 通过 unsafe 比较字符串头确认复用（同包测试可直接访问）
	if !sameStringData(normalizeName(clean), clean) {
		t.Error("已归一化姓名应复用原串底层数组，实际发生了拷贝")
	}
}

// TestGradeValueDomainFollowsKnownGrades 验证年段值域从 knownGrades 派生。
// 变异事实（2026-09-26 第六轮探针实测）：把新年段只追加到 knownGrades 后，
// 运维提示正确列出该文件、gradeOrder 正确排位，但 loadStudents 报
// 「文件名与年级标题不一致」拒绝加载合法名单，Search 返回 total=0 且与
// 真不存在的年段结果完全一致——四处硬编码使扩展路径彻底失效，
// 而跨语言对拍因两端同时缺失而无法发现。
func TestGradeValueDomainFollowsKnownGrades(t *testing.T) {
	original := knownGrades
	knownGrades = append(append([]Grade{}, original...), Grade("高四"))
	// splitGradeClass 直接遍历 knownGrades 与 gradeAliases，无缓存正则需要重建。
	t.Cleanup(func() { knownGrades = original })

	// 查询解释：子串匹配必须认识新年段
	if got := parseGrade("福清一中2025级高四编班名单"); got != Grade("高四") {
		t.Errorf("parseGrade(高四标题) = %q，期望 高四", got)
	}

	// 年级+班级连写必须认识新年段
	if grade, classPart, ok := splitGradeClass("高四一班"); !ok || grade != Grade("高四") || classPart != "一" {
		t.Errorf("splitGradeClass(高四一班) = (%q, %q, %v)，期望 (高四, 一, true)",
			grade, classPart, ok)
	}

	// 加载侧：标题校验经 parseGrade，必须放行合法的新年段名单
	dir := t.TempDir()
	body := `{"标题":"福清一中2025级高四编班名单","名单":{"1班":[{"姓名":"探测同学"}]}}`
	if err := os.WriteFile(filepath.Join(dir, "高四.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	students, err := loadStudents(dir)
	if err != nil {
		t.Fatalf("追加高四后 loadStudents 失败 = %v，期望加载成功", err)
	}
	if len(students) != 1 || students[0].Grade != Grade("高四") {
		t.Fatalf("加载结果 = %+v，期望 1 名高四学生", students)
	}

	// 用户可见行为：查询该年段必须命中
	if got := Search(students, "高四", 10, 0).Total; got != 1 {
		t.Errorf("Search(高四).Total = %d，期望 1", got)
	}
}
