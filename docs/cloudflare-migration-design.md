# FindMyClassmate 迁移到 Cloudflare 全程托管：设计规格

日期：2026-11-15
状态：已与主人对齐决策（会话内批准本规格即批准进入实施计划阶段）
对应代码基线：v0.12.0（多学校模式，Go 单二进制 + Docker 自托管）

## 1. 背景与目标

FindMyClassmate 当前是「Go 标准库查询服务 + React 前端」的单二进制应用：
前端产物经 `//go:embed` 嵌入 Go 二进制，单进程同时托管页面与 `/api`，Docker 部署、
端口 3078、数据目录文件热重载。真实名单存 `data/*.json`，**不入 git、不进发布包**。

目标：深度重构为 Cloudflare Workers 全托管（免服务器、免运维），满足：

1. **行为等价**：`/api/health`、`/api/search`、`/api/schools` 的 wire 契约逐字段不变
   （响应形状、错误码、分页语义、排序语义零变化）；前端 `src/` 除查询解析的 import
   归属外零改动。
2. **纪律平移**：契约语料 `docs/query-contract.json` 仍是逐条断言的唯一输入集合；
   变异实验、双向承重、单一 owner 纪律全部保留；Go 特有纪律（分配预算、race、
   gofmt）随 Go 一并退役并记录原因。
3. **隐私红线延续**：真实名单不进 git、不进发布包；API 仍只返回
   name/school/grade/class。
4. **热更新保留**：管理端上传新 roster → 全站更新（接受 KV 最终一致性，最多约
   60 秒生效，见「已知行为变化」）。
5. **零服务器**：部署为 `wrangler deploy`，无 Docker、无容器、无常驻进程。

**成功判据**：迁移完成时，契约语料全部条目在 TS 侧逐条全绿；API 响应与迁移前
逐字段一致（迁移期用并行对拍验证）；前端全部用例通过；管理 API 可用；
`server/`、Dockerfile、docker-compose.yml 已删除。

## 2. 决策记录

| # | 决策 | 结论 | 依据 |
|---|---|---|---|
| 1 | 名单存储与更新 | 存 KV；管理 API 更新（Bearer token） | 延续「真实名单不进 git」红线；保留热更新语义 |
| 2 | 迁移深度 | 完整迁移：行为 + 纪律全保留 | 本仓库最值钱的是纪律体系，不全搬等于扔资产 |
| 3 | 管理端形态 | 纯 API + 脚本，无管理网页 | YAGNI；管理网页是额外攻击面 |
| 4 | 整体架构 | 方案 A：单 Worker + Workers Assets | 与「单二进制」心智同构；静态请求免费不限量 |
| 5 | KV 组织 | 按校分 key：`roster:{school}` + `roster:index` | 带 school 过滤的搜索只读一个 key；批量读兜底全局搜索 |

## 3. 目标架构

### 3.1 组件布局

```
仓库根（现有项目原地演进）
├── wrangler.jsonc            # Worker + Assets + KV 绑定 + 环境配置
├── src/shared/               # 新增：纯逻辑单一 owner 层
│   ├── query.ts              # 查询解析（原 src/lib/query.ts 迁入；现被前端与 Worker 共用）
│   ├── classparse.ts         # 班级解析（现 Go server/classparse.go 逻辑平移；前端镜像收编于此）
│   ├── grades.ts             # knownGrades / gradeValues / gradeAliases 单一声明（现 Go config.go + 前端 types.ts 双写，收编）
│   └── contract.ts           # 跨语言契约常量（MAX_LIMIT 等；现 Go config.go + 前端 config.ts 双写，收编）
├── src/worker/               # 新增：Worker 专属
│   ├── index.ts              # fetch handler + 路由装配 + 中间件链
│   ├── middleware.ts         # securityHeaders 平移（CSP 等）+ 访问日志（IP 脱敏平移）
│   ├── rosterStore.ts        # KV 存储层（加载/校验/批量读/管理写）
│   ├── search.ts             # Search / SearchInSchool / 排序 / 分页（现 Go search.go 逻辑平移）
│   ├── schools.ts            # schoolList 派生（现 Go schools.go 平移）
│   ├── handlers.ts           # API 端点翻译（health/search/schools/admin）
│   └── errors.ts             # 错误码表与 JSON 写出（现 Go errors.go 平移）
├── src/lib/                  # 现有前端（api.ts / searchReducer / components 等保留不动）
├── dist/                     # vite build 产物 → Workers Assets 目录
├── public/                   # 现有静态资源（字体等）
└── scripts/
    ├── upload-roster.mjs     # 管理脚本：读 data/*.json → PUT 管理 API
    └── local-dev.mjs         # 本地冒烟：wrangler dev 与旧 Go 的响应对拍（迁移期）
```

### 3.2 存储层（KV）

- `roster:{school}` → 该校 roster JSON 原文（原始文本存字符串，不预解析）。
- `roster:index` → `["福清一中", ...]`（学校列表，写入时维护）。
- 读取：`/api/search` 与 `/api/schools` 先读 index，再 `env.ROSTER.get(keys, { type: "json", cacheTtl: 60 })`
  一次批量读全部名单（KV 批量 get 上限 100 key，返回 Map；cacheTtl 最小 30 秒，
  此处取 60）。cacheTtl 兜底冷读，热路径走边缘缓存。
- 每请求都重新读（不引入模块级缓存与版本 key）——名单约 400KB，`cacheTtl: 60`
  已把冷读摊薄。升级路径已标注：若实测 CPU 接近免费层 10ms 上限，再加模块级
  缓存 + `roster:version` 版本 key。
- CPU 核算（2026-11 官方 limits）：免费层每次调用 10ms CPU；KV 读（网络等待）
  不计 CPU；400KB JSON.parse 约 1-3ms、全量搜索约 1ms，留有余量。若实测超限，
  先上模块缓存（见上），仍超限再考虑 Workers Paid（30s CPU，$5/月）。
- 免费层限额核对：读 100k/天、写 1k/天、值上限 25MiB、存储 1GB、同 key 写
  1 次/秒（2026-11 官方 limits）。校内流量绰绰有余。

### 3.3 查询解析单一 owner（迁移的核心红利）

- 现状：Go `search.go/classparse.go/config.go` 与前端 `query.ts/types.ts/config.ts`
  双写同一套语义，靠跨语言对拍（Go 读 TS 源码抽字面量）兜底；对拍对「两端一致地
  错」结构性失明（本仓第十一、十八、十九轮均有实例）。
- 迁移后：全部收进 `src/shared/`，前端与 Worker 共用同一实现 → **跨语言对拍消灭，
  单一 owner 取代**（`src/lib/query.ts` 迁入 `src/shared/query.ts`，前端 import
  路径随之更新）。
- 契约语料从「前后端对拍基准」升格为「唯一权威」，继续逐条断言。
- `contract_*_test.go`（Go 读 TS 源码）随 Go 删除；其职责由 shared 层的编译期检查
  （`as const satisfies`、`Exclude` 反向完整性）继承——这些检查现已在
  query.ts/types.ts，迁入时保持。

### 3.4 API 面（与现状逐字段一致）

- `GET /api/health` → 200 `{status:"ok", version}` / 503 `{status:"degraded",
  reason:"data", version}`。version 从环境变量 `VERSION` 读（CI 注入），
  不再 ldflags。
- `GET /api/search?q=&school=&limit=&offset=` → 响应形状、错误码
  （invalid_limit/invalid_offset/invalid_query/data_unavailable/rate_limited/
  not_found/method_not_allowed）、分页语义（limit 1..50、hasMore、offset 校验）
  逐字段与现状一致。
- `GET /api/schools` → `{schools:[...]}`，含「只列有高中年级的学校」规则平移。
- 未知 `/api/*` → JSON 404 `{"error":"not_found"}`。
- 管理端（新增，Bearer 认证）：
  - `PUT /api/admin/roster/{school}`，body = roster JSON → 复用名单加载校验逻辑
    （TS 化）→ 校验通过后 KV put + 更新 index → 201。校验失败按现有错误文案
    （班级格式异常/学生记录格式异常等）返回 400。
  - `DELETE /api/admin/roster/{school}` → KV delete + index 更新。
  - `GET /api/admin/roster` → 已上传学校与元信息（运维现状视图）。
  - 认证：请求头 `Authorization: Bearer <token>`，`ADMIN_TOKEN` 存 Workers
    secret，`crypto.subtle.timingSafeEqual` 常量时间比较。

### 3.5 静态资源（Workers Assets）

- `wrangler.jsonc`：`[assets] directory = "./dist"`、
  `run_worker_first = ["/api/*"]`、
  `not_found_handling = "single-page-application"`。
- 静态请求免费且不限量；ETag/gzip/MIME/单页回退由平台托管，替代现有 web.go
  手写缓存逻辑（该逻辑退役）。
- CSP 等安全头由 middleware 对 API 响应设置；静态资源缓存头由 Assets 平台
  默认处理（强于自研）。

### 3.6 限流：Workers Rate Limiting binding（唯一与现状不等价的纪律项）

- 现状：Go 令牌桶 per-IP（cap 60、1/s 回填），状态驻留进程内存。
- 迁移后：Workers 无共享进程内存，per-IP 令牌桶需要为每个 IP 建一个 Durable
  Object——只为限流引入状态组件属过度设计。
- 采用：**Workers Rate Limiting binding**（`wrangler.jsonc` 的 `ratelimits`
  配置，官方 2026 文档确认可用）。配置：
  `{ "name": "API_RATE_LIMITER", "namespace_id": "<正整数>", "simple": { "limit": 60, "period": 60 } }`，
  代码侧 `await env.API_RATE_LIMITER.limit({ key })` 返回 `{ success }`，
  失败时复用现有 429 响应形状 `{"error":"rate_limited"}` + `Retry-After`。
- key 取 `CF-Connecting-IP`：官方文档明确不建议用 IP（共享 IP / 移动 NAT
  会误伤同一 NAT 后的多位用户），但本站无登录态，可用行为标识只有 IP，且旧
  实现本就是 per-IP——**按 IP 是延续现状**。取舍写进 ARCHITECTURE.md。
- 行为差异（如实记录）：
  - CF 是**每个 Cloudflare 边缘位置独立计数、近似限流**（后台异步同步），
    非旧实现的全局平滑令牌桶；突发可能短暂超过阈值。
  - 防御目标（防刷名单/爬虫）不受影响；校内流量远低于阈值。
- 这是「行为 + 纪律全保留」承诺下唯一的例外，理由与换算写进 CLAUDE.md 与
  ARCHITECTURE.md；Go 令牌桶测试与 ratelimit.go 随实现删除并记录原因。
- 免费层对 ratelimits 绑定数量是否有别于 WAF 规则（WAF 规则免费 1 条）——
  文档未明示绑定侧免费层配额，列入实施阶段验证点（本地 `wrangler deploy
  --dry-run` 可先验配置合法性）。

### 3.7 测试体系（vitest 4 双项目）

- 依赖升级：vitest ^3.2.7 → ^4.1.0，新增 `@cloudflare/vitest-plugin`
  （官方当前推荐包，`cloudflareTest({ wrangler: { configPath: "./wrangler.jsonc" } })`
  插件式接入）、`wrangler`、`@cloudflare/workers-types`。
- 双项目配置（`vitest.config.ts` 用 `test.projects`）：
  - 前端项目（node + jsdom）：现有 React 组件测试、`searchReducer` 等，原样
    平移（文件头 `@vitest-environment jsdom` 声明机制保留）。
  - Worker 项目（cloudflareTest 插件，workerd 运行时 + miniflare KV binding）：
    rosterStore 加载/校验、搜索端点集成（SELF.fetch）、管理端点认证与写入、
    shared 解析的语料测试。
- 契约语料测试归属：parseQuery 语料测试在 shared（前端与 Worker 共用同一实现，
  测一份即可）；Search 级语料测试（原 Go search_test 消费）在 Worker 项目。
- 变异实验纪律保留：破坏实现 → 相应语料用例翻红。分配预算/race/gofmt 纪律
  随 Go 退役，退役原因写进记忆库。
- vitest 4 行为差异（act 环境、jsdom 工厂等）列入实施验证点，现有已踩过的坑
  （`IS_REACT_ACT_ENVIRONMENT`）在升级时复核。

### 3.8 部署与 CI

- `ci.yml`：Node 24；typecheck / build / 双项目测试；wrangler 配置校验
  （`wrangler deploy --dry-run`）；真实名单零数据防误提检测保留（data/ 不入
  git）。
- 部署（`deploy.yml`）：`wrangler deploy`，`CLOUDFLARE_API_TOKEN` +
  `CF_ACCOUNT_ID` 走 GitHub secrets；`VERSION=${{ github.ref_name }}` 注入。
- workers.dev 域名（默认；自定义域后续可加，不影响架构）。

### 3.9 数据迁移

- `scripts/upload-roster.mjs`：读 `data/*_roster.json`（内容含校名）→ 调管理 API
  PUT。迁移时对现有学校执行一次。
- 流程：管理员（或 CI 中手动步骤）持有 ADMIN_TOKEN，脚本上传后全站热更新。

## 4. 实施顺序（分阶段，每阶段可验证）

1. **阶段 0：基线**。跑现有全量测试（Go + 前端 194 条 + typecheck + gofmt），
   记录全绿基线；确认工作区无未提交改动与迁移冲突。
2. **阶段 1：shared 层**。`src/lib/query.ts` 迁入 `src/shared/query.ts`（import
   路径更新）；Go `classparse.go/config.go` 的解析语义平移为 `src/shared/classparse.ts`
   + `grades.ts` + `contract.ts`；语料测试双绿（shared 解析 ↔ 语料；Go 侧仍绿，
   作为对拍过渡）。
3. **阶段 2：Worker 逻辑**。`search.ts`（Search/SearchInSchool/排序/分页）、
   `schools.ts`、`rosterStore.ts`（校验逻辑平移，错误文案一致）在纯 TS 下实现 +
   语料测试全绿（此时 TS 与 Go 双绿，即迁移期的「对拍」）。
4. **阶段 3：Worker 装配**。`index.ts`/`middleware.ts`/`handlers.ts`/`errors.ts`；
   `wrangler.jsonc` + vite 配置对齐（build 产物 → dist）；本地 `wrangler dev` +
   miniflare 集成测试；`local-dev.mjs` 并行对拍旧 Go 服务与 Worker 的响应
   逐字段一致。
5. **阶段 4：管理 API 与脚本**。admin 端点 + token 认证 + `upload-roster.mjs`；
   数据迁移执行；真实 HTTP 冒烟（/api/health、纯分隔符 total=0、「一一班」、
   高一全量、静态资源缓存头）。
6. **阶段 5：CI/部署**。GitHub Actions 双项目测试 + `wrangler deploy`；首次真实
   部署到 workers.dev 并全量冒烟。
7. **阶段 6：清理与文档**。删除 `server/`、Dockerfile、docker-compose.yml、
   Go 相关 CI 步骤；CLAUDE.md/ARCHITECTURE.md/CONTEXT.md/CHANGELOG 更新（含全部
   退役纪律与「限流平台化」例外）。
8. **阶段 7：验收**。契约语料全绿；前端用例全绿；冒烟清单逐项通过；确认 git
   中无名单数据。

## 5. 已知行为变化（诚实边界）

1. **热更新延迟**：KV 最终一致，写入后各边缘节点缓存最多约 60 秒才刷新——
   从「1 秒内生效」变为「最多约 60 秒生效」。
2. **限流语义**：per-IP 令牌桶（全局平滑）→ Workers Rate Limiting binding
   （每边缘位置近似计数、IP 作 key，见 3.6）。
3. **CPU 上限**：免费层每次调用 10ms CPU；400KB 名单冷读 + JSON.parse + 全量
   搜索逼近上限，用 `cacheTtl: 60` 摊薄，超限则升级模块缓存（已标定升级路径）。
   若实测频繁超限，可升 Workers Paid（30s CPU，$5/月）。
4. **版本号来源**：ldflags 编译期注入 → 环境变量注入（部署时定值）。

## 6. 假设

- 使用默认 workers.dev 域名（未要求自定义域）。
- 中文内容编码保持 UTF-8；名单仍从现有 `data/` 迁移，不改变源格式。
- GitHub Actions 部署用现仓库的 secrets 机制；管理员持有 ADMIN_TOKEN 日常操作
  名单。
- 迁移期间旧 Go 服务与 Worker 平行存在，对拍通过后才切流量（本地验证为主，
  不做灰度路由）。

## 7. 风险与缓解

| 风险 | 缓解 |
|---|---|
| vitest 3 → 4 升级破坏现有测试行为 | 阶段 0 记录基线；升级后先跑前端项目全量，逐项修复环境差异 |
| KV 批量读 + JSON.parse 逼近 10ms CPU | cacheTtl 60 摊薄；模块缓存 + version key 升级路径已标注 |
| 「单二进制心智」迁移走样（API 与静态分离） | run_worker_first 保证 /api/* 全部进 Worker，与现状路由一致 |
| 管理 API 成为攻击面 | Bearer token + 常量时间比较 + 仅管理端点；无管理网页 |
| Rate Limiting 绑定的免费层配额与近似语义 | 实施期先 `wrangler deploy --dry-run` 验配置、本地 miniflare 验行为；若免费层不允许该绑定，退回 WAF 单规则（返回体不可定制）或接受更粗粒度 |
| 契约语料在单侧化后失去「对拍」校验 | shared 层编译期检查继承；语料测试照常全绿是迁移期双绿的硬门槛 |