import type { SearchResponse, Student } from "../types";

interface ApiStudent extends Omit<Student, "className"> {
  class?: string;
  className?: string;
}

export async function searchApi(query: string, limit = 10, offset = 0, signal?: AbortSignal): Promise<SearchResponse> {
  const params = new URLSearchParams({ q: query, limit: String(limit), offset: String(offset) });
  const response = await fetch("/api/search?" + params, { signal });
  if (!response.ok) throw new Error("request-failed");
  const payload = (await response.json()) as Omit<SearchResponse, "items"> & { items: ApiStudent[] };
  if (!payload || !Array.isArray(payload.items) || typeof payload.total !== "number" || typeof payload.limit !== "number" || typeof payload.offset !== "number" || typeof payload.hasMore !== "boolean") throw new Error("invalid-response");
  return { ...payload, items: payload.items.map((item) => ({ name: item.name, grade: item.grade, className: item.className ?? item.class ?? "" })) };
}
