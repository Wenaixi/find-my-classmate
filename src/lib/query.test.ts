import { describe, expect, it } from "vitest";
import { normalizeName, parseQuery, searchStudents } from "./query";
import type { Student } from "../types";

const fixture: Student[] = [
  { name: "示例同学", grade: "高二", className: "18班" },
  { name: "示 例 同 学", grade: "高二", className: "6班" },
  { name: "EXAMPLE STUDENT", grade: "高一", className: "11班" },
  { name: "高一同学", grade: "高一", className: "1班" },
];

describe("query contract", () => {
  it("normalizes internal whitespace and case", () => {
    expect(normalizeName(" eXample  student ")).toBe("EXAMPLESTUDENT");
  });

  it("supports grade, class aliases and every separator", () => {
    expect(searchStudents(fixture, "高1 1").items).toHaveLength(1);
    expect(searchStudents(fixture, "高二，六班").items).toHaveLength(1);
    expect(searchStudents(fixture, "高一，一班").items).toHaveLength(1);
    expect(parseQuery("高1、六班")).toMatchObject({ grade: "高一", classNumber: 6 });
    expect(searchStudents(fixture, "高二、示例同学、18班").items).toHaveLength(1);
    expect(searchStudents(fixture, "高二, 示例同学").items).toHaveLength(2);
    expect(searchStudents(fixture, "高二+示例同学+18班").items).toHaveLength(1);
    expect(searchStudents(fixture, "示例，18班").items[0].className).toBe("18班");
    expect(searchStudents(fixture, "一班").items).toHaveLength(1);
    expect(searchStudents(fixture, "18").items[0].className).toBe("18班");
  });

  it("treats numeric input as class text", () => {
    expect(parseQuery("223").classNumber).toBe(223);
    expect(searchStudents(fixture, "223").items).toEqual([]);
  });

  it("paginates broad results without hiding them", () => {
    const first = searchStudents([...fixture, ...fixture], "示例", 2, 0);
    const second = searchStudents([...fixture, ...fixture], "示例", 2, 2);
    expect(first.items).toHaveLength(2);
    expect(first.total).toBe(4);
    expect(first.hasMore).toBe(true);
    expect(second.items).toHaveLength(2);
    expect(second.offset).toBe(2);
    expect(second.hasMore).toBe(false);
  });

  it("returns an empty paged response for blank input", () => {
    expect(searchStudents(fixture, "")).toEqual({ items: [], total: 0, limit: 10, offset: 0, hasMore: false });
  });

  // F16：年级子串输入（口语化）与 Go 端语义一致：按年级筛选而非姓名
  it("parses grade substring like Go (口语化输入)", () => {
    const q = parseQuery("高二三班");
    expect(q.grade).toBe("高二");
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

  // F22：超长数字按姓名处理（与 Go -1 语义一致 → 无匹配）
  it("treats overflow digits as name token", () => {
    expect(searchStudents(fixture, "99999999999999999999").items).toEqual([]);
  });

  // F23：排序含 Grade 二级键（同分跨年级时高一在前）
  it("sorts by grade when scores tie", () => {
    const students: Student[] = [
      { name: "林宇", grade: "高二", className: "1班" },
      { name: "林宇", grade: "高一", className: "2班" },
    ];
    const result = searchStudents(students, "林宇");
    expect(result.items[0].grade).toBe("高一");
    expect(result.items[1].grade).toBe("高二");
  });
});
