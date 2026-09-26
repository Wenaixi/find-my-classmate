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

  it("works reliably when AbortSignal.any is undefined (Safari < 17.4 compatibility)", async () => {
    const originalAny = AbortSignal.any;
    try {
      // 模拟不支持 AbortSignal.any 的老旧环境
      (AbortSignal as any).any = undefined;
      const fetchMock = vi.fn().mockResolvedValue({
        ok: true, status: 200,
        json: async () => ({ items: [], total: 0, limit: 10, offset: 0, hasMore: false }),
      });
      vi.stubGlobal("fetch", fetchMock);

      await searchApi("张三", 10, 0);
      const [, opts] = fetchMock.mock.calls[0];
      expect(opts.signal).toBeDefined();
      expect(opts.signal.aborted).toBe(false);
    } finally {
      (AbortSignal as any).any = originalAny;
    }
  });

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
