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

名单源文件：`data/高一.json`、`data/高二.json`。文件结构为：

```json
{
  "标题": "福清一中2025级高一编班名单",
  "名单": {
    "1班": [ { "姓名": "张三" } ]
  }
}
```

加载时执行的规范化（server/data.go）：

- 剥离 UTF-8 BOM
- 校验：文件名与标题中的年段必须一致、名单结构完整、班级名可解析为班号、按（年段+班级+姓名）去重
- 归一化输出统一模型：`{ Name, NameKey, Grade, ClassName }`

**隐私红线（不可破坏）**：

- 对外 API 只输出 name、grade、class 三个字段
- 不读取、不存储、不返回学籍辅号等任何额外字段
- 前端不写 localStorage，不把查询词或结果写入 URL

## 3. 查询契约（前后端镜像实现）

前端 `src/lib/query.ts` 与后端 `server/search.go` 是**同一套查询逻辑的两份实现**，必须保持行为一致：

- 分隔符：中文逗号、英文逗号、顿号、加号、连续空白，均视为 token 分隔
- 班级 token：阿拉伯数字或中文数字（一~十），可带可省略"班"字
- 年段 token：`高一/高二`、`高1/高2` 为别名
- 姓名匹配键：删除中英文空格、全角空格、制表符，统一大写（normalizeName）
- 匹配规则：所有姓名 token 都必须包含匹配（AND 语义），年段精确匹配，班级按班号匹配
- 排序：完整匹配（0 分）< 前缀匹配（1 分）< 包含匹配（2 分），同分按班级号升序
- 分页：`{ items, total, limit, offset, hasMore }`；limit 默认 10，上限 50

修改查询逻辑时必须**同时修改两份实现**并更新两侧测试。

## 4. 请求生命周期

```
浏览器输入 → 前端 parseQuery（即时校验/提示）
  → GET /api/search?q=&limit=&offset=
  → 后端参数校验（limit 1-50、offset ≥0、q ≤80 字符）
  → 数据快照（热重载检查）→ Search() → 分页 JSON
  → 前端校验响应结构 → 渲染
```

前端要点：

- 提交前用 AbortController 取消旧请求（防竞态）
- loading 最短展示 1000ms（searchTiming.ts），保证思维球反馈稳定
- 首屏查询替换结果；加载更多只追加、不改变阅读位置
- 输入法组合期间 Enter 不提交；Escape 清空

## 5. 前端状态机

搜索区状态（SearchState）是 UI 的唯一事实来源：

`idle → editing → loading → success | duplicate | empty | too-many | error`

- 输入变化 → editing
- 提交 → loading（清空旧结果）
- 响应 0 条 → empty；1 条 → success；多条 → duplicate（分页展示）
- 请求失败 → error（保留重试入口）
- 清空（Escape / 清空按钮）→ 回到 idle

结果区仅在非 idle/editing 状态渲染，滚动定位由 state 变化触发。

## 6. 前端模块边界

| 模块 | 职责 | 依赖 |
| --- | --- | --- |
| src/App.tsx | 页面组装、状态机、交互编排 | 全部 |
| src/lib/api.ts | 网络适配与响应结构校验 | types |
| src/lib/query.ts | 查询解析/匹配/排序（镜像后端） | types |
| src/lib/searchTiming.ts | 最短反馈时长 | 无 |
| src/types.ts | 领域类型与状态枚举 | 无 |

动效依赖（border-beam / thinking-orbs / liquid-gooey）全部是表现层，不承载逻辑；若替换，禁止改变查询与状态语义。

## 7. 后端模块边界

| 模块 | 职责 |
| --- | --- |
| main.go | 装配路由、参数校验、安全头、日志、端口 |
| data.go | 数据加载、规范化、去重、快照热重载（RWMutex） |
| search.go | 查询解析/匹配/排序（镜像前端） |
| web.go | 前端静态资源嵌入与托管 |

数据热重载策略：每次请求检查文件 size+mtime，变化则重载；并发用读写锁保护快照。

## 8. 部署与发布约定

- 本地：`npm run build` → `go run ./server`，端口 3078
- Docker：多阶段构建，单一端口映射，数据目录只读挂载（热重载仍生效）
- CI：push/PR 跑全量测试（前端 typecheck+test+build、后端 gofmt+go test+go vet、数据契约校验）；tag 触发交叉编译三平台二进制并打 Release
- 构建产物 `server/web/` 与本地记忆文件（CLAUDE.md、.superpowers/）不入库

## 9. 启动自举与日志

启动流程（main.go）保证"开箱即起"：

- 幂等创建数据目录（os.MkdirAll），随后创建日志目录
- 数据文件缺失时给出明确指引并退出（"请将 高一.json 放入数据目录"），不静默空跑
- Docker 场景：数据目录挂载、日志目录 FMC_LOG_DIR 指向容器可写区

日志约定：

- 双写：文件（server.log）+ stdout，容器由 docker 收集 stdout
- 分级：FMC_LOG_LEVEL=error|warn|info（默认 info），代码内用 logInfof/logWarnf/logErrorf
- 访问日志：每个请求记录"方法 路径 状态 耗时 脱敏IP"；**查询参数永不入日志**（隐私红线）
- 不内置轮转：交由部署层（logrotate / docker json-file）

## 10. 测试策略

- 前端：Vitest——查询契约（query.test.ts）、最短时长（searchTiming.test.ts）
- 后端：go test——查询与数据加载的镜像测试（search_test.go）
- CI 数据契约 job：校验名单 JSON 结构、字段白名单（仅"姓名"）、去重
- 查询逻辑改动：必须先改测试，再同步改前后端两份实现

## 10. 演进原则

- 单一事实来源：数据只有一份（JSON 文件），契约只有一份（查询语义），UI 状态只有一份（SearchState）
- 双端镜像：查询逻辑改动必须前后端同步，测试兜底
- 隐私优先：任何新字段、新存储、新接口都要过隐私红线检查
- 单产物优先：新增功能优先考虑"仍是一个二进制"的形态
- 本文档是唯一架构文档：结构性变更必须回写本文
