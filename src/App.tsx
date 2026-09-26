import { FormEvent, Suspense, lazy, useEffect, useRef, useState } from "react";
import { BorderBeam } from "border-beam";
import { fetchVersion, searchApi } from "./lib/api";
import { useSearchController } from "./lib/useSearchController";
import { useSearchInput } from "./lib/useSearchInput";
import ErrorBoundary from "./components/ErrorBoundary";
import siteConfig from "./site.config";
import { MAX_QUERY_LENGTH, PAGE_SIZE } from "./config";
import { deriveResultSummary } from "./lib/resultSummary";
import { resultSectionOf, shouldScrollToResults } from "./lib/searchReducer";

const ResultList = lazy(() => import("./components/ResultList"));
const StatusOrb = lazy(() => import("./components/StatusOrb"));

export function App() {
  // 查询会话深模块：state（渲染派生）+ controller（意图动作）两面消费，
  // 编排（守卫/判别联合/竞态）全部收进模块，组件不再理解 reducer/session。
  const { state, controller } = useSearchController({ pageSize: PAGE_SIZE, api: { search: searchApi } });
  const { query, items, total, hasMore, state: uiState, statusText, loadingMore, loadMoreError, isComposing } = state;
  // 输入交互语义（IME 组合守卫、Enter 提交、Escape 清空）由 useSearchInput 统一收口：
  // 组件只透传回调，不再手写组合判断——该逻辑此前内联在此处且零测试覆盖。
  const inputHandlers = useSearchInput(controller, isComposing);
  const [version, setVersion] = useState("");
  const resultsRef = useRef<HTMLElement | null>(null);
  const searchWrapRef = useRef<HTMLFormElement | null>(null);
  const shouldScrollRef = useRef(false);

  // 卸载时中止在途请求（竞态编排归属 useSearchController 内部 session）
  useEffect(() => () => controller.abortAll(), []);

  // 页脚版本号：读 /api/health（ldflags 注入的运行时版本），失败静默隐藏。
  useEffect(() => {
    const controller = new AbortController();
    fetchVersion(controller.signal).then(setVersion).catch(() => {});
    return () => controller.abort();
  }, []);

  useEffect(() => {
    // 滚动时机由 shouldScrollToResults 单点判定（与结果区显隐同源）。
    if (!shouldScrollRef.current || !shouldScrollToResults(uiState)) return;
    shouldScrollRef.current = false;
    const frame = window.requestAnimationFrame(() => resultsRef.current?.scrollIntoView({ behavior: "smooth", block: "start" }));
    return () => window.cancelAnimationFrame(frame);
  }, [uiState]);

  async function submit(event?: FormEvent) {
    event?.preventDefault();
    shouldScrollRef.current = true;
    // 守卫/判别联合/竞态同步全部由深模块内部完成，组件只表达意图
    await controller.submit();
  }

  async function loadMore() {
    // 同上：守卫（!hasMore/loadingMore/loading）与结果分派都在模块内
    await controller.loadMore();
  }

  function clear() {
    // invalidate + 清空由模块内 session 与 reducer 协同
    controller.clear();
  }


  function renderResultBody() {
    // 状态到区段的映射由 resultSectionOf 单点持有，组件只按返回值选择 JSX。
    // 此前四个 if 依次判定内联在此处，与滚动、显隐构成同一知识的四份判断，
    // 且零测试覆盖（改坏 duplicate 分支后 121 条用例全绿）。
    switch (resultSectionOf(uiState)) {
      case "loading":
        return <div className="result-loading" role="status"><span>扫描名单索引</span><span className="loading-pulse" aria-hidden="true" /></div>;
      case "list": {
        const summary = deriveResultSummary(items.length, total, hasMore);
        return <ResultList items={items} summary={summary} hasMore={hasMore} loadingMore={loadingMore} loadMoreError={loadMoreError} onLoadMore={() => void loadMore()} />;
      }
      case "empty":
        return <div className="result-message" data-od-id="empty-state"><strong>查无此人</strong><p>换个写法试试。可以只输入姓氏，或补充年段 / 班级缩小范围。</p><button className="text-action" onClick={() => document.getElementById("query")?.focus()}>继续输入 <span aria-hidden="true">↗</span></button></div>;
      case "error":
        return <div className="result-message" data-od-id="error-state"><strong>查询没有完成</strong><p>{statusText}</p><button className="text-action" data-od-id="retry-cta" onClick={() => void submit()}>重新查询 <span aria-hidden="true">↗</span></button></div>;
      default:
        return null;
    }
  }

  const hasResultSection = resultSectionOf(uiState) !== null;
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
                <input className="search-input" id="query" type="text" autoComplete="off" spellCheck={false} maxLength={MAX_QUERY_LENGTH} value={query} onChange={(event) => inputHandlers.onChange(event.target.value)} onFocus={() => searchWrapRef.current?.scrollIntoView({ behavior: "smooth", block: "start" })} onCompositionStart={inputHandlers.onCompositionStart} onCompositionEnd={inputHandlers.onCompositionEnd} onKeyDown={(event) => inputHandlers.onKeyDown(event.key, event.nativeEvent.isComposing)} placeholder="输入姓名 / 班级 / 年段" aria-label="查询条件" aria-describedby="search-hint" />
                {query.length > 0 && <button className="search-clear" data-od-id="search-clear" type="button" onClick={clear} aria-label="清空输入"><svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" aria-hidden="true"><path d="M18 6 6 18" /><path d="m6 6 12 12" /></svg></button>}
                <button className="search-send" data-od-id="search-cta" type="submit" disabled={uiState === "loading"} aria-label={uiState === "loading" ? "正在检索" : "开始搜索"}>
                  <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="m22 2-7 20-4-9-9-4Z" /><path d="M22 2 11 13" /></svg>
                </button>
              </div>
            </BorderBeam>
            <div className="search-hint" id="search-hint" data-od-id="search-hint"><span>例：张三，18班 · 李四 高二（可用空格或逗号隔开）</span><span>ENTER 查询 / ESC 清空</span></div>
            <div className="status-line" id="status-line" data-od-id="status-feedback" data-state={uiState} aria-live="polite"><Suspense fallback={null}><StatusOrb state={uiState} /></Suspense><span>{statusText}</span></div>
          </form>
        </section>
        {hasResultSection && <section className="results-section" ref={resultsRef} data-od-id="results-section" aria-labelledby="results-title" aria-live="polite"><div className="results-head"><div><p className="section-kicker">SEARCH OUTPUT</p><h2 id="results-title" data-od-id="results-title">查询结果</h2></div><div className="result-count-block"><span className="result-count" data-od-id="result-count">{total || "--"}</span><span className="result-count-label">MATCHES</span></div></div><ErrorBoundary><Suspense fallback={<div className="result-loading"><span>正在加载结果组件</span></div>}>{renderResultBody()}</Suspense></ErrorBoundary></section>}
        <section className="privacy-band" id="privacy" data-od-id="privacy-band"><div className="footer-label" data-od-id="privacy-label">隐私边界 / PRIVATE BY DEFAULT</div><p data-od-id="privacy-copy">名单仅用于班级查询。查询内容不写入本地存储，也不会通过页面地址保留。{siteConfig.dataController ? `数据处理：${siteConfig.dataController}。` : ""}</p></section>
      </main>
      <footer className="site-footer" data-od-id="site-footer"><div className="footer-stack"><span data-od-id="footer-brand">FINDMYCLASSMATE / ARCHIVE ACCESS</span><span className="footer-fine" data-od-id="footer-source">数据来源：{siteConfig.dataSource}</span><span className="footer-fine" data-od-id="footer-team">运营团队：{siteConfig.team}</span>{version && <span className="footer-fine footer-version" data-od-id="footer-version">版本 {version}</span>}</div><a className="footer-repo" data-od-id="footer-repo" href="https://github.com/Wenaixi/find-my-classmate" target="_blank" rel="noopener noreferrer"><svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12" /></svg>GitHub 仓库 <span aria-hidden="true">↗</span></a></footer>
    </div>
  );
}