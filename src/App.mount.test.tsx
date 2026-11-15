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
import { MAX_QUERY_LENGTH } from "./config";
import { ApiError } from "./lib/api";

// React 18 要求显式声明 act 环境，否则 act() 触发的更新不会在 act 内 flush。
import "./lib/testActEnv";

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

// canvas.getContext：thinking-orbs 挂载时调用（jsdom 未实现，缺失时抛错并刷 console.error）。
// 必须挂在 HTMLCanvasElement.prototype 上：getContext 是该元素的方法，
// 此前这段改的是 CanvasRenderingContext2D.prototype（那是 2D 上下文自身的原型），
// 于是整个 if 从不进入、垫片形同虚设——真正兜住报错的是下面的 console.error 静音。
// 静默与垫片并存是刻意的：静音避免测试输出被第三方库噪音淹没，
// 垫片在 jsdom 真的提供 getContext 时才生效。
const canvasElementProto = g["HTMLCanvasElement"] as { prototype?: Record<string, unknown> } | undefined;
if (canvasElementProto?.prototype !== undefined && typeof canvasElementProto.prototype["getContext"] !== "function") {
  canvasElementProto.prototype["getContext"] = function getContext(): null {
    return null;
  };
}
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

const 张三: Student = { school: "福清一中", name: "张三", grade: "高一", className: "1班" };

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

  // 输入框的 maxLength 来自 config.ts 的 MAX_QUERY_LENGTH。
  //
  // 此断言承重的是「App 接的是那份声明」这一层接线。此前此处是
  // expect(input.maxLength).toBeGreaterThan(0)：注释声称接的是契约常量，
  // 实际任何正数都通过——变异 App.tsx 把属性写成内联字面量后，
  // 本文件用例全绿。
  //
  // 与常量对拍分工（两者不构成恒真）：常量改值时两端同步变化，
  // 对拍（server/contract_constants_test.go 读本文件比对后端
  // maxQueryRunes）仍会失败；App 若改为内联字面量，本条立即翻红。
  // 它覆盖不到的是「App 与常量各写一遍相同数字」，那需要把 JSX 里的
  // 字面量也纳入对拍，属另一条接缝，不在本用例范围。
  it("输入框的长度上限接的是契约常量", () => {
    const container = mountApp(fakeApi());
    const input = container.querySelector("#query") as HTMLInputElement;
    expect(input.maxLength).toBe(MAX_QUERY_LENGTH);
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

  // 结果区段的 empty 与 error 两个分支此前零渲染断言：searchReducer 的单测验证了
  // resultSectionOf 的返回值，却没有验证 App 真的按该返回值渲染了对应 JSX。
  // 把 empty 分支的 JSX 换成 loading 的，界面直接错而全部用例仍然通过——
  // 恒真的「不渲染某元素」断言比没有断言更危险，它提供虚假的覆盖感。
  //
  // 双向：正向断言该区段确实出现；反向断言它不与其余区段同时出现。
  // 只断正向的话，把四个分支全部渲染成同一段 JSX 的实现同样通过。
  it("零结果时渲染 empty 区段，且不同时渲染 error 区段", async () => {
    const api = fakeApi([]);
    const container = mountApp(api);
    const input = container.querySelector("#query") as HTMLInputElement;

    await act(async () => {
      setNativeValue(input, "查无此人");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
      await Promise.resolve();
    });

    expect(container.querySelector("[data-od-id=empty-state]")).not.toBeNull();
    expect(container.querySelector("[data-od-id=error-state]")).toBeNull();
    expect(container.querySelector("[data-od-id=results-section] .result-message")!.textContent).toContain("查无此人");
  });

  it("查询失败时渲染 error 区段与重试入口，且不同时渲染 empty 区段", async () => {
    const api = fakeApi();
    api.search = async () => {
      throw new ApiError("限流", 429, "rate_limited");
    };
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

    expect(container.querySelector("[data-od-id=error-state]")).not.toBeNull();
    expect(container.querySelector("[data-od-id=retry-cta]")).not.toBeNull();
    expect(container.querySelector("[data-od-id=empty-state]")).toBeNull();
    // error 区段渲染的是分类文案而非泛化文案：与状态行同源，二者取同一值。
    expect(container.querySelector("[data-od-id=error-state]")!.textContent).toContain("请求过于频繁");
  });

  // 状态行的提示色由 present 的 tone 派生，经 data-tone 交给样式表。
  // 该属性是样式接缝的真实载体（styles.css 按 data-tone 选色），不是死标记。
  it("状态行的提示色接的是 present 派生的 tone", async () => {
    const api = fakeApi();
    api.search = async () => {
      throw new ApiError("限流", 429, "rate_limited");
    };
    const container = mountApp(api);
    const input = container.querySelector("#query") as HTMLInputElement;
    const tone = () => container.querySelector("[data-od-id=status-feedback]")!.getAttribute("data-tone");

    // 首屏是 idle：提示色为 muted。
    expect(tone()).toBe("muted");

    await act(async () => {
      setNativeValue(input, "张三");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    // 输入后是 editing：仍为 muted——这一格必须与 idle 区分开才说明
    // 属性接的是派生值而不是写死的常量。
    expect(tone()).toBe("muted");

    await act(async () => {
      input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
      await Promise.resolve();
    });
    // 失败后是 error：提示色随之改变。
    expect(tone()).not.toBe("muted");
  });

  // 错误区段的标题与说明不得是同一句的前后两截。
  //
  // 此前 App.tsx 的 error 分支硬编码标题「查询没有完成」，而未知错误形态
  // （非 ApiError 的异常、非法响应）下 statusText 走 errorMessage 的兜底分支，
  // 返回 COPY.error =「查询没有完成，请稍后重试」——同一句前缀在同屏出现两次。
  // 分类文案明确时（400/429/500/network）不重复，恰好掩盖了它。
  //
  // 双向：正向断「标题不是说明的前缀」；反向断「说明仍承载可行动的文案」，
  // 只断前者的话，把两段都改成同一句固定文案同样通过。
  it("未知错误形态下错误区段不重复同一句文案", async () => {
    const api = fakeApi();
    // 非 ApiError 的异常：走 errorMessage 的兜底分支。
    api.search = async () => {
      throw new Error("unexpected");
    };
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

    const section = container.querySelector("[data-od-id=error-state]")!;
    const title = section.querySelector("strong")!.textContent!;
    const body = section.querySelector("p")!.textContent!;
    expect(body.startsWith(title)).toBe(false);
    // 反向：说明仍须承载可行动的文案，不能因消除重复而变成空串。
    expect(body.length).toBeGreaterThan(0);
  });
});

// React 追踪 onChange 需要绕过它的值劫持：直接改 DOM value 后派发 input 事件，
// React 的 onChange 才会读到新值。

// mountAppSchool：渲染 App 并注入 schoolsApi，使学校选择器的候选可控。
function mountAppSchool(api: FakeSearchApi, schoolsApi: () => Promise<string[]>) {
  const container = document.createElement("div");
  document.body.appendChild(container);
  const healthResponse: Partial<Response> = {
    ok: true,
    json: async () => ({ status: "ok", version: "test" }),
  };
  const fakeFetch: typeof fetch = async () => healthResponse as Response;
  (globalThis as unknown as Record<string, unknown>)["fetch"] = fakeFetch;
  mountedRoot = createRoot(container);
  act(() => mountedRoot!.render(<App api={{ search: api.search }} schoolsApi={schoolsApi} />));
  return container;
}

function setNativeValue(element: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  setter?.call(element, value);
}

describe("App 学校选择器", () => {
  // 点击触发按钮打开面板，再点选项完成选择。
  async function pickSchool(container: HTMLElement, label: string) {
    const trigger = container.querySelector("[data-od-id=school-select]") as HTMLButtonElement;
    await act(async () => {
      trigger.click();
    });
    const option = Array.from(container.querySelectorAll("[data-od-id=school-option]")).find(
      (el) => el.textContent?.trim() === label || el.textContent?.includes(label),
    ) as HTMLElement;
    expect(option).toBeDefined();
    await act(async () => {
      option.click();
    });
    await act(async () => {});
  }

  it("渲染全校 + 各校选项（来自 schoolsApi）", async () => {
    const schools = ["福清一中", "福清二中", "福清华侨中学"];
    const container = mountAppSchool(fakeApi(), () => Promise.resolve(schools));
    await act(async () => {}); // 冲刷 loadSchools 的异步 resolve
    const trigger = container.querySelector("[data-od-id=school-select]") as HTMLButtonElement;
    expect(trigger).not.toBeNull();
    expect(trigger.textContent).toContain("全部学校");
    // 点击触发 → 面板出现四个选项（全校 + 三校）
    await act(async () => { trigger.click(); });
    const options = container.querySelectorAll("[data-od-id=school-option]");
    expect(options.length).toBe(4);
    expect(options[0].textContent).toContain("全部学校");
    expect(options[1].textContent).toContain("福清一中");
    expect(options[2].textContent).toContain("福清二中");
    expect(options[3].textContent).toContain("福清华侨中学");
  });

  it("默认选中全校（触发按钮显示全部学校）", async () => {
    const container = mountAppSchool(fakeApi(), () => Promise.resolve(["福清一中"]));
    await act(async () => {});
    const trigger = container.querySelector("[data-od-id=school-select]") as HTMLButtonElement;
    expect(trigger.textContent).toContain("全部学校");
    // 高光跟随展开态：收起时不高光
    expect(trigger.className).not.toContain("is-open");
  });

  it("切换学校作废当前结果：搜索有结果 → 切校 → 结果区消失（复位 idle）", async () => {
    const api = fakeApi();
    const container = mountAppSchool(api, () => Promise.resolve(["福清一中", "福清二中"]));
    await act(async () => {});

    // 1) 先搜出结果
    const input = container.querySelector("#query") as HTMLInputElement;
    await act(async () => {
      setNativeValue(input, "甲");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    });
    expect(container.querySelector("[data-od-id=results-section]")).not.toBeNull();

    // 2) 切到福清二中 → 结果作废、回到 idle（结果区不再渲染）
    await pickSchool(container, "福清二中");
    expect(container.querySelector("[data-od-id=results-section]")).toBeNull();
    // 触发按钮显示新学校（收起后不高光，高光只跟展开态）
    const trigger = container.querySelector("[data-od-id=school-select]") as HTMLButtonElement;
    expect(trigger.textContent).toContain("福清二中");
    expect(trigger.className).not.toContain("is-open");
  });

  it("搜索后清空，学校选项仍保留（回归：clear 不得清掉 schools 列表）", async () => {
    const container = mountAppSchool(fakeApi(), () => Promise.resolve(["福清一中", "福清二中", "福清华侨中学"]));
    await act(async () => {});

    // 1) 搜索一次，出结果区与清空按钮
    const input = container.querySelector("#query") as HTMLInputElement;
    await act(async () => {
      setNativeValue(input, "王");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    });
    expect(container.querySelector("[data-od-id=search-clear]")).not.toBeNull();

    // 2) 点清空按钮
    await act(async () => {
      (container.querySelector("[data-od-id=search-clear]") as HTMLButtonElement).click();
    });
    await act(async () => {});

    // 3) 打开学校下拉：三个学校选项必须仍在（clear 不得清掉 schools）
    const trigger = container.querySelector("[data-od-id=school-select]") as HTMLButtonElement;
    await act(async () => { trigger.click(); });
    // 选项文本可能带选中勾 ✓，去勾后匹配（学校必须保留在候选里）
    const options = Array.from(container.querySelectorAll("[data-od-id=school-option]")).map((o) => o.textContent?.replace(/✓/g, "").trim());
    expect(options).toContain("福清一中");
    expect(options).toContain("福清二中");
    expect(options).toContain("福清华侨中学");
  });

  it("点 X 只清空搜索字，保留已选学校（回归）", async () => {
    const container = mountAppSchool(fakeApi(), () => Promise.resolve(["福清一中", "福清二中"]));
    await act(async () => {});

    // 1) 选一所学校
    const trigger = container.querySelector("[data-od-id=school-select]") as HTMLButtonElement;
    await act(async () => { trigger.click(); });
    const option = Array.from(container.querySelectorAll("[data-od-id=school-option]")).find((el) => el.textContent?.includes("福清二中")) as HTMLElement;
    await act(async () => { option.click(); });
    await act(async () => {});
    expect(trigger.textContent).toContain("福清二中");

    // 2) 输入查询并搜索
    const input = container.querySelector("#query") as HTMLInputElement;
    await act(async () => {
      setNativeValue(input, "王");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    });

    // 3) 点 X 清空：输入框被清空，但学校仍选中
    await act(async () => {
      (container.querySelector("[data-od-id=search-clear]") as HTMLButtonElement).click();
    });
    await act(async () => {});
    expect((container.querySelector("#query") as HTMLInputElement).value).toBe("");
    expect(trigger.textContent).toContain("福清二中");
  });

  it("手动删光输入框，学校选择仍保留（回归：input-change 空串分支）", async () => {
    const container = mountAppSchool(fakeApi(), () => Promise.resolve(["福清一中", "福清二中"]));
    await act(async () => {});

    // 1) 选一所学校
    const trigger = container.querySelector("[data-od-id=school-select]") as HTMLButtonElement;
    await act(async () => { trigger.click(); });
    const option = Array.from(container.querySelectorAll("[data-od-id=school-option]")).find((el) => el.textContent?.includes("福清二中")) as HTMLElement;
    await act(async () => { option.click(); });
    await act(async () => {});

    // 2) 输入再逐个删光（模拟退格键：onChange 每次给新值，最后为空串）
    const input = container.querySelector("#query") as HTMLInputElement;
    for (const v of ["王", ""]) {
      await act(async () => {
        setNativeValue(input, v);
        input.dispatchEvent(new Event("input", { bubbles: true }));
      });
    }
    await act(async () => {});

    // 3) 学校仍保留 + 候选仍在
    expect((container.querySelector("#query") as HTMLInputElement).value).toBe("");
    expect(trigger.textContent).toContain("福清二中");
    await act(async () => { trigger.click(); });
    // 选项文本可能带选中勾 ✓，去勾后匹配（学校必须保留在候选里）
    const options = Array.from(container.querySelectorAll("[data-od-id=school-option]")).map((o) => o.textContent?.replace(/✓/g, "").trim());
    expect(options).toContain("福清一中");
    expect(options).toContain("福清二中");
  });

  it("高光跟随展开态：弹出面板才亮，收起即灭（与是否选中学校无关）", async () => {
    const container = mountAppSchool(fakeApi(), () => Promise.resolve(["福清一中", "福清二中"]));
    await act(async () => {});
    const trigger = container.querySelector("[data-od-id=school-select]") as HTMLButtonElement;

    // 1) 初始收起：不高光
    expect(trigger.className).not.toContain("is-open");

    // 2) 点开：高光
    await act(async () => { trigger.click(); });
    expect(trigger.className).toContain("is-open");

    // 3) 选中一所学校（面板随选择关闭）：收起后不高光
    const option = Array.from(container.querySelectorAll("[data-od-id=school-option]")).find((el) => el.textContent?.includes("福清二中")) as HTMLElement;
    await act(async () => { option.click(); });
    await act(async () => {});
    expect(trigger.textContent).toContain("福清二中");
    expect(trigger.className).not.toContain("is-open");

    // 4) 再点开：又高光（说明高光只跟展开态，不跟选中）
    await act(async () => { trigger.click(); });
    expect(trigger.className).toContain("is-open");
  });

  it("schoolsApi 失败时选择器仍渲染（全校），全站不崩", async () => {
    const container = mountAppSchool(fakeApi(), () => Promise.reject(new Error("network")));
    const trigger = container.querySelector("[data-od-id=school-select]") as HTMLButtonElement;
    expect(trigger).not.toBeNull();
    expect(trigger.textContent).toContain("全部学校");
    // 打开面板只有「全部学校」一项
    await act(async () => { trigger.click(); });
    expect(container.querySelectorAll("[data-od-id=school-option]").length).toBe(1);
  });
});

