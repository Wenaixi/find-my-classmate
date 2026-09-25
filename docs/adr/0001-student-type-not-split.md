# 决策：Student 不拆分为内部模型与 wire DTO

- 状态：已决策（2026-09-26）
- 相关代码：server/search.go、server/serialize_test.go、server/api.go

## 背景

架构评审提出候选：把 `Student` 拆为内部模型（承载 `NameKey`/`ClassNo`/`GradeIdx` 派生字段）与
wire DTO（仅 `name`/`grade`/`class`），使隐私红线由类型形状保证，而非依赖 `json:"-"` 标签
与"必须经 `newStudent` 构造"的纪律。

## 决策

**不拆分。** 保持 `Student` 单一类型，隐私与派生字段的正确性继续由既有机制与测试保证。

## 理由（附实测证据）

1. **隐私已由类型强制。** `NameKey`、`ClassNo`、`GradeIdx` 三个内部字段均已带 `json:"-"`
   （search.go:26-33）。派生字段无法被意外序列化——漏写标签才会暴露，而该情形已被
   `main_test.go:101` 的 `TestSearchResponseKeys` 覆盖：它逐一断言 `NameKey`/`Name`/
   `ClassName`/`Grade`/`ClassNo`/`GradeIdx` 六个字段名均**不**出现在响应中。
   `serialize_test.go` 另有一层直序列化断言，双重覆盖。

2. **`newStudent` 纪律无实际绕过路径。** 派生字段只在 `newStudent`（search.go:38）写入，
   只在 `search.go` 的排序比较中读取。构造 `Student` 并传入 `Search` 的路径只有
   `loadStudents`（data.go:93），而它正是使用 `newStudent` 的那一条。

3. **成本真实、收益边际。** 拆分需改动 9 个文件，其中包含 CLAUDE.md 记录的
   零分配热路径（整年段查询 819µs/1762 allocs → 34.8µs/7 allocs，F73）。
   实测（`AllocsPerRun`，2112 条合成名单）DTO 转换每请求**固定新增 1 次分配**
   （切片头），当前 6/7/8 预算 12。成本可承担，但换来的是对已有双重测试所保证事项的
   重复保证。

## 重启条件

若将来出现以下任一情形，本决策应重开：

- 响应 DTO 与内部模型的实际字段集合开始分叉（而非当前的三字段子集关系）；
- `json:"-"` 标签被证明不足以约束新增字段（例如批量生成 DTO 的代码路径出现）；
- 分配预算（`TestSearchAllocsBudget`）放宽到足以吸收转换成本，且能换来可测量的其他收益。
