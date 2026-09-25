import type { SearchResponse } from "../types";

// 请求竞态编排：requestId + abort 的唯一归属（原 App.tsx 的 requestRef/abortRef）。
// 可注入 api 便于测试——真实 bug 藏在"过期响应被丢弃"的调用方式里，必须能单测。
export interface SearchSessionApi {
  search(q: string, limit: number, offset: number, signal: AbortSignal): Promise<SearchResponse>;
}

export interface SearchSession {
  begin(): number;
  isCurrent(id: number): boolean;
  abortAll(): void;
  submit(q: string, limit: number, signal: AbortSignal): Promise<SearchResponse | undefined>;
  loadMore(q: string, limit: number, offset: number, signal: AbortSignal): Promise<SearchResponse | undefined>;
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

  return {
    begin,
    isCurrent: (id) => id === currentId,
    abortAll: () => controller?.abort(),
    async submit(q, limit, signal) {
      // submit 通过 begin 取得全新会话 id：与旧 App.tsx submit 的
      // `const requestId = ++requestRef.current` 语义一致——新查询使旧查询的 id 过期。
      // 若本请求期间另有新 begin，返回 undefined 丢弃过期响应。
      const id = begin();
      const combined = new AbortController();
      const onAbort = () => combined.abort();
      signal?.addEventListener("abort", onAbort, { once: true });
      try {
        const response = await api.search(q, limit, 0, combined.signal);
        return id === currentId ? response : undefined;
      } finally {
        signal?.removeEventListener("abort", onAbort);
      }
    },
    async loadMore(q, limit, offset, signal) {
      // loadMore 跟随当前会话 id（不 begin）——滚动追加不该使 submit 的结果失效，
      // 与旧 App.tsx 的 loadMore 复用 requestRef.current 语义一致。
      const id = currentId;
      const combined = new AbortController();
      const onAbort = () => combined.abort();
      signal?.addEventListener("abort", onAbort, { once: true });
      try {
        const response = await api.search(q, limit, offset, combined.signal);
        return id === currentId ? response : undefined;
      } finally {
        signal?.removeEventListener("abort", onAbort);
      }
    },
  };
}
