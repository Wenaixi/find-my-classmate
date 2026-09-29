import { describe, expect, it } from "vitest";
import { gradeDomain, hasNameCondition, normalizeName, parseQuery } from "./query";
import type { Grade } from "../types";
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

  // 含年段串的姓名：修复前被读成年段条件，此处返回 false，界面因此提示
  // 「已匹配整个年段」而实际返回整个年段的全量。修复后走姓名条件。
  // 这是前端唯一消费的布尔（searchReducer 经它派生提示文案），
  // 解析层有契约语料对拍，但该布尔本身此前零断言。
  it("true for names containing a grade string", () => {
    expect(hasNameCondition("高一鸣")).toBe(true);
    expect(hasNameCondition("高1鸣")).toBe(true);
    expect(hasNameCondition("高一同学")).toBe(true);
  });

  // 对照侧：纯年段与「年段+班」仍必须为 false。只断上一侧的话，
  // 把判断退化为「一律按姓名处理」同样会通过（那样提示文案永远
  // 显示为姓名查询，而实际返回整个年段）。
  it("false for pure grade token with optional 班 suffix", () => {
    expect(hasNameCondition("高一班")).toBe(false);
    expect(hasNameCondition("高1班")).toBe(false);
  });

  it("true for mixed name + class", () => {
    expect(hasNameCondition("张三，18班")).toBe(true);
  });
});

// 年段值域声明的不变量。
//
// 这组断言锁的不是「切分用的正则长什么样」，而是「声明本身完整且自洽」：
// 别名必须映射回已声明的规范名，否则 parseGrade 会返回一个不在 Grade 值域内的年段。
describe("gradeDomain", () => {
  it("每个别名都映射回已声明的规范年段", () => {
    for (const [alias, grade] of gradeDomain.aliases) {
      expect(gradeDomain.values).toContain(grade);
      expect(parseQuery(alias).grade).toBe(grade);
    }
  });

  it("规范年段无重复", () => {
    expect(gradeDomain.values.length).toBe(new Set(gradeDomain.values).size);
  });

  it("别名与规范名不重合：别名存在正是为了另一种书写", () => {
    const aliases = gradeDomain.aliases.map(([alias]) => alias);
    for (const alias of aliases) {
      expect(gradeDomain.values).not.toContain(alias);
    }
  });

  // 「Grade 联合与年段值域互相完整」这条反向完整性由编译器强制：
  // query.ts 的 GradeDomainGap 检查在 Grade 联合出现 gradeValues 未声明的年段时
  // 直接报 tsc 错误并点名缺失项。此前这里用一份手抄的 gradeUnion 数组承担，
  // 而手抄副本在 Grade 联合追加成员时不会跟着长——实测加「高四」后
  // tsc 干净、全部用例通过，parseQuery 已不再识别该年段。
  // 那两条断言因此一并删除：它们有判别力但方向反直觉（gradeValues 增长而手抄
  // 副本没跟上时反而翻红，惩罚一次正确的扩展），而在真正关心的裂缝上恒绿。
});
