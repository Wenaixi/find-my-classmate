import type { Grade, ParsedQuery } from "../types";

const separators = /[，,、+]+/g;

// 汉字数字的单字符集是班级域的跨语言契约：Go 端 classparse.go 的三处用途
// （正则字符类、连写守门、值映射）与本文件的三处此前各手抄一遍，共六份。
// 第十八轮变异实验实测：把两端 classNumberHead 同时删掉「四」与「七」，
// Go 全量测试与前端 194 条全部零翻红——语料与对拍在结构上表达不出
// 「两端一致地错」，而守门收窄会让「高一四班」从「高一 + 4 班」
// 退化为整年段全量返回。
//
// 因此这组字符集必须有单一 owner：下方 chineseDigitOnes 是它，
// 三处用途全部由它派生。
//
// 「十」单独处理而非并入：它的值是 10 而不是位置即值，
// 而 chineseDigitOnes 的顺序恰好是值序（index+1），两者不能共用一条推导。
const chineseDigitOnes = "一二三四五六七八九";
const chineseOnesClass = "[" + chineseDigitOnes + "]";
// classDigits 是汉字数字到班号的真值表，与 chineseDigitOnes 同源派生。
// 「十」不在 chineseDigitOnes 里（它的值不是位置即值），单独补入。
//
// 元组用 as const 而非 Record：显式标注会把键拓宽回 string，
// 而 chineseNumberToInt 按运行时字符串查表，下游确实需要一个可索引的
// 宽类型——见下方 classDigits 的 Record 标注。
//
// 此前此处有一条「键集自洽检查」（Exclude 元组键联合, keyof typeof classDigits）。
// 它恒不发作：classDigits 经 Object.fromEntries 构造，其键类型被拓宽为 string
// （标注与 fromEntries 自身的 lib.es2019 签名都会拓宽，删掉标注也无济于事），
// 于是 Exclude<X, string> 对任何 X 都是 never。
// 变异实验实测：把值表整个换成 { "十": "10" }，tsc -b 仍然 CLEAN；
// 而把 chineseDigitOnes 漏掉「四」也不报警——表由该字符串派生，两者同步变化。
// 它声称防的「漏字」在派生写法下本就结构性不可能，故按本仓纪律删除
// 一条不存在的防线，而不是留一个让读者以为有保护的空转检查。
// 该行的真实保护是：值表由 chineseDigitOnes 派生（漏字不可能），
// 解析行为由契约语料覆盖（classNumber 非空的样本有 25 条）。
const classDigitEntries = [
  ...[...chineseDigitOnes].map((d, i) => [d, String(i + 1)] as const),
  ["十", "10"] as const,
] as const;
const classDigits: Record<string, string> = Object.fromEntries(classDigitEntries);

// classNumberPattern 精确编码班号的合法形态：阿拉伯数字，或汉字数字的
// 「单字 / 十开头 / 第二字为十」三种。
// 汉字部分此前写 [一二三四五六七八九十]+ 贪婪匹配任意长度，而
// chineseNumberToInt 只认那三种形态，于是超长输入「匹配成功却被静默截断」。
// 与 Go 端 classparse.go 的 classNumberPattern 保持同一份语义，
// 字符类由 chineseDigitOnes 派生，使两端无法分叉。
const chineseNumberPattern = chineseOnesClass + "|十" + chineseOnesClass + "?|" + chineseOnesClass + "十" + chineseOnesClass + "?";
const classNumberPattern = "\\d+|" + chineseNumberPattern;
const classToken = new RegExp(`^(${classNumberPattern})班?$`);
// classNumberHead 只判「以班号字符开头」，不判整段可解析：
// 年级+班级连写用它守门，「一一」开头像班号却解析失败，须放行到降级路径。
// 字符集与 chineseDigitOnes 同源派生：它曾零对拍且收窄即致整年段全量。
const classHeadChars = "[0-9" + chineseDigitOnes + "十]";
const classNumberHead = new RegExp("^" + classHeadChars);
// 年段值域与 Go 端 search.go 的 knownGrades 保持同一份事实。
// 硬编码会使「扩展年段只需在 knownGrades 追加」在两端同时失效，
// 且跨语言对拍无法发现——两端一致地不认识新年段，对拍语料必须先有该年段样本。
//
// 声明为字面量元组而非 ReadonlyArray<Grade>：显式类型标注会把字面量拓宽回 Grade，
// 于是「Grade 联合里有、gradeValues 里缺」在类型层面恒成立，下方的反向完整性
// 检查形同虚设。as const 保留字面量类型，satisfies 保住另一个方向：写出的值
// 必须属于 Grade。两者缺一不可——只写 as const 会丢掉正向强制，只写 satisfies
// 则类型仍被拓宽。
const gradeValues = ["高一", "高二", "高三"] as const satisfies readonly Grade[];
// 别名 → 规范名：用户可写「高1」，而 grade 字段与后端仍用规范名。
// 别名的规范名目标同样受 satisfies 约束，指向未声明年段时点名报错。
const gradeAliases = [
  ["高1", "高一"],
  ["高2", "高二"],
  ["高3", "高三"],
] as const satisfies readonly (readonly [string, Grade])[];
// 声明的年段全貌，供测试锁住「规范名与别名一一对应且都属于合法 Grade」。
export const gradeDomain = { values: gradeValues, aliases: gradeAliases };

// 年段值域的反向完整性：Grade 联合里出现 gradeValues 未声明的年段时报错。
// 两处反方向检查各由一处承担——正向（列表里的值必须属于 Grade）由上面的
// satisfies 强制，本处强制反向（Grade 里的值必须已在列表中声明）。
// 此前这条反向由 query.test.ts 里手抄的枚举数组承担，而手抄副本在 Grade
// 联合追加新成员时不会跟着长：实测加「高四」后 tsc 干净、全部用例通过，
// 而 parseQuery 已不再识别该年段。断言存在却不承重它声称的对象，比没有
// 断言更危险——它提供虚假的覆盖感。
// 报错形态刻意带出缺失项本身（缺「高四」时该类型退化为 { missing: "高四" }），
// 而不是让 Exclude 直接得 never 后报一条无从解读的 never 不兼容。
//
// 检查必须落在非测试文件：.dockerignore 排除 src/**/*.test.ts，
// 放进测试文件则容器镜像构建的 tsc -b 不再检查它，会造出一条「本地绿、
// 镜像不查」的隐形防线。该常量在生产代码中无人消费，tsc 不报错
//（tsconfig.app.json 未开 noUnusedLocals，全仓无 eslint 配置与 lint 脚本）。
type GradeDomainGap = Exclude<Grade, (typeof gradeValues)[number]>;
const gradeDomainExhaustive: GradeDomainGap extends never ? true : { missing: GradeDomainGap } = true;
void gradeDomainExhaustive;
// splitGradeClass 把「年段+班级连写」切成年段与班级两段，语义与 Go 端
// search.go 的同名函数逐条对齐。
//
// 为什么不用一条正则切：年段组与班级组共享汉字字符集，引擎回溯时会把班级部分
// 从中间切开——「高二十二班」的班级部分被切成「十二」而非「二十二」，
// 而数据加载路径 chineseNumberToInt("二十二") 得到 22。同一串字两条路径分裂。
// 改用「最长年段前缀 + 剩余整体交给班级解析」后，两条路径必然同源。
// JS 的 length 与 Go 的 len 同样按 UTF-16 码元计，但最长前缀的比较在这里
// 只用于挑选候选、不用于切片，因此与 Go 端 utf8.RuneCountInString 的差异
// 不会显现（中文年段在两者下长度一致）。
function splitGradeClass(token: string): { grade: Grade; classPart: string } | null {
  let best = "";
  let grade: Grade | undefined;
  for (const g of gradeValues) {
    if (token.startsWith(g) && g.length > best.length) {
      best = g;
      grade = g;
    }
  }
  for (const [alias, canonical] of gradeAliases) {
    if (token.startsWith(alias) && alias.length > best.length) {
      best = alias;
      grade = canonical;
    }
  }
  if (best === "" || grade === undefined) return null;
  const rest = token.slice(best.length);
  if (rest === "") return null;
  const classPart = rest.endsWith("班") ? rest.slice(0, -1) : rest;
  // 守门只判「开头像班号」，不判整段可解析：「高一同学」是姓名而非连写，
  // 而「一一」开头像班号却解析失败，必须放行到降级路径。
  if (!classNumberHead.test(classPart)) return null;
  return { grade, classPart };
}

// 空白集合与 Go 端 unicode.IsSpace 逐码位对齐，不用 JS 的 \s 近似：
// 两者的差集有两个码位且方向相反——U+0085（NEL）是 Go 的空白而 \s 不含，
// U+FEFF 自 Unicode 4.0.1 起不在 White_Space 内、Go 不认而 \s 认。
// 用 \s 会在 U+FEFF 上静默与 Go 分叉：查询分词被切开、姓名匹配键被删除。
// 下方两行逐码位枚举 Go 的集合，是这两处差集的唯一修正点。
const goSpaceChars = "\\t\\n\\v\\f\\r \\u0085\\u00a0\\u1680\\u2000-\\u200a\\u2028\\u2029\\u202f\\u205f\\u3000";
const goSpace = new RegExp("[" + goSpaceChars + "]", "g");
const goSpaceSplit = new RegExp("[" + goSpaceChars + "]+");

// 姓名匹配键：删除与 Go 相同的空白集合，再统一大写。
// 用 toUpperCase 而非 toLocaleUpperCase：后者随宿主 locale 变化（tr/az 下
// "i" 映射为 U+0130），而 Go strings.ToUpper 是 locale 无关的。
export function normalizeName(value: string): string {
  return value.replace(goSpace, "").toUpperCase();
}

// parseGrade 与 Go 端 search.go 的 parseGradeInToken 语义一致：精确匹配。
//
// 曾用子串匹配（token.includes），与 Go 端同样会把「高一鸣」这类含年段串的
// 姓名读成年段条件，使姓名条件被吞掉。Go 端已把标题侧（子串）与查询侧（精确）
// 拆成两个函数；前端没有标题校验的消费方，故只需这一个精确语义。
// 遍历声明的年段值域而非硬编码比较：扩展年段只需在 gradeValues / gradeAliases 追加。
function parseGrade(token: string): Grade | undefined {
  for (const grade of gradeValues) {
    if (token === grade || token === grade + "班") return grade;
  }
  for (const [alias, grade] of gradeAliases) {
    if (token === alias || token === alias + "班") return grade;
  }
  return undefined;
}

// chineseNumberToInt 与 Go 端 search.go 一致：支持一~九十九。
function chineseNumberToInt(value: string): number {
  if (!value) return 0;
  const runes = [...value];
  if (runes[0] === "十") {
    return 10 + (runes.length > 1 ? Number(classDigits[runes[1]] ?? 0) : 0);
  }
  if (runes.length >= 2 && runes[1] === "十") {
    const tens = Number(classDigits[runes[0]] ?? 0);
    const ones = runes.length > 2 ? Number(classDigits[runes[2]] ?? 0) : 0;
    return tens > 0 ? tens * 10 + ones : 0;
  }
  return Number(classDigits[value] ?? 0);
}

// classCondition 把一个班级片段解释为「已解释的条件片段」，与 Go 端
// classparse.go 的同名函数同形：两端的降级策略收在同一处，两条路径必然同源。
//
// matchPart 是班级部分（用于判定班号），rawToken 是用户输入的整个 token
// （用于降级为姓名条件时的匹配键）。两者必须分开：查询「一一班」时班级部分是
// 「一一」，而降级后的姓名匹配键必须是「一一班」——用户输入的是后者。
//
// 不变量：无法解析与数字溢出都降级为姓名条件，绝不丢弃 token。丢弃会让全部
// 条件落空，Search 退化成与用户输入无关的全校检索——这不是理论风险，v0.9.1
// 修复前输入「一一班」曾返回全校 1047 条。
//
// 修复前该策略在 parseQuery 内联了四处（连写+溢出、连写+非法、纯班级+溢出、
// 纯班级+非法），其中两对逐字等价、每处自带一份理由、部分理由只存在于其中
// 一处靠「照抄旁边那段」维持。Go 端 classCondition 的注释自陈它经历过同样的
// 阶段（「此前 parseQuery 有四段近乎逐字重复」），而前端至今停在那里——同一条
// 规则的两个镜像实现分叉过一次（Go 判「高二」而 TS 为 undefined，靠补语料
// 才发现）。形状不变，裂缝会再开。
//
// 返回值约定（两条，与 Go 端 classCondition 一致）：classNo > 0 表示解析为
// 班级条件；否则 asName 非空，调用方把它追加到姓名条件。两端都没有第三种
// 返回态——判定「是否为班级 token」由调用点的守门承担（classToken 整段匹配
// 与 classNumberHead 首字符匹配），不落在本函数内。
function classCondition(matchPart: string, rawToken: string): { classNo: number; asName: string } {
  const parsed = classNumber(matchPart);
  if (parsed.ok && parsed.classNo > 0) {
    return { classNo: parsed.classNo, asName: "" };
  }
  // 降级：匹配键取 rawToken（用户原始输入）而非 matchPart——「降级时用哪个形态
  // 参与匹配」与「降级」本身是同一个决定，收在这里使调用方无法传错。
  return { classNo: 0, asName: normalizeName(rawToken) };
}

export function parseQuery(raw: string): ParsedQuery {
  // 与 Go 端 strings.Fields 对齐：按 goSpaceSplit 切分，而不是 JS 的 \s。
  // 注意不能用 JS 的 trim 收尾——它会移除首尾的 U+FEFF，而 Go 的 strings.TrimSpace
  // 不认 U+FEFF；"␣18班" 因此会在 JS 被削成 "18班"（判为班级条件）、
  // 在 Go 整体保留为一个姓名 token。切分本身已过滤空串，无需再 trim。
  const tokens = raw.replace(separators, " ").split(goSpaceSplit).filter(Boolean);
  const parsed: ParsedQuery = { tokens, nameTokens: [] };

  for (const token of tokens) {
    // 年段+班级连写（"高二三班"）：切分由 splitGradeClass 单点承担，
    // 班级判定与降级策略由 classCondition 单点承担，两个调用点形状不同
    // （连写要额外保留年段条件、纯班级没有年段可保留），策略本身不泄漏。
    const gradeClass = splitGradeClass(token);
    if (gradeClass) {
      parsed.grade = gradeClass.grade;
      const condition = classCondition(gradeClass.classPart, token);
      if (condition.classNo > 0) {
        parsed.classNumber = condition.classNo;
      } else {
        parsed.nameTokens.push(condition.asName);
      }
      continue;
    }
    if (classToken.test(token)) {
      const condition = classCondition(token, token);
      if (condition.classNo > 0) {
        parsed.classNumber = condition.classNo;
      } else {
        parsed.nameTokens.push(condition.asName);
      }
      continue;
    }
    const grade = parseGrade(token);
    if (grade) {
      parsed.grade = grade;
      continue;
    }
    parsed.nameTokens.push(normalizeName(token));
  }
  return parsed;
}

// 班级判定的显式三态，与 Go 端 ClassParseResult 同形。
// 修复前用 -1（溢出）与 0（非法/不匹配）两个哨兵表达，而哨兵之间行为完全相同——
// 零信息量，且「0 既是非法又是合法 0 班号」正是 Go 端第十三轮消除的零值歧义。
//
// Overflow 一值在两端都零生产消费方：三态化的收益是类型层——非法与溢出在
// 类型上可区分，调用方不必靠「两个值恰好同行为」来推断它没漏判——
// 而不是行为层的差异化处置。当前两端对二者同策略（都按姓名处理）。
export type ClassNumberResult =
  | { ok: true; classNo: number }
  | { ok: false; reason: "invalid" }
  | { ok: false; reason: "overflow" };

function classNumber(value: string): ClassNumberResult {
  const match = value.match(classToken);
  if (!match) return { ok: false, reason: "invalid" };
  const digits = match[1];
  if (/^\d+$/.test(digits)) {
    const n = Number(digits);
    // 溢出判定与 Go 端对齐：Atoi 失败即视为无效班级。
    // 前端用 Number.isSafeInteger 表达同一上限（超出安全整数范围的数字串即无效）。
    if (!Number.isSafeInteger(n)) return { ok: false, reason: "overflow" };
    return { ok: true, classNo: n };
  }
  const parsed = chineseNumberToInt(digits);
  return parsed > 0 ? { ok: true, classNo: parsed } : { ok: false, reason: "invalid" };
}

// 判定查询是否含姓名条件（任一 token 解析后成为姓名匹配词）。
// 供 App 决定"纯年段/班级查询"提示；替代原先 App.tsx 内嵌的 hasNameCondition 正则，
// 消除查询语义的第三份实现（query.ts / search.go / App.tsx 三拷贝 → 两份 + 消费方）。
export function hasNameCondition(raw: string): boolean {
  return parseQuery(raw).nameTokens.length > 0;
}

