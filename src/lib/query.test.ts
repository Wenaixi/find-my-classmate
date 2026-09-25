import { describe, expect, it } from "vitest";
import { hasNameCondition, normalizeName, parseQuery } from "./query";

// 前端只保留查询解释语义：解析查询串、判断是否含姓名条件。
// 运行时搜索（匹配、排序、分页）唯一归属 Go 端 search.go，
// 其行为契约由 server/search_test.go 与跨语言 corpus 验证。
// 本文件因此只断言 parseQuery 的输出，不在前端复刻匹配与排序实现。

describe("query contract", () => {
  it("normalizes internal whitespace and case", () => {
    expect(normalizeName(" eXample  student ")).toBe("EXAMPLESTUDENT");
  });

  it("parses every separator into the same tokens", () => {
    expect(parseQuery("高1、六班").nameTokens).toEqual([]);
    expect(parseQuery("高1、六班")).toMatchObject({ grade: "高一", classNumber: 6 });
    expect(parseQuery("高二，六班")).toMatchObject({ grade: "高二", classNumber: 6 });
    expect(parseQuery("高三+示例同学+18班")).toMatchObject({ grade: "高三", classNumber: 18, nameTokens: ["示例同学"] });
    expect(parseQuery("示例，18班")).toMatchObject({ classNumber: 18, nameTokens: ["示例"] });
  });

  it("parses grade three (高三 / 高3)", () => {
    expect(parseQuery("高三")).toMatchObject({ grade: "高三" });
    expect(parseQuery("高3")).toMatchObject({ grade: "高三" });
  });

  it("treats numeric input as class text", () => {
    expect(parseQuery("223").classNumber).toBe(223);
  });

  // F16/F71：年级+班级连写的口语化输入（"高二三班"）应解析出精确班级而非只按年级
  it("parses grade+class compound into precise class", () => {
    const q = parseQuery("高二三班");
    expect(q.grade).toBe("高二");
    expect(q.classNumber).toBe(3);
    expect(q.nameTokens).toEqual([]);
  });

  // F16：姓名含高/班字不被误判
  it("does not misparse names containing 高 or 班", () => {
    const q = parseQuery("高翔");
    expect(q.grade).toBeUndefined();
    expect(q.nameTokens).toContain("高翔");
  });

  // F22：汉字多位班级与 Go 一致
  it("parses chinese multi-digit class", () => {
    expect(parseQuery("十一班").classNumber).toBe(11);
    expect(parseQuery("二十班").classNumber).toBe(20);
    expect(parseQuery("十八班").classNumber).toBe(18);
  });

  // F22：超长数字按姓名处理（与 Go -1 语义一致 → 不作为班级条件）
  it("treats overflow digits as name token", () => {
    const q = parseQuery("99999999999999999999");
    expect(q.classNumber).toBeUndefined();
    expect(q.nameTokens).toEqual(["99999999999999999999"]);
  });
});

// Step D：查询语义第三拷贝归零——App 的"纯年段/班级查询"提示必须与解析结果一致，
// 而非另一套内嵌正则（原 App.tsx:112 的 hasNameCondition 正则已移入 query.ts）。
describe("hasNameCondition", () => {
  it("true for name tokens", () => {
    expect(hasNameCondition("张三")).toBe(true);
    expect(hasNameCondition("李四，高一")).toBe(true);
  });

  it("false for pure class token", () => {
    expect(hasNameCondition("18")).toBe(false);
    expect(hasNameCondition("18班")).toBe(false);
  });

  it("false for pure grade token", () => {
    expect(hasNameCondition("高一")).toBe(false);
    expect(hasNameCondition("高3")).toBe(false);
  });

  it("false for grade+class compound", () => {
    expect(hasNameCondition("高二三班")).toBe(false);
  });

  it("true for overflow digits (treated as name)", () => {
    expect(hasNameCondition("99999999999999999999")).toBe(true);
  });

  it("true for mixed name + class", () => {
    expect(hasNameCondition("张三，18班")).toBe(true);
  });
});
