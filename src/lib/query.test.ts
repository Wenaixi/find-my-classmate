import { describe, expect, it } from "vitest";
import { normalizeName, parseQuery, searchStudents } from "./query";
import type { Student } from "../types";

const fixture: Student[] = [
  { name: "示例同学", grade: "高二", className: "18班" },
  { name: "示 例 同 学", grade: "高二", className: "6班" },
  { name: "EXAMPLE STUDENT", grade: "高二", className: "11班" },
  { name: "高一同学", grade: "高一", className: "1班" }
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
});
