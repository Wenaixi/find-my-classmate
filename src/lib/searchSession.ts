import type { SearchResponse } from "../types";

// 请求竞态编排：requestId + abort 的唯一归属（原 App.tsx 的 requestRef/abortRef）。
// 可注入 api 便于测试——真实 bug 藏在"过期响应被丢弃"的调用方式里，必须能单测。
export interface SearchSessionApi {
  search(q: string, limit: number, offset: number, signal: AbortSignal): Promise<SearchResponse>;
}
// SearchResult 判别联合：把"过期/中止被丢弃"从返回类型里显式化。
// 旧实现 resolve undefined 让调用方无法区分 stale 与真实失败；判别联合
// 使 App.tsx 只需 `if (!result.ok)` 统一处理，stale 永不进入业务失败路径。
export type SearchResult =
  | { ok: true; response: SearchResponse }
  | { ok: false; reason: "stale" }
  | { ok: false; reason: "error"; cause: unknown };

// stale 与 error 的构造点：成功但过期、中止、或过期会话的失败都归为 stale。
// 真实失败（未中止且仍当前）保留 cause，供调用方按错误分类文案。
export const staleResult: SearchResult = { ok: false, reason: "stale" };
export interface SearchSession {
  invalidate(): void;
  abortAll(): void;
  submit(q: string, limit: number): Promise<SearchResult>;
  loadMore(q: string, limit: number, offset: number): Promise<SearchResult>;
}
// begin/isCurrent 收归内部闭包：请求 id 与会话当前性只属于实现，
// 调用方只需 submit/loadMore/invalidate/abortAll 四个意图动作。

export function createSearchSession(api: SearchSessionApi): SearchSession {
  let currentId = 0;
  let controller: AbortController | null = null;

  const begin = () => {
    currentId += 1;
    controller?.abort();
    controller = new AbortController();
    return currentId;
  };
  const isCurrent = (id: number) => id === currentId;

  // 组合中止源：session 自身 controller 中止即真实终止在途 fetch。
  // sessionController 用闭包捕获——begin/invalidate 会替换 controller 变量，不能用运行时取值。
  // 允许 null：loadMore 可在无在途请求时跟随当前会话（controller 尚未建立）。
  const listen = (sessionController: AbortController | null, onAbort: () => void) => {
    sessionController?.signal.addEventListener("abort", onAbort, { once: true });
  };
  const unlisten = (sessionController: AbortController | null, onAbort: () => void) => {
    sessionController?.signal.removeEventListener("abort", onAbort);
  };

  // perform 是竞态骨架的唯一归属：监听会话中止 → 发起请求 → 判定当前性 → 清理监听。
  // submit 与 loadMore 只差 id 来源（begin() 的新会话 vs 跟随 currentId）与 offset，
  // 骨架本身不允许各自复制一份——判别联合（ok/stale/error）在此单点构造，
  // 过期响应与中止归为 stale、仅未中止且仍当前的失败才是真实错误。
  const perform = async (id: number, q: string, limit: number, offset: number): Promise<SearchResult> => {
    const sessionController = controller;
    const combined = new AbortController();
    const onAbort = () => combined.abort();
    listen(sessionController, onAbort);
    try {
      const response = await api.search(q, limit, offset, combined.signal);
      return isCurrent(id) ? { ok: true, response } : staleResult;
    } catch (cause) {
      if (combined.signal.aborted || !isCurrent(id)) return staleResult;
      return { ok: false, reason: "error", cause };
    } finally {
      unlisten(sessionController, onAbort);
    }
  };

  return {
    // 作废在途请求：与 begin 共享"递增 id + 中止真实 fetch"语义，但不发起新请求。
    // clear / 输入变化 / IME 组合变化时调用，使旧响应要么被 id 拦截、要么被 abort 打断。
    invalidate: () => {
      void begin();
    },
    abortAll: () => controller?.abort(),
    async submit(q, limit) {
      // submit 通过 begin 取得全新会话 id：新查询使旧查询的 id 过期。
      return perform(begin(), q, limit, 0);
    },
    async loadMore(q, limit, offset) {
      // loadMore 跟随当前会话 id（不 begin）——滚动追加不该使 submit 的结果失效。
      return perform(currentId, q, limit, offset);
    },
  };
}
