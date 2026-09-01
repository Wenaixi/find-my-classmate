import { describe, expect, it, vi, afterEach } from "vitest";
import { searchApi } from "./api";

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
