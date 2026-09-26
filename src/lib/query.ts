import type { Grade, ParsedQuery } from "../types";

const separators = /[，,、+]+/g;
const classDigits: Record<string, string> = { 一: "1", 二: "2", 三: "3", 四: "4", 五: "5", 六: "6", 七: "7", 八: "8", 九: "9", 十: "10" };
const classToken = /^(\d+|[一二三四五六七八九十]+)班?$/;
// 年级+班级连写（"高二三班" / "高二1班" / "高一十八班"）→ 精确解析为年段+班级
const gradeClassToken = /^(高一|高二|高三|高1|高2|高3)(\d+|[一二三四五六七八九十]+)班?$/;

// 与 Go 端 unicode.IsSpace 对齐（含 U+0085 NEL）：JS \s 不覆盖 NEL，需显式补上。
// 姓名匹配键必须两端删除同一集合的空白，否则 corpus 盲区会漂移（核实见 docs/query-contract.json）。
export function normalizeName(value: string): string {
  return value.replace(/[\s\u3000\t\u0085]/g, "").toLocaleUpperCase();
}

// parseGrade 与 Go 端 search.go 语义一致：子串匹配（已是班级连写的 token 由 gradeClassToken 优先精确解析）。
function parseGrade(token: string): Grade | undefined {
  if (token.includes("高一") || token.includes("高1")) return "高一";
  if (token.includes("高二") || token.includes("高2")) return "高二";
  if (token.includes("高三") || token.includes("高3")) return "高三";
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
  // 与 Go 端 strings.Fields 对齐：Fields 按 unicode.IsSpace 切分，含 U+0085（NEL）。
  // JS \s 不含 NEL，此处显式补上，避免两端 token 集合漂移（核实见 docs/query-contract.json）。
  const tokens = raw.trim().replace(separators, " ").split(/[\s\u0085]+/).filter(Boolean);
  const parsed: ParsedQuery = { tokens, nameTokens: [] };

  for (const token of tokens) {
    // 年级+班级连写（"高二三班"）优先于年级子串，精确解析为年段+班级
    const gradeClass = token.match(gradeClassToken);
    if (gradeClass) {
      const classNo = classNumber(gradeClass[2]);
      if (classNo < 0) {
        parsed.nameTokens.push(normalizeName(token));
        continue;
      }
      parsed.grade = parseGrade(gradeClass[1]) ?? parsed.grade;
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

