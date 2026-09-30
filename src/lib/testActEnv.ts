// 挂载测试的 act 环境声明：React 18 要求显式声明，否则 force() 触发的更新
// 不会在 act 内 flush，测试会读到未刷新的旧快照而产生假阴性。
//
// 收敛到一处的原因不是「四处相同显得冗余」，而是这条纪律本身容易漏：
// 新增挂载测试若忘记声明，用例会在断言阶段报出与被测行为无关的失败，
// 排查成本高。四份文件头注释各写一遍时，漏写的那份不会被任何人发现。
//
// 刻意不做 vitest setupFiles：全局 setup 会让默认 node 环境的纯逻辑测试
// 也带上该标志，使「纯逻辑测试零 DOM 依赖」这条现有纪律失效——
// 那条纪律是有意的（见 vitest.config.ts 的 environment 注释）。
// 本模块由需要它的测试文件显式导入，导入行为本身就是一次显式声明。
const actEnv = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
actEnv.IS_REACT_ACT_ENVIRONMENT = true;
