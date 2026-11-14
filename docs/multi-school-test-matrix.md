# 多学校升级 · 深度测试矩阵（2026-11-15）

> 本文记录 v0.12.0 多学校升级的全部测试覆盖与实证结果。
> 验证命令：go test ./... + npx vitest run + npm run build + 真实服务 curl 集成。
> 结论：后端全绿、前端 216/216、构建通过、集成 13 场景全过。

## 0. 总览

| 层 | 新增测试 | 结果 |
|---|---|---|
| 后端数据层 | 10 | 全绿（server/schools_test.go） |
| 后端查询层 | 7 | 全绿 |
| 后端 API 层 | 4 | 全绿 |
| 前端 orchestrator | 6 | 全绿 |
| 前端 App 渲染 | 4 | 全绿 |
| 真实服务集成 | 13 | 全过（curl 手测，见 §5） |

## 1. 后端数据层

| 场景 | 测试 | 断言 |
|---|---|---|
| 单文件多学校 | TestLoadStudentsMultiSchoolSingleFile | 两校均加载，school 归属正确 |
| 跨文件校名重复 | TestLoadStudentsDuplicateSchoolAcrossFiles | 报错含「校名重复」 |
| 校名为空 | TestLoadStudentsRejectsEmptySchoolName | 报错含「缺少校名」 |
| 年级名为空 | TestLoadStudentsRejectsEmptyGradeName | 报错含「缺少年级名」 |
| 班名为空 | TestLoadStudentsRejectsEmptyClassName | 报错含「缺少班名」 |
| 跨校同名保留 | TestLoadStudentsKeepsSameNameAcrossSchools | 去重键含学校，同名不互吞 |
| 学校加载序 | TestLoadStudentsSchoolOrderFollowsFilename | SchoolIdx 随文件名排序递增 |
| BOM/空名/坏班/溢出 | 既有 TestLoadStudents* | 未回归 |

## 2. 后端查询层

| 场景 | 测试 | 断言 |
|---|---|---|
| school 精确过滤 | TestSearchInSchoolFiltersExactly | 只命中目标校 |
| 空串=全校 | TestSearchInSchoolEmptyMeansAll | 不过滤 |
| 不存在校 | TestSearchInSchoolUnknownSchoolReturnsEmpty | 返回空，不退化为全校 |
| 只有 school 无查询条件 | TestSearchWithSchoolOnlyReturnsEmpty | 空串/纯分隔符均返回空（防全量） |
| 学校+年段+班级组合 | TestSearchInSchoolCombinedConditions | 条件叠加 |
| school 层排序 | TestSearchSortsBySchoolLayer | 甲校→乙校→无学校 |
| 查询契约零改动 | 既有 TestParseQueryContractCorpus 等 | 未回归 |

## 3. 后端 API 层

| 场景 | 测试 | 断言 |
|---|---|---|
| GET /api/schools | TestAPISchoolsEndpoint | 返回 {schools:[...]} |
| schools 只接受 GET | TestAPISchoolsMethodNotAllowed | 405 |
| schools 数据不可用 | TestAPISchoolsDataUnavailable | 500 data_unavailable |
| search school 参数 | TestAPISearchSchoolParameter | 过滤生效；超长→400 |
| 响应 4 键 | TestSearchResponseKeys（既有改） | name/school/grade/class |

## 4. 前端

### orchestrator

| 场景 | 断言 |
|---|---|
| loadSchools 注入 | schools 写入 state，默认校空 |
| loadSchools 失败 | 静默，schools 空，不抛错 |
| selectSchool 复位 | 结果清空→idle，school 更新 |
| selectSchool 切回全校 | school 变空串 |
| submit 传 school | search 收第 5 参 |
| loadMore 传 school | 翻页不丢过滤 |

### App 渲染

| 场景 | 断言 |
|---|---|
| 渲染全校+各校 | 4 个 option（全校+3 校） |
| 默认全校 | select value 空串 |
| 切校作废结果 | 搜索有结果→切校→结果区消失 |
| schoolsApi 失败 | 选择器仍渲染全校，不崩 |

## 5. 真实服务集成（curl）

| # | 场景 | 结果 |
|---|---|---|
| 1 | health | 200 ok |
| 2 | schools | 三校 |
| 3 | 全校搜张 | total=254 |
| 4 | school=福清一中搜张 | total=115（精确过滤） |
| 5 | 只有 school 无 q | 空（防全量守卫） |
| 6 | school=不存在校 | 空 |
| 7 | schools POST | 405 |
| 8 | 未知 /api/nope | 404 |
| 9 | 首页 | 200 |
| 10 | 破坏数据→health | 503 degraded |
| 11 | 破坏数据→search | 500 data_unavailable |
| 12 | 破坏数据→schools | 500 data_unavailable |
| 13 | 恢复→health | 200（热重载恢复） |

## 6. 深度测试发现并修复的缺陷

| 缺陷 | 症状 | 修复 |
|---|---|---|
| selectSchool 清空查询串 | 用户打好字切学校后输入框被清空 | reducer 保留 state.query，只作废结果不清输入 |

## 7. 结论

- 后端：数据边界、查询语义、API 端点/参数/错误码全部有断言锁定。
- 前端：选择器渲染/交互/失败降级/作废语义全部覆盖。
- 集成：13 场景含损坏与恢复闭环。
- 未覆盖：真实浏览器视觉走查（由渲染测试覆盖行为，视觉由既有设计语言保证）。
