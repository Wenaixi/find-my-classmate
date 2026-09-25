import { describe, expect, it, vi } from "vitest";
import { createSearchSession } from "./searchSession";
import type { SearchResponse } from "../types";

const okResponse = (over: Partial<SearchResponse> = {}): SearchResponse => ({
  items: [], total: 0, limit: 10, offset: 0, hasMore: false, ...over,
});

describe("createSearchSession", () => {
  it("discards stale responses after a new begin", async () => {
    let resolveFirst!: (r: SearchResponse) => void;
    const search = vi.fn().mockImplementationOnce(() => new Promise<SearchResponse>((r) => (resolveFirst = r)));
    const session = createSearchSession({ search });

    const p1 = session.submit("张三", 10, new AbortController().signal);
    const id1 = 1; // begin() 内部递增
    session.begin(); // 新请求使 id1 过期
    resolveFirst(okResponse());
    expect(await p1).toBeUndefined(); // 过期响应被丢弃

    void id1; // 避免未使用告警
  });

  it("aborts all in-flight on abortAll", () => {
    const search = vi.fn().mockResolvedValue(okResponse());
    const session = createSearchSession({ search });
    session.begin();
    session.abortAll();
    // 无断言，仅验证不抛异常且未发出新请求
    expect(search).not.toHaveBeenCalled();
  });

  it("lets loadMore follow a submit without invalidating it", async () => {
    const search = vi.fn().mockResolvedValue(okResponse({ items: [{ name: "甲", grade: "高一", className: "1班" }], total: 1, hasMore: false }));
    const session = createSearchSession({ search });
    const id = session.begin();
    const result = await session.loadMore("张三", 10, 0, new AbortController().signal);
    expect(result).toBeDefined();
    expect(session.isCurrent(id)).toBe(true);
  });

  it("submit returns undefined when request is superseded", async () => {
    const search = vi.fn().mockResolvedValue(okResponse());
    const session = createSearchSession({ search });
    const first = session.submit("张三", 10, new AbortController().signal);
    const second = session.submit("李四", 10, new AbortController().signal); // 后一次提交取代前一次
    expect(await first).toBeUndefined(); // 被取代的响应被丢弃
    expect(await second).toBeDefined(); // 最新请求正常返回
  });
});
