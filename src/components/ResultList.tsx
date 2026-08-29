import { Liquid } from "liquid-gooey";
import type { Student } from "../types";

interface ResultListProps {
  items: Student[];
  total: number;
  hasMore: boolean;
  loadingMore: boolean;
  loadMoreError: boolean;
  onLoadMore: () => void;
  progress: number;
}

function ResultCard({ student, index }: { student: Student; index: number }) {
  const isLatin = /^[A-Za-z ]+$/.test(student.name);
  return (
    <div className={"result-card" + (index >= 10 ? " no-anim" : "")} role="listitem" data-od-id={"result-card-" + index}>
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
      <span className="result-check" aria-label="已确认匹配" />
    </div>
  );
}

export default function ResultList({ items, total, hasMore, loadingMore, loadMoreError, onLoadMore, progress }: ResultListProps) {
  return (
    <>
      <div className="results-toolbar">
        <span>显示 {items.length} / {total} 条记录</span>
        <span className="results-toolbar-state">{hasMore ? "下方继续加载" : "已全部加载"}</span>
      </div>
      <div className="results-liquid">
        <div className="results-list" role="list" aria-label="查询匹配记录">
          <div className="results-list-head" aria-hidden="true"><span>序号</span><span>姓名</span><span>所属位置</span><span>状态</span></div>
          <Liquid blur={10} contrast={24} fill="rgba(255,255,255,.1)" shadow="0 18px 50px rgba(255,255,255,.06)" className="liquid-result-group">
            {items.map((student, index) => (
              <Liquid.Item key={student.name + student.grade + student.className} className="liquid-result-item" {...(index < 10 ? { morph: { shape: true, speed: 0.85, bounce: 0.3, contentBlur: 0 } } : {})}>
                <ResultCard student={student} index={index} />
              </Liquid.Item>
            ))}
          </Liquid>
        </div>
      </div>
      <div className="load-more-zone" data-od-id="load-more-zone">
        <div className="load-progress" aria-hidden="true"><span style={{ width: progress + "%" }} /></div>
        <div className="load-more-copy"><span>{items.length} / {total} 条记录</span><span>{hasMore ? "还有 " + (total - items.length) + " 条" : "已全部加载"}</span></div>
        {hasMore && <button className="load-more-button" data-od-id="load-more-cta" type="button" onClick={onLoadMore} disabled={loadingMore} aria-label={"继续加载，剩余 " + (total - items.length) + " 条结果"}><span>{loadingMore ? "正在加载" : "继续加载"}</span><span className="button-arrow" aria-hidden="true">↗</span></button>}
        {loadMoreError && <div className="load-more-error" role="alert">加载失败，请再次点击继续加载。</div>}
      </div>
    </>
  );
}
