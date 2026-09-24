import { bench, describe } from "vitest";
import { searchStudents } from "./query";
import type { Grade, Student } from "../types";

// 合成 2112 条名单，规模与真实数据一致（真实姓名不入库）。
function benchStudents(): Student[] {
  const surnames = ["张", "王", "李", "赵", "陈", "刘", "杨", "黄", "周", "吴"];
  const given = ["伟", "芳", "娜", "敏", "静", "强", "磊", "洋", "艳", "勇", "军", "杰", "娟", "涛", "明", "超"];
  const grades: Grade[] = ["高一", "高二", "高三"];
  const students: Student[] = [];
  grades.forEach((grade, gi) => {
    for (let className = 1; className <= 22; className++) {
      for (let k = 0; k < 32; k++) {
        const name =
          surnames[(gi * 7 + className + k) % surnames.length] +
          given[(gi * 13 + className * 3 + k * 5) % given.length] +
          given[(gi * 3 + className * 7 + k * 11) % given.length];
        students.push({ name, grade, className: className + "班" });
      }
    }
  });
  return students;
}

const students = benchStudents();

describe("searchStudents（2112 条名单）", () => {
  bench("单姓名查询", () => {
    searchStudents(students, "张伟");
  });
  bench("整年段查询", () => {
    searchStudents(students, "高一");
  });
  bench("组合查询", () => {
    searchStudents(students, "高二, 张, 3班");
  });
});