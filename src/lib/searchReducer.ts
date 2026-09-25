import type { SearchState, Student } from "../types";
import { ApiError } from "./api";
import { PAGE_SIZE } from "../config";

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
  | { type: "submit-success"; items: Student[]; total: number; hasMore: boolean; state: SearchState; statusText: string }
  | { type: "submit-error"; statusText: string }
  | { type: "load-more-start" }
  | { type: "load-more-append"; items: Student[]; total: number; hasMore: boolean }
  | { type: "load-more-error" }
  | { type: "load-more-settle" }
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
// 原 App.tsx errorMessage 只看 status，这里补上 code 维度：invalid_response 等统一归类为服务问题。
export function errorMessage(cause: unknown): string {
  if (cause instanceof ApiError) {
    if (cause.status === 400) return "查询条件有误，请精简到 80 字以内后重试";
    if (cause.status === 429) return "请求过于频繁，请稍候再试";
    if (cause.status === 500) return "名单数据暂时不可用，请稍后重试";
    if (cause.code === "network") return "网络连接异常，请检查后重试";
  }
  return COPY.error;
}

// statusText 派生（含 F36 纯年段/班级提示分支）。
// hasName 由调用方（App.tsx）用 hasNameCondition(submitted) 计算后传入，reducer 保持纯状态机。
export function statusTextFor(state: SearchState, total: number, hasName: boolean): string {
  if (state === "duplicate" && total >= PAGE_SIZE) {
    const prefix = hasName ? COPY.duplicate : total >= 100 ? "已匹配整个年段/班级" : COPY.duplicate;
    return prefix + "，先显示前 " + PAGE_SIZE + " 条";
  }
  return COPY[state];
}

export function searchReducer(state: SearchControllerState, action: SearchAction): SearchControllerState {
  switch (action.type) {
    case "input-change":
      // 输入变化：清空后回 idle（其余字段全复位，与旧 effect 语义一致）；非 IME 组合期间切到 editing。
      if (action.query.trim() === "") {
        return { ...initialState, query: action.query, isComposing: state.isComposing };
      }
      return { ...state, query: action.query, state: state.isComposing ? state.state : "editing" };
    case "submit-start":
      return { ...state, items: [], total: 0, hasMore: false, state: "loading", statusText: COPY.loading, loadingMore: false, loadMoreError: false };
    case "submit-success":
      return { ...state, items: action.items, total: action.total, hasMore: action.hasMore, state: action.state, statusText: action.statusText };
    case "submit-error":
      return { ...state, items: [], state: "error", statusText: action.statusText };
    case "load-more-start":
      return { ...state, loadingMore: true, loadMoreError: false };
    case "load-more-append":
      return { ...state, items: [...state.items, ...action.items], total: action.total, hasMore: action.hasMore, loadingMore: false };
    case "load-more-error":
      return { ...state, loadMoreError: true, loadingMore: false };
    case "load-more-settle":
      return { ...state, loadingMore: false };
    case "clear":
      // 新引用而非 initialState 本身：React useReducer 对同引用跳过重渲染
      return { ...initialState };
    case "composition-start":
      return { ...state, isComposing: true };
    case "composition-end":
      return { ...state, isComposing: false };
  }
}
