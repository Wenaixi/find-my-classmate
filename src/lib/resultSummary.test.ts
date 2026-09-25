import { describe, expect, it } from "vitest";
import { deriveResultSummary } from "./resultSummary";

describe("deriveResultSummary", () => {
  it("derives progress, labels and remaining from the same pair", () => {
    const s = deriveResultSummary(10, 40, true);
    expect(s.progress).toBe(25);
    expect(s.countLabel).toBe("显示 10 / 40 条记录");
    expect(s.toolbarState).toBe("下方继续加载");
    expect(s.loadMoreLabel).toBe("还有 30 条");
    expect(s.remaining).toBe(30);
  });

  it("settles to loaded when nothing remains", () => {
    const s = deriveResultSummary(40, 40, false);
    expect(s.progress).toBe(100);
    expect(s.loadMoreLabel).toBe("已全部加载");
    expect(s.toolbarState).toBe("已全部加载");
    expect(s.remaining).toBe(0);
  });

  it("zero total yields zero progress and empty remaining", () => {
    const s = deriveResultSummary(0, 0, false);
    expect(s.progress).toBe(0);
    expect(s.remaining).toBe(0);
    expect(s.countLabel).toBe("显示 0 / 0 条记录");
  });

  it("clamps remaining at zero when loaded exceeds total", () => {
    const s = deriveResultSummary(12, 10, false);
    expect(s.remaining).toBe(0);
    expect(s.progress).toBe(120);
  });
});
