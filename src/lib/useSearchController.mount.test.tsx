// @vitest-environment jsdom
// 回归测试：useSearchController 的 hook 壳必须把 orchestrator 的最新 state 暴露给组件。
//
// 该测试的存在理由：此前 vitest 只收集 .test.ts 且无任何 React 挂载测试，
// useSearchController.ts 末行的 useMemo 快照把 state 冻结在首帧（提交后仍显示 idle），
// 而纯逻辑层 createSearchOrchestrator 的测试完全测不到这一点。
// 挂载测试是唯一能穿过 hook 壳这个 interface 的测试面。
import { afterEach, describe, expect, it, vi } from "vitest";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { createElement, type ReactNode } from "react";
import { ApiError } from "./api";
import { useSearchController, type SearchOrchestrator } from "./useSearchController";
import type { SearchResponse } from "../types";

import "./testActEnv";

const response = (over: Partial<SearchResponse> = {}): SearchResponse => ({
  items: [], total: 0, limit: 10, offset: 0, hasMore: false, ...over,
});

const foundResponse = (over: Partial<SearchResponse> = {}): SearchResponse =>
  response({ items: [{ name: "甲", grade: "高一", className: "1班" }], total: 1, ...over });

let mountedRoot: Root | null = null;

afterEach(() => {
  if (mountedRoot) act(() => mountedRoot!.unmount());
  mountedRoot = null;
});

// 挂载一个真实消费 useSearchController 的组件，把每次渲染读到的 state 记进 renders。
function mountController(api: { search: ReturnType<typeof vi.fn> }) {
  const renders: { state: string; items: number; statusText: string }[] = [];
  let controller: SearchOrchestrator | null = null;

  function Harness() {
    const view = useSearchController({ pageSize: 10, api });
    controller = view.controller;
    renders.push({ state: view.state.state, items: view.state.items.length, statusText: view.state.statusText });
    return createElement("div", null, view.state.state);
  }

  const container = document.createElement("div");
  document.body.appendChild(container);
  mountedRoot = createRoot(container);
  act(() => mountedRoot!.render(createElement(Harness)));
  return { renders, container, current: () => controller!, last: () => renders[renders.length - 1] };
}

describe("useSearchController（hook 壳）", () => {
  it("输入后组件应看到 editing 状态", () => {
    const h = mountController({ search: vi.fn().mockResolvedValue(foundResponse()) });
    act(() => h.current().onInput("甲"));
    expect(h.last().state).toBe("editing");
  });

  it("提交完成后组件应看到 success 与结果条目，而不是首帧快照", async () => {
    const h = mountController({ search: vi.fn().mockResolvedValue(foundResponse()) });

    act(() => h.current().onInput("甲"));
    await act(async () => {
      await h.current().submit();
    });

    // 修复前：这里拿到的是首帧 useMemo 快照，state 停留在 idle/editing。
    expect(h.last().state).toBe("success");
    expect(h.last().items).toBe(1);
    expect(h.container.textContent).toBe("success");
  });

  it("查询无结果时组件应看到 empty 状态", async () => {
    const h = mountController({ search: vi.fn().mockResolvedValue(response()) });
    act(() => h.current().onInput("查无此人"));
    await act(async () => {
      await h.current().submit();
    });
    expect(h.last().state).toBe("empty");
  });

  it("查询失败时组件应看到 error 状态与分类文案", async () => {
    const { ApiError } = await import("./api");
    const h = mountController({ search: vi.fn().mockRejectedValue(new ApiError("x", 429, "rate_limited")) });
    act(() => h.current().onInput("甲"));
    await act(async () => {
      await h.current().submit();
    });
    expect(h.last().state).toBe("error");
    expect(h.last().statusText).toBe("请求过于频繁，请稍候再试");
  });

  it("清空后组件应看到 idle 状态", async () => {
    const h = mountController({ search: vi.fn().mockResolvedValue(foundResponse()) });
    act(() => h.current().onInput("甲"));
    await act(async () => {
      await h.current().submit();
    });
    act(() => h.current().clear());
    expect(h.last().state).toBe("idle");
    expect(h.last().items).toBe(0);
  });

  // 请求在途期间必须真的渲染出 loading 帧。此前四条用例全部在 await submit()
  // 之后取样，从不落在异步动作的中间段，因此「wrap 只在 Promise 落定后 force()」
  // 导致 loading 从未进入任何一次渲染这件事一直无人发现——renders 早已在采集，
  // 只是从不断言它。
  //
  // 手动 resolver 是关键：mockResolvedValue 会在微任务里立刻落定，loading 窗口
  // 短到无法取样；挂起未决的 Promise 才能把中间段固定住。
  it("请求在途期间组件应看到 loading 状态，而不是等落定才首次重渲染", async () => {
    let settle: (value: SearchResponse) => void = () => {};
    const search = vi.fn(() => new Promise<SearchResponse>((resolve) => { settle = resolve; }));
    const h = mountController({ search });

    act(() => h.current().onInput("甲"));
    // submit 必须在 act 内发起：壳的 force() 触发的是 React 更新，
    // 在 act 之外调用时 React 不会把这次更新 flush，测试会读到未刷新的旧渲染。
    // 这是挂载测试的既有纪律，不是本用例特有的写法。
    let submitted!: Promise<void>;
    act(() => { submitted = h.current().submit(); });

    // 此刻请求仍未落定：界面必须已经进入 loading，否则用户面对的是
    // 一个「点了没反应」的界面，而状态行与提交按钮都还停在上一帧。
    expect(h.renders.map((r) => r.state)).toContain("loading");

    await act(async () => {
      settle(foundResponse());
      await submitted;
    });
    expect(h.last().state).toBe("success");
  });

  // 在途查询被组合期的输入变化作废后，状态必须被释放，不得停在 loading。
  //
  // 失效路径：组合期 input 经 onInput 无条件 session.invalidate() → fetch 被 abort
  // → perform 归为 stale → orchestrator 的 submit 此前只对 error 分支写回状态，
  // stale 直接 return，于是 state 永远停在 loading；而 submit 的守卫恰恰是
  // 「loading 中不发起新请求」，形成死角：状态行显示「正在检索完整名单」、
  // 提交按钮禁用，按 Enter 没有任何反应。这与 loadMore 侧早已修复的同族缺口
  // （reducer 内显式复位 loadingMore）只差一个分支。
  it("组合期输入作废在途查询后应释放 loading，使界面能继续响应", async () => {
    // 第一次请求挂起（用于制造 stale），第二次请求立即落定——断言的只是
    // 「守卫没有锁死提交、第二次确实发起了」，第二次的落定值与本用例无关。
    let abortRequest: (reason: unknown) => void = () => {};
    const search = vi
      .fn()
      .mockImplementationOnce(
        () => new Promise<SearchResponse>((_, reject) => { abortRequest = reject; })
      )
      .mockResolvedValue(foundResponse());
    const h = mountController({ search });

    act(() => h.current().onInput("甲"));
    let submitted!: Promise<void>;
    act(() => { submitted = h.current().submit(); });
    expect(h.last().state).toBe("loading");

    // 用户开始中文输入：组合开始后在途请求被作废。
    act(() => h.current().onCompositionStart());
    act(() => h.current().onInput("甲乙"));
    await act(async () => {
      abortRequest(Object.assign(new Error("aborted"), { name: "AbortError" }));
      await submitted;
    });

    // 关键断言：不得停在 loading。停在 loading 意味着提交按钮被守卫锁死。
    expect(h.current().getState().state).not.toBe("loading");

    // 且必须能真正再次发起查询——死角的可观测后果是「按 Enter 无反应」。
    let second!: Promise<void>;
    act(() => { second = h.current().submit(); });
    expect(search).toHaveBeenCalledTimes(2);
    await act(async () => { await second; });
  });
});
