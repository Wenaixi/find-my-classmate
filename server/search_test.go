package main

import "testing"

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
		got, _ := Search(testStudents(), tt.query, 10, 0)
		if len(got.Items) != tt.want {
			t.Fatalf("%s failed: got %d want %d", tt.name, len(got.Items), tt.want)
		}
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
