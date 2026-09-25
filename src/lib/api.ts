import type { Grade, SearchResponse, Student } from "../types";
import { REQUEST_TIMEOUT_MS } from "../config";

// ApiStudent 是 /api/search 的 wire 形状，仅在本模块内用于解码。
// 服务端 canonical 字段是 class；className 只是旧客户端兼容别名，不对外输出。
interface ApiStudent {
  name?: unknown;
  grade?: unknown;
  class?: unknown;
  className?: unknown;
}

// gradeIndex 是服务端可能返回的年段全集，与 server/search.go 的 knownGrades 对齐。
// 固定三值用 Record 查表，避免为静态字面量分配 Set。
const gradeIndex: Record<string, true> = { 高一: true, 高二: true, 高三: true };

// decodeItem 把一条 wire 记录收敛为合法领域形状，或返回 null 表示不可接受。
// 兼容规则：class 与 className 至少一个非空字符串；两者同时存在时必须一致。
// 缺字段、类型错误、未知年段与别名冲突都不静默降级为缺省值，避免错误数据进入界面。
function decodeItem(raw: unknown): Student | null {
  if (!raw || typeof raw !== "object") return null;
  const item = raw as ApiStudent;
  if (typeof item.name !== "string" || item.name === "") return null;
  if (typeof item.grade !== "string" || gradeIndex[item.grade] !== true) return null;
  const wireClass = typeof item.class === "string" && item.class !== "" ? item.class : undefined;
  const legacyClass = typeof item.className === "string" && item.className !== "" ? item.className : undefined;
  if (!wireClass && !legacyClass) return null;
  if (wireClass && legacyClass && wireClass !== legacyClass) return null;
  const className = wireClass ?? legacyClass;
  if (className === undefined) return null;
  return { name: item.name, grade: item.grade as Grade, className };
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

function combineSignals(a?: AbortSignal, b?: AbortSignal): AbortSignal | undefined {
  if (!a) return b;
  if (!b) return a;
  if (typeof AbortSignal.any === "function") {
    return AbortSignal.any([a, b]);
  }
  const controller = new AbortController();
  if (a.aborted || b.aborted) {
    controller.abort();
    return controller.signal;
  }
  const onAbort = () => controller.abort();
  a.addEventListener("abort", onAbort, { once: true });
  b.addEventListener("abort", onAbort, { once: true });
  return controller.signal;
}

// 读取运行时版本（/api/health 由 ldflags 注入 main.version，200 与 503 degraded 均携带）。
// 展示为尽力而为：失败由调用方静默忽略。
export async function fetchVersion(signal?: AbortSignal): Promise<string> {
  const combined = combineSignals(signal, AbortSignal.timeout(REQUEST_TIMEOUT_MS));
  let body: { version?: unknown };
  try {
    const response = await fetch("/api/health", { signal: combined });
    body = (await response.json()) as { version?: unknown };
  } catch (cause) {
    if (signal?.aborted) throw cause;
    throw new ApiError("health-unreachable", undefined, "network");
  }
  if (typeof body.version !== "string" || body.version === "") {
    throw new ApiError("invalid-response", undefined, "invalid-response");
  }
  return body.version;
}

export async function searchApi(query: string, limit = 10, offset = 0, signal?: AbortSignal): Promise<SearchResponse> {
  const params = new URLSearchParams({ q: query, limit: String(limit), offset: String(offset) });
  const timeoutSignal = AbortSignal.timeout(REQUEST_TIMEOUT_MS);
  const combined = combineSignals(signal, timeoutSignal);
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
  const payload = (await response.json()) as Omit<SearchResponse, "items"> & { items: unknown };
  if (
    !payload ||
    !Array.isArray(payload.items) ||
    typeof payload.total !== "number" ||
    typeof payload.limit !== "number" ||
    typeof payload.offset !== "number" ||
    typeof payload.hasMore !== "boolean"
  ) {
    throw new ApiError("invalid-response", response.status, "invalid-response");
  }
  const items: Student[] = [];
  for (const raw of payload.items) {
    const student = decodeItem(raw);
    if (!student) throw new ApiError("invalid-response", response.status, "invalid-response");
    items.push(student);
  }
  return { items, total: payload.total, limit: payload.limit, offset: payload.offset, hasMore: payload.hasMore };
}
