// 结果摘要单点派生：进度、计数与剩余文案的唯一来源。
// 此前 App.tsx（progress）、ResultList（"X / Y 条"、"已全部加载"、剩余数）
// 各自从同一对 items.length/total 派生，文案碎片散落两组件。
// 归位后组件只接收派生结果，不再自行计算。

export interface ResultSummary {
  /** 已加载条数 */
  loaded: number;
  /** 总命中条数 */
  total: number;
  /** 进度百分比（0-100），total 为 0 时为 0 */
  progress: number;
  /** "显示 X / Y 条记录" */
  countLabel: string;
  /** 工具栏状态：还有更多 → "下方继续加载"，否则 "已全部加载" */
  toolbarState: string;
  /** 加载区文案：还有更多 → "还有 N 条"，否则 "已全部加载" */
  loadMoreLabel: string;
  /** 剩余条数（不小于 0） */
  remaining: number;
}

export function deriveResultSummary(loaded: number, total: number, hasMore: boolean): ResultSummary {
  const remaining = Math.max(0, total - loaded);
  const progress = total > 0 ? (loaded / total) * 100 : 0;
  return {
    loaded,
    total,
    progress,
    countLabel: `显示 ${loaded} / ${total} 条记录`,
    toolbarState: hasMore ? "下方继续加载" : "已全部加载",
    loadMoreLabel: hasMore ? `还有 ${remaining} 条` : "已全部加载",
    remaining,
  };
}
