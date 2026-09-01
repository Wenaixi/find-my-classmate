import type { SearchResponse, Student } from "../types";

interface ApiStudent extends Omit<Student, "className"> {
  class?: string;
  className?: string;
}

// ApiError 携带 HTTP 状态与错误码，供前端按 400/429/500 分文案。
export class ApiError extends Error {
  status?: number;
  code?: string;
  constructor(message: string, status?: number, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

const REQUEST_TIMEOUT_MS = 10_000;

export async function searchApi(query: string, limit = 10, offset = 0, signal?: AbortSignal): Promise<SearchResponse> {
  const params = new URLSearchParams({ q: query, limit: String(limit), offset: String(offset) });
  const timeoutSignal = AbortSignal.timeout(REQUEST_TIMEOUT_MS);
  const combined = signal ? AbortSignal.any([signal, timeoutSignal]) : timeoutSignal;
  let response: Response;
  try {
    response = await fetch("/api/search?" + params, { signal: combined });
  } catch (cause) {
    if (signal?.aborted) throw cause; // 用户主动取消：原样抛出
    throw new ApiError("request-timeout-or-network", undefined, "network");
  }
  if (!response.ok) {
    let code: string | undefined;
    try {
      const body = (await response.json()) as { error?: string };
      code = body.error;
    } catch {
      // 非 JSON 错误体（理论不会发生，429 已统一 JSON）
    }
    throw new ApiError("request-failed", response.status, code);
  }
  const payload = (await response.json()) as Omit<SearchResponse, "items"> & { items: ApiStudent[] };
  if (
    !payload ||
    !Array.isArray(payload.items) ||
    typeof payload.total !== "number" ||
    typeof payload.limit !== "number" ||
    typeof payload.offset !== "number" ||
    typeof payload.hasMore !== "boolean" ||
    !payload.items.every((item) => typeof item.name === "string" && item.name !== "")
  ) {
    throw new ApiError("invalid-response", response.status, "invalid-response");
  }
  return { ...payload, items: payload.items.map((item) => ({ name: item.name, grade: item.grade, className: item.className ?? item.class ?? "" })) };
}
