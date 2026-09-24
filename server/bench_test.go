package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// benchStudents 合成 2112 条名单（规模与真实数据一致；真实姓名不入库，符合隐私红线）。
func benchStudents() []Student {
	surnames := []string{"张", "王", "李", "赵", "陈", "刘", "杨", "黄", "周", "吴"}
	given := []string{"伟", "芳", "娜", "敏", "静", "强", "磊", "洋", "艳", "勇", "军", "杰", "娟", "涛", "明", "超"}
	grades := []Grade{GradeOne, GradeTwo, GradeThree}
	students := make([]Student, 0, 2112)
	for gi, grade := range grades {
		for class := 1; class <= 22; class++ {
			for k := 0; k < 32; k++ {
				name := surnames[(gi*7+class+k)%len(surnames)] +
					given[(gi*13+class*3+k*5)%len(given)] +
					given[(gi*3+class*7+k*11)%len(given)]
				students = append(students, newStudent(name, grade, fmt.Sprintf("%d班", class)))
			}
		}
	}
	return students
}

func BenchmarkSearchByName(b *testing.B) {
	students := benchStudents()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Search(students, "张伟", 10, 0)
	}
}

func BenchmarkSearchByGrade(b *testing.B) {
	students := benchStudents()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Search(students, "高一", 10, 0)
	}
}

func BenchmarkSearchCombined(b *testing.B) {
	students := benchStudents()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Search(students, "高二, 张, 3班", 10, 0)
	}
}

func BenchmarkStoreView(b *testing.B) {
	dir := b.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "高一.json"),
		[]byte(`{"标题":"福清一中2025级高一编班名单","名单":{"1班":[{"姓名":"张三"}]}}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "高二.json"),
		[]byte(`{"标题":"福清一中2025级高二编班名单","名单":{"2班":[{"姓名":"李四"}]}}`), 0o644)
	store, err := newStudentStore(dir)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.view(); err != nil {
			b.Fatal(err)
		}
	}
}

// TestSearchAllocsBudget 分配次数硬预算：与机器速度无关的确定性回归防线。
// 耗时随负载抖动，分配次数不会——一旦优化退化（例如排序重新引入正则），此测试立即失败。
func TestSearchAllocsBudget(t *testing.T) {
	students := benchStudents()
	tests := []struct {
		name   string
		query  string
		budget float64
	}{
		{"单姓名查询", "张", 12},
		{"整年段查询", "高一", 12},
		{"组合查询", "高二, 张, 3班", 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(100, func() { _, _ = Search(students, tt.query, 10, 0) })
			if allocs > tt.budget {
				t.Fatalf("分配次数退化：实测 %.1f，预算 %.0f", allocs, tt.budget)
			}
			t.Logf("%s 分配次数 %.1f（预算 %.0f）", tt.name, allocs, tt.budget)
		})
	}
}

// TestStoreViewAllocsBudget 视图路径分配预算：view 在探测节流命中时应零分配。
func TestStoreViewAllocsBudget(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "高一.json"),
		[]byte(`{"标题":"福清一中2025级高一编班名单","名单":{"1班":[{"姓名":"张三"}]}}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "高二.json"),
		[]byte(`{"标题":"福清一中2025级高二编班名单","名单":{"2班":[{"姓名":"李四"}]}}`), 0o644)
	store, err := newStudentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	// 预热：首次 view 触发探测与加载，不计入预算
	if _, err := store.view(); err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(100, func() {
		if _, err := store.view(); err != nil {
			t.Fatal(err)
		}
	})
	if allocs > 4 {
		t.Fatalf("视图路径分配次数退化：实测 %.1f，预算 4", allocs)
	}
	t.Logf("视图路径分配次数 %.1f（预算 4）", allocs)
}
