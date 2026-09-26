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
		newStudent("示例同学", GradeThree, "18班"),
		newStudent("示 例 同 学", GradeOne, "6班"),
		newStudent("EXAMPLE STUDENT", GradeOne, "11班"),
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
		{"纯数字按姓名处理", "223", 0},
		{"空输入", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := Search(testStudents(), tt.query, 10, 0)
			if len(got.Items) != tt.want {
				t.Fatalf("got %d want %d", len(got.Items), tt.want)
			}
		})
	}
}

func TestSearchPagination(t *testing.T) {
	students := append(testStudents(), testStudents()...)
	first, _ := Search(students, "示例", 2, 0)
	second, _ := Search(students, "示例", 2, 2)
	if first.Total != 4 || len(first.Items) != 2 || !first.HasMore {
		t.Fatalf("unexpected first page: total=%d items=%d hasMore=%v", first.Total, len(first.Items), first.HasMore)
	}
	if second.Offset != 2 || len(second.Items) != 2 || second.HasMore {
		t.Fatalf("unexpected second page: offset=%d items=%d hasMore=%v", second.Offset, len(second.Items), second.HasMore)
	}
	if second.Items[0].Name == first.Items[0].Name || second.Items[1].Name == first.Items[1].Name {
		t.Fatal("pagination repeated an item")
	}
	last, _ := Search(students, "示例", 2, 999999999999)
	if last.Offset != 4 || len(last.Items) != 0 || last.HasMore {
		t.Fatalf("unexpected out-of-range page: offset=%d items=%d hasMore=%v", last.Offset, len(last.Items), last.HasMore)
	}
}

// Search 必须自守分页前置约定：offset 早于列表开头时按第一页处理。
// 该约定此前只由 buildMux 的 HTTP 层校验兜底，Search 自身对负 offset 会
// panic（slice bounds out of range）。把不变量收进接口，调用者无需记忆。
func TestSearchNegativeOffsetTreatedAsFirstPage(t *testing.T) {
	students := append(testStudents(), testStudents()...)
	got, _ := Search(students, "示例", 2, -3)
	if got.Offset != 0 {
		t.Fatalf("负 offset 应归一到 0，实际 %d", got.Offset)
	}
	first, _ := Search(students, "示例", 2, 0)
	if len(got.Items) != len(first.Items) {
		t.Fatalf("负 offset 与 offset=0 结果条目数应一致：%d vs %d", len(got.Items), len(first.Items))
	}
	if got.Total != first.Total || got.HasMore != first.HasMore {
		t.Fatalf("负 offset 与 offset=0 的分页元数据应一致：total %d/%d hasMore %v/%v",
			got.Total, first.Total, got.HasMore, first.HasMore)
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
			if got := classNumber(c.in); got != c.want {
				t.Fatalf("classNumber(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

// 查询 "十一班" 应命中 11 班而不是全校
func TestSearchChineseMultiDigitClass(t *testing.T) {
	// fixture 中 11 班有 EXAMPLE STUDENT（1 条）；18 班与 6 班不应命中
	got, _ := Search(testStudents(), "十一班", 10, 0)
	if len(got.Items) != 1 {
		t.Fatalf("查询十一班应精确命中 1 条（11 班），实际 %d", len(got.Items))
	}
	if got.Items[0].ClassName != "11班" {
		t.Errorf("命中班级 = %s，期望 11班", got.Items[0].ClassName)
	}
}

// 超长数字班级串应被安全处理（不 panic、按姓名处理返回空）
func TestClassNumberOverflowSafe(t *testing.T) {
	if got := classNumber("99999999999999999999班"); got != -1 {
		t.Fatalf("超长数字应解析为 -1（无效班级标记），实际 %d", got)
	}
	// 查询超长数字不应 panic；按姓名处理（姓名不含数字）应返回空结果
	got, _ := Search(testStudents(), "99999999999999999999", 10, 0)
	if len(got.Items) != 0 {
		t.Fatalf("超长数字查询应返回空，实际 %d", len(got.Items))
	}
}

// 姓名包含"高"/"班"字不应被误判为年级/班级（回归保护）
func TestNameTokensNotMisparsed(t *testing.T) {
	students := []Student{
		newStudent("高翔", GradeOne, "1班"),
		newStudent("班长", GradeTwo, "2班"),
	}
	if got, _ := Search(students, "高翔", 10, 0); len(got.Items) != 1 {
		t.Errorf("查询高翔应命中 1 条（作为姓名），实际 %d", len(got.Items))
	}
	if got, _ := Search(students, "班长", 10, 0); len(got.Items) != 1 {
		t.Errorf("查询班长应命中 1 条（作为姓名），实际 %d", len(got.Items))
	}
}

// 年级+班级连写输入（"高三三班"）精确解析为年段+班级：
// 旧语义按年级子串处理返回全年级，现改为精确班级筛选（用户报告缺陷）。
func TestGradeSubstringBehavior(t *testing.T) {
	got, q := Search(testStudents(), "高三三班", 10, 0)
	if q.Grade != GradeThree {
		t.Errorf("高三三班 应解析为年级=高三，实际 %q", q.Grade)
	}
	if q.ClassNo != 3 {
		t.Errorf("高三三班 应解析出班级=3，实际 %d", q.ClassNo)
	}
	// fixture 中高三只有 18 班，三班应精确命中 0 条（不再返回整个高三年段）
	if len(got.Items) != 0 {
		t.Errorf("高三三班 应精确命中 0 条（fixture 高三无三班），实际 %d", len(got.Items))
	}
}

// 年级+班级连写（"高二三班"/"高二1班"）精确筛选对应班级的人
func TestGradeClassCompoundPrecise(t *testing.T) {
	students := []Student{
		newStudent("甲", GradeTwo, "1班"),
		newStudent("乙", GradeTwo, "2班"),
		newStudent("丙", GradeOne, "1班"),
		newStudent("丁", GradeTwo, "12班"),
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
			got, q := Search(students, c.query, 10, 0)
			if c.query == "高一1班" && q.Grade != GradeOne {
				t.Fatalf("%s 年级 = %q，期望 高一", c.query, q.Grade)
			}
			if c.query != "高一1班" && q.Grade != GradeTwo {
				t.Fatalf("%s 年级 = %q，期望 高二", c.query, q.Grade)
			}
			if len(got.Items) != 1 || got.Items[0].Name != c.want {
				t.Fatalf("%s 应精确命中 %s，实际 %+v", c.query, c.want, got.Items)
			}
		})
	}
}

// 按数据目录实际文件探测年段，缺失的年级文件跳过不报错
func TestLoadStudentsSkipMissingGrade(t *testing.T) {
	dir := t.TempDir()
	// 只放高三，高一高二不存在
	_ = os.WriteFile(filepath.Join(dir, "高三.json"), []byte(`{"标题":"福清一中2025级高三编班名单","名单":{"3班":[{"姓名":"高三甲"}]}}`), 0o644)
	students, err := loadStudents(dir)
	if err != nil {
		t.Fatalf("仅高三存在应成功加载: %v", err)
	}
	if len(students) != 1 {
		t.Fatalf("应加载 1 条，实际 %d", len(students))
	}
	if students[0].Grade != GradeThree {
		t.Errorf("Grade = %q，期望 高三", students[0].Grade)
	}
}

// 空目录应给出明确指引，不静默空跑
func TestLoadStudentsEmptyDirFails(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadStudents(dir); err == nil {
		t.Fatal("空目录应报错，不应静默空跑")
	}
}

// 高三/高二的排序权重与声明序一致
func TestGradeOrderAcrossGrades(t *testing.T) {
	students := []Student{
		newStudent("林宇", GradeThree, "1班"),
		newStudent("林宇", GradeTwo, "1班"),
		newStudent("林宇", GradeOne, "1班"),
	}
	got, _ := Search(students, "林宇", 10, 0)
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
	s := newStudent("张三", GradeTwo, "18班")
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
	spaced := newStudent("张 三", GradeOne, "6班")
	if spaced.NameKey != "张三" {
		t.Errorf("NameKey = %q，期望 张三（去空白）", spaced.NameKey)
	}
	if spaced.ClassNo != 6 {
		t.Errorf("中文班级 ClassNo = %d，期望 6", spaced.ClassNo)
	}
	// 非法班级格式：ClassNo 为 0（与 classNumber 对非班级串的返回值一致）
	if bad := newStudent("李四", GradeOne, "未知"); bad.ClassNo != 0 {
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
