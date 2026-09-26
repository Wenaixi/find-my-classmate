import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    // .tsx 必须纳入收集：hook 与组件的回归测试需要真实挂载，此前只收 .ts，
    // 整个前端测试树没有任何位置能挂载 React，hook 壳的行为因此从未被验证。
    include: ["src/**/*.test.{ts,tsx}"],
    // 默认 node：纯逻辑测试不需要 DOM。需要 DOM 的测试在文件头用
    // @vitest-environment jsdom 单独声明，避免纯逻辑测试平白多付环境启动成本。
    environment: "node"
  }
});
