# FindMyClassmate 架构指引

> 本文件是项目的唯一架构文档，描述稳定不变的逻辑骨架与契约。实现细节会演进，本文只记录不会过期的部分。改动架构时必须同步更新本文。

## 1. 总体形态

单仓库、双语言、单产物：

- 前端：React + TypeScript + Vite，构建产物输出到 `server/web/`
- 后端：Go 标准库 HTTP 服务，用 `//go:embed all:web` 把前端产物嵌入二进制
- 最终交付物：**一个二进制 + 一个数据目录**。二进制同时托管页面和 API，监听单一端口（默认 3078）

这个形态决定了三个不可破坏的边界：

1. 前端构建产物是嵌入资产，不是独立部署物
2. 后端是唯一入口，页面与 API 同源同端口
3. 数据目录通过环境变量 `FMC_DATA_DIR` 指向（默认 ./data），运行期可热重载

## 2. 数据模型与隐私边界

名单源文件：`data/高一.json`、`data/高二.json`、`data/高三.json`（年段按需，只放实际存在的文件）。文件结构为：

```json
{
  "标题": "福清一中2025级高一编班名单",
  "名单": {
    "1班": [ { "姓名": "张三" } ]
  }
}
```

加载时执行的规范化（server/data.go）：

- 按声明序 `knownGrades`（高一/高二/高三）探测数据目录，存在的文件才加载，缺失文件自动跳过；目录为空时明确报错
- 剥离 UTF-8 BOM
- 校验：文件名与标题中的年段必须一致、名单结构完整、班级名可解析为班号、按（年段+班级+姓名）去重
- 归一化输出统一模型：`{ Name, NameKey, Grade, ClassName }`

**隐私红线（不可破坏）**：

- 对外 API 只输出 name、grade、class 三个字段
- 不读取、不存储、不返回学籍辅号等任何额外字段
- 前端不写 localStorage，不把查询词或结果写入 URL

## 3. 查询契约（前端解释 + 后端执行）

查询语义的**运行时执行唯一归属后端** `server/search.go`；前端 `src/lib/query.ts` 只保留查询解释（解析 token、判断是否含姓名条件、姓名归一化），供页面提示文案使用。前端不再复刻匹配、排序与分页实现。

两侧必须一致的**解析**契约：

- 分隔符：中文逗号、英文逗号、顿号、加号、连续空白，均视为 token 分隔
- 班级 token：阿拉伯数字或汉字数字（一~九十九，含十位组合如十一、二十、二十一），可带可省略"班"字
- 年段 token：`高一/高二/高三`、`高1/高2/高3` 为别名
- 姓名匹配键：删除中英文空格、全角空格、制表符，统一大写（normalizeName）
- 超长数字串按姓名处理，不作为班级条件（与 Go 的 -1 语义一致）

后端独占的**执行**契约：

- 匹配规则：所有姓名 token 都必须包含匹配（AND 语义），年段精确匹配，班级按班号匹配
- 排序：完整匹配（0 分）< 前缀匹配（1 分）< 包含匹配（2 分），同分按年级声明序（高一<高二<高三，gradeOrder）再按班级号升序
- 分页：`{ items, total, limit, offset, hasMore }`；limit 默认 10，上限 50


解析契约的**可执行事实源**是 `docs/query-contract.json`：`src/lib/query.test.ts` 与 `server/contract_test.go` 各自消费同一份语料，任何一侧漂移都会在两侧测试中同时失败。语料中的期望值以 Go 端实测结果为准。

## 4. 契约常量（双端各一份）

跨端契约的数值常量无法跨语言共享，后端 `server/config.go` 与前端 `src/config.ts` 各持一份，改动必须两侧同步：

- 查询串 rune 上限：80（maxQueryRunes / MAX_QUERY_LENGTH，App.tsx 输入框 maxLength 同值）
- 分页：limit 默认 10、上限 50（defaultLimit / maxLimit / PAGE_SIZE）
- 前端请求超时 10s（REQUEST_TIMEOUT_MS，仅前端消费）
- 后端端口 3078、限流（突发 60、每秒回补 1）、静态资源 immutable 缓存头（仅后端消费）

E2E 契约（错误码、分页响应结构、脱敏格式）在文档其余章节声明；本条只约束"同一数值两份定义必须一致"。

## 5. 请求生命周期


中间件链（由外到内）：`rateLimit → accessLog → securityHeaders → mux`，顺序由 `newHandlerChain` 单点定义。两条跨模块政策：

- **429 不写访问日志**：限流位于 accessLog 之外，被拒请求不进入日志层，避免攻击流量放大日志磁盘写入。
- **429 仍带安全响应头**：安全头是全站响应契约，`writeRateLimited` 在写出拒绝响应前调用 `setSecurityHeaders`，使被拒请求不成为头部例外。安全头定义只有 `setSecurityHeaders` 一处。

```
浏览器输入 → 前端 parseQuery（即时校验/提示）
  → GET /api/search?q=&limit=&offset=
  → 后端参数校验（limit 1-50、offset ≥0、q ≤80 字符）
  → 数据视图（热重载检查）→ Search() → 分页 JSON
  → 前端校验响应结构 → 渲染
```

前端要点：

- 提交前用 AbortController 取消旧请求（防竞态）；竞态编排集中在 searchSession.ts（requestId + abort）
- 查询响应返回即渲染（无最短展示延迟），思维球反馈在 loading 期间稳定展示
- 首屏查询替换结果；加载更多只追加、不改变阅读位置
- 输入法组合期间 Enter 不提交；Escape 清空
- **状态与提示文案必须一致**：`input-change` 切到 `editing` 时同步刷新 `statusText`，否则会显示 editing 状态配上上一轮的错误或结果文案
- **只有查询词变化才作废在途请求**：`onChange` 触发 `invalidate()`；IME `composition-start/end` 不触发。组合开始不是"查询已过时"的信号，若在此作废请求且组合未产生输入变化，状态会卡在 loading 且提交按钮持续 disabled

## 5.1 错误契约

| 状态码 | 错误码 | 场景 |
| --- | --- | --- |
| 400 | invalid_limit | limit 非数字/越界（1-50） |
| 400 | invalid_offset | offset 非数字/负数 |
| 400 | invalid_query | q 超过 80 个字符（rune 计） |
| 404 | not_found | 未知 /api/* 路径 |
| 405 | method_not_allowed | 非 GET 访问 /api/search（带 Allow: GET） |
| 429 | rate_limited | 限流（JSON + Retry-After 整数秒） |
| 500 | data_unavailable | 数据文件缺失/损坏 |
| 503 | degraded | /api/health 数据不可用（status=degraded, reason=data, version） |

- 错误响应统一为 `{"error": "<code>"}`（429 除外，为 `{"error":"rate_limited"}`，与全站 JSON 一致）。
- 空 q 返回 200 + 空分页（`{items:[], limit, offset}`）。

## 6. 前端状态机

搜索区状态（SearchState）是 UI 的唯一事实来源：

`idle → editing → loading → success | duplicate | empty | error`

- 输入变化 → editing
- 提交 → loading（清空旧结果）
- 响应 0 条 → empty；1 条 → success；多条 → duplicate（分页展示）
- 请求失败 → error（保留重试入口）
- 清空（Escape / 清空按钮）→ 回到 idle

结果区仅在非 idle/editing 状态渲染，滚动定位由 state 变化触发。

## 7. 前端模块边界

| 模块 | 职责 | 依赖 |
| --- | --- | --- |
| src/App.tsx | 页面组装 + 装配 searchReducer/searchSession | 全部 |
| src/config.ts | 前端契约常量（PAGE_SIZE/MAX_QUERY_LENGTH/REQUEST_TIMEOUT_MS） | 无 |
| src/lib/api.ts | 网络适配与响应结构校验 | types, config |
| src/lib/query.ts | 查询解释（解析 token、hasNameCondition、姓名归一化），不含匹配/排序/分页 | types |
| src/lib/searchReducer.ts | 搜索状态机纯 reducer（状态派生/错误文案/F36 提示） | types, api, config |
| src/lib/searchSession.ts | 请求竞态编排（requestId + abort） | types |
| src/site.config.ts | 站点展示文案（数据来源/运营团队/数据处理方） | 无 |
| src/types.ts | 领域类型与状态枚举 | 无 |

动效依赖（border-beam / thinking-orbs / liquid-gooey）全部是表现层，不承载逻辑；若替换，禁止改变查询与状态语义。

## 8. 后端模块边界

| 模块 | 职责 |
| --- | --- |
| main.go | 装配路由、参数校验、安全头、日志、端口 |
| config.go | 后端契约常量（端口/分页/上限/限流/缓存头） |
| data.go | 数据加载、规范化、去重、热重载（view 唯一只读入口） |
| ip.go | 客户端 IP 解析唯一入口（clientIP/maskedIP） |
| search.go | 查询执行（解析/匹配/排序/分页），运行时搜索的唯一实现 |
| ratelimit.go | 令牌桶限流（IP 提取统一走 ip.go 的 clientIP） |
| web.go | 前端静态资源嵌入与托管（缓存按来源隔离，immutable 只给存在资源，raw/gzip/304 协商头一致） |

数据热重载策略：文件指纹探测带 1 秒节流（原子 CAS 保证同窗口单请求探测权）；变化则互斥重载，并发用读写锁保护（view() 为唯一数据访问入口）；重载失败有 2 秒冷却（指纹驱动）且旧数据不对外服务（一致性优先于可用性的设计决策，F59）。

健康检查语义（/api/health）：进程存活 + 数据可用性。数据损坏/缺失时返回 503 {"status":"degraded","reason":"data"}；响应携带 version（ldflags -X main.version，本地构建为 dev）。

## 9. 部署与发布约定

- **单实例边界（F49）**：限流桶与数据视图（内存名单）均为进程内状态，不支持多副本横向扩展；热重载为运维盲操作（写文件即生效），生产更新用原子替换（写临时文件 → mv）并随后请求验证。

- 本地：`npm run build` → `go run ./server`，端口 3078
- Docker：多阶段构建，单一端口映射，数据目录只读挂载（热重载仍生效）
- CI：frontend job 完成 typecheck+test+build 并上传 `frontend-build` 产物；backend job **复用该产物**（`needs: frontend` + `download-artifact`）而非重建前端，保证测试与编译看到与 CI 验证过的相同字节；另有 gofmt/go test -race/go vet、零数据守卫与 Docker buildx 构建验证
- 发布：tag 触发三平台交叉编译，归档只含二进制、文档与空 data 占位目录。**归档不含 server/web**——前端已嵌入二进制，保留它会形成第二个资产事实源
- 构建产物 `server/web/` 与本地记忆文件（CLAUDE.md、.superpowers/）不入库

## 10. 启动自举与日志

启动流程（main.go）保证"开箱即起"：

- 幂等创建数据目录（os.MkdirAll），随后创建日志目录
- 数据目录全空时给出明确指引并退出（"请将 高一.json、高二.json 或 高三.json 之一放入数据目录"），不静默空跑；年段文件按需加载，缺失的年段不算错误
- Docker 场景：数据目录挂载、日志目录 FMC_LOG_DIR 指向容器可写区

日志约定：

- 双写：文件（server.log）+ stdout，容器由 docker 收集 stdout
- 分级：FMC_LOG_LEVEL=error|warn|info（默认 info），代码内用 logInfof/logWarnf/logErrorf
- 访问日志：每个请求记录"方法 路径 状态 耗时 脱敏IP"；**查询参数永不入日志**（隐私红线）
- 不内置轮转：交由部署层（logrotate / docker json-file）

## 11. 测试策略

- 前端：Vitest——查询契约（query.test.ts）、状态机（searchReducer.test.ts）、竞态编排（searchSession.test.ts）
- 后端：go test——查询执行与数据加载（search_test.go）
- CI 数据契约 job：校验名单 JSON 结构、字段白名单（仅"姓名"）、去重
- 查询改动：解析规则先改测试再同步前后端；匹配/排序/分页只改后端

## 12. 演进原则

- 单一事实来源：数据只有一份（JSON 文件），契约只有一份（查询语义），UI 状态只有一份（SearchState）
- 解析双端一致：解析规则改动必须前后端同步，测试兜底；执行语义单端（后端）
- 隐私优先：任何新字段、新存储、新接口都要过隐私红线检查
- 单产物优先：新增功能优先考虑"仍是一个二进制"的形态
- 本文档是唯一架构文档：结构性变更必须回写本文