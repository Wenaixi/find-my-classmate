import { describe, expect, it } from "vitest";
import { getState, errorMessage, initialState, present, searchReducer, statusTextFor, resultSectionOf, shouldScrollToResults, deriveStatusHint } from "./searchReducer";
import { ApiError } from "./api";
import type { SearchState, Student } from "../types";

describe("getState", () => {
  it("derives idle/empty/success/duplicate", () => {
    expect(getState([], "", 0)).toBe("idle");
    expect(getState([], "张三", 0)).toBe("empty");
    expect(getState([{ name: "张三", grade: "高一", className: "1班" }], "张三", 1)).toBe("success");
    expect(getState([{ name: "张三", grade: "高一", className: "1班" }], "张三", 2)).toBe("duplicate");
  });
});

describe("errorMessage", () => {
  it("classifies by status", () => {
    expect(errorMessage(new ApiError("x", 400, "invalid_query"))).toContain("查询条件有误");
    expect(errorMessage(new ApiError("x", 429, "rate_limited"))).toContain("请求过于频繁");
    expect(errorMessage(new ApiError("x", 500, "data_unavailable"))).toContain("名单数据暂时不可用");
  });

  it("classifies network code", () => {
    expect(errorMessage(new ApiError("x", undefined, "network"))).toContain("网络连接异常");
  });

  it("falls back to generic error", () => {
    expect(errorMessage(new Error("boom"))).toContain("查询没有完成");
  });
});

describe("resultSectionOf", () => {
  it("maps every query state to exactly one section", () => {
    // 穷尽性断言：7 个状态必须全部有归属。
    // 此前映射表内联在组件里，新增查询状态时无任何测试会提醒补映射，
    // 未覆盖的状态会静默落进「不渲染结果区」。
    const all: SearchState[] = ["idle", "editing", "loading", "success", "duplicate", "empty", "error"];
    const mapped = all.map(resultSectionOf);
    expect(mapped).toEqual([null, null, "loading", "list", "list", "empty", "error"]);
    // 7 个状态恰好收敛为 5 种区段归属（success 与 duplicate 同为 list）。
    // 用 Record 统计而非 Set：键是有限的静态字面量。
    const distinct: Record<string, number> = {};
    for (const section of mapped) {
      const key = String(section);
      distinct[key] = (distinct[key] ?? 0) + 1;
    }
    expect(Object.keys(distinct).sort()).toEqual(["empty", "error", "list", "loading", "null"]);
  });

  it("renders the list for both single and multiple matches", () => {
    // 变异回归锁：曾把 duplicate 从列表分支移除（只认 success），
    // 重名同学的结果列表完全不渲染，而 121 条用例全部通过。
    expect(resultSectionOf("success")).toBe("list");
    expect(resultSectionOf("duplicate")).toBe("list");
  });

  it("renders no section before a query has run", () => {
    expect(resultSectionOf("idle")).toBeNull();
    expect(resultSectionOf("editing")).toBeNull();
  });
});

describe("shouldScrollToResults", () => {
  it("scrolls only after a query settles", () => {
    // loading 期间不滚动：结果尚未返回，滚过去是空白。
    expect(shouldScrollToResults("loading")).toBe(false);
    expect(shouldScrollToResults("success")).toBe(true);
    expect(shouldScrollToResults("duplicate")).toBe(true);
    expect(shouldScrollToResults("empty")).toBe(true);
    expect(shouldScrollToResults("error")).toBe(true);
  });

  it("never scrolls before a query has run", () => {
    expect(shouldScrollToResults("idle")).toBe(false);
    expect(shouldScrollToResults("editing")).toBe(false);
  });

  it("stays consistent with the section mapping", () => {
    // 滚动集合与区段集合派生自同一处，不可能出现「滚到了但不显示」
    // 或「显示了但不滚动」的错位。此前两者在 App.tsx 里各写一份。
    const all: SearchState[] = ["idle", "editing", "loading", "success", "duplicate", "empty", "error"];
    for (const state of all) {
      const section = resultSectionOf(state);
      expect(shouldScrollToResults(state)).toBe(section !== null && section !== "loading");
    }
  });
});

describe("searchReducer", () => {
  it("tracks editing on input during composition", () => {
    const s = searchReducer(initialState, { type: "input-change", query: "张" });
    expect(s.state).toBe("editing");
  });

  // 状态与提示文案必须一致：切到 editing 时不能保留上一轮的
  // 错误或结果文案，否则用户会看到 editing 状态配上"网络异常"之类的提示。
  it("refreshes status text when leaving error for editing", () => {
    const afterError = searchReducer(initialState, { type: "submit-error", cause: new ApiError("x", undefined, "network") });
    expect(afterError.state).toBe("error");
    const s = searchReducer(afterError, { type: "input-change", query: "张" });
    expect(s.state).toBe("editing");
    expect(s.statusText).not.toBe("网络连接异常，请检查后重试");
    expect(s.statusText).toContain("姓名");
  });

  it("refreshes status text when leaving success for editing", () => {
    const items: Student[] = [{ name: "张三", grade: "高一", className: "1班" }];
    const afterSuccess = searchReducer(initialState, {
      type: "submit-success",
      items,
      total: 1,
      hasMore: false,
      query: "张三",
    });
    const s = searchReducer(afterSuccess, { type: "input-change", query: "李" });
    expect(s.state).toBe("editing");
    expect(s.statusText).not.toBe("已定位 1 位同学");
  });

  // IME 组合开始即视为一次新的编辑意图：切到 editing 并刷新文案，
  // 否则查询失败后开始打字会持续显示 error 状态配旧的错误文案。
  it("refreshes state and status text when composition starts from error", () => {
    const afterError = searchReducer(initialState, { type: "submit-error", cause: new ApiError("x", undefined, "network") });
    const s = searchReducer(afterError, { type: "composition-start" });
    expect(s.state).toBe("editing");
    expect(s.isComposing).toBe(true);
    expect(s.statusText).not.toBe("网络连接异常，请检查后重试");
  });

  it("refreshes state and status text when composition starts from success", () => {
    const items: Student[] = [{ name: "张三", grade: "高一", className: "1班" }];
    const afterSuccess = searchReducer(initialState, {
      type: "submit-success",
      items,
      total: 1,
      hasMore: false,
      query: "张三",
    });
    const s = searchReducer(afterSuccess, { type: "composition-start" });
    expect(s.state).toBe("editing");
    expect(s.statusText).not.toBe("已定位 1 位同学");
  });

  // 组合不影响在途请求：处于 loading 时组合开始必须保留 loading，
  // 否则响应返回时会落到已被改写的状态上。
  it("keeps loading when composition starts during a request", () => {
    const loading = searchReducer(initialState, { type: "submit-start" });
    const s = searchReducer(loading, { type: "composition-start" });
    expect(s.state).toBe("loading");
    expect(s.isComposing).toBe(true);
  });

  // 组合进行中的候选文字变化不应再改写状态与文案。
  it("keeps state and status text during composition", () => {
    const composing = searchReducer(initialState, { type: "composition-start" });
    const s = searchReducer(composing, { type: "input-change", query: "张" });
    expect(s.state).toBe("editing");
    expect(s.statusText).toBe(composing.statusText);
  });

  it("clears results on submit-start", () => {
    const withResults = { ...initialState, items: [] as Student[], state: "duplicate" as SearchState };
    const s = searchReducer(withResults, { type: "submit-start" });
    expect(s.state).toBe("loading");
    expect(s.items).toEqual([]);
    expect(s.loadingMore).toBe(false);
  });

  it("settles to success/duplicate/empty", () => {
    const items = [{ name: "张三", grade: "高一" as const, className: "1班" }];
    expect(searchReducer(initialState, { type: "submit-success", items, total: 1, hasMore: false, query: "张三" }).state).toBe("success");
    expect(searchReducer(initialState, { type: "submit-success", items, total: 2, hasMore: true, query: "张三" }).state).toBe("duplicate");
    expect(searchReducer(initialState, { type: "submit-success", items: [], total: 0, hasMore: false, query: "查无此人" }).state).toBe("empty");
  });

  it("load-more-result ok appends items and resets loadingMore", () => {
    const base = { ...initialState, items: [{ name: "A", grade: "高一" as const, className: "1班" }], state: "duplicate" as SearchState, loadingMore: true };
    const s = searchReducer(base, { type: "load-more-result", result: { ok: true, response: { items: [{ name: "B", grade: "高一" as const, className: "1班" }], total: 2, limit: 10, offset: 1, hasMore: false } } });
    expect(s.items.map((i) => i.name)).toEqual(["A", "B"]);
    expect(s.loadingMore).toBe(false);
    expect(s.hasMore).toBe(false);
  });

  it("load-more-result error sets loadMoreError and resets loadingMore", () => {
    const s = searchReducer({ ...initialState, loadingMore: true }, { type: "load-more-result", result: { ok: false, reason: "error", cause: new Error("boom") } });
    expect(s.loadMoreError).toBe(true);
    expect(s.loadingMore).toBe(false);
  });

  it("load-more-result stale silently resets loadingMore", () => {
    // 回归锁：stale 复位此前依赖 App 无条件 dispatch settle 兜底；
    // 收成单 action 后，静默复位必须在 reducer 内显式完成，否则删除 settle 后 stale 会卡死 loadingMore。
    const s = searchReducer({ ...initialState, loadingMore: true }, { type: "load-more-result", result: { ok: false, reason: "stale" } });
    expect(s.loadingMore).toBe(false);
    expect(s.loadMoreError).toBe(false);
  });

  it("resets on clear", () => {
    const s = searchReducer(initialState, { type: "clear" });
    expect(s).toEqual(initialState);
  });
});

describe("statusTextFor", () => {
  it("hints whole-grade message for pure grade query", () => {
    expect(statusTextFor("duplicate", 200, false)).toContain("整个年段/班级");
  });
  it("normal text for named query", () => {
    expect(statusTextFor("duplicate", 200, true)).toContain("先显示前");
  });
});

// 编排归位：调用方只提供原始事实（响应或错误），状态派生与文案由 reducer
// 内部完成。此前 getState/hasNameCondition/statusTextFor/errorMessage 必须在
// App.tsx 的每个分支手工串联，派生规则没有测试覆盖，调用点也无法证伪。
describe("orchestration inside the reducer", () => {
  it("derives success and its status text from a raw response", () => {
    const items: Student[] = [{ name: "张三", grade: "高一", className: "1班" }];
    const s = searchReducer(initialState, { type: "submit-start" });
    const next = searchReducer(s, { type: "submit-success", items, total: 1, hasMore: false, query: "张三" });
    expect(next.state).toBe("success");
    expect(next.statusText).toBe("已定位 1 位同学");
  });

  it("derives empty when the response carries no match", () => {
    const s = searchReducer(initialState, { type: "submit-start" });
    const next = searchReducer(s, { type: "submit-success", items: [], total: 0, hasMore: false, query: "查无此人" });
    expect(next.state).toBe("empty");
    expect(next.statusText).toBe("没有找到匹配记录");
  });

  it("hints the whole grade for a pure grade query without the caller deriving it", () => {
    const s = searchReducer(initialState, { type: "submit-start" });
    const next = searchReducer(s, { type: "submit-success", items: [], total: 200, hasMore: true, query: "高一" });
    expect(next.state).toBe("duplicate");
    expect(next.statusText).toContain("整个年段/班级");
  });

  it("classifies the failure cause into status text", () => {
    const s = searchReducer(initialState, { type: "submit-start" });
    const next = searchReducer(s, { type: "submit-error", cause: new ApiError("x", 429, "rate_limited") });
    expect(next.state).toBe("error");
    expect(next.statusText).toBe("请求过于频繁，请稍候再试");
  });
});

describe("deriveStatusHint", () => {
  // 这组断言锁的是「界面提示只由查询状态派生」，此前三条支路改坏都不会有任何用例翻红：
  // 把 App.tsx 的 disabled 改成 false、把 data-state 硬编码、让 StatusOrb 对任何状态都渲染，
  // 132 条用例全部通过。断言必须锁住派生值本身，野生支路才无法复活。
  const all: SearchState[] = ["idle", "editing", "loading", "success", "duplicate", "empty", "error"];

  it("只有 loading 状态禁用提交并渲染加载指示", () => {
    for (const state of all) {
      const hint = deriveStatusHint(state);
      expect(hint.busy).toBe(state === "loading");
      expect(hint.showOrb).toBe(state === "loading");
      expect(hint.sendLabel).toBe(state === "loading" ? "正在检索" : "开始搜索");
    }
  });

  it("提示色按命中与出错分档", () => {
    expect(deriveStatusHint("success").tone).toBe("ink");
    expect(deriveStatusHint("duplicate").tone).toBe("ink");
    expect(deriveStatusHint("error").tone).toBe("dim");
    for (const state of ["idle", "editing", "loading", "empty"] as SearchState[]) {
      expect(deriveStatusHint(state).tone).toBe("muted");
    }
  });

  it("提交禁用与滚动时机各自独立，新增忙碌态时两处不会错位", () => {
    // 忙碌期间不滚动但禁用提交；查询落定后两者同时放开。
    // 这两条此前分处组件与模块，若只改一处不会报错——这里锁住它们的一致性。
    expect(deriveStatusHint("loading").busy).toBe(true);
    expect(shouldScrollToResults("loading")).toBe(false);
    for (const state of ["success", "duplicate", "empty", "error"] as SearchState[]) {
      expect(deriveStatusHint(state).busy).toBe(false);
      expect(shouldScrollToResults(state)).toBe(true);
    }
  });
});

// present 把视图需要的全部展示派生收成一次调用。
//
// 增量在哪：既有测试分别打 resultSectionOf / shouldScrollToResults / deriveStatusHint
// 三个函数，没有任何断言锁住它们对同一状态给出一致的组合。组件此前必须同时
// import 三个派生并另调 controller，界面知识横跨两个 module。
// 下列断言从「同一次派生」这一接缝验证一致性，改任一派生即翻红。
describe("present", () => {
  it("每个查询状态的展示派生组合都可用且自洽", () => {
    const all: SearchState[] = ["idle", "editing", "loading", "success", "duplicate", "empty", "error"];
    for (const state of all) {
      const p = present({ ...initialState, state, query: "张三" });
      expect(p.section).toBe(resultSectionOf(state));
      expect(p.scroll).toBe(shouldScrollToResults(state));
      expect(p.busy).toBe(state === "loading");
      expect(p.showOrb).toBe(state === "loading");
      expect(p.sendLabel).toBe(deriveStatusHint(state).sendLabel);
      expect(p.tone).toBe(deriveStatusHint(state).tone);
    }
  });

  it("有结果区段才滚动，但加载中除外", () => {
    expect(present({ ...initialState, state: "loading" })).toMatchObject({ section: "loading", scroll: false });
    expect(present({ ...initialState, state: "error" })).toMatchObject({ section: "error", scroll: true });
    expect(present({ ...initialState, state: "success" })).toMatchObject({ section: "list", scroll: true });
    expect(present({ ...initialState, state: "idle" })).toMatchObject({ section: null, scroll: false });
  });

  it("忙碌态下不滚动：正在检索时把用户送到结果区是错位的", () => {
    const loading = present({ ...initialState, state: "loading" });
    expect(loading.busy).toBe(true);
    expect(loading.scroll).toBe(false);
  });
});