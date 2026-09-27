// 年段值域与 lib/query.ts 的 gradeValues 保持一致：新增年段时两处同改。
// 两个方向各由一处强制，缺一不可：
//   正向（列表里的值必须属于 Grade）由 gradeValues 的
//   `as const satisfies readonly Grade[]` 承担——写出联合之外的值即编译报错；
//   反向（Grade 里的值必须已在列表中声明）由 query.ts 的 GradeDomainGap
//   编译期检查承担，报错直接点名缺失的年段。
// 两者都是 as const satisfies 形态的原因：as const 保留字面量类型（否则
// typeof gradeValues[number] 会被拓宽回 Grade，反向检查恒成立、形同虚设），
// satisfies 保住正向强制（只写 as const 会丢掉它，只写 satisfies 仍被拓宽）。
// 该反向检查刻意不放在测试文件里：.dockerignore 排除 src/**/*.test.ts，
// 放进测试则容器镜像构建的 tsc -b 不再检查它，会造出一条「本地绿、镜像不查」
// 的隐形防线。
export type Grade = "高一" | "高二" | "高三";

export interface Student {
  name: string;
  grade: Grade;
  className: string;
}

export interface SearchResponse {
  items: Student[];
  total: number;
  limit: number;
  offset: number;
  hasMore: boolean;
}

export type SearchState = "idle" | "editing" | "loading" | "success" | "duplicate" | "empty" | "error";

export interface ParsedQuery {
  tokens: string[];
  nameTokens: string[];
  grade?: Grade;
  classNumber?: number;
}
