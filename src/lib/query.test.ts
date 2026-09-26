import { describe, expect, it } from "vitest";
import { hasNameCondition, normalizeName, parseQuery } from "./query";
import contract from "../../docs/query-contract.json";

// 前端只保留查询解释语义：解析查询串、判断是否含姓名条件。
// 运行时搜索（匹配、排序、分页）唯一归属 Go 端 search.go。
// 解析契约由 docs/query-contract.json 单一事实源定义，两侧测试各自消费；
// Go 端是运行时唯一执行者，corpus 以 Go 行为为准，前端必须对齐。

interface ContractCase {
  raw: string;
  /** 分词中间结果：归一后按空白切出的全部 token，在分类之前就已确定 */
  tokens: string[];
  nameTokens: string[];
  grade: string | null;
  classNumber: number | null;
}

const contractCases = contract.cases as ContractCase[];

describe("cross-language parse contract", () => {
  it("covers every case in the shared corpus", () => {
    expect(contractCases.length).toBeGreaterThan(0);
  });

  for (const c of contractCases) {
    it(`parses ${JSON.stringify(c.raw)} exactly as the corpus declares`, () => {
      const parsed = parseQuery(c.raw);
      // tokens 纳入对拍后，「两端空白集合逐码位对齐」这条不变量在两侧各有承重断言。
      // 此前它是前端独有字段、Go 侧无对应物，分词行为只能在单侧验证。
      expect(parsed.tokens).toEqual(c.tokens);
      expect(parsed.nameTokens).toEqual(c.nameTokens);
      expect(parsed.grade ?? null).toBe(c.grade);
      expect(parsed.classNumber ?? null).toBe(c.classNumber);
    });
  }
});

describe("query contract", () => {
  it("normalizes internal whitespace and case", () => {
    expect(normalizeName(" eXample  student ")).toBe("EXAMPLESTUDENT");
  });
  // 与 Go 端 strings.Fields 对齐：U+0085（NEL）在 Go 是分隔符（unicode.IsSpace），
  // JS \s 不覆盖。tokenizer 补上 NEL 后，NEL 分隔的两个汉字 token 各自走分类：
  // "张" 是姓名，"三" 是汉字数字 → 班级条件。两端行为一致（corpus 锁定）。
  it("splits on NEL (U+0085) and classifies each token", () => {
    const parsed = parseQuery("张\u0085三");
    expect(parsed.tokens).toEqual(["张", "三"]);
    expect(parsed.nameTokens).toEqual(["张"]);
    expect(parsed.classNumber).toBe(3);
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
