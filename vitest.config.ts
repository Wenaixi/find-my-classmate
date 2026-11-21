import { defineConfig } from "vitest/config";

export default defineConfig({
  // vitest 读本文件、不读 vite.config.ts，因此测试锚点开关必须在这里再声明
  // 一次。值为 true 意味着测试中 data-od-id 原样出现，测试可按它定位节点；
  // 生产构建的 false 由 vite.config.ts 的 define 承担（mode === "production"）。
  // 两处是同一个常量的两个消费环境，不是两份可漂移的声明：谁改都只改
  // 「测试真、生产假」这一条语义。
  define: { __OD_ID__: JSON.stringify(true) },
  test: {
    // .tsx 必须纳入收集：hook 与组件的回归测试需要真实挂载，此前只收 .ts，
    // 整个前端测试树没有任何位置能挂载 React，hook 壳的行为因此从未被验证。
    include: ["src/**/*.test.{ts,tsx}"],
    // 默认 node：纯逻辑测试不需要 DOM。需要 DOM 的测试在文件头用
    // @vitest-environment jsdom 单独声明，避免纯逻辑测试平白多付环境启动成本。
    environment: "node"
  }
});
