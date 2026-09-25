import type { SearchResponse } from "../types";

// 请求竞态编排：requestId + abort 的唯一归属（原 App.tsx 的 requestRef/abortRef）。
// 可注入 api 便于测试——真实 bug 藏在"过期响应被丢弃"的调用方式里，必须能单测。
export interface SearchSessionApi {
  search(q: string, limit: number, offset: number, signal: AbortSignal): Promise<SearchResponse>;
}

export interface SearchSession {
  begin(): number;
  isCurrent(id: number): boolean;
  invalidate(): void;
  abortAll(): void;
  submit(q: string, limit: number, signal?: AbortSignal): Promise<SearchResponse | undefined>;
  loadMore(q: string, limit: number, offset: number, signal?: AbortSignal): Promise<SearchResponse | undefined>;
}

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

  // 组合中断源：调用方 signal 与 session 自身 controller 任一中止，都真实终止在途 fetch。
  // sessionController 用闭包捕获——begin/invalidate 会替换 controller 变量，不能用运行时取值。
  const listen = (sessionController: AbortController | null, signal: AbortSignal | undefined, onAbort: () => void) => {
    signal?.addEventListener("abort", onAbort, { once: true });
    sessionController?.signal.addEventListener("abort", onAbort, { once: true });
  };
  const unlisten = (sessionController: AbortController | null, signal: AbortSignal | undefined, onAbort: () => void) => {
    signal?.removeEventListener("abort", onAbort);
    sessionController?.signal.removeEventListener("abort", onAbort);
  };

  return {
    begin,
    isCurrent,
    // 作废在途请求：与 begin 共享"递增 id + 中止真实 fetch"语义，但不发起新请求。
    // clear / 输入变化 / IME 组合变化时调用，使旧响应要么被 id 拦截、要么被 abort 打断。
    invalidate: () => {
      void begin();
    },
    abortAll: () => controller?.abort(),
    async submit(q, limit, signal) {
      // submit 通过 begin 取得全新会话 id：与旧 App.tsx submit 的
      // `const requestId = ++requestRef.current` 语义一致——新查询使旧查询的 id 过期。
      // 若本请求期间另有新 begin/invalidate，过期响应（含过期失败）统一返回 undefined 丢弃。
      const id = begin();
      const sessionController = controller;
      const combined = new AbortController();
      const onAbort = () => combined.abort();
      listen(sessionController, signal, onAbort);
      try {
        const response = await api.search(q, limit, 0, combined.signal);
        return id === currentId ? response : undefined;
      } catch (cause) {
        // 中止（invalidate/新 begin/abortAll/调用方 signal）或过期会话的失败静默丢弃；
        // 仅在响应未被中止且会话仍当前时才向上抛（App 才能按真实错误分类文案）。
        if (combined.signal.aborted || !isCurrent(id)) return undefined;
        throw cause;
      } finally {
        unlisten(sessionController, signal, onAbort);
      }
    },
    async loadMore(q, limit, offset, signal) {
      // loadMore 跟随当前会话 id（不 begin）——滚动追加不该使 submit 的结果失效，
      // 与旧 App.tsx 的 loadMore 复用 requestRef.current 语义一致。
      const id = currentId;
      const sessionController = controller;
      const combined = new AbortController();
      const onAbort = () => combined.abort();
      listen(sessionController, signal, onAbort);
      try {
        const response = await api.search(q, limit, offset, combined.signal);
        return id === currentId ? response : undefined;
      } catch (cause) {
        if (combined.signal.aborted || !isCurrent(id)) return undefined;
        throw cause;
      } finally {
        unlisten(sessionController, signal, onAbort);
      }
    },
  };
}
