import type { ParsedQuery, SearchResponse, Student } from "../types";

const separators = /[，,、+]+/g;
const classDigits: Record<string, string> = { 一: "1", 二: "2", 三: "3", 四: "4", 五: "5", 六: "6", 七: "7", 八: "8", 九: "9", 十: "10" };
const classToken = /^(\d+|[一二三四五六七八九十]+)班?$/;

export function normalizeName(value: string): string {
  return value.replace(/[\s\u3000\t]/g, "").toLocaleUpperCase();
}

// parseGrade 与 Go 端 search.go 语义一致：子串匹配（口语化输入如"高二三班"按年级处理）。
function parseGrade(token: string): "高一" | "高二" | undefined {
  if (token.includes("高一") || token.includes("高1")) return "高一";
  if (token.includes("高二") || token.includes("高2")) return "高二";
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
    if (!Number.isSafeInteger(n) || n > 1_000_000) return -1;
    return n;
  }
  return chineseNumberToInt(digits);
}

function nameScore(nameKey: string, token: string): number {
  if (nameKey === token) return 0;
  if (nameKey.startsWith(token)) return 1;
  return 2;
}

export function searchStudents(students: Student[], raw: string, limit = 10, offset = 0): SearchResponse {
  const query = parseQuery(raw);
  if (!raw.trim()) return { items: [], total: 0, limit, offset: 0, hasMore: false };

  const items = students
    .map((student) => ({ student, nameKey: normalizeName(student.name) }))
    .filter(({ student, nameKey }) => {
      const nameMatch = query.nameTokens.every((token) => nameKey.includes(token));
      const classMatch = !query.classNumber || classNumber(student.className) === query.classNumber;
      return nameMatch && (!query.grade || student.grade === query.grade) && classMatch;
    })
    .sort((a, b) => {
      const scoreA = query.nameTokens.reduce((sum, token) => sum + nameScore(a.nameKey, token), 0);
      const scoreB = query.nameTokens.reduce((sum, token) => sum + nameScore(b.nameKey, token), 0);
      if (scoreA !== scoreB) return scoreA - scoreB;
      // F23：与 Go 端一致，同分时按年级升序（高一 < 高二）
      if (a.student.grade !== b.student.grade) {
        return a.student.grade === "高一" ? -1 : 1;
      }
      return classNumber(a.student.className) - classNumber(b.student.className);
    })
    .map(({ student }) => student);

  const safeOffset = Math.min(Math.max(offset, 0), items.length);
  const page = items.slice(safeOffset, safeOffset + limit);
  return { items: page, total: items.length, limit, offset: safeOffset, hasMore: safeOffset + page.length < items.length };
}
