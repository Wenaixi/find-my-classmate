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

// React 18 要求显式声明 act 环境，否则 force() 触发的更新不会在 act 内被 flush，
// 测试会读到未刷新的旧快照而产生假阴性。
const actEnv = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
actEnv.IS_REACT_ACT_ENVIRONMENT = true;

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
});
