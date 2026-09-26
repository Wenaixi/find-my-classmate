import { ThinkingOrb } from "thinking-orbs";
import type { SearchState } from "../types";

export default function StatusOrb({ state }: { state: SearchState }) {
  if (state !== "loading") return null;
  return (
    <ThinkingOrb
      state="searching"
      size={64}
      theme="dark"
      // paused 参数在库内从不被消费（死参数），移除
    />
  );
}
