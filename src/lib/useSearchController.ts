import { useMemo, useReducer, useRef } from "react";
import { initialState, searchReducer, SearchControllerState, type SearchAction } from "./searchReducer";
import { createSearchSession, SearchSessionApi } from "./searchSession";
import type { SearchResult } from "./searchSession";
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

  // run 是「一次查询动作」的唯一实现：启动态 → 发起 → 派发结果。
  //
  // 此前 submit 与 loadMore 各手写这套骨架，start/result 这对 action 的配对
  // 与「三分支必须一次派发完毕」这条不变量在两处各写一份而互不引用。
  // 那条不变量是永久卡死缺陷换来的：stale 无分支可走时 state 永远停在
  // loading，而守卫正是「loading 中不发起新请求」，按 Enter 毫无反应。
  //
  // 收进 run 后配对由类型承载：result 以字面量类型传入，模板字面量类型
  // `${S}-result` 把它约束为与 start 同名的那条，传不成对的动作编不过。
  // 守卫仍留在各自的动作里——两者的守卫集合本就不同，合并会掩盖差异。
  type StartAction = { type: "submit-start" } | { type: "load-more-start" };
  const run = async <S extends StartAction["type"]>(
    start: Extract<SearchAction, { type: S }>,
    resultType: S extends `${infer Base}-start` ? `${Base}-result` : never,
    invoke: () => Promise<SearchResult>,
  ) => {
    state = searchReducer(state, start);
    const outcome = await invoke();
    // 判别联合整体交给 reducer：ok 落定、error 置错、stale 静默释放启动态。
    // 三分支必须一次派发完毕，理由见上。
    state = searchReducer(state, { type: resultType, result: outcome });
  };

  const submit = async () => {
    // 守卫：空查询、loading 中、加载更多进行中都不发起新请求
    const q = state.query.trim();
    if (!q || state.state === "loading" || state.loadingMore) return;
    await run({ type: "submit-start" }, "submit-result", () => session.submit(q, opts.pageSize));
  };

  const loadMore = async () => {
    // 守卫：空查询、无更多、加载中、加载更多进行中都不发起
    const q = state.query.trim();
    if (!q || !state.hasMore || state.loadingMore || state.state === "loading") return;
    // offset 在守卫之后、启动态之前读：它必须反映已加载条数。
    const offset = state.items.length;
    await run({ type: "load-more-start" }, "load-more-result", () => session.loadMore(q, opts.pageSize, offset));
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
