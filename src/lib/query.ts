import type { Grade, ParsedQuery } from "../types";

const separators = /[，,、+]+/g;
const classDigits: Record<string, string> = { 一: "1", 二: "2", 三: "3", 四: "4", 五: "5", 六: "6", 七: "7", 八: "8", 九: "9", 十: "10" };
// classNumberPattern 精确编码班号的合法形态：阿拉伯数字，或汉字数字的
// 「单字 / 十开头 / 第二字为十」三种。
// 汉字部分此前写 [一二三四五六七八九十]+ 贪婪匹配任意长度，而
// chineseNumberToInt 只认那三种形态，于是超长输入「匹配成功却被静默截断」。
// 与 Go 端 classparse.go 的 classNumberPattern 保持同一份语义。
const chineseNumberPattern = "[一二三四五六七八九]|十[一二三四五六七八九]?|[一二三四五六七八九]十[一二三四五六七八九]?";
const classNumberPattern = "\\d+|" + chineseNumberPattern;
const classToken = new RegExp(`^(${classNumberPattern})班?$`);
// classNumberHead 只判「以班号字符开头」，不判整段可解析：
// 年级+班级连写用它守门，「一一」开头像班号却解析失败，须放行到降级路径。
const classNumberHead = /^[0-9一二三四五六七八九十]/;
// 年段值域与 Go 端 search.go 的 knownGrades 保持同一份事实。
// 硬编码会使「扩展年段只需在 knownGrades 追加」在两端同时失效，
// 且跨语言对拍无法发现——两端一致地不认识新年段，对拍语料必须先有该年段样本。
const gradeValues: ReadonlyArray<Grade> = ["高一", "高二", "高三"];
// 别名 → 规范名：用户可写「高1」，而 grade 字段与后端仍用规范名。
const gradeAliases: ReadonlyArray<readonly [string, Grade]> = [
  ["高1", "高一"],
  ["高2", "高二"],
  ["高3", "高三"],
];
// 声明的年段全貌，供测试锁住「规范名与别名一一对应且都属于合法 Grade」。
export const gradeDomain = { values: gradeValues, aliases: gradeAliases };
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

// parseGrade 与 Go 端 search.go 语义一致：子串匹配（已是班级连写的 token 由 splitGradeClass 优先精确解析）。
// 遍历声明的年段值域而非硬编码比较：扩展年段只需在 gradeValues / gradeAliases 追加。
function parseGrade(token: string): Grade | undefined {
  for (const grade of gradeValues) {
    if (token.includes(grade)) return grade;
  }
  for (const [alias, grade] of gradeAliases) {
    if (token.includes(alias)) return grade;
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

export function parseQuery(raw: string): ParsedQuery {
  // 与 Go 端 strings.Fields 对齐：按 goSpaceSplit 切分，而不是 JS 的 \s。
  // 注意不能用 JS 的 trim 收尾——它会移除首尾的 U+FEFF，而 Go 的 strings.TrimSpace
  // 不认 U+FEFF；"␣18班" 因此会在 JS 被削成 "18班"（判为班级条件）、
  // 在 Go 整体保留为一个姓名 token。切分本身已过滤空串，无需再 trim。
  const tokens = raw.replace(separators, " ").split(goSpaceSplit).filter(Boolean);
  const parsed: ParsedQuery = { tokens, nameTokens: [] };

  for (const token of tokens) {
    // 年级+班级连写（"高二三班"）优先于年级子串，精确解析为年段+班级。
    // 切分由 splitGradeClass 单点承担（最长年段前缀 + 剩余整体走班级解析），
    // 与 Go 端 search.go 同名函数逐条对齐。
    const gradeClass = splitGradeClass(token);
    if (gradeClass) {
      const classNo = classNumber(gradeClass.classPart);
      if (classNo < 0) {
        // 超长数字：班级不可解析，但年级部分仍然可解析。
        // 保留年级条件并把整个 token 按姓名处理——与下方 classNo <= 0 分支同策略，
        // 也与 Go 端 search.go 降级时先写 query.Grade 的行为一致。
        // 修复前本分支直接 continue，parsed.grade 从未赋值：同一条规则的两个镜像
        // 实现因此分叉（Go 判「高二」而 TS 为 undefined），且契约语料缺此形态，
        // 分叉长期无人发现。语料「高二99999999999999999999班」锁住该行为。
        parsed.grade = gradeClass.grade;
        parsed.nameTokens.push(normalizeName(token));
        continue;
      }
      parsed.grade = gradeClass.grade;
      if (classNo <= 0) {
        // 年级可解析而班级不可解析：整个 token 按姓名处理并保留年级条件。
        // 与 Go 端 parseQuery 同策略：不降级为「纯年级」，那会把一次精确查询
        // 放大成整个年段的全量结果；也不静默丢弃，那会退化成全校检索。
        parsed.nameTokens.push(normalizeName(token));
        continue;
      }
      parsed.classNumber = classNo;
      continue;
    }
    const classMatch = token.match(classToken);
    if (classMatch) {
      const classNo = classNumber(token);
      if (classNo < 0) {
        // 超长数字（Go 端 -1 语义）：按姓名处理，避免"返回全部"
        parsed.nameTokens.push(normalizeName(token));
        continue;
      }
      if (classNo <= 0) {
        // 汉字数字查表未命中（0 班号不存在）：按姓名处理而非静默丢弃。
        parsed.nameTokens.push(normalizeName(token));
        continue;
      }
      parsed.classNumber = classNo;
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

function classNumber(value: string): number {
  const match = value.match(classToken);
  if (!match) return 0;
  const digits = match[1];
  if (/^\d+$/.test(digits)) {
    const n = Number(digits);
    // 溢出判定与 Go 端 classNumber 对齐：Atoi 失败即视为无效班级。
    // 前端用 Number.isSafeInteger 表达同一上限（超出安全整数范围的数字串即无效）。
    if (!Number.isSafeInteger(n)) return -1;
    return n;
  }
  return chineseNumberToInt(digits);
}

// 判定查询是否含姓名条件（任一 token 解析后成为姓名匹配词）。
// 供 App 决定"纯年段/班级查询"提示；替代原先 App.tsx 内嵌的 hasNameCondition 正则，
// 消除查询语义的第三份实现（query.ts / search.go / App.tsx 三拷贝 → 两份 + 消费方）。
export function hasNameCondition(raw: string): boolean {
  return parseQuery(raw).nameTokens.length > 0;
}

