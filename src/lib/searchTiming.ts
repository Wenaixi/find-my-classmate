export const MIN_SEARCH_DURATION = 1000;

export function getRemainingSearchDelay(startedAt: number, now: number, minimum = MIN_SEARCH_DURATION): number {
  // F12：NaN 防御——非法时钟输入时返回完整最小窗口，避免击穿 1000ms 承诺
  if (!Number.isFinite(startedAt) || !Number.isFinite(now) || !Number.isFinite(minimum)) {
    return minimum;
  }
  return Math.max(0, Math.ceil(startedAt + minimum - now));
}
