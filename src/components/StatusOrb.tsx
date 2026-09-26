import { ThinkingOrb } from "thinking-orbs";

// 加载指示只消费派生布尔，不再认识 SearchState 枚举。
// 此前本组件直比 state !== "loading"，是「查询状态 → 界面」派生通道之外的一条野生支路；
// 变异实验证明让本组件对任何状态都渲染时，全部用例仍然通过（零承重）。
// 判定改由 searchReducer 的 deriveStatusHint 单点持有，组件不再重复这一事实。
export default function StatusOrb({ show }: { show: boolean }) {
  if (!show) return null;
  return (
    <ThinkingOrb
      state="searching"
      size={64}
      theme="dark"
      // paused 参数在库内从不被消费（死参数），移除
    />
  );
}
