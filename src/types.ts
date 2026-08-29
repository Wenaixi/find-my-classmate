export type Grade = "高一" | "高二";

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

export type SearchState = "idle" | "editing" | "loading" | "success" | "duplicate" | "empty" | "too-many" | "error";

export interface ParsedQuery {
  tokens: string[];
  nameTokens: string[];
  grade?: Grade;
  classNumber?: number;
}
