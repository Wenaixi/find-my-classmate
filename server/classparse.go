package main

import (
	"regexp"
	"strconv"
	"strings"
)

// 班级解析基础设施：班级名 → 班号，名单域共享概念。
// search.go 的查询解析与 data.go 的名单加载校验都依赖它，
// 归位到独立文件后，改查询语义不会静默改变数据文件校验行为。

// 汉字数字的单字符集是班级域的跨语言契约，两端各需一份声明：
// 本文件的三处用途（正则字符类、连写守门、值映射）此前各手抄一遍，
// TS 端 query.ts 同样手抄三处，共六份。第十八轮变异实验实测：
// 把两端 classNumberHead 同时删掉「四」与「七」，Go 全量测试与前端 194 条
// 全部零翻红——语料与对拍在结构上表达不出「两端一致地错」，
// 而守门收窄会让「高一四班」从「高一 + 4 班」退化为整年段全量返回。
//
// 因此这组字符集必须有单一 owner：下方 chineseDigitOnes 是它，
// 正则字符类、classNumberHead 与 classDigits 全部由它派生。
//
// 「十」单独声明而非并入：它的值是 10 而不是位置即值，
// 而 chineseDigitOnes 的顺序恰好是值序（index+1），两者不能共用一条推导。
const chineseDigitOnes = "一二三四五六七八九"

// chineseNumberPattern 是班号的合法形态，精确编码两种写法：
// 阿拉伯数字（任意长度，是否溢出由 Atoi 判定）与汉字数字的
// 「单字 / 十开头 / 第二字为十」三种形态。
//
// 汉字部分此前写 [一二三四五六七八九十]+ 贪婪匹配任意长度，
// 而 chineseNumberToInt 只认上述三种形态，于是超长输入
// 「匹配成功却被静默截断」：「九十九十九」取前三位得 99、「十十」得 20、
// 「二十一十」得 21，且都声称 Valid=true——畸形班名因此被当成合法班号。
// 正则的接受域必须与解析器的接受域一致，超长输入应在匹配阶段就落选。
//
// 字符类由 chineseDigitOnes 派生而非手写：手写副本曾与 classNumberHead
// 的字符集分叉（后者漏字无从发现，因为两端可同时漏）。
// 拼接在编译期完成，不把正则构造放进热路径。
const (
	chineseOnesClass     = "[" + chineseDigitOnes + "]"
	chineseNumberPattern = chineseOnesClass + "|十" + chineseOnesClass + "?|" + chineseOnesClass + "十" + chineseOnesClass + "?"
	// classNumberPattern 是完整班号（阿拉伯数字或汉字数字）。
	// 查询侧的年级+班级连写正则与数据侧的班级名正则共用它，
	// 使两条路径对同一串字必然得到同一个班号。
	classNumberPattern = "[0-9]+|" + chineseNumberPattern
)

var classToken = regexp.MustCompile("^(" + classNumberPattern + ")班?$")

// classNumberHead 只判「以班号字符开头」，不判整段可解析。
// 年级+班级连写的识别用它守门：「高一同学」开头不是班号字符（是姓名），
// 而「一一」开头是班号字符却解析失败——后者必须放行到 classCondition 降级，
// 整段匹配（classToken）会把它一并挡在降级路径之外。
//
// 字符集与 chineseDigitOnes 同源派生：它是本文件唯一一处曾零对拍而
// 收窄即致整年段全量的声明，现已无法与 chineseNumberPattern 分叉。
var classNumberHead = regexp.MustCompile("^" + classHeadChars)

// classHeadChars 是 classNumberHead 的字符集：阿拉伯数字加十个汉字数字。
// 单独提出为常量，使跨语言对拍能直接锁它——锁整个正则会被 "^" 与
// 字符类的包装形式干扰，而真正需要两端一致的是这串字符。
const classHeadChars = "[0-9" + chineseDigitOnes + "十]"

// classDigits 是汉字数字到班号的真值表，与 chineseDigitOnes 同源派生。
// 「十」不在 chineseDigitOnes 里（它的值不是位置即值），单独补入。
// 派生而非手写，使值表与两处正则无法分叉。
var classDigits = func() map[string]int {
	digits := make(map[string]int, len(chineseDigitOnes)+1)
	// 序号取字符位置而非字节偏移：Go 的 range 遍历字符串时 i 是字节下标，
	// 中文字符占三字节，按 i+1 赋值会得到「四:10」「五:13」这类错值。
	// 正则字符类全对而值表错时只有走值表的用例翻红——探针实测才发现。
	pos := 0
	for _, r := range chineseDigitOnes {
		digits[string(r)] = pos + 1
		pos++
	}
	digits["十"] = 10
	return digits
}()

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
	// 班号必须是正数：「0」语法上是合法数字（strconv.Atoi 成功且无错），
	// 但班级域里不存在 0 班。此处原样把 0 包成 Valid=true，违背本文件声明的
	// 「Valid ⇒ ClassNo > 0」不变量——下游 classCondition 因此既不产生班级条件
	// 也不降级为姓名条件，token 静默消失；与年级连写时（"高一0班"）条件落空，
	// Search 退化为整年段全量返回。不变量必须由实现强制，不能只写在注释里。
	if number, err := strconv.Atoi(match[1]); err == nil {
		// Atoi 成功即证明这是格式合法的数字，因此不可能是溢出。
		// 班号必须为正：「0」在班级域里不存在，属非法而非溢出——
		// 若落入下方 isAllDigits 分支会被谎报成 Overflow，两种语义不可混同。
		if number > 0 {
			return ClassParseResult{ClassNo: number, Valid: true}
		}
		return ClassParseResult{}
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
// 返回值约定（两条，无第三态）：
//   - asName 非空：token 降级为姓名条件，调用方应把 asName 追加到姓名条件；
//   - classNo > 0：token 解析为班级条件，调用方应设为班级条件。
//
// 「两者都空」在本函数的返回值域内是空集，因此调用方无需第三种出口：
// Valid 为真时 ClassNo 恒大于零（parseClassName 强制该不变量，Atoi 成功的
// 零值已在那里被判为非法），!Valid 时 asName 恒非空（rawToken 来自 tokenize，
// 恒含至少一个非空白字符）。两处调用点也都先守门再调用本函数。
// 第十八轮核实：原注释的第三条「两者都空 → 走非班级分支」是一个永不触发的
// 返回态，按它实现新调用点的人会多写一个死分支。
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
