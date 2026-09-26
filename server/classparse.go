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

// chineseNumberToInt 解析汉字数字（支持 一~九十九 与 十~十九）。
// 第二个返回值报告解析是否成功：classDigits 查表未命中时旧实现返回零值 0，
// 而零值与「合法的 0 班号」不可区分，调用方会把无法解析误当成合法结果。
func chineseNumberToInt(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	// 形如 "二十"：十位 * 10 + 个位；"二十一"：20 + 1；"十一"：10 + 1；"十"：10。
	// 先取十位：若以"十"开头（十/十一）十位=1；若含"十"且前面有数字（二十）十位=该数字。
	tens, ones := 0, 0
	runes := []rune(value)
	if runes[0] == '十' {
		tens = 1
		if len(runes) > 1 {
			var ok bool
			if ones, ok = classDigits[string(runes[1])]; !ok {
				return 0, false
			}
		}
	} else if len(runes) >= 2 && runes[1] == '十' {
		var ok bool
		if tens, ok = classDigits[string(runes[0])]; !ok {
			return 0, false
		}
		if len(runes) > 2 {
			if ones, ok = classDigits[string(runes[2])]; !ok {
				return 0, false
			}
		}
	} else {
		// 单字一~九：查表未命中说明该字符串不是合法汉字数字。
		number, ok := classDigits[value]
		return number, ok
	}
	if tens == 0 || ones == 0 && len(runes) > 2 {
		return 0, false
	}
	return tens*10 + ones, true
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
	if classNo, ok := chineseNumberToInt(match[1]); ok {
		return ClassParseResult{ClassNo: classNo, Valid: true}
	}
	// 汉字数字无法解析：格式非法而非溢出，交由调用方按非法处理。
	return ClassParseResult{}
}

// classCondition 把一个班级 token 解释为「已解释的条件片段」。
//
// matchPart 是正则捕获到的班级部分（用于判定班号），rawToken 是用户原始输入的
// 整个 token（用于降级为姓名条件时的匹配键）。两者必须分开：查询「一一班」时
// 班级部分是「一一」，而降级后的姓名匹配键必须是「一一班」——用户输入的是后者。
// 契约语料 TestParseQueryContractCorpus/一一班 锁住这一行为。
//
// 降级策略与它的理由收敛到本函数。此前 parseQuery 有四段近乎逐字重复的
// 「追加 normalizeName(token); continue」，而解释为什么必须降级、不能静默
// 丢弃的注释只写在其中一段——其余三段靠「照抄旁边那段」维持这条安全不变量，
// 改任一段时看不到理由，容易被当成冗余代码清理掉。
//
// 不变量：无法解析（!Valid）与数字溢出（Overflow）都必须降级为姓名条件，
// 绝不丢弃 token。丢弃会让全部条件落空，Search 退化成与用户输入无关的
// 全校检索——这不是理论风险，v0.9.1 修复前输入「一一班」曾返回全校 1047 条。
//
// 返回值约定：
//   - asName 非空：token 降级为姓名条件，调用方应把 asName 追加到姓名条件；
//   - classNo > 0：token 解析为班级条件，调用方应设为班级条件；
//   - 两者都空：token 不是班级 token，调用方应走非班级分支。
//
// 溢出与无法解析同策略：两者都不是「合法班号」，都会让班级条件落空。
func classCondition(matchPart, rawToken string) (classNo int, asName string) {
	parsed := parseClassName(matchPart)
	if parsed.Valid {
		return parsed.ClassNo, ""
	}
	// !Valid 或 Overflow：按姓名处理。匹配键取 rawToken（用户原始输入）而非
	// matchPart——「降级时用哪个形态参与匹配」与「降级」本身是同一个决定，
	// 收在这里使调用方无法传错。
	return 0, normalizeName(rawToken)
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
