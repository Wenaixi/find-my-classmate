import type { Grade, ParsedQuery } from "../types";

const separators = /[，,、+]+/g;
const classDigits: Record<string, string> = { 一: "1", 二: "2", 三: "3", 四: "4", 五: "5", 六: "6", 七: "7", 八: "8", 九: "9", 十: "10" };
const classToken = /^(\d+|[一二三四五六七八九十]+)班?$/;
// F71：年级+班级连写（"高二三班" / "高二1班" / "高一十八班"）→ 精确解析为年段+班级
const gradeClassToken = /^(高一|高二|高三|高1|高2|高3)(\d+|[一二三四五六七八九十]+)班?$/;

export function normalizeName(value: string): string {
  return value.replace(/[\s\u3000\t]/g, "").toLocaleUpperCase();
}

// parseGrade 与 Go 端 search.go 语义一致：子串匹配（已是班级连写的 token 由 gradeClassToken 优先精确解析，F71）。
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
  const tokens = raw.trim().replace(separators, " ").split(/\s+/).filter(Boolean);
  const parsed: ParsedQuery = { tokens, nameTokens: [] };

  for (const token of tokens) {
    // F71：年级+班级连写（"高二三班"）优先于年级子串，精确解析为年段+班级
    const gradeClass = token.match(gradeClassToken);
    if (gradeClass) {
      const classNo = classNumber(gradeClass[2]);
      if (classNo < 0) {
        parsed.nameTokens.push(normalizeName(token));
        continue;
      }
      parsed.grade = parseGrade(gradeClass[1]) ?? parsed.grade;
      if (classNo > 0) parsed.classNumber = classNo;
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

