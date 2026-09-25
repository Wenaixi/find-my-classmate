import { describe, expect, it, vi } from "vitest";
import { createSearchSession } from "./searchSession";
import type { SearchResponse } from "../types";

const okResponse = (over: Partial<SearchResponse> = {}): SearchResponse => ({
  items: [], total: 0, limit: 10, offset: 0, hasMore: false, ...over,
});

describe("createSearchSession", () => {
  it("discards stale responses after a new begin", async () => {
    let resolveFirst!: (r: SearchResponse) => void;
    const search = vi.fn().mockImplementationOnce(() => new Promise<SearchResponse>((r) => (resolveFirst = r)));
    const session = createSearchSession({ search });

    const p1 = session.submit("张三", 10);
    const id1 = 1; // begin() 内部递增
    session.begin(); // 新请求使 id1 过期
    resolveFirst(okResponse());
    expect(await p1).toEqual({ ok: false, reason: "stale" }); // 过期响应显式判别

    void id1; // 避免未使用告警
  });

  it("aborts all in-flight on abortAll", () => {
    const search = vi.fn().mockResolvedValue(okResponse());
    const session = createSearchSession({ search });
    session.begin();
    session.abortAll();
    // 无断言，仅验证不抛异常且未发出新请求
    expect(search).not.toHaveBeenCalled();
  });

  it("lets loadMore follow a submit without invalidating it", async () => {
    const search = vi.fn().mockResolvedValue(okResponse({ items: [{ name: "甲", grade: "高一", className: "1班" }], total: 1, hasMore: false }));
    const session = createSearchSession({ search });
    const id = session.begin();
    const result = await session.loadMore("张三", 10, 0);
    expect(result).toEqual(expect.objectContaining({ ok: true }));
    expect(session.isCurrent(id)).toBe(true);
  });

  it("submit returns undefined when request is superseded", async () => {
    const search = vi.fn().mockResolvedValue(okResponse());
    const session = createSearchSession({ search });
    const first = session.submit("张三", 10);
    const second = session.submit("李四", 10); // 后一次提交取代前一次
    expect(await first).toEqual({ ok: false, reason: "stale" }); // 被取代的响应被丢弃
    expect((await second).ok).toBe(true); // 最新请求正常返回
  });

  it("discards a submit response in flight after invalidate (clear)", async () => {
    let resolveFirst!: (r: SearchResponse) => void;
    const search = vi.fn().mockImplementationOnce(() => new Promise<SearchResponse>((r) => (resolveFirst = r)));
    const session = createSearchSession({ search });

    const inFlight = session.submit("张三", 10);
    session.invalidate(); // clear/输入变化/IME 变化都会调用
    resolveFirst(okResponse());
    expect(await inFlight).toEqual({ ok: false, reason: "stale" }); // 过期响应被丢弃
  });

  it("aborts the real fetch on invalidate", () => {
    let captured!: AbortSignal;
    const search = vi.fn().mockImplementation((_q: string, _l: number, _o: number, signal: AbortSignal) => {
      captured = signal;
      return new Promise<SearchResponse>((_resolve, reject) =>
        signal.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")), { once: true })
      );
    });
    const session = createSearchSession({ search });

    const inFlight = session.submit("张三", 10);
    expect(captured).toBeDefined();
    expect(captured.aborted).toBe(false);
    session.invalidate();
    expect(captured.aborted).toBe(true); // abort 真实触达 api
    return expect(inFlight).resolves.toEqual({ ok: false, reason: "stale" });
  });

  it("aborts the real fetch on abortAll", () => {
    let captured!: AbortSignal;
    const search = vi.fn().mockImplementation((_q: string, _l: number, _o: number, signal: AbortSignal) => {
      captured = signal;
      return new Promise<SearchResponse>((_resolve, reject) =>
        signal.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")), { once: true })
      );
    });
    const session = createSearchSession({ search });

    const inFlight = session.submit("张三", 10);
    session.abortAll();
    return expect(inFlight).resolves.toEqual({ ok: false, reason: "stale" });
  });

  it("aborts a real submit when the caller disconnects", () => {
    let captured!: AbortSignal;
    const search = vi.fn().mockImplementation((_q: string, _l: number, _o: number, signal: AbortSignal) => {
      captured = signal;
      return new Promise<SearchResponse>((_resolve, reject) =>
        signal.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")), { once: true })
      );
    });
    const session = createSearchSession({ search });

    const caller = new AbortController();
    const inFlight = session.submit("张三", 10, caller.signal);
    caller.abort();
    expect(captured.aborted).toBe(true);
    return expect(inFlight).resolves.toEqual({ ok: false, reason: "stale" });
  });

  it("discards a loadMore response in flight after invalidate", async () => {
    let resolveFirst!: (r: SearchResponse) => void;
    const search = vi.fn().mockImplementationOnce(() => new Promise<SearchResponse>((r) => (resolveFirst = r)));
    const session = createSearchSession({ search });

    session.begin(); // 既有会话
    const inFlight = session.loadMore("张三", 10, 0);
    session.invalidate(); // loadMore 在途时失效
    resolveFirst(okResponse());
    expect(await inFlight).toEqual({ ok: false, reason: "stale" }); // 被丢弃
  });

  it("discards an invalidated submit rejection instead of rethrowing", async () => {
    const search = vi.fn().mockImplementation((_q: string, _l: number, _o: number, signal: AbortSignal) =>
      new Promise<SearchResponse>((_resolve, reject) =>
        signal.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")), { once: true })
      )
    );
    const session = createSearchSession({ search });

    const inFlight = session.submit("张三", 10);
    session.invalidate();
    // invalidate 使 fetch 真实中止 → 后端按 AbortError 拒绝 → 不应当被 App 当作业务失败
    await expect(inFlight).resolves.toEqual({ ok: false, reason: "stale" });
  });
});
