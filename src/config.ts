// 前端契约常量：数值的唯一事实源。与后端 server/config.go 双写同步
// （80 字上限、limit 默认等跨端契约常量无法共享，两处各一份，docs/ARCHITECTURE.md 已注明）。

/** 首屏分页条数（与后端 defaultLimit 同步） */
export const PAGE_SIZE = 10;

/** 查询输入框最大长度（与后端 maxQueryRunes 同步，rune 计） */
export const MAX_QUERY_LENGTH = 80;

/** 网络请求超时（毫秒） */
export const REQUEST_TIMEOUT_MS = 10_000;
