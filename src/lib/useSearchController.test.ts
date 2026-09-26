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

  it("clear invalidates and resets state", async () => {
    const api = { search: vi.fn().mockResolvedValue(okResponse()) };
    const orch = createSearchOrchestrator({ pageSize: 10, api });
    orch.onInput("甲");
    await orch.submit();
    orch.clear();
    expect(orch.getState().state).toBe("idle");
  });
});
