// @vitest-environment jsdom
// 交互语义的行为锁：输入法组合期间 Enter 不提交、Escape 清空、组合事件不误作废请求。
//
// 该测试的存在理由：这三条规则此前内联在 App.tsx 的 JSX 事件回调里，
// 没有任何测试能穿过——组件之外的调用方都复制不到这份语义。
// 交互语义收进 useSearchInput 后，它是唯一的 interface，也是唯一的测试面。
import { afterEach, describe, expect, it, vi } from "vitest";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { createElement } from "react";
import { useSearchController, type SearchOrchestrator } from "./useSearchController";
import { useSearchInput } from "./useSearchInput";

// React 18 要求显式声明 act 环境，否则事件触发的更新不会在 act 内被 flush。
const actEnv = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
actEnv.IS_REACT_ACT_ENVIRONMENT = true;

interface Stub {
  controller: SearchOrchestrator;
  submitted: number;
  cleared: number;
  composed: string[];
  invalidateSpy: ReturnType<typeof vi.fn>;
}

function makeStub(isComposing = false): Stub {
  const stub: Partial<Stub> = {
    submitted: 0,
    cleared: 0,
    composed: [],
    invalidateSpy: vi.fn(),
  };
  stub.controller = {
    onInput: vi.fn(),
    submit: vi.fn(async () => {
      stub.submitted = (stub.submitted ?? 0) + 1;
    }),
    clear: vi.fn(() => {
      stub.cleared = (stub.cleared ?? 0) + 1;
    }),
    onCompositionStart: vi.fn(() => stub.composed!.push("start")),
    onCompositionEnd: vi.fn(() => stub.composed!.push("end")),
  } as unknown as SearchOrchestrator;
  return stub as Stub;
}

let mountedRoot: Root | null = null;

afterEach(() => {
  if (mountedRoot) act(() => mountedRoot!.unmount());
  mountedRoot = null;
});

interface Handlers {
  onChange?: (value: string) => void;
  onKeyDown?: (key: string, nativeIsComposing: boolean) => void;
  onCompositionStart?: () => void;
  onCompositionEnd?: () => void;
}

function mountInput(stub: Stub, state: { isComposing: boolean }) {
  const container = document.createElement("div");
  document.body.appendChild(container);
  let handlers: Handlers = {};

  function Harness() {
    const input = useSearchInput(stub.controller, state.isComposing);
    handlers = {
      onChange: input.onChange,
      onKeyDown: input.onKeyDown,
      onCompositionStart: input.onCompositionStart,
      onCompositionEnd: input.onCompositionEnd,
    };
    return null;
  }

  mountedRoot = createRoot(container);
  act(() => mountedRoot!.render(createElement(Harness)));
  return { handlers: () => handlers };
}

describe("useSearchInput", () => {
  it("routes input changes to the controller", () => {
    const stub = makeStub();
    const { handlers } = mountInput(stub, { isComposing: false });
    handlers().onChange!("新值");
    expect(stub.controller.onInput).toHaveBeenCalledWith("新值");
  });

  it("submits on Enter when not composing", () => {
    const stub = makeStub();
    const { handlers } = mountInput(stub, { isComposing: false });
    handlers().onKeyDown!("Enter", false);
    expect(stub.submitted).toBe(1);
  });

  it("does not submit on Enter while an IME composition is active", () => {
    // 组合期间 Enter 是"确认候选词"而非"提交查询"：
    // nativeEvent.isComposing 与 state.isComposing 任一为真都必须拦住提交。
    const nativeComposing = makeStub();
    const h1 = mountInput(nativeComposing, { isComposing: false });
    h1.handlers().onKeyDown!("Enter", true);
    expect(nativeComposing.submitted).toBe(0);

    const stateComposing = makeStub();
    const h2 = mountInput(stateComposing, { isComposing: true });
    h2.handlers().onKeyDown!("Enter", false);
    expect(stateComposing.submitted).toBe(0);
  });

  it("clears on Escape", () => {
    const stub = makeStub();
    const { handlers } = mountInput(stub, { isComposing: false });
    handlers().onKeyDown!("Escape", false);
    expect(stub.cleared).toBe(1);
  });

  it("ignores other keys", () => {
    const stub = makeStub();
    const { handlers } = mountInput(stub, { isComposing: false });
    handlers().onKeyDown!("a", false);
    expect(stub.submitted).toBe(0);
    expect(stub.cleared).toBe(0);
  });

  it("forwards composition lifecycle to the controller", () => {
    const stub = makeStub();
    const { handlers } = mountInput(stub, { isComposing: false });
    handlers().onCompositionStart!();
    expect(stub.composed).toEqual(["start"]);
    handlers().onCompositionEnd!();
    expect(stub.composed).toEqual(["start", "end"]);
  });
});

// 集成：真实把 useSearchController 与 useSearchInput 串起来驱动，
// 锁住「两个 hook 组合后仍能走通 输入 → Enter 提交 → 拿到结果」这条链路。
//
// 覆盖范围要说清楚：它不加载 App.tsx，因此不覆盖页面的 JSX 接线。
// 接线由 TypeScript 在编译期锁住——onKeyDown 少传 isComposing 参数会得到
// TS2554（实测断开即报）。两层职责不要混淆。
describe("useSearchInput 与控制器的接线", () => {
  it("输入后按 Enter 走完整链路并拿到结果", async () => {
    const search = vi.fn().mockResolvedValue({
      items: [{ name: "甲", grade: "高一" as const, className: "1班" }],
      total: 1,
      limit: 10,
      offset: 0,
      hasMore: false,
    });
    const seen: { state: string; items: number }[] = [];
    let handlers: Handlers = {};

    function Harness() {
      const view = useSearchController({ pageSize: 10, api: { search } });
      const input = useSearchInput(view.controller, view.state.isComposing);
      handlers = {
        onChange: input.onChange,
        onKeyDown: input.onKeyDown,
        onCompositionStart: input.onCompositionStart,
        onCompositionEnd: input.onCompositionEnd,
      };
      seen.push({ state: view.state.state, items: view.state.items.length });
      return null;
    }

    const container = document.createElement("div");
    document.body.appendChild(container);
    mountedRoot = createRoot(container);
    act(() => mountedRoot!.render(createElement(Harness)));

    act(() => handlers.onChange!("甲"));
    expect(seen[seen.length - 1].state).toBe("editing");

    await act(async () => {
      handlers.onKeyDown!("Enter", false);
    });

    expect(search).toHaveBeenCalledWith("甲", 10, 0, expect.anything());
    expect(seen[seen.length - 1].items).toBe(1);
  });
});
