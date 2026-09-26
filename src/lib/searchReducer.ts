import type { SearchState, Student } from "../types";
import { ApiError } from "./api";
import { hasNameCondition } from "./query";
import type { SearchResult } from "./searchSession";
import { MAX_QUERY_LENGTH, PAGE_SIZE } from "../config";

// 搜索区状态机的唯一事实来源（原 App.tsx 内联的 9 个 state 集合）。
export interface SearchControllerState {
  query: string;
  items: Student[];
  total: number;
  hasMore: boolean;
  state: SearchState;
  statusText: string;
  loadingMore: boolean;
  loadMoreError: boolean;
  isComposing: boolean;
}

export type SearchAction =
  | { type: "input-change"; query: string }
  | { type: "submit-start" }
  | { type: "submit-success"; items: Student[]; total: number; hasMore: boolean; query: string }
  | { type: "submit-error"; cause: unknown }
  | { type: "load-more-start" }
  | { type: "load-more-result"; result: SearchResult }
  | { type: "clear" }
  | { type: "composition-start" }
  | { type: "composition-end" };

const COPY: Record<SearchState, string> = {
  idle: "输入姓名、班级或年段后开始查询",
  editing: "支持姓名、班级和年段组合查询",
  loading: "正在检索完整名单",
  success: "已定位 1 位同学",
  duplicate: "已定位多位同学",
  empty: "没有找到匹配记录",
  error: "查询没有完成，请稍后重试",
};

export const initialState: SearchControllerState = {
  query: "",
  items: [],
  total: 0,
  hasMore: false,
  state: "idle",
  statusText: COPY.idle,
  loadingMore: false,
  loadMoreError: false,
  isComposing: false,
};

// 派生搜索状态：idle/empty/success/duplicate（原 App.tsx getState）。
export function getState(items: Student[], query: string, total = items.length): SearchState {
  if (!query.trim()) return "idle";
  if (items.length === 0 && total === 0) return "empty";
  return total === 1 ? "success" : "duplicate";
}

// 错误文案：按 ApiError 分类（400=输入问题、429=限流、500=数据问题、network=网络）。
// 其余 HTTP 状态（如 502 等）与非法响应统一归为服务问题（COPY.error）。
export function errorMessage(cause: unknown): string {
  if (cause instanceof ApiError) {
    if (cause.status === 400) return `查询条件有误，请精简到 ${MAX_QUERY_LENGTH} 字以内后重试`;
    if (cause.status === 429) return "请求过于频繁，请稍候再试";
    if (cause.status === 500) return "名单数据暂时不可用，请稍后重试";
    if (cause.code === "network") return "网络连接异常，请检查后重试";
  }
  return COPY.error;
}

// statusText 派生（含纯年段/班级查询无姓名条件时的整段命中提示分支）。
// hasName 由 reducer 内部以 hasNameCondition(action.query) 计算，调用方不再传入。
export function statusTextFor(state: SearchState, total: number, hasName: boolean): string {
  if (state === "duplicate" && total >= PAGE_SIZE) {
    const prefix = hasName ? COPY.duplicate : total >= 100 ? "已匹配整个年段/班级" : COPY.duplicate;
    return prefix + "，先显示前 " + PAGE_SIZE + " 条";
  }
  return COPY[state];
}

// 结果区段：查询状态到「该渲染哪一块界面结构」的映射。
// 此前这张映射表内联在 App.tsx 的 renderResultBody 里，由四个 if 依次判定，
// 与「结果区是否渲染」「是否滚动定位」构成同一知识的四份独立判断，且零测试覆盖：
// 把 duplicate 分支改坏（多结果时不渲染列表）后 121 条用例仍全绿。
// 抽出后成为可脱离 React 直接测试的纯函数，App.tsx 只按返回值 switch。
export type ResultSection = "list" | "loading" | "empty" | "error";

// resultSectionOf 判定查询状态对应的结果区段。
// null 表示不渲染结果区（idle 与 editing：尚无查询或正在编辑）。
// "list" 同时覆盖 success 与 duplicate——两者界面结构相同，仅数据量不同。
export function resultSectionOf(state: SearchState): ResultSection | null {
  if (state === "loading") return "loading";
  if (state === "success" || state === "duplicate") return "list";
  if (state === "empty") return "empty";
  if (state === "error") return "error";
  return null;
}

// shouldScrollToResults 判定查询结束后是否滚动定位到结果区。
// 与 resultSectionOf 派生自同一处：两者判定的是「本次状态变化是否产生了一个
// 可供阅读的结果」，此前散在 App.tsx 的滚动 effect 与结果区显隐两处，
// 且两者的状态集合在 7 值全集上恰好互补——新增查询状态时只改一处不会报错，
// 只会让滚动与显隐静默错位。
export function shouldScrollToResults(state: SearchState): boolean {
  const section = resultSectionOf(state);
  return section !== null && section !== "loading";
}

// 界面提示派生：查询状态到「这条状态该禁用提交 / 显示哪种提示色 / 是否渲染加载指示」的映射。
// 此前这三项各自绕开 resultSectionOf 直读状态字面量：App.tsx:102 直比 "loading" 取禁用态，
// App.tsx:108 把原始枚举喂给 data-state 与 StatusOrb，StatusOrb.tsx:5 再直比一次，
// styles.css:87-88 第四、第五次镜像同一批字面量。
// 变异实验（2026-09-26 架构评审第四轮）证明这三条支路零承重：
// 把 App.tsx 的 disabled 改成 false、把 data-state 与 orb 状态硬编码、
// 让 StatusOrb 对任何状态都渲染，132 条用例全部仍然通过。
// 新增一个查询状态时，编译器会因 Record 穷尽性强制在此补齐，
// 而不再依赖改开发者记得同步四个位置的字面量。
export type StatusTone = "muted" | "ink" | "dim";

export interface StatusHint {
  /** 查询进行中：提交按钮禁用，按钮标签切换为"正在检索" */
  busy: boolean;
  /** 提交按钮的无障碍标签 */
  sendLabel: string;
  /** 状态行提示色：muted 常态 / ink 已命中 / dim 出错 */
  tone: StatusTone;
  /** 是否渲染加载指示（此前由 StatusOrb 直比状态字面量决定） */
  showOrb: boolean;
}

export function deriveStatusHint(state: SearchState): StatusHint {
  const busy = state === "loading";
  return {
    busy,
    sendLabel: busy ? "正在检索" : "开始搜索",
    tone: state === "success" || state === "duplicate" ? "ink" : state === "error" ? "dim" : "muted",
    showOrb: busy,
  };
}

export function searchReducer(state: SearchControllerState, action: SearchAction): SearchControllerState {
  switch (action.type) {
    case "input-change":
      // 输入变化：清空后回 idle（其余字段全复位，与旧 effect 语义一致）；非 IME 组合期间切到 editing。
      if (action.query.trim() === "") {
        return { ...initialState, query: action.query, isComposing: state.isComposing };
      }
      if (state.isComposing) {
        // IME 组合期间不改状态，也不改文案：组合中的候选文字不是一次新的编辑意图
        return { ...state, query: action.query };
      }
      // 切到 editing 时同步刷新 statusText：状态与提示文案必须一致，
      // 否则失败后继续输入会显示 editing 状态配上"网络异常"之类的旧文案。
      return { ...state, query: action.query, state: "editing", statusText: COPY.editing };
    case "submit-start":
      return { ...state, items: [], total: 0, hasMore: false, state: "loading", statusText: COPY.loading, loadingMore: false, loadMoreError: false };
    case "submit-success":
      // 派生归位：调用方只交原始事实，状态与文案在此一次性算出，
      // 避免 App.tsx 在每个分支手工串联 getState/hasNameCondition/statusTextFor。
      {
        const next = getState(action.items, action.query, action.total);
        return {
          ...state,
          items: action.items,
          total: action.total,
          hasMore: action.hasMore,
          state: next,
          statusText: statusTextFor(next, action.total, hasNameCondition(action.query)),
        };
      }
    case "submit-error":
      return { ...state, items: [], state: "error", statusText: errorMessage(action.cause) };
    case "load-more-start":
      return { ...state, loadingMore: true, loadMoreError: false };
    case "load-more-result": {
      // 单 action 收编三分支：ok 追加、error 置错、stale 静默复位。
      // stale 的 loadingMore 复位必须在 reducer 内显式完成（原由 App 无条件 settle 兜底）。
      const r = action.result;
      if (r.ok) {
        return { ...state, items: [...state.items, ...r.response.items], total: r.response.total, hasMore: r.response.hasMore, loadingMore: false };
      }
      if (r.reason === "error") {
        return { ...state, loadMoreError: true, loadingMore: false };
      }
      return { ...state, loadingMore: false }; // stale：静默复位，不进入业务错误路径
    }
    case "clear":
      // 新引用而非 initialState 本身：React useReducer 对同引用跳过重渲染
      return { ...initialState };
    case "composition-start":
      // 组合开始意味着用户正在输入新的查询词：切到 editing 并刷新文案，
      // 否则查询失败后开始打字会一直显示 error 状态配"网络异常"旧文案。
      // 处于 loading 时保留 loading——组合不影响在途请求，
      // 其响应返回后会正常派发 submit-success / submit-error。
      if (state.state === "loading") {
        return { ...state, isComposing: true };
      }
      return { ...state, isComposing: true, state: "editing", statusText: COPY.editing };
    case "composition-end":
      return { ...state, isComposing: false };
  }
}
