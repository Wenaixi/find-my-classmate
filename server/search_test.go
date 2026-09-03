package main

import (
	"strings"
	"testing"
)

func testStudents() []Student {
	return []Student{
		{Name: "示例同学", NameKey: "示例同学", Grade: GradeTwo, ClassName: "18班"},
		{Name: "示 例 同 学", NameKey: "示例同学", Grade: GradeOne, ClassName: "6班"},
		{Name: "EXAMPLE STUDENT", NameKey: "EXAMPLESTUDENT", Grade: GradeOne, ClassName: "11班"},
	}
}

func TestSearchRules(t *testing.T) {
	tests := []struct {
		name, query string
		want        int
	}{
		{"姓名去空格", "示 例", 2},
		{"组合筛选", "高二, 示例同学", 1},
		{"加号组合筛选", "高二+示例同学+18班", 1},
		{"班级数字", "18", 1},
		{"中文班级", "六班", 1},
		{"年级数字别名", "高1", 2},
		{"混合分隔符", "高一，六班", 1},
		{"高一筛选", "高一", 2},
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

// F22：汉字多位班级号（十一~九十九）应解析为数值
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

// F22 关联：查询 "十一班" 应命中 11 班而不是全校
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

// F22 关联：超长数字班级串应被安全处理（不 panic、按姓名处理返回空）
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

// F62：姓名包含"高"/"班"字不应被误判为年级/班级（回归保护）
func TestNameTokensNotMisparsed(t *testing.T) {
	students := []Student{
		{Name: "高翔", NameKey: "高翔", Grade: GradeOne, ClassName: "1班"},
		{Name: "班长", NameKey: "班长", Grade: GradeTwo, ClassName: "2班"},
	}
	if got, _ := Search(students, "高翔", 10, 0); len(got.Items) != 1 {
		t.Errorf("查询高翔应命中 1 条（作为姓名），实际 %d", len(got.Items))
	}
	if got, _ := Search(students, "班长", 10, 0); len(got.Items) != 1 {
		t.Errorf("查询班长应命中 1 条（作为姓名），实际 %d", len(got.Items))
	}
}

// F16 回归：年级子串输入（口语化）在 Go 端保持年级语义
func TestGradeSubstringBehavior(t *testing.T) {
	got, q := Search(testStudents(), "高二三班", 10, 0)
	if q.Grade != GradeTwo {
		t.Errorf("高二三班 应解析为年级=高二，实际 %q", q.Grade)
	}
	if len(got.Items) != 1 {
		t.Errorf("高二三班 应命中高二年级 1 条，实际 %d", len(got.Items))
	}
	if strings.Contains("示例同学", "高二三班") {
		t.Fatal("fixture 不应包含该姓名")
	}
}
