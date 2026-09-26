package main

import (
	"os"
	"path/filepath"
	"testing"
)

// C3：classNumber 三哨兵返回码（0=非法 / -1=溢出）跨上下文复用，
// 数据路径只检 ==0，-1 静默流入 Student.ClassNo 参与排序（非法班级排到最前）。
// parseClassName 用显式三态结果类型，-1 不可能再漏进数据路径。

func TestParseClassNameStates(t *testing.T) {
	valid := parseClassName("18班")
	if !valid.Valid || valid.Overflow || valid.ClassNo != 18 {
		t.Errorf("合法班级解析错误: %+v", valid)
	}

	invalid := parseClassName("未知")
	if invalid.Valid || invalid.Overflow {
		t.Errorf("非法班级应 Valid=false: %+v", invalid)
	}

	overflow := parseClassName("99999999999999999999")
	if overflow.Valid || !overflow.Overflow {
		t.Errorf("超长数字应 Overflow=true: %+v", overflow)
	}

	chinese := parseClassName("十二班")
	if !chinese.Valid || chinese.Overflow || chinese.ClassNo != 12 {
		t.Errorf("汉字班级解析错误: %+v", chinese)
	}
}

// 不变量锁：Valid 为真时班号必须为正。
// 汉字数字走 classDigits 查表，未命中时 Go 返回零值 0，旧实现把 0 无条件包成
// Valid=true，使「无法解析」与「解析为 0」不可区分：查询侧静默丢弃该 token
// 后条件全空，Search 返回全校第一页；数据侧畸形类名以 ClassNo=0 排到最前。
func TestParseClassNameValidImpliesPositiveClassNo(t *testing.T) {
	// 逐个覆盖查表未命中的汉字数字组合：单字重复与非法十位组合。
	for _, className := range []string{"一一班", "八八班", "零零班", "九十九十九班"} {
		got := parseClassName(className)
		if got.Valid && got.ClassNo <= 0 {
			t.Errorf("班级 %q 解析为 Valid=true 但 ClassNo=%d，Valid 谎报: %+v",
				className, got.ClassNo, got)
		}
	}
}

// 回归锁：无法解析的班级名不得被当成无条件查询。
// 用户输入「一一班」时，条件被静默丢弃会让 Search 返回与输入无关的全校名单。
func TestSearchUnparsableClassDoesNotReturnEveryone(t *testing.T) {
	students := []Student{
		newStudent("张三", GradeOne, "1班", parseClassName("1班")),
		newStudent("李四", GradeOne, "2班", parseClassName("2班")),
	}
	got := Search(students, "一一班", 10, 0)
	if got.Total == len(students) {
		t.Errorf("输入无法解析的班级「一一班」返回了全部 %d 条记录，条件被静默丢弃: %+v",
			len(students), got.Items)
	}
}

// 回归锁：溢出类名此前被 data.go 的 ==0 校验静默放行（classNumber 返回 -1），
// -1 以 ClassNo 流入排序比较；三态化后数据路径必须拒绝溢出类名。
func TestLoadStudentsRejectsOverflowClass(t *testing.T) {
	dir := t.TempDir()
	content := `{"标题":"福清一中2025级高一编班名单","名单":{"99999999999999999999班":[{"姓名":"张三"}]}}`
	if err := os.WriteFile(filepath.Join(dir, "高一.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadStudents(dir); err == nil {
		t.Fatal("溢出类名应被数据校验拒绝，当前静默放行")
	}
}
