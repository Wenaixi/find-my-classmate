import type { ParsedQuery, SearchResponse, Student } from "../types";

const separators = /[，,、+]+/g;
const gradeAliases: Record<string, "高一" | "高二"> = { 高一: "高一", 高1: "高一", 高二: "高二", 高2: "高二" };
const classDigits: Record<string, string> = { 一: "1", 二: "2", 三: "3", 四: "4", 五: "5", 六: "6", 七: "7", 八: "8", 九: "9", 十: "10" };
const classToken = /^(\d+|[一二三四五六七八九十]+)班?$/;

export function normalizeName(value: string): string {
  return value.replace(/[\s\u3000\t]/g, "").toLocaleUpperCase();
}

export function parseQuery(raw: string): ParsedQuery {
  const tokens = raw.trim().replace(separators, " ").split(/\s+/).filter(Boolean);
  const parsed: ParsedQuery = { tokens, nameTokens: [] };

  for (const token of tokens) {
    const classMatch = token.match(classToken);
    if (classMatch) {
      parsed.classNumber = Number(classMatch[1]) || Number(classDigits[classMatch[1]] ?? 0);
      continue;
    }
    if (gradeAliases[token]) {
      parsed.grade = gradeAliases[token];
      continue;
    }
    parsed.nameTokens.push(normalizeName(token));
  }
  return parsed;
}

function classNumber(value: string): number {
  const match = value.match(classToken);
  return match ? Number(match[1]) || Number(classDigits[match[1]] ?? 0) : 0;
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
      return scoreA - scoreB || classNumber(a.student.className) - classNumber(b.student.className);
    })
    .map(({ student }) => student);

  const safeOffset = Math.min(Math.max(offset, 0), items.length);
  const page = items.slice(safeOffset, safeOffset + limit);
  return { items: page, total: items.length, limit, offset: safeOffset, hasMore: safeOffset + page.length < items.length };
}
