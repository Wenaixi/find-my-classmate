import { describe, expect, it } from "vitest";
import { od } from "./od";

// 契约：od 在测试环境返回 { "data-od-id": id }（测试靠这些锚点定位节点），
// 生产构建由 vite 的 __OD_ID__ 常量折叠为 () => ({})（产物零锚点键）。
// 本文件只锁测试侧行为；生产侧折叠由构建后的 grep 验证（plan Task 1 Step 8）。
describe("od 测试锚点", () => {
  it("测试环境返回带 data-od-id 的 props 对象", () => {
    expect(od("result-card-0")).toEqual({ "data-od-id": "result-card-0" });
  });

  it("拼接表达式与字面量走同一路径", () => {
    expect(od("result-name-" + 3)).toEqual({ "data-od-id": "result-name-3" });
  });
});
