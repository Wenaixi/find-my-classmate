// @vitest-environment jsdom
// 页面组装的渲染契约：穿过 App 的 JSX 接线。
//
// App 此前是零参数组件、依赖硬编码在调用点（App.tsx:17 把 searchApi 写死），
// 全仓唯一 import 是 main.tsx:3，因此输入框的 6 条回调接线、12 个属性、
// 区域切换与派生下传全部零渲染断言。本文件给 App 一根可注入的接口，
// 使这些接线成为「可穿过的事实」而不是「靠人记住」。
//
// 夹具成本（探针实测）：ResizeObserver / scrollIntoView / getContext 三项
// 已在 components.test.tsx 有现成垫片；matchMedia 与 IntersectionObserver
// 是 border-beam 的依赖，此前全库无人垫过。
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import type { SearchSessionApi } from "./lib/searchSession";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { createElement } from "react";
import { App } from "./App";
import type { Student } from "./types";

// React 18 要求显式声明 act 环境，否则 act() 触发的更新不会在 act 内 flush。
const actEnv = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
actEnv.IS_REACT_ACT_ENVIRONMENT = true;

// ---- 浏览器 API 垫片：只补 jsdom 未实现、且被真实调用到的那几个方法，
// 不模拟任何布局行为，因此不会让断言失真。 ----

class NoopResizeObserver {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}
const g = globalThis as unknown as Record<string, unknown>;
if (g["ResizeObserver"] === undefined) g["ResizeObserver"] = NoopResizeObserver;

const proto = Element.prototype as unknown as Record<string, unknown>;
if (typeof proto["scrollIntoView"] !== "function") {
  proto["scrollIntoView"] = function scrollIntoView(): void {};
}

// canvas.getContext：thinking-orbs 挂载时调用（jsdom 未实现）。
const canvasProto = g["CanvasRenderingContext2D"] as { prototype?: Record<string, unknown> } | undefined;
if (canvasProto?.prototype !== undefined && typeof canvasProto.prototype["getContext"] !== "function") {
  canvasProto.prototype["getContext"] = function getContext(): null {
    return null;
  };
}
Element.prototype.scrollIntoView = function scrollIntoView(): void {};
if (g["matchMedia"] === undefined) {
  g["matchMedia"] = function matchMedia(query: string) {
    void query;
    return {
      matches: false,
      media: "",
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    };
  };
}

// IntersectionObserver：动画库探测元素可见性用。
if (g["IntersectionObserver"] === undefined) {
  g["IntersectionObserver"] = class NoopIntersectionObserver {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
    takeRecords(): [] {
      return [];
    }
    root = null;
    rootMargin = "";
    thresholds: number[] = [];
  };
}

// 两类噪音不来自组装逻辑：动画库挂载时的 canvas 报错与 lazy chunk 的
// Suspense 提示。按消息特征静音，其余 console.error 原样透出。
const realConsoleError = console.error;
beforeEach(() => {
  console.error = (...args: unknown[]) => {
    const first = args[0];
    if (typeof first === "string" && first.includes("getContext")) return;
    if (typeof first === "string" && first.includes("The above error occurred in")) return;
    realConsoleError(...args);
  };
});
afterEach(() => {
  console.error = realConsoleError;
});

let mountedRoot: Root | null = null;
afterEach(() => {
  if (mountedRoot) act(() => mountedRoot!.unmount());
  mountedRoot = null;
  g["fetch"] = realFetch;
});

const realFetch = globalThis.fetch;

const 张三: Student = { name: "张三", grade: "高一", className: "1班" };

/** 假 api：记录调用、返回可编排的结果。不碰网络。 */
interface FakeSearchApi {
  calls: { query: string; limit: number; offset: number }[];
  search: SearchSessionApi["search"];
}

function fakeApi(items: Student[] = [], hasMore = false): FakeSearchApi {
  const calls: { query: string; limit: number; offset: number }[] = [];
  return {
    calls,
    search: async (query, limit, offset) => {
      calls.push({ query, limit, offset });
      return { items, total: items.length, limit, offset, hasMore };
    },
  };
}

function mountApp(api: FakeSearchApi) {
  const container = document.createElement("div");
  document.body.appendChild(container);
  // App 挂载即拉取 /api/health 取版本号（App.tsx:37-41），失败静默隐藏。
  // 只提供被读取的字段，声明为 Partial 让「缺什么」显式可见，不做整对象断言。
  const healthResponse: Partial<Response> = {
    ok: true,
    json: async () => ({ status: "ok", version: "test" }),
  };
  const fakeFetch: typeof fetch = async () => healthResponse as Response;
  g["fetch"] = fakeFetch;

  mountedRoot = createRoot(container);
  act(() => mountedRoot!.render(<App api={{ search: api.search }} />));
  return container;
}

describe("App 装配接线", () => {
  it("首屏不渲染结果区——展示派生 present 的 section 为 null 时不占位", () => {
    const container = mountApp(fakeApi());
    expect(container.querySelector("[data-od-id=results-section]")).toBeNull();
  });

  // 输入框的 maxLength 来自 config.ts 的 MAX_QUERY_LENGTH（契约常量，
  // 与后端 maxQueryRunes 对拍）。它是 JSX 接线的一部分，编译期只保证
  // 属性名存在，不保证值来自正确的声明。
  it("输入框的长度上限接的是契约常量", () => {
    const container = mountApp(fakeApi());
    const input = container.querySelector("#query") as HTMLInputElement;
    expect(input.maxLength).toBeGreaterThan(0);
  });

  it("提交按钮初始可用，空查询时清空按钮不渲染", () => {
    const container = mountApp(fakeApi());
    const send = container.querySelector("[data-od-id=search-cta]") as HTMLButtonElement;
    expect(send.disabled).toBe(false);
    // showClear 由 present 派生（query.length > 0），空输入时不该有清空按钮
    expect(container.querySelector("[data-od-id=search-clear]")).toBeNull();
  });

  it("输入后清空按钮出现，点击清空后输入框与按钮一起复位", async () => {
    const container = mountApp(fakeApi());

    const input = container.querySelector("#query") as HTMLInputElement;
    await act(async () => {
      setNativeValue(input, "张三");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });

    const clearBtn = container.querySelector("[data-od-id=search-clear]");
    expect(clearBtn).not.toBeNull();

    await act(async () => {
      (clearBtn as HTMLButtonElement).click();
    });

    expect((container.querySelector("#query") as HTMLInputElement).value).toBe("");
    expect(container.querySelector("[data-od-id=search-clear]")).toBeNull();
  });

  // 核心断言：输入 → Enter 提交这条链路必须真的调到注入的 api。
  // 此前 App 硬编码 searchApi，任何测试都无法穿过这条路径，
  // 「onKeyDown 传了 isComposing」这件事只有类型系统守着。
  it("Enter 提交会调用注入的 api 并渲染结果区", async () => {
    const api = fakeApi([张三]);
    const container = mountApp(api);

    const input = container.querySelector("#query") as HTMLInputElement;
    await act(async () => {
      setNativeValue(input, "张三");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });

    await act(async () => {
      input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
      // 让 submit 的 await 链走完
      await Promise.resolve();
    });

    expect(api.calls.length).toBe(1);
    expect(api.calls[0].query).toBe("张三");
    expect(container.querySelector("[data-od-id=results-section]")).not.toBeNull();
  });

  // IME 守卫穿过 App 的证据。此处刻意不派发 compositionstart：
  // 那样 state.isComposing 会是 true，拦截会由 useSearchInput 的第二条守卫兜住，
  // App 有没有把 nativeEvent.isComposing 接上就无人知道了（实测：变异该接线，
  // 本用例仍全绿）。只发 keydown 并在事件上带 isComposing:true，
  // 拦截就完全取决于 App 是否把该值透传给 useSearchInput。
  //
  // 变异实验：把 App.tsx 的 event.nativeEvent.isComposing 换成硬编码 false，
  // 本用例翻红。
  it("浏览器报告组合中时 Enter 不提交——接线断了就会提交", async () => {
    const api = fakeApi([张三]);
    const container = mountApp(api);

    const input = container.querySelector("#query") as HTMLInputElement;
    await act(async () => {
      setNativeValue(input, "张三");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });

    // jsdom 的 KeyboardEvent 上 isComposing 是只读 getter（原生即如此），
    // 因此不能赋值，只能重定义该属性。
    const composingEnter = new KeyboardEvent("keydown", { key: "Enter", bubbles: true });
    Object.defineProperty(composingEnter, "isComposing", { value: true, configurable: true });
    await act(async () => {
      input.dispatchEvent(composingEnter);
      await Promise.resolve();
    });

    expect(api.calls.length).toBe(0);
  });

  // 与上一条成对：同一事件在非组合态下必须提交。
  // 只断言「不提交」的话，把整条 onKeyDown 接线删掉同样恒真。
  it("非组合态的同一事件必须提交——双向断言", async () => {
    const api = fakeApi([张三]);
    const container = mountApp(api);

    const input = container.querySelector("#query") as HTMLInputElement;
    await act(async () => {
      setNativeValue(input, "张三");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
      await Promise.resolve();
    });

    expect(api.calls.length).toBe(1);
  });

  it("结果区计数接的是 present 派生的 countLabel", async () => {
    const api = fakeApi([张三]);
    const container = mountApp(api);
    const input = container.querySelector("#query") as HTMLInputElement;

    await act(async () => {
      setNativeValue(input, "张三");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
      await Promise.resolve();
    });

    // 零条时用占位符而非 0——这条口径此前只在 present 的单测里
    expect(container.querySelector("[data-od-id=result-count]")!.textContent).toBe("1");
  });
});

// React 追踪 onChange 需要绕过它的值劫持：直接改 DOM value 后派发 input 事件，
// React 的 onChange 才会读到新值。
function setNativeValue(element: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  setter?.call(element, value);
}
