import { useEffect, useRef, useState } from "react";
import { od } from "../od";

// 学校选择器：胶囊触发按钮 + 浮层选项面板。
//
// 弃用原生 <select> 的原因：原生下拉的展开层由浏览器绘制，无法贴合本项目的
// 深色设计语言（surface 分层、mono 眉标、--ease-move 动效），且各浏览器渲染
// 差异明显（Safari/Chrome/Firefox 的弹层样式、圆角、滚动条各不相同）。
// 自绘面板让「触发态」与「展开态」都在同一套 token 下，跨浏览器一致。
//
// 交互契约（与 useSearchController.selectSchool 对齐）：
//   - 选中即回调 onChange，由上层作废结果并回到 idle；
//   - 空串表示「全部学校」。
//
// 无障碍：触发按钮 aria-haspopup=listbox + aria-expanded；面板 role=listbox；
// 选项 role=option + aria-selected。键盘：↑↓ 移动高亮、Enter/Space 选中、
// Esc 关闭、Tab 离开即关闭；点击外部关闭。

interface SchoolPickerProps {
  /** 候选学校名（不含「全部学校」项，由组件自行补） */
  schools: string[];
  /** 当前选中值，空串表示全部学校 */
  value: string;
  /** 选中回调（值可能是空串 = 全部学校） */
  onChange: (school: string) => void;
}

const ALL_LABEL = "全部学校";

export default function SchoolPicker({ schools, value, onChange }: SchoolPickerProps) {
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);

  // 选项 = 全部学校 + 各校；空串项排第一。
  const options = ["", ...schools];
  const label = value === "" ? ALL_LABEL : value;
  const selectedIndex = Math.max(0, options.indexOf(value));

  // 打开时把高亮定位到当前选中项
  useEffect(() => {
    if (open) setActive(selectedIndex);
  }, [open, selectedIndex]);

  // 点击外部关闭。用 mousedown 而非 click：避免与选项自身的 click 抢顺序。
  useEffect(() => {
    if (!open) return;
    const onDown = (event: MouseEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  const commit = (index: number) => {
    const next = options[index] ?? "";
    setOpen(false);
    triggerRef.current?.focus();
    if (next !== value) onChange(next);
  };

  const onTriggerKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>) => {
    if (event.key === "ArrowDown" || event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      setOpen(true);
      return;
    }
    if (event.key === "Escape") setOpen(false);
  };

  const onPanelKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      setOpen(false);
      triggerRef.current?.focus();
      return;
    }
    if (event.key === "Tab") {
      setOpen(false);
      return;
    }
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActive((i) => (i + 1) % options.length);
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      setActive((i) => (i - 1 + options.length) % options.length);
      return;
    }
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      commit(active);
    }
  };

  return (
    <div className="school-row" {...od("school-row")}>
      <span className="school-row-label" id="school-label">学校</span>
      <div className="school-picker" ref={rootRef} {...od("school-picker")}>
        <button
          type="button"
          ref={triggerRef}
          // 高光跟随展开态（is-open）：点击弹出面板时亮、收回即灭；
          // 不再随选中状态（选中学校不等于高光）。
          className={"school-trigger" + (open ? " is-open" : "")}
          {...od("school-select")}
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-labelledby="school-label school-value"
          onClick={() => setOpen((v) => !v)}
          onKeyDown={onTriggerKeyDown}
        >
          <span className="school-value" id="school-value">{label}</span>
          <svg className="school-chevron" viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg>
        </button>

        {open && (
          <div
            className="school-panel"
            role="listbox"
            aria-labelledby="school-label"
            {...od("school-panel")}
            tabIndex={-1}
            onKeyDown={onPanelKeyDown}
          >
            {options.map((option, index) => {
              const text = option === "" ? ALL_LABEL : option;
              const isSelected = option === value;
              const isActive = index === active;
              return (
                <div
                  key={option === "" ? "__all__" : option}
                  role="option"
                  aria-selected={isSelected}
                  {...od("school-option")}
                  className={"school-option" + (isSelected ? " is-selected" : "") + (isActive ? " is-active" : "")}
                  onMouseEnter={() => setActive(index)}
                  onClick={() => commit(index)}
                >
                  <span className="school-option-text">{text}</span>
                  {isSelected && <span className="school-option-check" aria-hidden="true">✓</span>}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}