// 年段值域与 lib/query.ts 的 gradeValues 保持一致：新增年段时两处同改。
// gradeValues 声明为 ReadonlyArray<Grade>，两者不一致由编译器直接报错，无需测试兜底。
export type Grade = "高一" | "高二" | "高三";

export interface Student {
  name: string;
  grade: Grade;
  className: string;
}

export interface SearchResponse {
  items: Student[];
  total: number;
  limit: number;
  offset: number;
  hasMore: boolean;
}

export type SearchState = "idle" | "editing" | "loading" | "success" | "duplicate" | "empty" | "error";

export interface ParsedQuery {
  tokens: string[];
  nameTokens: string[];
  grade?: Grade;
  classNumber?: number;
}
