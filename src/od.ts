// 测试锚点：源码保留可读 id，生产构建由 vite 的 define 把 __OD_ID__ 折叠为
// false，本函数退化成 `() => ({})`，展开后 JSX 里没有该键，产物中连字符串
// "data-od-id" 都不存在。
//
// 为什么返回对象而不是字符串：`data-od-id={od("x")}` 的形式即使 od 折叠为
// no-op，JSX 的 props 键名 "data-od-id" 仍是源码字面量，压缩器不会消除它
// （实测产物残留 35 处）。`{...od("x")}` 把键名收进函数内部，函数被折叠掉时
// 键名随之消失。
//
// 为什么不直接复用 import.meta.env.DEV：Vite 自身依赖它做开发/生产分支，
// 用 define 覆盖会连带改变其他行为。__OD_ID__ 是只为锚点存在的独立开关。
declare const __OD_ID__: boolean;

export const od = (id: string): Record<string, string> => (__OD_ID__ ? { "data-od-id": id } : {});
