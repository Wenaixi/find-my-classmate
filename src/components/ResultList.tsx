import { useEffect, useRef } from "react";
import { Liquid } from "liquid-gooey";
import type { Student } from "../types";
import type { ResultSummary } from "../lib/resultSummary";

interface ResultListProps {
  items: Student[];
  summary: ResultSummary;
  hasMore: boolean;
  loadingMore: boolean;
  loadMoreError: boolean;
  onLoadMore: () => void;
}

function ResultCard({ student, index }: { student: Student; index: number }) {
  const isLatin = /^[A-Za-z ]+$/.test(student.name);
  return (
    <div className="result-card" role="listitem" data-od-id={"result-card-" + index}>
      <span className="result-index" aria-hidden="true">{String(index + 1).padStart(2, "0")}</span>
      <div className="result-identity">
        <h3 className={"result-name" + (isLatin ? " is-latin" : "")} data-od-id={"result-name-" + index}>{student.name}</h3>
        <p className="result-label">匹配记录</p>
      </div>
      <div className="result-location" data-od-id={"result-meta-" + index}>
        <span className="result-location-label">年段</span>
        <strong>{student.grade}</strong>
        <span className="result-location-divider" aria-hidden="true" />
        <span className="result-location-label">班级</span>
        <strong>{student.className}</strong>
      </div>
      {/* result-check 是纯装饰视觉标记，aria-label 在无 role 的 span 上违反 aria-prohibited-attr 并造成读屏冗余播报 */}
      <span className="result-check" aria-hidden="true" />
    </div>
  );
}

export default function ResultList({ items, summary, hasMore, loadingMore, loadMoreError, onLoadMore }: ResultListProps) {
  const liquidWrapRef = useRef<HTMLDivElement | null>(null);

  // liquid-gooey 渲染两个装饰 SVG：data-gooey-svg 与 data-gooey-overlay。
  // 二者含幽灵 g 节点，会被 Chrome 捕获进 Tab 序列，落在「搜索」与「继续加载」
  // 之间造成键盘焦点陷落——纯装饰层却能吃掉键盘焦点。SVG 是 aria-hidden 的装饰
  // 层，对其 inert 阻断焦点。
  //
  // 两层都要覆盖：库自己的 MutationObserver 用 closest 匹配
  // "[data-gooey-svg], [data-gooey-overlay]" 把二者当同类，overlay 的 z-index 是
  // 9999、库注释自陈「Above the content layer by design」，内含与 svg 层结构对称的
  // g/defs/mask。此前只选择 svg 层，只挡住一半。
  useEffect(() => {
    const decorative = liquidWrapRef.current?.querySelectorAll("[data-gooey-svg], [data-gooey-overlay]");
    decorative?.forEach((node) => {
      (node as HTMLElement).setAttribute("inert", "");
    });
  }, [items.length]);

  return (
    <>
      <div className="results-toolbar">
        <span>{summary.countLabel}</span>
        <span className="results-toolbar-state">{summary.toolbarState}</span>
      </div>
      <div className="results-liquid" ref={liquidWrapRef}>
        <div className="results-list" role="list" aria-label="查询匹配记录">
          <div className="results-list-head" aria-hidden="true"><span>序号</span><span>姓名</span><span>所属位置</span><span>状态</span></div>
          <Liquid blur={10} contrast={24} fill="rgba(255,255,255,.1)" shadow="0 18px 50px rgba(255,255,255,.06)" className="liquid-result-group">
            {items.map((student, index) => (
              <Liquid.Item
                key={student.name + student.grade + student.className}
                className="liquid-result-item"
                morph={{ shape: true, speed: 0.85, bounce: 0.3, contentBlur: 0 }}
              >
                <ResultCard student={student} index={index} />
              </Liquid.Item>
            ))}
          </Liquid>
        </div>
      </div>
      <div className="load-more-zone" data-od-id="load-more-zone">
        <div className="load-progress" aria-hidden="true"><span style={{ width: summary.progress + "%" }} /></div>
        <div className="load-more-copy"><span>{summary.countLabel}</span><span>{summary.loadMoreLabel}</span></div>
        {hasMore && <button className="load-more-button" data-od-id="load-more-cta" type="button" onClick={onLoadMore} disabled={loadingMore} aria-label={"继续加载，剩余 " + summary.remaining + " 条结果"}><span>{loadingMore ? "正在加载" : "继续加载"}</span><span className="button-arrow" aria-hidden="true">↗</span></button>}
        {loadMoreError && <div className="load-more-error" role="alert">加载失败，请再次点击继续加载。</div>}
      </div>
    </>
  );
}