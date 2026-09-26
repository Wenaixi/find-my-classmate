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

// gradeAliases 是年段的书写别名，与 knownGrades 同步维护。
// 别名让用户可以写「高1」，而文件命名、排序与存储仍用规范名「高一」。
// 扩展年段时只需在 knownGrades 追加规范名、在此处追加其别名。
var gradeAliases = []struct {
	name  string
	grade Grade
}{
	{"高1", GradeOne},
	{"高2", GradeTwo},
	{"高3", GradeThree},
}

// gradePattern 由 knownGrades 与 gradeAliases 生成年段匹配源串。
// 每次调用重新生成而非包级 var：包级变量在初始化时即固定，
// 而 knownGrades 是可声明的包级变量，预先求值会让之后追加的年段在正则中缺席。
//
// 为什么必须派生：data.go 的名单标题校验反过来依赖 parseGrade，
// 而 parseGrade 与 gradeClassToken 此前各自硬编码年段字面量。
// 变异事实（2026-09-26 第六轮探针实测）：把新年段只追加到 knownGrades 后，
// 运维提示正确列出该文件、gradeOrder 正确排位，但 loadStudents 报
// 「文件名与年级标题不一致」拒绝加载合法名单，查询返回 total=0 且与
// 真不存在的年段结果完全一致。跨语言对拍无法发现——两端一致地不认识新年段。
func gradePattern() string {
	parts := make([]string, 0, len(knownGrades)+len(gradeAliases))
	for _, g := range knownGrades {
		parts = append(parts, regexp.QuoteMeta(string(g)))
	}
	for _, a := range gradeAliases {
		parts = append(parts, regexp.QuoteMeta(a.name))
	}
	return strings.Join(parts, "|")
}

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
// parsed 由数据加载路径解析一次后传入（C4：不再对同一类名重复正则+Atoi）。
func newStudent(name string, grade Grade, className string, parsed ClassParseResult) Student {
	classNo := 0
	if parsed.Valid {
		classNo = parsed.ClassNo
	}
	return Student{
		Name:      name,
		NameKey:   normalizeName(name),
		Grade:     grade,
		ClassName: className,
		ClassNo:   classNo,
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

// tokenize 把原始查询串切成 token：分隔符归一为空白后按 unicode.IsSpace 切分。
// 抽出的理由是跨语言对拍——前端 parseQuery 返回的 tokens 此前在 Go 侧没有对应物，
// 「两端空白集合逐码位对齐」这条不变量因此只能在单侧断言。把切分单点化后，
// contract_test.go 经同一个函数对拍，避免测试复制一份切分规则。
//
// 注意不能用 strings.FieldsFunc 之类的近似：U+0085（NEL）在 unicode.IsSpace 内
// 是空白、在 JS 的 \s 外不是，两端差集恰为 U+0085 与 U+FEFF 两处且方向相反。
func tokenize(raw string) []string {
	return strings.Fields(querySeparators.Replace(strings.TrimSpace(raw)))
}

// gradeClassToken 匹配年级+班级连写（"高二三班"/"高二1班"/"高一十八班"）。
// 年段部分由 knownGrades 派生，扩展年段无需改动此处。
// 编译结果必须缓存：调用点在 token 循环内，每次重新编译会让整年段查询的
// 分配次数从 6 涨到 117（实测 2026-09-26），是不可接受的零分配热路径退化。
// knownGrades 在生产中不可变，扩展它（如测试构造新年段）后须调用 rebuildGradePattern。
var gradeClassToken = rebuildGradePattern()

// rebuildGradePattern 按当前 knownGrades 重新编译年段+班级连写正则。
// 运行时初始化走包级 var 的首次求值，无需显式调用。
func rebuildGradePattern() *regexp.Regexp {
	return regexp.MustCompile("^(" + gradePattern() + ")([0-9]+|[一二三四五六七八九十]+)班?$")
}

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

// parseGrade 按规范名与别名做子串匹配，与 knownGrades 声明的年段保持一致。
// 不得回退为硬编码比较：data.go 的名单标题校验依赖本函数，
// 漏认识新年段会让合法名单文件被判为「文件名与年级标题不一致」而拒绝加载。
func parseGrade(title string) Grade {
	for _, g := range knownGrades {
		if strings.Contains(title, string(g)) {
			return g
		}
	}
	for _, a := range gradeAliases {
		if strings.Contains(title, a.name) {
			return a.grade
		}
	}
	return ""
}

func parseQuery(raw string) Query {
	query := Query{}
	for _, token := range tokenize(raw) {
		// 年级+班级连写（"高二三班"）优先于年级子串，精确解析为年段+班级
		if match := gradeClassToken.FindStringSubmatch(token); match != nil {
			// 降级策略由 classCondition 单点承载：!Valid 与 Overflow 一律按姓名处理。
			// 本分支额外保留年级条件——不降级为「纯年级」，那会把一次精确查询
			// 放大成整个年段的全量结果，与静默丢弃同属要防的「返回全部」错误。
			classNo, asName := classCondition(match[2], token)
			if asName != "" {
				// 整个 token（连同学级前缀）作为姓名匹配词，而非只取班级部分：
				// 原始输入是「高一一一班」，用户要的就是这个字符串本身。
				// 匹配键由 classCondition 产出，调用方不再自行归一化，
				// 避免「降级用哪种形态」在两处各写一遍。
				query.Grade = parseGrade(match[1])
				query.NameTokens = append(query.NameTokens, asName)
				continue
			}
			query.Grade = parseGrade(match[1])
			query.ClassNo = classNo
			continue
		}
		if match := classToken.FindStringSubmatch(token); match != nil {
			classNo, asName := classCondition(match[1], token)
			if asName != "" {
				query.NameTokens = append(query.NameTokens, asName)
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
func Search(students []Student, raw string, limit, offset int) SearchResponse {
	// 分页钳制必须先于任何提前返回完成，否则「Search 自守分页前置约定」
	// 只在非空查询路径成立：空查询会把负 limit/offset 原样回显进响应。
	// offset 的上界钳制依赖匹配结果长度，留在匹配循环之后。
	if limit < 0 {
		limit = 0
	}
	if offset < 0 {
		offset = 0
	}
	query := parseQuery(raw)
	// 判空依据是「解析后是否存在任何条件」，而非「原始串剥空白后是否为空」。
	// 两处归一化原本不一致：querySeparators 已把中英文逗号、顿号、加号换成空格，
	// 而此处只剥空白，导致纯分隔符输入（如「、」）既不算空查询、也不产生任何条件——
	// 三个筛选条件全部不约束，Search 退化成与输入无关的全校检索。
	// 修复前实测：输入「、」「+++」返回全校 2112 条（2112 为 benchStudents 规模）。
	// 与 v0.9.1 修过的「无法解析/溢出的班级 token 不得静默丢弃」同源：
	// 任何让全部条件落空的输入都必须退化为空结果，而不是全校。
	if len(query.NameTokens) == 0 && query.Grade == "" && query.ClassNo == 0 {
		return SearchResponse{Items: []Student{}, Limit: limit, Offset: offset}
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
	// offset 上界钳制到匹配条数，越界时取最近有效边界。负向钳制已在函数
	// 开头完成（必须先于空查询的提前返回），此处只处理依赖匹配结果的部分。
	if offset > len(matches) {
		offset = len(matches)
	}
	end := len(matches)
	if limit < len(matches)-offset {
		end = offset + limit
	}
	items := matches[offset:end]
	return SearchResponse{Items: items, Total: len(matches), Limit: limit, Offset: offset, HasMore: end < len(matches)}
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
