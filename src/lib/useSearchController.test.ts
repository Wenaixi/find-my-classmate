import { describe, expect, it, vi } from "vitest";
import { createSearchOrchestrator } from "./useSearchController";
import type { SearchResponse } from "../types";

const okResponse = (over: Partial<SearchResponse> = {}): SearchResponse => ({
  items: [], total: 0, limit: 10, offset: 0, hasMore: false, ...over,
});

describe("createSearchOrchestrator", () => {
  it("submit runs query through api and maps success to state", async () => {
    const api = { search: vi.fn().mockResolvedValue(okResponse({ items: [{ name: "甲", grade: "高一", className: "1班" }], total: 1, hasMore: false })) };
    const orch = createSearchOrchestrator({ pageSize: 10, api });
    orch.onInput("甲");
    await orch.submit();
    expect(orch.getState().items).toHaveLength(1);
    expect(orch.getState().state).toBe("success");
  });

  it("submit ignores stale responses (superseded by a newer submit)", async () => {
    let resolveFirst!: (r: SearchResponse) => void;
    const api = {
      search: vi
        .fn()
        .mockImplementationOnce(() => new Promise<SearchResponse>((r) => (resolveFirst = r)))
        .mockResolvedValue(okResponse()),
    };
    const orch = createSearchOrchestrator({ pageSize: 10, api });
    orch.onInput("甲");
    const p1 = orch.submit();
    orch.onInput("乙");
    await orch.submit();
    resolveFirst(okResponse());
    await p1;
    expect(orch.getState().state).not.toBe("loading"); // stale 不进入业务状态
  });

  it("loadMore guards on hasMore and appends via load-more-result", async () => {
    const api = { search: vi.fn().mockResolvedValue(okResponse({ items: [{ name: "甲", grade: "高一", className: "1班" }], total: 1, hasMore: true })) };
    const orch = createSearchOrchestrator({ pageSize: 10, api });
    orch.onInput("甲");
    await orch.submit();
    await orch.loadMore();
    expect(orch.getState().loadingMore).toBe(false);
  });

  it("loadMore appends the next page to items", async () => {
    // 追加语义此前只有 searchReducer 层单独覆盖：编排层的 loadMore 用例
    // 只断言 loadingMore 为 false，把 loadMore 改成永不追加也依然全绿。
    // 本条从编排入口断言 items 真的增长，锁住「守卫放行 → 结果抵达 reducer」这段接线。
    const first = okResponse({ items: [{ name: "甲", grade: "高一", className: "1班" }], total: 2, hasMore: true });
    const second = okResponse({ items: [{ name: "乙", grade: "高一", className: "2班" }], total: 2, hasMore: false });
    const api = { search: vi.fn().mockResolvedValueOnce(first).mockResolvedValueOnce(second) };
    const orch = createSearchOrchestrator({ pageSize: 10, api });
    orch.onInput("甲");
    await orch.submit();
    expect(orch.getState().items.map((s) => s.name)).toEqual(["甲"]);
    await orch.loadMore();
    expect(orch.getState().items.map((s) => s.name)).toEqual(["甲", "乙"]);
  });

  it("loadMore does not request when there is no next page", async () => {
    // 守卫的另一侧：无后续页时不发起请求。此前守卫被改成无条件 return 也无人发现。
    const api = { search: vi.fn().mockResolvedValue(okResponse({ items: [{ name: "甲", grade: "高一", className: "1班" }], total: 1, hasMore: false })) };
    const orch = createSearchOrchestrator({ pageSize: 10, api });
    orch.onInput("甲");
    await orch.submit();
    const callsAfterSubmit = api.search.mock.calls.length;
    await orch.loadMore();
    expect(api.search.mock.calls.length).toBe(callsAfterSubmit);
    expect(orch.getState().items.map((s) => s.name)).toEqual(["甲"]);
  });

  it("clear invalidates and resets state", async () => {
    const api = { search: vi.fn().mockResolvedValue(okResponse()) };
    const orch = createSearchOrchestrator({ pageSize: 10, api });
    orch.onInput("甲");
    await orch.submit();
    orch.clear();
    expect(orch.getState().state).toBe("idle");
  });
});
