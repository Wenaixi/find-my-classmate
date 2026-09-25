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

// classNumber 解析班级名为班号。
// 返回 0 表示班级格式非法；-1 表示数字溢出（由查询解析决定按姓名处理）。
func classNumber(value string) int {
	match := classToken.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return 0
	}
	if number, err := strconv.Atoi(match[1]); err == nil {
		return number
	}
	// 数字溢出（Atoi 失败，如超长数字串）：返回 -1 标记"无效班级"，
	// 由 parseQuery 决定按姓名处理，避免静默变成"不筛选返回全部"。
	if isAllDigits(match[1]) {
		return -1
	}
	return chineseNumberToInt(match[1])
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
