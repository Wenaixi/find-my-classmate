import { ThinkingOrb } from "thinking-orbs";
import type { SearchState } from "../types";

export default function StatusOrb({ state }: { state: SearchState }) {
  if (state !== "loading") return null;
  return (
    <ThinkingOrb
      state="searching"
      size={64}
      theme="dark"
      paused={false}
      aria-label="正在检索名单"
    />
  );
}
