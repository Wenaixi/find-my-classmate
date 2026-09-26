package main

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

type Grade string

const (
	GradeOne   Grade = "高一"
	GradeTwo   Grade = "高二"
	GradeThree Grade = "高三"
)

// knownGrades 系统支持的年段列表（按自然顺序）。
// loadStudents 与 dataStamps 按此列表探测数据目录，存在哪个文件就加载哪个。
var knownGrades = []Grade{GradeOne, GradeTwo, GradeThree}

// Student 对外 JSON 契约：只输出 name/grade/class（隐私红线，派生字段与 NameKey 永不出现在任何序列化中）。
type Student struct {
	Name      string `json:"name"`
	NameKey   string `json:"-"`
	Grade     Grade  `json:"grade"`
	ClassName string `json:"class"`
	// 派生字段：加载期预计算，避免排序热路径反复正则解析班级号。
	// ClassNo 为 0 表示班级格式非法；GradeIdx 未知年段为 len(knownGrades)。
	ClassNo  int `json:"-"`
	GradeIdx int `json:"-"`
}

// newStudent 构造 Student 并填充派生字段。
// 所有 Student 必须经此构造：排序比较直接读派生字段，绕过构造会导致排序静默错乱。
func newStudent(name string, grade Grade, className string) Student {
	return Student{
		Name:      name,
		NameKey:   normalizeName(name),
		Grade:     grade,
		ClassName: className,
		ClassNo:   classNumber(className),
		GradeIdx:  gradeOrder(grade),
	}
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

// querySeparators 查询 token 分隔符（中文逗号、英文逗号、顿号、加号）。
// 包级复用：strings.Replacer 构造需编译替换表，每请求重建是纯浪费。
var querySeparators = strings.NewReplacer("，", " ", ",", " ", "、", " ", "+", " ")


// gradeClassToken 匹配年级+班级连写（"高二三班"/"高二1班"/"高一十八班"）。
var gradeClassToken = regexp.MustCompile("^(高一|高二|高三|高1|高2|高3)([0-9]+|[一二三四五六七八九十]+)班?$")


// needsNormalize 判定是否真的需要归一化处理。
// 绝大多数中文姓名既无空白也无小写字母，此时可原样返回，省去一次字符串分配。
func needsNormalize(value string) bool {
	for _, r := range value {
		// unicode.IsSpace 已涵盖空格、制表符与全角空格 U+3000
		if unicode.IsSpace(r) || unicode.IsLower(r) {
			return true
		}
	}
	return false
}

// normalizeName 删除所有空白并统一大写，作为姓名匹配键。
// 快路径：已归一化的输入直接返回原串，使 Name 与 NameKey 共享同一底层数组。
func normalizeName(value string) string {
	if !needsNormalize(value) {
		return value
	}
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range strings.ToUpper(value) {
		if unicode.IsSpace(r) {
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
	if strings.Contains(title, "高三") || strings.Contains(title, "高3") {
		return GradeThree
	}
	return ""
}

func parseQuery(raw string) Query {
	normalized := querySeparators.Replace(strings.TrimSpace(raw))
	query := Query{}
	for _, token := range strings.Fields(normalized) {
		// 年级+班级连写（"高二三班"）优先于年级子串，精确解析为年段+班级
		if match := gradeClassToken.FindStringSubmatch(token); match != nil {
			classNo := classNumber(match[2])
			if classNo < 0 {
				query.NameTokens = append(query.NameTokens, normalizeName(token))
				continue
			}
			query.Grade = parseGrade(match[1])
			if classNo > 0 {
				query.ClassNo = classNo
			}
			continue
		}
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
	// 预分配匹配结果，避免增长到上千条时反复扩容
	matches := make([]Student, 0, min(len(students), 256))
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
		if query.ClassNo > 0 && item.ClassNo != query.ClassNo {
			ok = false
		}
		if ok {
			matches = append(matches, item)
		}
	}
	// 排序热路径只做整数比较与子串计分，不再调用正则
	slices.SortStableFunc(matches, func(a, b Student) int {
		if c := cmp.Compare(nameScoreSum(a.NameKey, query.NameTokens), nameScoreSum(b.NameKey, query.NameTokens)); c != 0 {
			return c
		}
		if c := cmp.Compare(a.GradeIdx, b.GradeIdx); c != 0 {
			return c
		}
		return cmp.Compare(a.ClassNo, b.ClassNo)
	})
	// 分页区间钳制到 [0, len(matches)]：越界的 offset 按最近的有效边界处理，
	// 使 Search 自守分页前置约定，不依赖调用方先行校验。
	// 负 offset 若不在此归一，切片下界会为负并 panic。
	if offset < 0 {
		offset = 0
	}
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

// nameScoreSum 计算姓名匹配总分（越低越优先），供排序比较使用。
func nameScoreSum(nameKey string, tokens []string) int {
	total := 0
	for _, token := range tokens {
		total += nameScore(nameKey, token)
	}
	return total
}

// gradeOrder 返回年段自然顺序，用于跨年段同分排序。
// 顺序来源于 knownGrades 声明序，扩展年段时只需在 knownGrades 末尾追加。
func gradeOrder(grade Grade) int {
	for i, g := range knownGrades {
		if g == grade {
			return i
		}
	}
	return len(knownGrades)
}

