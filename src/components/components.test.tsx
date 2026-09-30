// @vitest-environment jsdom
// 展示组件的渲染契约。
//
// 三个组件此前全部零渲染断言，变异实验证明保护缺失是真的：
// 删掉 StatusOrb 的 if (!show) return null 后，前端 144 条用例全部通过。
// 但组件薄是好事——它们不该被「加深」成有逻辑的模块，
// 缺的是一条能穿过它们的接缝，让「装饰层是否阻断焦点」「错误是否降级」
// 从「靠人记住」变成「靠测试记住」。
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { createElement, type ReactNode } from "react";
import ResultList from "./ResultList";
import StatusOrb from "./StatusOrb";
import ErrorBoundary from "./ErrorBoundary";
import { deriveResultSummary } from "../lib/resultSummary";

// React 18 要求显式声明 act 环境，否则 act() 触发的更新不会在 act 内 flush，
// 测试会读到未刷新的旧快照而产生假阴性。
const actEnv = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
actEnv.IS_REACT_ACT_ENVIRONMENT = true;

// liquid-gooey 依赖 ResizeObserver，jsdom 未实现该 API，缺失时组件挂载即抛错。
// 缺的是浏览器 API 而非组件逻辑，补最小垫片即可让组件进入可测状态——
// 垫片只提供被调用到的 observe/unobserve/disconnect，不模拟任何布局行为，
// 因此不会让断言失真。
class NoopResizeObserver {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}
if (typeof globalThis.ResizeObserver === "undefined") {
  // unchecked cast：jsdom 的 globalThis 类型未声明 ResizeObserver，
  // 而这里赋值的 NoopResizeObserver 满足其运行时契约（上述三个方法）。
  const target = globalThis as unknown as { ResizeObserver: typeof NoopResizeObserver };
  target.ResizeObserver = NoopResizeObserver;
}

// 两类噪音都不来自组件逻辑：thinking-orbs 挂载时调用 canvas.getContext
// 绘制动画（jsdom 未实现），以及子节点按设计抛错时 React 为被边界捕获的
// 错误打的警告。二者都会刷屏、掩盖真实失败，故按消息特征静音——
// 其余 console.error 一律原样透出，真失败不会被吞掉。
const realConsoleError = console.error;
beforeEach(() => {
  console.error = (...args: unknown[]) => {
    const first = args[0];
    if (typeof first === "string" && first.includes("HTMLCanvasElement.prototype.getContext")) return;
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
});

function mount(node: ReactNode): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  mountedRoot = createRoot(container);
  act(() => mountedRoot!.render(node));
  return container;
}

describe("StatusOrb", () => {
  it("show=false 时不渲染任何内容", () => {
    expect(mount(createElement(StatusOrb, { show: false })).innerHTML).toBe("");
  });

  it("show=true 时渲染加载指示", () => {
    expect(mount(createElement(StatusOrb, { show: true })).querySelector("canvas")).not.toBeNull();
  });

  // 变异实验：删掉 if (!show) return null 后，本用例与上一条同时翻红。
  // 它是「查询进行中才显示加载指示」这条界面知识的唯一保护。
  it("同一实例在 show 翻转时增删渲染内容", () => {
    const container = mount(createElement(StatusOrb, { show: false }));
    expect(container.querySelector("canvas")).toBeNull();
    act(() => mountedRoot!.render(createElement(StatusOrb, { show: true })));
    expect(container.querySelector("canvas")).not.toBeNull();
    act(() => mountedRoot!.render(createElement(StatusOrb, { show: false })));
    expect(container.querySelector("canvas")).toBeNull();
  });
});

describe("ErrorBoundary", () => {
  function Boom(): ReactNode {
    throw new Error("结果渲染失败");
  }

  it("子节点正常时不介入", () => {
    const container = mount(createElement(ErrorBoundary, null, createElement("p", null, "查询结果")));
    expect(container.textContent).toContain("查询结果");
    expect(container.querySelector("[role=alert]")).toBeNull();
  });

  // 变异实验：getDerivedStateFromError 改为返回 hasError:false 后本用例翻红，
  // 降级文案改动同样翻红。异步 chunk 加载失败（部署切换后旧 chunk 404）
  // 会冒泡到 React 根导致整页白屏，该边界的意义正在于此。
  it("子节点抛错时给出可恢复的降级界面", () => {
    const container = mount(createElement(ErrorBoundary, null, createElement(Boom)));
    const alert = container.querySelector("[role=alert]");
    expect(alert).not.toBeNull();
    expect(alert!.textContent).toContain("查询结果未能加载");
    // 降级界面必须给出恢复手段，否则用户只能刷新整个页面。
    const button = alert!.querySelector("button");
    expect(button).not.toBeNull();
    expect(button!.textContent).toContain("刷新页面");
  });
});

describe("ResultList", () => {
  const students = [
    { name: "张三", grade: "高一" as const, className: "1班" },
    { name: "李四", grade: "高二" as const, className: "2班" },
  ];

  function mountList(over: { hasMore?: boolean; loadMoreError?: boolean } = {}) {
    return mount(
      createElement(ResultList, {
        items: students,
        summary: deriveResultSummary(students.length, students.length, over.hasMore ?? false),
        hasMore: over.hasMore ?? false,
        loadingMore: false,
        loadMoreError: over.loadMoreError ?? false,
        onLoadMore: () => {},
      }),
    );
  }

  it("逐条渲染学生与所在班级", () => {
    const container = mountList();
    expect(container.textContent).toContain("张三");
    expect(container.textContent).toContain("李四");
    expect(container.textContent).toContain("1班");
    expect(container.textContent).toContain("2班");
  });

  it("hasMore 为假时不渲染继续加载按钮", () => {
    expect(mountList().querySelector("[data-od-id=load-more-cta]")).toBeNull();
  });

  it("hasMore 为真时渲染继续加载按钮", () => {
    const button = mountList({ hasMore: true }).querySelector("[data-od-id=load-more-cta]");
    expect(button).not.toBeNull();
    expect(button!.getAttribute("aria-label")).toContain("继续加载");
  });

  it("加载更多失败时给出可重试提示", () => {
    const alert = mountList({ loadMoreError: true }).querySelector("[role=alert]");
    expect(alert).not.toBeNull();
    expect(alert!.textContent).toContain("加载失败");
  });
  // 变异实验：去掉 loadMoreError 条件后，仅断言「出错时出现提示」仍然通过——
  // 元素恒在则断言恒真。该条件必须双向断言才有承重。
  it("loadMoreError 为假时不渲染失败提示", () => {
    expect(mountList().querySelector(".load-more-error")).toBeNull();
  });

  // 本组唯一涉及第三方库副作用的断言，也是最不可替代的一条。
  // liquid-gooey 渲染的装饰 SVG 含幽灵 g 节点，会被 Chrome 捕获进 Tab 序列，
  // 落在「搜索」与「继续加载」之间造成键盘焦点陷落——纯装饰层却能吃掉键盘焦点，
  // 键盘用户会被困在结果区里出不来。ResultList 用 inert 阻断该行为。
  //
  // 此前无任何断言覆盖，删掉整个 useEffect 全部测试仍然通过。
  // 变异实验：移除 setAttribute("inert", "") 后本用例翻红。
  it("全部装饰 SVG 层标记为 inert，阻断键盘焦点陷落", () => {
    // liquid-gooey 渲染两个装饰 SVG：data-gooey-svg 与 data-gooey-overlay。
    // 库自己把二者当同类（其 MutationObserver 用 closest 匹配
    // "[data-gooey-svg], [data-gooey-overlay]"），而 overlay 的 z-index 是 9999、
    // 库注释自陈「Above the content layer by design」，内含与 svg 层结构对称的
    // g/defs/mask——即幽灵节点的来源与 svg 层相同。
    //
    // 修复此前只选择 [data-gooey-svg]，只挡住一半。逐层断言而非断言总数：
    // 断「总数 > 0」的话，只覆盖一层的实现同样通过。
    for (const selector of ["[data-gooey-svg]", "[data-gooey-overlay]"]) {
      const nodes = mountList().querySelectorAll(selector);
      expect(nodes.length, `选择器 ${selector} 未命中任何节点`).toBeGreaterThan(0);
      nodes.forEach((node) => {
        expect(node.hasAttribute("inert")).toBe(true);
      });
    }
  });
});
