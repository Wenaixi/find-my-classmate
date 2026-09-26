import { useMemo, useReducer, useRef } from "react";
import { initialState, searchReducer, SearchControllerState } from "./searchReducer";
import { createSearchSession, SearchSessionApi } from "./searchSession";
import type { Student } from "../types";

// 查询会话深模块：把「发起并展示一次查询」的编排（守卫、判别联合、session/reducer
// 同步）收进一根接口。App 只消费 state（渲染派生）与 controller（意图动作），
// 不再理解 reducer action 面、session 竞态或错误分类。
//
// 拆成两层：createSearchOrchestrator 是纯逻辑（不依赖 React，可脱离渲染单测），
// useSearchController 是薄 hook 壳（引用稳定 + 重渲染触发）。测试走纯逻辑层，
// 接口即测试面：守卫与判别联合的 bug 藏在这里，必须能直接单测。

export interface OrchestratorOptions {
  pageSize: number;
  api: SearchSessionApi;
}

export interface SearchOrchestrator {
  getState(): SearchControllerState;
  onInput(query: string): void;
  submit(): Promise<void>;
  loadMore(): Promise<void>;
  clear(): void;
  abortAll(): void;
  onCompositionStart(): void;
  onCompositionEnd(): void;
}

export function createSearchOrchestrator(opts: OrchestratorOptions): SearchOrchestrator {
  let state = initialState;
  const session = createSearchSession(opts.api);

  const submit = async () => {
    const q = state.query.trim();
    // 守卫：空查询、loading 中、加载更多进行中都不发起新请求
    if (!q || state.state === "loading" || state.loadingMore) return;
    state = searchReducer(state, { type: "submit-start" });
    const result = await session.submit(q, opts.pageSize);
    if (!result.ok) {
      // 判别联合：stale 永不进入业务错误路径；真实错误按 cause 分类
      if (result.reason === "error") state = searchReducer(state, { type: "submit-error", cause: result.cause });
      return;
    }
    state = searchReducer(state, { type: "submit-success", items: result.response.items, total: result.response.total, hasMore: result.response.hasMore, query: q });
  };

  const loadMore = async () => {
    const q = state.query.trim();
    // 守卫：空查询、无更多、加载中、加载更多进行中都不发起
    if (!q || !state.hasMore || state.loadingMore || state.state === "loading") return;
    state = searchReducer(state, { type: "load-more-start" });
    const result = await session.loadMore(q, opts.pageSize, state.items.length);
    // 判别联合直接交给 reducer：ok 追加、error 置错、stale 静默复位
    state = searchReducer(state, { type: "load-more-result", result });
  };

  const clear = () => {
    // invalidate（递增 id + 中止在途 fetch）使过期响应失效后清空
    session.invalidate();
    state = searchReducer(state, { type: "clear" });
  };

  const onInput = (query: string) => {
    // 输入变化即作废在途请求（与旧 App.tsx onChange 的 session.invalidate() 语义一致）
    session.invalidate();
    state = searchReducer(state, { type: "input-change", query });
  };

  return {
    getState: () => state,
    onInput,
    submit,
    loadMore,
    clear,
    abortAll: () => session.abortAll(),
    onCompositionStart: () => {
      state = searchReducer(state, { type: "composition-start" });
    },
    onCompositionEnd: () => {
      state = searchReducer(state, { type: "composition-end" });
    },
  };
}

// 薄 hook 壳：orchestrator 是纯逻辑，hook 负责在每次动作后触发重渲染。
// createSearchOrchestrator 内部直接改闭包状态（不触发 React），
// 因此这里把每个意图动作包装为「执行 + force 重渲染」，App 才能看到最新 state。
export function useSearchController(opts: OrchestratorOptions) {
  const orchRef = useRef<SearchOrchestrator | null>(null);
  if (!orchRef.current) orchRef.current = createSearchOrchestrator(opts);
  const [, force] = useReducer((x: number) => x + 1, 0);
  const controller = useMemo<SearchOrchestrator>(() => {
    const orch = orchRef.current!;
    const wrap = <K extends keyof SearchOrchestrator>(key: K) =>
      ((...args: Parameters<SearchOrchestrator[K]>) => {
        const fn = orch[key] as (...a: typeof args) => unknown;
        const out = fn(...args);
        if (out instanceof Promise) return out.finally(() => force());
        force();
        return out;
      }) as SearchOrchestrator[K];
    return {
      getState: () => orch.getState(),
      onInput: wrap("onInput"),
      submit: wrap("submit"),
      loadMore: wrap("loadMore"),
      clear: wrap("clear"),
      abortAll: () => orch.abortAll(),
      onCompositionStart: wrap("onCompositionStart"),
      onCompositionEnd: wrap("onCompositionEnd"),
    };
  }, [force]);
  // 每次渲染都直读 orchestrator 的最新 state：不能用 useMemo 缓存。
  // controller 的引用恒定（依赖 [force]，而 force 来自 useReducer 的 dispatch，
  // 引用永不改变），把它放进依赖数组会让 useMemo 永久命中缓存，
  // 使组件拿到首帧 state 快照——force() 重渲染后查询状态仍停留在 idle，
  // 整条搜索链路对用户不可用。挂载回归测试见 useSearchController.mount.test.tsx。
  return { state: controller.getState(), controller };
}
