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
