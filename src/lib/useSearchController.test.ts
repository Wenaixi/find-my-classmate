import { describe, expect, it, vi } from "vitest";
import { createSearchOrchestrator } from "./useSearchController";
import { errorMessage } from "./searchReducer";
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
    // 变异态下此处为 "error"，本条立即翻红。
    const settled = orch.getState();
    expect(settled.state).not.toBe("error");

    // 状态文案也不得是错误文案。
    // 断言具体文案而非「失败」二字：COPY.error 是「查询没有完成，请稍后重试」，
    // 断言不含「失败」会恒真通过——恒真断言比没有断言更危险，它提供虚假的覆盖感。
    // 此处断言「文案与 error 态下算出的文案不同」，任何让 stale 落到错误文案的
    // 实现都会使其相等。
    const errorText = errorMessage(new Error("stale leak"));
    expect(settled.statusText).not.toBe(errorText);

    // 双向断言之二（反向）：也不能停在 loading 态。stale 响应抵达后本次查询
    // 已经结束，界面不得继续显示「正在检索」。只断这一侧的话，把
    // 「stale 让状态卡在 loading」的错误实现同样会通过。
    expect(settled.state).not.toBe("loading");
  });

  // stale 的落定状态必须是「正在编辑、可以再次提交」，而不只是「不是 loading」。
  //
  // 上一条用 not.toBe("loading") 断言 stale 后不卡在 loading——这挡住了死角，
  // 但没锁定它落到哪里：把 stale 当作空成功（既不置错也不释放）同样能通过那一侧，
  // 而那种实现会让界面停在「已定位 0 位同学」的错误计数上。
  //
  // 取样点是 stale 抵达之后、后续 submit 之前：中途已被成功结果覆盖时，
  // 两个实现给同一个值，断言失去判别力。
  it("在途查询被输入变化作废后，状态落到可再次提交的 editing 而非任何结果态", async () => {
    let resolveFirst!: (r: SearchResponse) => void;
    const api = {
      search: vi
        .fn()
        .mockImplementationOnce(() => new Promise<SearchResponse>((r) => (resolveFirst = r))),
    };
    const orch = createSearchOrchestrator({ pageSize: 10, api });
    orch.onInput("甲");
    const p1 = orch.submit();
    expect(orch.getState().state).toBe("loading");

    // 输入变化作废在途请求（组合期的输入走的是同一条路径）。
    orch.onInput("甲乙");
    resolveFirst(okResponse());
    await p1;

    const settled = orch.getState();
    // 落定到 editing：组合期的输入已写入 query，而针对它的查询尚未发起。
    expect(settled.state).toBe("editing");
    expect(settled.statusText).not.toBe(errorMessage(new Error("stale leak")));
    // 不得保留作废查询的条目或计数。
    expect(settled.items).toHaveLength(0);
    expect(settled.total).toBe(0);
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

  // loadMore 的 offset 此前零断言：既有 loadMore 用例全部在 await 之后取样，
  // 只断 items 增长或请求次数，从不看请求参数。
  // 变异实验：把 offset 改成常量 0（永远重新请求第一页并原地追加），
  // 前端全量仍绿——而追加语义已彻底坏掉。
  // 断言必须双向：正向断「offset 等于已加载条数」，
  // 反向断「两次请求的 offset 不同」，否则把 offset 硬编码为任何
  // 与已加载条数恰好相等的常量都能通过。
  it("loadMore 请求的 offset 是已加载条数，不重复第一页", async () => {
    const first = okResponse({ items: [{ name: "甲", grade: "高一", className: "1班" }], total: 3, hasMore: true });
    const second = okResponse({ items: [{ name: "乙", grade: "高一", className: "2班" }], total: 3, hasMore: false });
    const api = { search: vi.fn().mockResolvedValueOnce(first).mockResolvedValueOnce(second) };
    const orch = createSearchOrchestrator({ pageSize: 10, api });
    orch.onInput("甲");
    await orch.submit();
    await orch.loadMore();

    const offsets = api.search.mock.calls.map((c) => c[2] as number);
    expect(offsets[0]).toBe(0);
    expect(offsets[1]).toBe(1);
    // 反向：两次请求取样点不同，恒返同一值的实现不能通过。
    expect(offsets[0]).not.toBe(offsets[1]);
  });

  // 加载更多进行中按 Enter 必须被拦住：新查询的 submit-start 会清空 items，
  // 而追加结果随后抵达，两条并发请求会让用户看到结果被覆盖或重复。
  // 变异实验：删掉 submit 守卫里的 loadingMore 项，前端全量仍绿。
  it("加载更多进行中不发起新查询", async () => {
    let resolveLoadMore!: (r: SearchResponse) => void;
    const first = okResponse({ items: [{ name: "甲", grade: "高一", className: "1班" }], total: 2, hasMore: true });
    const api = {
      search: vi
        .fn()
        .mockResolvedValueOnce(first)
        .mockImplementationOnce(() => new Promise<SearchResponse>((r) => (resolveLoadMore = r))),
    };
    const orch = createSearchOrchestrator({ pageSize: 10, api });
    orch.onInput("甲");
    await orch.submit();
    const callsBeforeLoadMore = api.search.mock.calls.length;

    // 不 await：请求在途，此刻 submit 必须被守卫拦住。
    const pending = orch.loadMore();
    await orch.submit();
    expect(api.search.mock.calls.length).toBe(callsBeforeLoadMore + 1);

    resolveLoadMore(okResponse({ items: [{ name: "乙", grade: "高一", className: "2班" }], total: 2, hasMore: false }));
    await pending;
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
