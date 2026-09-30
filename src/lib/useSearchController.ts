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
    // 判别联合直接交给 reducer：ok 落定、error 置错、stale 静默释放 loading。
    // 三分支必须一次派发完毕——调用方若只对 error 写回状态，stale 会让 state
    // 永远停在 loading，而 submit 的守卫正是「loading 中不发起新请求」，
    // 形成按 Enter 无反应的死角。
    state = searchReducer(state, { type: "submit-result", result });
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
        // Promise 型动作（submit / loadMore）分两段渲染：先渲染它同步写入的启动态
        // （submit-start 的 loading、load-more-start 的 loadingMore），再在落定后
        // 渲染终态。缺了前一段，orchestrator 在第一个 await 之前同步改完 state 就挂起，
        // 整段请求期间零次重渲染——loading 与 loadingMore 永不进入任何一次渲染，
        // 整套加载界面（结果区 loading 区段、StatusOrb、提交禁用、加载更多禁用）不可达。
        // 该 Promise 签名本身不携带「会改几次 state」这一事实，因此壳必须显式两段。
        if (out instanceof Promise) {
          force();
          return out.finally(() => force());
        }
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
