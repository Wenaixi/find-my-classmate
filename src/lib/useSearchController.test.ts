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

  // 「stale 不进入业务状态」的判别力问题：原断言是 expect(state).not.toBe("loading")，
  // 它声称的对象是「stale 响应不会到达 reducer」，实际断言的对象只是
  // 「最终状态不是 loading」——只排除七个取值中的一个，放弃了对其余六个的判别力。
  // 变异实验（2026-09-30 第十六轮）：把下面的 reason === "error" 守卫去掉、
  // 使 stale 与 error 同等对待（stale 也派发 submit-error），本文件 6 条用例全绿，
  // 全量 178 条亦全绿；而行为确实坏了——用户会看到自己没发过的查询失败。
  //
  // 修法要点：取样点必须落在「stale 抵达之后、后续 submit 覆盖之前」。
  // 原用例在两次 submit 都完成后才断言，此时错误态已被成功结果覆盖，
  // 于是「首帧闪 error」这一唯一症状不可见。改为在 resolveFirst 之后立刻取样。
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

    // 第二次 submit 成功后，状态已被 success 覆盖；此时旧响应才迟到。
    resolveFirst(okResponse({ items: [{ name: "甲", grade: "高一", className: "1班" }], total: 1 }));
    await p1;

    // 双向断言之一（正向）：stale 不得把状态推进 error。
    // 变异态下此处为 "error" 且 statusText 是失败文案，本条立即翻红。
    const settled = orch.getState();
    expect(settled.state).not.toBe("error");
    expect(settled.statusText).not.toContain("失败");

    // 双向断言之二（反向）：也不能停在 loading 态。stale 响应抵达后本次查询
    // 已经结束，界面不得继续显示「正在检索」。只断这一侧的话，把
    // 「stale 让状态卡在 loading」的错误实现同样会通过。
    expect(settled.state).not.toBe("loading");
  });

  // 与上一条成对：stale 路径下界面必须呈现「后续查询的结果」，而不是任何错误态。
  // 只断言「不是 error」的话，把 stale 当作空成功（既不置错也不追加）同样通过——
  // 那种实现会让 items 停在旧值。本条锁定 stale 语义的实际结果。
  it("stale 响应不追加任何条目", async () => {
    let resolveFirst!: (r: SearchResponse) => void;
    const api = {
      search: vi
        .fn()
        .mockImplementationOnce(() => new Promise<SearchResponse>((r) => (resolveFirst = r)))
        .mockResolvedValue(okResponse({ items: [{ name: "乙", grade: "高一", className: "2班" }], total: 1 })),
    };
    const orch = createSearchOrchestrator({ pageSize: 10, api });
    orch.onInput("甲");
    const p1 = orch.submit();
    orch.onInput("乙");
    await orch.submit();
    expect(orch.getState().items.map((s) => s.name)).toEqual(["乙"]);

    resolveFirst(okResponse({ items: [{ name: "甲", grade: "高一", className: "1班" }], total: 1 }));
    await p1;

    // 过期响应不得追加进结果区：items 仍只有第二条查询的结果。
    expect(orch.getState().items.map((s) => s.name)).toEqual(["乙"]);
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
