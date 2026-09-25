import { describe, expect, it } from "vitest";
import { getState, errorMessage, initialState, searchReducer, statusTextFor } from "./searchReducer";
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

describe("searchReducer", () => {
  it("tracks editing on input during composition", () => {
    const s = searchReducer(initialState, { type: "input-change", query: "张" });
    expect(s.state).toBe("editing");
  });

  // 状态与提示文案必须一致：切到 editing 时不能保留上一轮的
  // 错误或结果文案，否则用户会看到 editing 状态配上"网络异常"之类的提示。
  it("refreshes status text when leaving error for editing", () => {
    const afterError = searchReducer(initialState, { type: "submit-error", statusText: "网络连接异常，请检查后重试" });
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
      state: "success",
      statusText: "已定位 1 位同学",
    });
    const s = searchReducer(afterSuccess, { type: "input-change", query: "李" });
    expect(s.state).toBe("editing");
    expect(s.statusText).not.toBe("已定位 1 位同学");
  });

  // IME 组合开始即视为一次新的编辑意图：切到 editing 并刷新文案，
  // 否则查询失败后开始打字会持续显示 error 状态配旧的错误文案。
  it("refreshes state and status text when composition starts from error", () => {
    const afterError = searchReducer(initialState, { type: "submit-error", statusText: "网络连接异常，请检查后重试" });
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
      state: "success",
      statusText: "已定位 1 位同学",
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
    expect(searchReducer(initialState, { type: "submit-success", items, total: 1, hasMore: false, state: "success", statusText: "已定位 1 位同学" }).state).toBe("success");
    expect(searchReducer(initialState, { type: "submit-success", items, total: 2, hasMore: true, state: "duplicate", statusText: "已定位多位同学" }).state).toBe("duplicate");
    expect(searchReducer(initialState, { type: "submit-success", items: [], total: 0, hasMore: false, state: "empty", statusText: "没有找到匹配记录" }).state).toBe("empty");
  });

  it("appends on load-more-append", () => {
    const base = { ...initialState, items: [{ name: "A", grade: "高一" as const, className: "1班" }], state: "duplicate" as SearchState };
    const s = searchReducer(base, { type: "load-more-append", items: [{ name: "B", grade: "高一" as const, className: "1班" }], total: 2, hasMore: false });
    expect(s.items.map((i) => i.name)).toEqual(["A", "B"]);
    expect(s.loadingMore).toBe(false);
    expect(s.hasMore).toBe(false);
  });

  it("resets on clear", () => {
    const s = searchReducer(initialState, { type: "clear" });
    expect(s).toEqual(initialState);
  });
});

describe("statusTextFor", () => {
  it("hints whole-grade message for pure grade query (F36)", () => {
    expect(statusTextFor("duplicate", 200, false)).toContain("整个年段/班级");
  });
  it("normal text for named query", () => {
    expect(statusTextFor("duplicate", 200, true)).toContain("先显示前");
  });
});