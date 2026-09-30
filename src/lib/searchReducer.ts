import type { SearchState, Student } from "../types";
import { ApiError } from "./api";
import { hasNameCondition } from "./query";
import { deriveResultSummary, type ResultSummary } from "./resultSummary";
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
  | { type: "submit-result"; result: SearchResult }
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
// 归位前这三项各自绕开 resultSectionOf 直读状态字面量：App 曾把原始枚举喂给
// data-state 与 StatusOrb，StatusOrb 再直比一次，样式表另有两份镜像。
// 变异实验（2026-09-26 架构评审第四轮）证明这三条支路零承重：
// 把 App 的 disabled 改成 false、把 data-state 与 orb 状态硬编码、
// 让 StatusOrb 对任何状态都渲染，132 条用例全部仍然通过。
// 归位后界面只消费本函数的派生结果，新增查询状态时不再依赖改开发者
// 记得同步多个位置的字面量——但**本函数自身没有编译期强制**，
// 它是三元表达式，新增状态会静默落进 muted/false 分支。
// 真正受 Record 穷尽性强制的是文件顶部的 COPY 表（缺键即 TS2741）。
// 本文件底部的 SearchStateExhaustive 检查把「状态集合已变」这件事
// 显式化，使新增状态时至少有一处必须被审视。
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

// Presentation 是视图需要的全部展示派生。
//
// 组件此前必须同时 import resultSectionOf / shouldScrollToResults / deriveStatusHint
// 三个派生并另调 useSearchController 取 state，界面知识横跨两个 module。
// 既有测试分别打三个函数，没有任何断言锁住它们对同一状态给出一致的组合——
// 这里收成一次派生，让一致性从「同一次调用」这一接缝可验证。
//
// 组件此前还各自持有三处展示派生（结果摘要的喂参、清空按钮显隐、结果区计数），
// 与本文件「视图需要的全部展示派生」的说法矛盾。这三处本就可由传入的 state 派生，
// 现已一并归位：present 收到的 state 含 items/total/hasMore/query，无需额外接口面。
export interface Presentation {
  /** 该渲染哪一块结果区；null 表示不渲染 */
  section: ResultSection | null;
  /** 本次状态变化是否产生可供阅读的结果 */
  scroll: boolean;
  /** 查询进行中：提交按钮禁用 */
  busy: boolean;
  /** 提交按钮的无障碍标签 */
  sendLabel: string;
  /** 状态行提示色 */
  tone: StatusTone;
  /** 是否渲染加载指示 */
  showOrb: boolean;
  /** 结果区头部的匹配条数文案：零条时用占位符而非 0 */
  countLabel: string;
  /** 是否渲染清空按钮（此前由组件直读 query.length > 0） */
  showClear: boolean;
  /** 结果区摘要：进度、计数与剩余文案的唯一来源 */
  summary: ResultSummary;
  /**
   * 错误区段的标题与说明。
   *
   * 二者此前分处两处：标题由组件硬编码，说明是 state.statusText。未知错误形态
   * （非 ApiError 的异常、非法响应）下 statusText 走 errorMessage 的兜底分支，
   * 返回的文案以标题那句为前缀——同一句在同屏出现两次。分类文案明确时
   * （400/429/500/network）不重复，恰好掩盖了它。
   *
   * 收到此处是因为「标题与说明不重复」是跨两个 module 的不变量：组件硬编码
   * 标题、文案由 reducer 派生，两侧各自改动都会破坏它，而编译器不报任何错。
   * 说明在说明以标题为前缀时去掉该前缀，使重复在派生处即被排除。
   */
  errorCopy: { title: string; detail: string };
  /**
   * 加载更多的三个界面知识：是否还有后续页、当前是否在加载、上次是否失败。
   *
   * 收到此处是因为它们此前住在 ResultList 里各判一次，而 present 自称
   * 折出「视图需要的全部展示派生」——新增查询状态时改 present 不会让它们跟随。
   * 同一形状的知识此前一分为二：submit 的忙碌态归 deriveStatusHint，
   * loadMore 的忙碌态却在组件里。
   *
   * show 取代原先单独下传的 hasMore：summary 内部已把同一个布尔编码过两次
   * （toolbarState 与 loadMoreLabel），再传一份独立的 hasMore 等于让同一事实
   * 跨接缝两次，组件因此能渲染出「工具栏说已全部加载、按钮却还在」的
   * 自相矛盾界面。收进这个 zone 后，一处判定、一处下传。
   */
  loadMoreZone: { show: boolean; busy: boolean; error: boolean };
}

// 错误区段的固定标题：与 COPY.error 的前缀同源，但两者不是同一句。
// 分类文案（400/429/500/network）不以此为前缀，说明原样呈现。
const ERROR_TITLE = "查询没有完成";

// splitErrorCopy 把状态文案拆成错误区段的标题与说明。
// 说明以标题为前缀时去掉该前缀（去掉后为空则保留原句），使同屏重复不可能发生。
// 刻意不新增「错误说明」文案表：说明已有唯一来源 errorMessage，此处只做切分。
function splitErrorCopy(statusText: string): { title: string; detail: string } {
  if (!statusText.startsWith(ERROR_TITLE)) {
    return { title: ERROR_TITLE, detail: statusText };
  }
  const rest = statusText.slice(ERROR_TITLE.length).replace(/^[，,、]\s*/, "");
  return { title: ERROR_TITLE, detail: rest || statusText };
}

export function present(state: SearchControllerState): Presentation {
  const hint = deriveStatusHint(state.state);
  return {
    section: resultSectionOf(state.state),
    scroll: shouldScrollToResults(state.state),
    busy: hint.busy,
    sendLabel: hint.sendLabel,
    tone: hint.tone,
    showOrb: hint.showOrb,
    countLabel: state.total ? String(state.total) : "--",
    showClear: state.query.length > 0,
    summary: deriveResultSummary(state.items.length, state.total, state.hasMore),
    errorCopy: splitErrorCopy(state.statusText),
    loadMoreZone: { show: state.hasMore, busy: state.loadingMore, error: state.loadMoreError },
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
    case "submit-result": {
      // 单 action 收编三分支，与 load-more-result 完全对称：ok 落定、error 置错、
      // stale 静默释放。ok 与 error 的派生也一并在此完成，调用方只交原始事实，
      // 不再手工串联 getState/hasNameCondition/statusTextFor。
      //
      // 收成单 action 的直接原因是 stale 曾无分支可走：ok 与 error 各占一个 action，
      // 而调用方只对 error 写回状态，stale 直接 return，state 永远停在 loading。
      //
      // stale 的 loading 释放必须在 reducer 内显式完成：stale 表示本次查询已被作废
      // （组合期的输入变化会 invalidate 在途请求），而 state 仍停在 submit-start 写入的
      // loading。submit 的守卫正是「loading 中不发起新请求」，因此不释放就形成死角：
      // 状态行显示「正在检索完整名单」、提交按钮禁用，按 Enter 没有任何反应。
      // 这与 loadMore 侧早已修复的同族缺口只差一个分支。
      //
      // stale 落定到 editing 而非 success/empty/error：组合期的输入已写入 state.query，
      // 而针对它的查询尚未发起，界面应回到「正在编辑、可以再次提交」。
      const r = action.result;
      if (r.ok) {
        const next = getState(r.response.items, state.query, r.response.total);
        return {
          ...state,
          items: r.response.items,
          total: r.response.total,
          hasMore: r.response.hasMore,
          state: next,
          statusText: statusTextFor(next, r.response.total, hasNameCondition(state.query)),
        };
      }
      if (r.reason === "error") {
        return { ...state, items: [], state: "error", statusText: errorMessage(r.cause) };
      }
      return { ...state, state: "editing", statusText: COPY.editing };
    }
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
      // 处于 loading 时保留 loading——组合本身不影响在途请求。但组合期的输入事件
      // 会经 onInput 调 session.invalidate() 作废它，因此该响应到达时得到的是 stale
      // 而非 success/error，由 submit-result 的 stale 分支负责释放 loading
      // （此前这里无分支可走，state 永远停在 loading，提交按钮被守卫锁死）。
      if (state.state === "loading") {
        return { ...state, isComposing: true };
      }
      return { ...state, isComposing: true, state: "editing", statusText: COPY.editing };
    case "composition-end":
      return { ...state, isComposing: false };
  }
}

// 声明式状态全集：把「一共有哪些查询状态」从各处的 if 链与测试里的手抄数组
// 收成一处，供下方穷尽性检查与消费方派生。
//
// 必须用 as const 而非 SearchState[]：显式标注会把字面量拓宽回 SearchState，
// 于是下方 Exclude 恒为 never、检查形同虚设——这正是 types.ts 的 GradeDomainGap
// 注释里记着的同一条教训。
//
// 导出供测试消费：此前它是 const 而非 export，测试里的四份手抄数组一个都没被收走，
// 于是「状态集合变大」时生产侧两处检查报错、而那四份「穷尽性断言」仍全绿——
// 它们遍历的数组里没有新成员。与 types.ts 的 GradeDomainGap 手抄数组（第十四轮修过）
// 是同一形态，此处是它的状态枚举版本。
export const searchStateValues = ["idle", "editing", "loading", "success", "duplicate", "empty", "error"] as const satisfies readonly SearchState[];

// SearchState 联合里出现 searchStateValues 未声明的状态时报错。
//
// 为什么需要它：COPY（文件顶部）是 Record<SearchState, string>，缺键会报 TS2741，
// 但那只是状态文案一处受强制。getState 与 resultSectionOf 都是 if 链，
// 新增状态时二者静默落 default 分支（getState 落 duplicate、resultSectionOf 落 null
// 即结果区完全不渲染），编译器一声不吭。deriveStatusHint 同样是三元表达式。
// 本检查把这些「无人审视」的派生点汇成一处：状态集合一变，这里必须被改。
//
// 报错形态刻意带出缺失项本身（缺 "pending" 时退化为 { missing: "pending" }），
// 而不是让 Exclude 得 never 后报一条无从解读的 never 不兼容。
//
// 检查必须落在非测试文件：.dockerignore 排除 src/**/*.test.ts，
// 放进测试文件则容器镜像构建的 tsc -b 不再检查它，会造出一条「本地绿、镜像不查」
// 的隐形防线。searchStateValues 在生产中无人消费，但 tsconfig.app.json 未开
// noUnusedLocals，声明本身不报错。
type SearchStateGap = Exclude<SearchState, (typeof searchStateValues)[number]>;
const searchStateExhaustive: SearchStateGap extends never ? true : { missing: SearchStateGap } = true;
void searchStateExhaustive;
