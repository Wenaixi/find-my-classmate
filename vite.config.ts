import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => ({
  // __OD_ID__ 只服务测试锚点：生产构建（mode === "production"）折叠为 false，
  // 其余（vitest 的 test、vite dev 的 development）为 true，测试因此仍能按
  // data-od-id 定位节点。不用 import.meta.env.DEV 是因为 Vite 自身依赖它。
  define: { __OD_ID__: JSON.stringify(mode !== "production") },
  plugins: [react()],
  build: {
    outDir: "server/web",
    emptyOutDir: true,
    // Vite 8 的构建器是 Rolldown：rollupOptions 已更名为 rolldownOptions，
    // output.manualChunks 的对象形式被移除、函数形式仅保留 deprecated。
    // 官方替代是 Rolldown 的 codeSplitting.groups，此处按 vendor 边界拆三组，
    // 与迁移前的分包意图一致（框架层与应用代码分离，便于长缓存命中）。
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            // react / react-dom / scheduler 合成 vendor-react：三者版本同步发布，
            // 单独拆开只会增加请求数而不增加缓存收益。
            {
              name: "vendor-react",
              test: /[\\/]node_modules[\\/](react|react-dom|scheduler)[\\/]/
            },
            // 光束动效库单独成组：它体积大且只在首屏搜索框使用，
            // 与应用代码解耦后改业务不会让它失效。
            {
              name: "vendor-beam",
              test: /[\\/]node_modules[\\/]border-beam[\\/]/
            }
          ]
        }
      }
    }
  },
  server: {
    port: 5173,
    proxy: { "/api": "http://localhost:3078" }
  },
  preview: { port: 4173 }
}));