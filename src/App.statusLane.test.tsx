// @vitest-environment jsdom
// 状态行那条 lane 的错误边界接线契约。
//
// 为何单开一个文件：App 用 lazy(() => import(...)) 加载 StatusOrb 与
// ResultList，而 vitest 的 vi.mock 是文件作用域且在模块求值期生效——
// 同一个文件里既要让某个用例走正常模块、又要让另一个用例走抛错模块，
// 必须用 resetModules + 动态 import，而那会把 App 整棵依赖树重新求值，
// 实测在 vitest 3 下触发 hoisting 限制。分成两个文件后，每个文件的模块图
// 各自确定，mock 的作用域就是它该有的样子。
//
// 承重对象是 App 中 ErrorBoundary 的**放置位置**，不是 ErrorBoundary 自身：
// 直接渲染 ErrorBoundary 只能证明边界能抓错，把 App 里的边界整块删掉它照样通过。
// 故障形状与生产同源——动态 import 的模块抛错在 lazy 渲染时以渲染错误出现，
// 落在 Suspense 内部、由其上方的边界捕获。
//
// 双向：正向断「状态行出现降级界面」；反向断「结果区未被带走」。
// 只断前者的话，把边界删掉同样会有降级界面（本仓记为恒真断言）。
import { afterEach, describe, expect, it, vi } from "vitest";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import type { SearchSessionApi } from "./lib/searchSession";
import { App } from "./App";
import type { Student } from "./types";
import "./lib/testActEnv";

// 渲染时抛错，与 chunk 加载失败在 lazy 边界上的表现同路径。
vi.mock("./components/StatusOrb", () => ({
  default: function BrokenStatusOrb(): never {
    throw new Error("chunk 404");
  },
}));

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
const canvasElementProto = g["HTMLCanvasElement"] as { prototype?: Record<string, unknown> } | undefined;
if (canvasElementProto?.prototype !== undefined && typeof canvasElementProto.prototype["getContext"] !== "function") {
  canvasElementProto.prototype["getContext"] = function getContext(): null {
    return null;
  };
}
if (g["matchMedia"] === undefined) {
  g["matchMedia"] = function matchMedia(): unknown {
    return { matches: false, media: "", onchange: null, addListener() {}, removeListener() {}, addEventListener() {}, removeEventListener() {}, dispatchEvent: () => false };
  };
}
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
    thresholds = [];
  };
}

// React 会把未捕获错误打到 console 上：这里显式吞掉，否则真实失败难辨。
// 断言不依赖 console——它依赖 DOM 是否还在，这是用户实际看到的东西。
const realConsoleError = console.error;
console.error = () => {};

const 张三: Student = { name: "张三", grade: "高一", className: "1班" };

let mountedRoot: Root | null = null;
afterEach(() => {
  if (mountedRoot) act(() => mountedRoot!.unmount());
  mountedRoot = null;
  console.error = realConsoleError;
});

describe("状态行的错误边界接线", () => {
  it("状态行抛错被边界捕获，结果区不被带走", async () => {
    const search: SearchSessionApi["search"] = async (_q, limit, offset) => ({
      items: [张三],
      total: 1,
      limit,
      offset,
      hasMore: false,
    });
    // 只提供被读取的字段：声明为 Partial 让「缺什么」显式可见，
    // 与 App.mount.test.tsx 同一手法——不做整对象断言，也不写 unsafe cast。
    const health = { ok: true, json: async () => ({ status: "ok", version: "test" }) } as Partial<Response>;
    g["fetch"] = (async () => health) as typeof fetch;

    const container = document.createElement("div");
    document.body.appendChild(container);
    mountedRoot = createRoot(container);
    await act(async () => {
      mountedRoot!.render(<App api={{ search }} />);
      await Promise.resolve();
    });
    // 提交一次查询，让结果区成为「页面其余部分」的取样点。
    const input = container.querySelector("#query") as HTMLInputElement;
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
    await act(async () => {
      setter?.call(input, "张三");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
      await Promise.resolve();
    });
    await act(async () => {
      await Promise.resolve();
    });

    // 反向：结果区仍在。缺了边界时状态行的错误会冒泡到 React 根，整页白屏，
    // 连有自己边界的 ResultList 一起消失——这一条正是白屏的判据。
    expect(container.querySelector("[data-od-id=results-section]")).not.toBeNull();
    // 正向：状态行自身给出可恢复的降级界面，错误没有继续冒泡。
    expect(container.querySelector("[role=alert]")).not.toBeNull();
  });
});