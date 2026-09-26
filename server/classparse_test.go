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

// 回归锁：0 班号不得被当成合法班级条件。
//
// strconv.Atoi("0") 语法成功且无错，旧实现据此把 0 包成 Valid=true，
// 违背「Valid ⇒ ClassNo > 0」。下游 classCondition 因此既不产生班级条件、
// 也不降级为姓名条件，token 静默消失；与年级连写时（「高一0班」）年级条件
// 仍成立而班级条件落空，查询被放大成整个年段的全量结果——实测返回全校。
// 这与 v0.9.1「无法解析的班级不得静默丢弃」、v0.10.0「纯分隔符不得退化为
// 全校检索」同源，是该不变量的第三个实例。
func TestZeroClassIsRejectedNotAccepted(t *testing.T) {
	// 解析层：0 不是合法班号，也不是溢出（溢出指超出 int 范围，装得下 0）。
	for _, className := range []string{"0班", "0", "00班", "000班"} {
		got := parseClassName(className)
		if got.Valid || got.Overflow {
			t.Errorf("班级 %q 应为非法，实际 {ClassNo:%d Valid:%v Overflow:%v}", className, got.ClassNo, got.Valid, got.Overflow)
		}
	}

	// 正数班号不得被本次修复误伤。
	for className, want := range map[string]int{"1班": 1, "10班": 10, "18班": 18, "十班": 10} {
		got := parseClassName(className)
		if !got.Valid || got.ClassNo != want {
			t.Errorf("班级 %q 应解析为 %d，实际 %+v", className, want, got)
		}
	}

	// 降级层：无法解析的班级必须降级为姓名条件，绝不静默丢弃。
	if _, asName := classCondition("0", "0班"); asName == "" {
		t.Error("「0班」应降级为姓名条件，实际既无班级条件也无姓名条件（token 静默消失）")
	}
}

// 回归锁：用户可见行为——「高一0班」不得返回整年段全量名单。
// 这是本缺陷的实际危害：一次精确查询被静默放大成全校枚举。
func TestSearchZeroClassDoesNotReturnEveryone(t *testing.T) {
	students := []Student{
		newStudent("张三", GradeOne, "1班", parseClassName("1班")),
		newStudent("李四", GradeOne, "2班", parseClassName("2班")),
	}
	if got := Search(students, "高一0班", 10, 0); got.Total == len(students) {
		t.Errorf("输入「高一0班」返回了全部 %d 条记录：班级条件落空后退化为整年段检索: %+v", len(students), got.Items)
	}
	// 对照：纯年级查询仍应返回该年段全量，确认上一条不是被空结果误判。
	if got := Search(students, "高一", 10, 0); got.Total != len(students) {
		t.Errorf("纯年级查询「高一」应返回 %d 条，实际 %d 条，年级条件被误伤", len(students), got.Total)
	}
}

// 回归锁：数据加载侧必须拒绝 0 班类名。
// 与 TestLoadStudentsRejectsOverflowClass 同型：畸形类名曾以 ClassNo=0
// 静默通过校验，并排到所有正常班级之前参与排序。
func TestLoadStudentsRejectsZeroClass(t *testing.T) {
	dir := t.TempDir()
	content := `{"标题":"福清一中2025级高一编班名单","名单":{"0班":[{"姓名":"零班生"}],"1班":[{"姓名":"张三"}]}}`
	if err := os.WriteFile(filepath.Join(dir, "高一.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadStudents(dir); err == nil {
		t.Fatal("0 班类名应被数据校验拒绝，当前静默放行")
	}
}
