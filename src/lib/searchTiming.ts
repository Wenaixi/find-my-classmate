export const MIN_SEARCH_DURATION = 1000;

export function getRemainingSearchDelay(startedAt: number, now: number, minimum = MIN_SEARCH_DURATION): number {
  return Math.max(0, Math.ceil(startedAt + minimum - now));
}
