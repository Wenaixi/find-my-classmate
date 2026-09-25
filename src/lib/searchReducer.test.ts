import { describe, expect, it } from "vitest";
import { getState, errorMessage, initialState, searchReducer, statusTextFor } from "./searchReducer";
import { ApiError } from "./api";
import type { SearchState } from "../types";

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

  it("clears results on submit-start", () => {
    const withResults = { ...initialState, items: [{} as any], state: "duplicate" as SearchState };
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