import { describe, expect, it, vi, afterEach } from "vitest";
import { ApiError, fetchVersion, searchApi } from "./api";

// expectInvalidResponse 断言解码层拒绝了不合法的 wire 形状。
// 用 instanceof 收窄到 ApiError，既验证错误码，也避免任何类型断言。
async function expectInvalidResponse(promise: Promise<unknown>) {
  const err = await promise.then(() => null, (e: unknown) => e);
  expect(err).toBeInstanceOf(ApiError);
  expect((err as ApiError).code).toBe("invalid-response");
}

afterEach(() => {
  vi.unstubAllGlobals();
});

function mockFetch(status: number, body: unknown) {
  const response = {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as Response;
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response));
}

describe("searchApi error classification", () => {
  it("throws ApiError with status 400 for invalid query", async () => {
    mockFetch(400, { error: "invalid_query" });
    const err = await searchApi("x".repeat(81)).then(() => null, (e) => e);
    expect(err).toBeInstanceOf(Error);
    expect((err as any).status).toBe(400);
    expect((err as any).code).toBe("invalid_query");
  });

  it("throws ApiError with status 429 for rate limit", async () => {
    mockFetch(429, { error: "rate_limited" });
    const err = await searchApi("张三").then(() => null, (e) => e);
    expect((err as any).status).toBe(429);
  });

  it("throws ApiError with status 500 for unavailable", async () => {
    mockFetch(500, { error: "data_unavailable" });
    const err = await searchApi("张三").then(() => null, (e) => e);
    expect((err as any).status).toBe(500);
  });

  it("rejects invalid-response when payload shape is wrong", async () => {
    mockFetch(200, { items: "nope", total: 1, limit: 10, offset: 0, hasMore: false });
    const err = await searchApi("张三").then(() => null, (e) => e);
    expect((err as any).code).toBe("invalid-response");
  });

  it("rejects invalid-response when item misses name", async () => {
    mockFetch(200, { items: [{ grade: "高一", className: "1班" }], total: 1, limit: 10, offset: 0, hasMore: false });
    const err = await searchApi("张三").then(() => null, (e) => e);
    expect((err as any).code).toBe("invalid-response");
  });

  it("accepts payload and maps class field", async () => {
    mockFetch(200, { items: [{ name: "张三", grade: "高一", class: "1班" }], total: 1, limit: 10, offset: 0, hasMore: false });
    const data = await searchApi("张三");
    expect(data.items[0].className).toBe("1班");
  });


  it("rejects invalid-response when class is missing entirely", async () => {
    mockFetch(200, { items: [{ name: "张三", grade: "高一" }], total: 1, limit: 10, offset: 0, hasMore: false });
    await expectInvalidResponse(searchApi("张三"));
  });


  // 年段取值域由后端 knownGrades 唯一保证，前端不复制一份会漂移的清单：
  // 后端扩展年段时前端必须自动跟随，因此这里接受任意非空年段字符串。
  it("accepts a grade outside the currently known set", async () => {
    mockFetch(200, { items: [{ name: "张三", grade: "高四", class: "1班" }], total: 1, limit: 10, offset: 0, hasMore: false });
    const data = await searchApi("张三");
    expect(data.items[0].grade).toBe("高四");
  });

  it("rejects invalid-response when grade is missing or empty", async () => {
    mockFetch(200, { items: [{ name: "张三", class: "1班" }], total: 1, limit: 10, offset: 0, hasMore: false });
    await expectInvalidResponse(searchApi("张三"));

    mockFetch(200, { items: [{ name: "张三", grade: "", class: "1班" }], total: 1, limit: 10, offset: 0, hasMore: false });
    await expectInvalidResponse(searchApi("张三"));
  });

  it("passes AbortSignal.timeout to fetch", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true, status: 200,
      json: async () => ({ items: [], total: 0, limit: 10, offset: 0, hasMore: false }),
    });
    vi.stubGlobal("fetch", fetchMock);
    await searchApi("张三");
    const [, opts] = fetchMock.mock.calls[0];
    expect(opts.signal).toBeDefined();
  });

  // 原「Safari < 17.4」用例在此：它调用 searchApi，而 searchApi 裸用
  // AbortSignal.timeout、不经过 combineSignals，把 AbortSignal.any 打成
  // undefined 对它毫无影响——降级分支从未被触及。真实覆盖见下方
  // fetchVersion 组合信号的 describe。

});

describe("searchApi request", () => {
  it("builds URL with q/limit/offset", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true, status: 200,
      json: async () => ({ items: [], total: 0, limit: 10, offset: 0, hasMore: false }),
    });
    vi.stubGlobal("fetch", fetchMock);
    await searchApi("张 三", 5, 10);
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain("/api/search?");
    expect(url).toContain("q=");
    expect(url).toContain("limit=5");
    expect(url).toContain("offset=10");
  });
});

describe("fetchVersion", () => {
  it("returns version from a healthy payload", async () => {
    mockFetch(200, { status: "ok", version: "v0.5.4" });
    expect(await fetchVersion()).toBe("v0.5.4");
  });

  it("returns version even from a degraded payload (503 still carries version)", async () => {
    mockFetch(503, { status: "degraded", reason: "data", version: "v0.5.4" });
    expect(await fetchVersion()).toBe("v0.5.4");
  });

  it("throws invalid-response when version is missing", async () => {
    mockFetch(200, { status: "ok" });
    const err = await fetchVersion().then(() => null, (e) => e);
    expect((err as any).code).toBe("invalid-response");
  });

  it("throws a network error when fetch rejects", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("fetch failed")));
    const err = await fetchVersion().then(() => null, (e) => e);
    expect((err as any).code).toBe("network");
  });
});

// 组合信号的降级路径（api.ts combineSignals 的手工桥接）是 fetchVersion 独有的
// 生产依赖：searchApi 裸用 AbortSignal.timeout，不经过组合器。
// 此前唯一的「Safari < 17.4」用例打的是 searchApi，因此降级分支零覆盖——
// 整段删掉它，测试依然全绿。下面按可观察行为断言：调用方取消时，
// 无论 AbortSignal.any 是否存在，传给 fetch 的信号都必须真的中止。
describe("fetchVersion 组合信号（AbortSignal.any 降级）", () => {
  const healthOk = { status: "ok", version: "v1.2.3" };

  // 挂起 fetch：让测试在请求进行中触发 abort，观察信号是否被中止。
  function stubPendingFetch() {
    const fetchMock = vi.fn((_url: string, opts: { signal?: AbortSignal }) => {
      // 手工 resolver 而非 Promise.withResolvers：后者需 es2024 lib，
      // 本项目 target 尚未到该版本，为一个测试夹具抬高编译目标不值得。
      let rejectFetch: (reason: unknown) => void = () => {};
      const promise = new Promise<Response>((_resolve, reject) => (rejectFetch = reject));
      opts.signal?.addEventListener("abort", () => rejectFetch(new DOMException("aborted", "AbortError")));
      return promise;
    });
    vi.stubGlobal("fetch", fetchMock);
    return fetchMock;
  }

  it("AbortSignal.any 缺失时，调用方 abort 必须中止组合信号", async () => {
    const originalAny = AbortSignal.any;
    try {
      (AbortSignal as { any?: unknown }).any = undefined;
      const fetchMock = stubPendingFetch();

      const caller = new AbortController();
      const pending = fetchVersion(caller.signal);
      caller.abort();

      await expect(pending).rejects.toBeDefined();
      const signal = fetchMock.mock.calls[0][1].signal as AbortSignal;
      expect(signal.aborted).toBe(true);
    } finally {
      (AbortSignal as { any?: unknown }).any = originalAny;
    }
  });

  it("AbortSignal.any 存在时，调用方 abort 同样必须中止组合信号", async () => {
    const fetchMock = stubPendingFetch();
    const caller = new AbortController();
    const pending = fetchVersion(caller.signal);
    caller.abort();

    await expect(pending).rejects.toBeDefined();
    const signal = fetchMock.mock.calls[0][1].signal as AbortSignal;
    expect(signal.aborted).toBe(true);
  });

  it("AbortSignal.any 缺失且信号已预先中止时，组合信号应立即处于中止态", async () => {
    const originalAny = AbortSignal.any;
    try {
      (AbortSignal as { any?: unknown }).any = undefined;
      const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => healthOk });
      vi.stubGlobal("fetch", fetchMock);

      const caller = new AbortController();
      caller.abort();
      await fetchVersion(caller.signal).catch(() => {});

      // 预先中止的调用方信号必须让组合信号一出生就是中止态（api.ts 的 a.aborted || b.aborted 分支）。
      const signal = fetchMock.mock.calls[0][1].signal as AbortSignal;
      expect(signal.aborted).toBe(true);
    } finally {
      (AbortSignal as { any?: unknown }).any = originalAny;
    }
  });

  it("AbortSignal.any 缺失时，timeout 侧触发也必须中止组合信号", async () => {
    const originalAny = AbortSignal.any;
    const originalTimeout = AbortSignal.timeout;
    // 持有 timeout 侧的 controller，在断言前同步触发其中止：
    // 用真实定时器会绑定时长、掩盖竞态，而这里要测的只是 b 的监听是否被接上。
    const timeoutSide = new AbortController();
    try {
      (AbortSignal as { any?: unknown }).any = undefined;
      (AbortSignal as { timeout: (ms: number) => AbortSignal }).timeout = () => timeoutSide.signal;
      const fetchMock = stubPendingFetch();

      // 必须同时提供调用方 signal：否则 combineSignals 在 !a 处直接返回 b，
      // 组合器根本不参与，测到的就不是降级分支。
      const caller = new AbortController();
      const pending = fetchVersion(caller.signal);
      // 单独触发 timeout 侧（b），调用方侧（a）保持未中止。
      timeoutSide.abort();

      await pending.then(() => {}, () => {});
      const signal = fetchMock.mock.calls[0][1].signal as AbortSignal;
      expect(signal.aborted).toBe(true);
    } finally {
      (AbortSignal as { any?: unknown }).any = originalAny;
      (AbortSignal as { timeout: (ms: number) => AbortSignal }).timeout = originalTimeout;
    }
  });
});
