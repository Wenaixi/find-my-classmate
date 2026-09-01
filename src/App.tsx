import { FormEvent, Suspense, lazy, useEffect, useRef, useState } from "react";
import { BorderBeam } from "border-beam";
import { ApiError, searchApi } from "./lib/api";
import { getRemainingSearchDelay } from "./lib/searchTiming";
import type { SearchState, Student } from "./types";
import ErrorBoundary from "./components/ErrorBoundary";

const ResultList = lazy(() => import("./components/ResultList"));
const StatusOrb = lazy(() => import("./components/StatusOrb"));

const PAGE_SIZE = 10;
const COPY: Record<SearchState, string> = {
  idle: "输入姓名、班级或年段后开始查询",
  editing: "支持姓名、班级和年段组合查询",
  loading: "正在检索完整名单",
  success: "已定位 1 位同学",
  duplicate: "已定位多位同学",
  empty: "没有找到匹配记录",
  error: "查询没有完成，请稍后重试",
};

function wait(milliseconds: number) {
  return new Promise<void>((resolve) => window.setTimeout(resolve, milliseconds));
}

function getState(items: Student[], query: string, total = items.length): SearchState {
  if (!query.trim()) return "idle";
  if (items.length === 0 && total === 0) return "empty";
  return total === 1 ? "success" : "duplicate";
}

// F2：按 ApiError 分类给出准确文案（400=输入问题、429=限流、其他=服务问题）
function errorMessage(cause: unknown): string {
  if (cause instanceof ApiError) {
    if (cause.status === 400) return "查询条件有误，请精简到 80 字以内后重试";
    if (cause.status === 429) return "请求过于频繁，请稍候再试";
    if (cause.status === 500) return "名单数据暂时不可用，请稍后重试";
    if (cause.code === "network") return "网络连接异常，请检查后重试";
  }
  return COPY.error;
}

export function App() {
  const [query, setQuery] = useState("");
  const [items, setItems] = useState<Student[]>([]);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [state, setState] = useState<SearchState>("idle");
  const [statusText, setStatusText] = useState(COPY.idle);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loadMoreError, setLoadMoreError] = useState(false);
  const [isComposing, setIsComposing] = useState(false);
  const requestRef = useRef(0);
  const abortRef = useRef<AbortController | null>(null);
  const resultsRef = useRef<HTMLElement | null>(null);
  const searchWrapRef = useRef<HTMLFormElement | null>(null);
  const shouldScrollRef = useRef(false);

  useEffect(() => {
    requestRef.current += 1;
    abortRef.current?.abort();
    setLoadingMore(false);
    if (!query.trim()) {
      setItems([]);
      setTotal(0);
      setHasMore(false);
      setState("idle");
      setStatusText(COPY.idle);
      return;
    }
    // F5：IME 组合期间不切换到 editing，避免每击键打扰读屏与文案抖动
    if (!isComposing) setState("editing");
  }, [query, isComposing]);

  useEffect(() => () => abortRef.current?.abort(), []);

  useEffect(() => {
    if (!shouldScrollRef.current || (state !== "success" && state !== "duplicate" && state !== "empty" && state !== "error")) return;
    shouldScrollRef.current = false;
    const frame = window.requestAnimationFrame(() => resultsRef.current?.scrollIntoView({ behavior: "smooth", block: "start" }));
    return () => window.cancelAnimationFrame(frame);
  }, [state]);

  async function submit(event?: FormEvent) {
    event?.preventDefault();
    const submitted = query.trim();
    if (!submitted || state === "loading" || loadingMore) return;
    const requestId = ++requestRef.current;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    const startedAt = performance.now();
    shouldScrollRef.current = true;
    setState("loading");
    setStatusText(COPY.loading);
    setItems([]);
    setTotal(0);
    setHasMore(false);
    setLoadMoreError(false);
    try {
      const response = await searchApi(submitted, PAGE_SIZE, 0, controller.signal);
      await wait(getRemainingSearchDelay(startedAt, performance.now()));
      if (controller.signal.aborted || requestId !== requestRef.current) return;
      setItems(response.items);
      setTotal(response.total);
      setHasMore(response.hasMore);
      const next = getState(response.items, submitted, response.total);
      setState(next);
      // F36：纯年段/班级查询（无姓名条件）时提示将返回整个年级/班级
      if (next === "duplicate" && response.total >= PAGE_SIZE) {
        setStatusText(COPY[state] + "，先显示前 " + PAGE_SIZE + " 条");
      } else {
        setStatusText(COPY[next]);
      }
    } catch (cause) {
      if (controller.signal.aborted || requestId !== requestRef.current) return;
      await wait(getRemainingSearchDelay(startedAt, performance.now()));
      if (controller.signal.aborted || requestId !== requestRef.current) return;
      setItems([]);
      setState("error");
      setStatusText(errorMessage(cause));
    }
  }

  async function loadMore() {
    const submitted = query.trim();
    if (!submitted || !hasMore || loadingMore || state === "loading") return;
    const requestId = requestRef.current;
    const controller = new AbortController();
    abortRef.current = controller;
    setLoadingMore(true);
    setLoadMoreError(false);
    try {
      const response = await searchApi(submitted, PAGE_SIZE, items.length, controller.signal);
      if (requestId !== requestRef.current) return;
      setItems((current) => [...current, ...response.items]);
      setTotal(response.total);
      setHasMore(response.hasMore);
    } catch (cause) {
      if (!controller.signal.aborted && requestId === requestRef.current) setLoadMoreError(true);
    } finally {
      if (requestId === requestRef.current) setLoadingMore(false);
    }
  }

  function clear() {
    requestRef.current += 1;
    abortRef.current?.abort();
    setQuery("");
    setItems([]);
    setTotal(0);
    setHasMore(false);
    setLoadMoreError(false);
    setLoadingMore(false);
    setState("idle");
    setStatusText(COPY.idle);
  }

  function renderResultBody() {
    if (state === "loading") return <div className="result-loading" role="status"><span>扫描名单索引</span><span className="loading-pulse" aria-hidden="true" /></div>;
    if (state === "success" || state === "duplicate") {
      const progress = total ? (items.length / total) * 100 : 0;
      return <ResultList items={items} total={total} hasMore={hasMore} loadingMore={loadingMore} loadMoreError={loadMoreError} onLoadMore={() => void loadMore()} progress={progress} />;
    }
    if (state === "empty") return <div className="result-message" data-od-id="empty-state"><strong>查无此人</strong><p>换个写法试试。可以只输入姓氏，或补充年段 / 班级缩小范围。</p><button className="text-action" onClick={() => document.getElementById("query")?.focus()}>继续输入 <span aria-hidden="true">↗</span></button></div>;
    if (state === "error") return <div className="result-message" data-od-id="error-state"><strong>查询没有完成</strong><p>{statusText}</p><button className="text-action" data-od-id="retry-cta" onClick={() => void submit()}>重新查询 <span aria-hidden="true">↗</span></button></div>;
    return null;
  }

  const hasResultSection = state !== "idle" && state !== "editing";
  return (
    <div className="app-shell" data-od-id="app-shell">
      <div className="ambient-line ambient-line-one" aria-hidden="true" /><div className="ambient-line ambient-line-two" aria-hidden="true" />
      <header className="topbar" data-od-id="topbar">
        <a className="brand" data-od-id="brand" href="#top"><img className="brand-mark" src="/logo.webp" alt="" width="28" height="28" /><span>FindMyClassmate</span></a>
        <nav className="topbar-meta" data-od-id="top-navigation" aria-label="页面信息"><a data-od-id="privacy-link" href="#privacy">隐私说明</a></nav>
      </header>
      <main id="top">
        <section className="hero" data-od-id="hero">
          <div className="hero-kicker"><p className="eyebrow" data-od-id="hero-eyebrow">校园名单 / 快速定位</p><span className="hero-stamp">FMC—01</span></div>
          <h1 data-od-id="hero-title">找到同学，<br /><em>从名字开始。</em></h1>
          <form className="search-wrap" data-od-id="search-form" ref={searchWrapRef} onSubmit={submit}>
            <label className="field-label" data-od-id="search-label" htmlFor="query">查询条件 <span>NAME / CLASS / GRADE</span></label>
            <BorderBeam size="md" colorVariant="colorful" theme="dark" borderRadius={999} duration={2.2} strength={1} brightness={2} saturation={2.2} hueRange={160}>
              <div className="search-track" data-od-id="search-track">
                <input className="search-input" id="query" type="text" autoComplete="off" spellCheck={false} maxLength={80} value={query} onChange={(event) => setQuery(event.target.value)} onFocus={() => searchWrapRef.current?.scrollIntoView({ behavior: "smooth", block: "start" })} onCompositionStart={() => setIsComposing(true)} onCompositionEnd={() => setIsComposing(false)} onKeyDown={(event) => { if (event.key === "Escape") clear(); if (event.key === "Enter" && !event.nativeEvent.isComposing && !isComposing) void submit(event); }} placeholder="输入姓名 / 班级 / 年段" aria-describedby="search-hint" />
                {query.length > 0 && <button className="search-clear" data-od-id="search-clear" type="button" onClick={clear} aria-label="清空输入"><svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" aria-hidden="true"><path d="M18 6 6 18" /><path d="m6 6 12 12" /></svg></button>}
                <button className="search-send" data-od-id="search-cta" type="submit" disabled={state === "loading"} aria-label={state === "loading" ? "正在检索" : "开始搜索"}>
                  <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="m22 2-7 20-4-9-9-4Z" /><path d="M22 2 11 13" /></svg>
                </button>
              </div>
            </BorderBeam>
            <div className="search-hint" id="search-hint" data-od-id="search-hint"><span>例：张三，18班 · 李四 高二（可用空格或逗号隔开）</span><span>ENTER 查询 / ESC 清空</span></div>
            <div className="status-line" id="status-line" data-od-id="status-feedback" data-state={state} aria-live="polite"><Suspense fallback={null}><StatusOrb state={state} /></Suspense><span>{statusText}</span></div>
          </form>
        </section>
        {hasResultSection && <section className="results-section" ref={resultsRef} data-od-id="results-section" aria-labelledby="results-title" aria-live="polite"><div className="results-head"><div><p className="section-kicker">SEARCH OUTPUT</p><h2 id="results-title" data-od-id="results-title">查询结果</h2></div><div className="result-count-block"><span className="result-count" data-od-id="result-count">{total || "--"}</span><span className="result-count-label">MATCHES</span></div></div><ErrorBoundary><Suspense fallback={<div className="result-loading"><span>正在加载结果组件</span></div>}>{renderResultBody()}</Suspense></ErrorBoundary></section>}
        <section className="privacy-band" id="privacy" data-od-id="privacy-band"><div className="footer-label" data-od-id="privacy-label">隐私边界 / PRIVATE BY DEFAULT</div><p data-od-id="privacy-copy">名单仅用于班级查询。查询内容不写入本地存储，也不会通过页面地址保留。数据处理：福清一中信息社（如需要删除或更正名单中的个人信息，请联系学校教务处）。</p></section>
      </main>
      <footer className="site-footer" data-od-id="site-footer"><div className="footer-stack"><span data-od-id="footer-brand">FINDMYCLASSMATE / ARCHIVE ACCESS</span><span className="footer-fine" data-od-id="footer-source">数据来源：福清一中公示数据提取</span><span className="footer-fine" data-od-id="footer-team">运营团队：福清一中信息社 · 联系方式：学校教务处</span></div><a className="footer-repo" data-od-id="footer-repo" href="https://github.com/Wenaixi/find-my-classmate" target="_blank" rel="noopener noreferrer"><svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12" /></svg>GitHub 仓库 <span aria-hidden="true">↗</span></a></footer>
    </div>
  );
}