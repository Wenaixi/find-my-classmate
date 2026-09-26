package main

import (
	"regexp"
	"strconv"
	"strings"
)

// 班级解析基础设施：班级名 → 班号，名单域共享概念。
// search.go 的查询解析与 data.go 的名单加载校验都依赖它，
// 归位到独立文件后，改查询语义不会静默改变数据文件校验行为。

var classToken = regexp.MustCompile("^([0-9]+|[一二三四五六七八九十]+)班?$")

var classDigits = map[string]int{"一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6, "七": 7, "八": 8, "九": 9, "十": 10}

// chineseNumberToInt 解析汉字数字（支持 一~九十九 与 十~十九），解析失败返回 0。
func chineseNumberToInt(value string) int {
	if value == "" {
		return 0
	}
	// 形如 "二十"：十位 * 10 + 个位；"二十一"：20 + 1；"十一"：10 + 1；"十"：10。
	// 先取十位：若以"十"开头（十/十一）十位=1；若含"十"且前面有数字（二十）十位=该数字。
	tens, ones := 0, 0
	runes := []rune(value)
	if runes[0] == '十' {
		tens = 1
		if len(runes) > 1 {
			ones = classDigits[string(runes[1])]
		}
	} else if len(runes) >= 2 && runes[1] == '十' {
		tens = classDigits[string(runes[0])]
		if len(runes) > 2 {
			ones = classDigits[string(runes[2])]
		}
	} else {
		// 单字一~九
		return classDigits[value]
	}
	if tens == 0 || ones == 0 && len(runes) > 2 {
		return 0
	}
	return tens*10 + ones
}

// ClassParseResult 班级名解析的显式三态结果。
// 旧实现用 0/-1 两个哨兵返回码，语义靠调用点各自记住（0=非法 / -1=溢出），
// 数据路径只检 ==0 导致 -1 静默流入 Student.ClassNo 参与排序。三态化后
// Valid/Overflow 语义由类型强制，-1 不可能再漏进派生字段。
type ClassParseResult struct {
	ClassNo  int
	Valid    bool
	Overflow bool
}

// parseClassName 解析班级名为三态结果：合法（含班号）/ 非法格式 / 数字溢出。
// 溢出指格式合法（全数字）但超出 int 范围；由数据路径拒绝、查询路径按姓名处理。
func parseClassName(value string) ClassParseResult {
	match := classToken.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return ClassParseResult{}
	}
	if number, err := strconv.Atoi(match[1]); err == nil {
		return ClassParseResult{ClassNo: number, Valid: true}
	}
	if isAllDigits(match[1]) {
		return ClassParseResult{Overflow: true} // 数字溢出：格式合法但超出 int
	}
	return ClassParseResult{ClassNo: chineseNumberToInt(match[1]), Valid: true}
}

// classNumber 保留为薄包装（返回班号；0 表示非法或溢出），供查询路径过渡使用。
// 数据路径应改用 parseClassName 获取完整三态语义。
func classNumber(value string) int {
	return parseClassName(value).ClassNo
}

// isAllDigits 判定字符串是否全为阿拉伯数字。
func isAllDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(value) > 0
}
