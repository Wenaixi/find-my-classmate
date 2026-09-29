// 前端契约常量：跨端数值的本端声明。与后端 server/config.go 双写同步
// （80 字上限、limit 默认等跨端契约常量无法共享，两处各一份，docs/ARCHITECTURE.md 已注明）。
// 因此本文件不是「唯一事实源」——两端各有一份，同步义务由
// server/contract_constants_test.go 读本文件比对后端常量来兜底。

/** 首屏分页条数（与后端 defaultLimit 同步） */
export const PAGE_SIZE = 10;

/** 查询输入框最大长度（与后端 maxQueryRunes 同步，rune 计） */
export const MAX_QUERY_LENGTH = 80;

/** 单次请求分页上限（与后端 maxLimit 同步） */
export const MAX_LIMIT = 50;

/** 网络请求超时（毫秒） */
export const REQUEST_TIMEOUT_MS = 10_000;
