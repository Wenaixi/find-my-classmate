// 年段值域与 lib/query.ts 的 gradeValues 保持一致：新增年段时两处同改。
// gradeValues 声明为 ReadonlyArray<Grade>，只强制「列表里的元素属于 Grade」这一个方向：
// 写出联合类型之外的值会编译报错，但「Grade 里有、列表里缺」拦不住——
// 漏追加新年段时 tsc 与全部用例都通过（实测），parseQuery 已不再识别该年段。
// 反向由 query.test.ts 的 gradeDomain 断言锁住，不依赖契约语料恰好含有该年段样本。
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
