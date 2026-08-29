import { describe, expect, it } from "vitest";
import { getRemainingSearchDelay } from "./searchTiming";

describe("search timing", () => {
  it("keeps loading visible until 500ms", () => {
    expect(getRemainingSearchDelay(1000, 1250)).toBe(250);
    expect(getRemainingSearchDelay(1000, 1500)).toBe(0);
    expect(getRemainingSearchDelay(1000, 1800)).toBe(0);
  });

  it("keeps the full window when the clock moves backwards", () => {
    expect(getRemainingSearchDelay(1000, 900)).toBe(600);
  });

  it("allows a custom minimum for deterministic callers", () => {
    expect(getRemainingSearchDelay(100, 225, 200)).toBe(75);
  });
});
