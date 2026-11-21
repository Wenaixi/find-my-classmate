// @vitest-environment jsdom
// SchoolPicker 的键盘契约。
//
// 缺陷形状：面板 onKeyDown 挂在 tabIndex={-1} 的面板上，而焦点从未进入面板——
// 所有按键落在触发按钮的 onKeyDown，它只开关面板，不移动高亮也不提交。
// 键盘用户因此选不中任何学校（WCAG 2.1.1 键盘可达性失败）。
//
// 测试必须只对**触发按钮**派发按键，并且在派的每个按键上附带当前焦点。
// 直接 press(panel, …) 会绕过焦点问题：那条路径在缺陷版本里也「通过」，
// 断言因此不承重（实测：只测 panel 时 5 条里有 4 条假绿）。
//
// 每条断言都必须在移除面板聚焦后翻红（变异验证）。
import { afterEach, describe, expect, it } from "vitest";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import SchoolPicker from "./SchoolPicker";

let root: Root | null = null;
afterEach(() => {
  if (root) act(() => root!.unmount());
  root = null;
  document.body.innerHTML = "";
});

function mount(value: string, onChange: (s: string) => void = () => {}) {
  const container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => {
    root!.render(<SchoolPicker schools={["福清一中", "福清二中"]} value={value} onChange={onChange} />);
  });
  return container;
}

// 模拟真实键盘：按键派发到当前焦点元素。焦点不在任何元素时退回触发按钮，
// 这正是缺陷版本的实际形态（焦点永远停在触发按钮）。
function pressKey(key: string, fallback: Element) {
  const target = document.activeElement ?? fallback;
  act(() => {
    target.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true }));
  });
}

describe("SchoolPicker 键盘可达性", () => {
  it("打开后焦点进入面板", () => {
    const container = mount("");
    const trigger = container.querySelector(".school-trigger") as HTMLButtonElement;
    act(() => {
      trigger.focus();
      trigger.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }));
    });
    expect(container.querySelector(".school-panel")).not.toBeNull();
    expect(document.activeElement).toBe(container.querySelector(".school-panel"));
  });

  it("↓ 移动高亮，Enter 选中当前项（全程只按触发按钮发起按键）", () => {
    const picked: string[] = [];
    const container = mount("", (s) => picked.push(s));
    const trigger = container.querySelector(".school-trigger") as HTMLButtonElement;
    act(() => {
      trigger.focus();
      trigger.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }));
    });
    pressKey("ArrowDown", trigger);
    pressKey("Enter", trigger);
    expect(picked).toEqual(["福清一中"]);
  });

  it("↑ 从首项回绕到末项", () => {
    const picked: string[] = [];
    const container = mount("", (s) => picked.push(s));
    const trigger = container.querySelector(".school-trigger") as HTMLButtonElement;
    act(() => {
      trigger.focus();
      trigger.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }));
    });
    pressKey("ArrowUp", trigger);
    pressKey("Enter", trigger);
    expect(picked).toEqual(["福清二中"]);
  });

  it("Escape 关闭并把焦点还给触发按钮", () => {
    const container = mount("");
    const trigger = container.querySelector(".school-trigger") as HTMLButtonElement;
    act(() => {
      trigger.focus();
      trigger.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }));
    });
    pressKey("Escape", trigger);
    expect(container.querySelector(".school-panel")).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it("高亮定位到当前选中项", () => {
    const container = mount("福清二中");
    const trigger = container.querySelector(".school-trigger") as HTMLButtonElement;
    act(() => {
      trigger.focus();
      trigger.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }));
    });
    const active = container.querySelector(".school-option.is-active");
    expect(active?.textContent).toContain("福清二中");
  });
});
