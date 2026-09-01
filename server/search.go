package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type Grade string

const (
	GradeOne Grade = "高一"
	GradeTwo Grade = "高二"
)

// Student 对外 JSON 契约：只输出 name/grade/class（隐私红线，NameKey 永不出现在任何序列化中）。
type Student struct {
	Name      string `json:"name"`
	NameKey   string `json:"-"`
	Grade     Grade  `json:"grade"`
	ClassName string `json:"class"`
}

type SearchResponse struct {
	Items   []Student `json:"items"`
	Total   int       `json:"total"`
	Limit   int       `json:"limit"`
	Offset  int       `json:"offset"`
	HasMore bool      `json:"hasMore"`
}

type Query struct {
	NameTokens []string
	Grade      Grade
	ClassNo    int
}

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

func normalizeName(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToUpper(value) {
		if unicode.IsSpace(r) || r == '\t' || r == '　' {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func parseGrade(title string) Grade {
	if strings.Contains(title, "高一") || strings.Contains(title, "高1") {
		return GradeOne
	}
	if strings.Contains(title, "高二") || strings.Contains(title, "高2") {
		return GradeTwo
	}
	return ""
}

func parseQuery(raw string) Query {
	normalized := strings.NewReplacer("，", " ", ",", " ", "、", " ", "+", " ").Replace(strings.TrimSpace(raw))
	query := Query{}
	for _, token := range strings.Fields(normalized) {
		if match := classToken.FindStringSubmatch(token); match != nil {
			classNo := classNumber(token)
			if classNo < 0 {
				// 无效班级（如超长数字）：按姓名处理，避免"返回全部"的静默错误
				query.NameTokens = append(query.NameTokens, normalizeName(token))
				continue
			}
			query.ClassNo = classNo
			continue
		}
		if grade := parseGrade(token); grade != "" {
			query.Grade = grade
			continue
		}
		query.NameTokens = append(query.NameTokens, normalizeName(token))
	}
	return query
}

func Search(students []Student, raw string, limit, offset int) (SearchResponse, Query) {
	query := parseQuery(raw)
	if strings.TrimSpace(raw) == "" {
		return SearchResponse{Items: []Student{}, Limit: limit, Offset: offset}, query
	}
	matches := make([]Student, 0)
	for _, item := range students {
		ok := true
		for _, token := range query.NameTokens {
			if !strings.Contains(item.NameKey, token) {
				ok = false
				break
			}
		}
		if query.Grade != "" && item.Grade != query.Grade {
			ok = false
		}
		if query.ClassNo > 0 && classNumber(item.ClassName) != query.ClassNo {
			ok = false
		}
		if ok {
			matches = append(matches, item)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		left, right := matches[i], matches[j]
		leftScore, rightScore := 0, 0
		for _, token := range query.NameTokens {
			leftScore += nameScore(left.NameKey, token)
			rightScore += nameScore(right.NameKey, token)
		}
		if leftScore != rightScore {
			return leftScore < rightScore
		}
		if left.Grade != right.Grade {
			return left.Grade < right.Grade
		}
		return classNumber(left.ClassName) < classNumber(right.ClassName)
	})
	if offset > len(matches) {
		offset = len(matches)
	}
	end := len(matches)
	if limit < len(matches)-offset {
		end = offset + limit
	}
	items := matches[offset:end]
	return SearchResponse{Items: items, Total: len(matches), Limit: limit, Offset: offset, HasMore: end < len(matches)}, query
}

func nameScore(nameKey, token string) int {
	if nameKey == token {
		return 0
	}
	if strings.HasPrefix(nameKey, token) {
		return 1
	}
	return 2
}

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

func isAllDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(value) > 0
}
