import { useCallback } from "react";
import type { SearchOrchestrator } from "./useSearchController";

// 交互语义收集：输入框的键盘与输入法行为从组件 JSX 收进这一处。
//
// 此前这几条规则内联在 App.tsx 的 onKeyDown/onCompositionStart/End 回调里，
// 没有任何测试能穿过——它们是纯组件私有逻辑，复制不到也测不到。
// 收进本模块后，它们有了唯一的 interface 与唯一的测试面：
// 组件只透传回调，不再手写「组合期间 Enter 不提交」这类判断。
//
// 组合状态取自两个来源，任一为真都不得提交——组合期间的 Enter 是
// 「确认候选词」而不是「提交查询」：
// 1. nativeIsComposing：浏览器本次按键确实处于组合中（onKeyDown 事件自带）；
// 2. isComposing：控制器记录的组合状态，由 onCompositionStart/End 同步。
// 变异验证：分别移除任一守卫都会让测试翻红，两条都承重。

export interface SearchInputHandlers {
  onChange(value: string): void;
  onKeyDown(key: string, nativeIsComposing: boolean): void;
  onCompositionStart(): void;
  onCompositionEnd(): void;
}

export function useSearchInput(controller: SearchOrchestrator, isComposing: boolean): SearchInputHandlers {
  const onChange = useCallback((value: string) => controller.onInput(value), [controller]);

  const onCompositionStart = useCallback(() => controller.onCompositionStart(), [controller]);

  const onCompositionEnd = useCallback(() => controller.onCompositionEnd(), [controller]);

  const onKeyDown = useCallback(
    (key: string, nativeIsComposing: boolean) => {
      if (key === "Escape") {
        controller.clear();
        return;
      }
      if (key === "Enter" && !nativeIsComposing && !isComposing) {
        void controller.submit();
      }
    },
    [controller, isComposing],
  );

  return { onChange, onKeyDown, onCompositionStart, onCompositionEnd };
}
