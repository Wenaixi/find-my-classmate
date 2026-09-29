import type { Grade, SearchResponse, Student } from "../types";
import { REQUEST_TIMEOUT_MS } from "../config";

// ApiStudent 是 /api/search 的 wire 形状，仅在本模块内用于解码。
// 服务端 canonical 字段是 class（search.go json:"class"），className 永不出现在序列化中。
interface ApiStudent {
  name?: unknown;
  grade?: unknown;
  class?: unknown;
}

// decodeItem 不在前端复制年段取值域：后端 knownGrades 是规范名的唯一事实源。
// 若前端硬编码一份年段清单，后端新增年段后前端会把全部响应判为 invalid-response，
// 表现为"后端数据正常但全站查询失败"。因此这里只校验 grade 是非空字符串，
// 值域正确性由后端保证；空串、缺失与非字符串仍被拒绝。
//
// 注意「扩展年段」的义务不止 knownGrades 一处：前端 types.ts 的 Grade 联合与
// query.ts 的 gradeValues 也须同步（加别名时两端 gradeAliases 亦然），
// 共四处。清单与各处的强制机制见 docs/ARCHITECTURE.md 的年段值域小节，
// 此处不复述——清单会随代码增删而腐化。

// decodeItem 把一条 wire 记录收敛为合法领域形状，或返回 null 表示不可接受。
// 只接受 canonical class 单字段：缺字段、类型错误与空串都不静默降级，
// 避免错误数据进入界面。
function decodeItem(raw: unknown): Student | null {
  if (!raw || typeof raw !== "object") return null;
  const item = raw as ApiStudent;
  if (typeof item.name !== "string" || item.name === "") return null;
  if (typeof item.grade !== "string" || item.grade === "") return null;
  if (typeof item.class !== "string" || item.class === "") return null;
  return { name: item.name, grade: item.grade as Grade, className: item.class };
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

// 组合中断源：调用方 signal 与超时信号任一中止，都真实终止在途 fetch。
// searchApi 的调用方是查询会话——SearchSessionApi.search 声明四参含 signal，
// 会话作废一条查询时 abort 经此抵达传输层。此前 searchApi 的签名只有三个
// 形参，第四个参数在接口处被静默丢弃，fetch 拿到的始终只是超时信号，
// 会话 abort 只让 perform 判定 stale，HTTP 请求继续跑满超时。
// fetchVersion 同样有调用方取消语义（App 卸载时 abort），走同一组合器。
// AbortSignal.any 不可用（Safari < 17.4）时手动桥接。
function combineSignals(a?: AbortSignal, b?: AbortSignal): AbortSignal | undefined {
  if (!a) return b;
  if (!b) return a;
  if (typeof AbortSignal.any === "function") {
    return AbortSignal.any([a, b]);
  }
  const controller = new AbortController();
  if (a.aborted || b.aborted) controller.abort();
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
  const combined = combineSignals(signal, AbortSignal.timeout(REQUEST_TIMEOUT_MS));
  let response: Response;
  try {
    response = await fetch("/api/search?" + params, { signal: combined });
  } catch (cause) {
    // 会话作废导致的取消由 searchSession 判定为 stale，不进入业务错误路径；
    // 其余（超时、网络失败）统一归为 network。
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
