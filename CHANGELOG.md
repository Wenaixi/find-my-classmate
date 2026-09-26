# 更新记录

本文件记录 FindMyClassmate 的版本变更。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [v0.10.4] - 2026-09-27

### 修复

- **名单测试接缝收口**：`TestSearchDataUnavailable` 不再从 `studentStore` 私有字段读取临时目录，改由测试 fixture 显式返回目录。生产名单视图的 `view()` 只读入口和热重载行为保持不变。

### 架构核实

- **名单可用性模块保持 deep**：热重载、失败冷却、fail-closed 与零拷贝快照继续集中在 `studentStore` 内部，不为拆分而拆分。
- **日志与契约候选保持现状**：启动日志当前只有一个实现，不引入没有第二个 adapter 支撑的日志 interface；`MAX_LIMIT` 继续作为跨语言契约声明保留，`rebuildGradePattern` 继续承担生产初始化。

## [v0.10.3] - 2026-09-26

### 修复

- **年段扩展路径断裂**：`data.go` 的名单标题校验反过来依赖 `parseGrade`，而 `parseGrade` 与 `gradeClassToken` 各自硬编码年段字面量（前端 `query.ts` 另有两处）。实测：往 `knownGrades` 追加新年段后，运维提示正确列出该文件、排序正确排位，但 `loadStudents` 报「文件名与年级标题不一致」拒绝加载合法名单，查询返回 0 条且与真不存在的年段结果完全一致。跨语言对拍无法发现——两端一致地不认识新年段，对拍语料必须先有该年段样本。现年段规范名与书写别名统一从 `knownGrades` 派生，加载侧与查询侧共用同一份值域。
- **`gradeClassToken` 编译结果的分配退化**：首版实现改为每次调用重新编译正则，整年段查询分配次数从 6 涨到 117（实测），被 `TestSearchAllocsBudget` 当场拦下。现缓存编译结果并提供 `rebuildGradePattern` 供扩展后重建，分配次数回到 6/7/8 基线。
- **CI 格式化检查失败**：`gofmt -l` 在本地报出 11 个文件，此前被记为「Windows CRLF 既存状况，由 CI 兜底」——该判断有误，CI 执行的是 `test -z "$(gofmt -l .)"`，任何文件不干净即硬失败。实际存在两类此前混为一谈的差异：map/struct 字面量的**对齐差异**，与整文件**行尾差异**。已全部 `gofmt -w` 规范化，`git diff -w` 确认无语义变化，分配预算 6/7/8 不变。
- **镜像构建的类型检查失败**：容器内 `npm run build` 报 `TS2307: Cannot find module '../../docs/query-contract.json'`。根因是 `tsconfig.app.json` 的 `include` 为 `["src"]`，把 `*.test.ts` 一并纳入类型检查，而 `query.test.ts` 依赖被 `.dockerignore` 排除的 `docs/query-contract.json`。已在 `.dockerignore` 排除测试文件——生产镜像本不应因测试代码而构建失败；本地 `npm run typecheck` 仍全量检查测试，类型覆盖不减少。

### 架构深化

- **wire 参数契约进入受测接缝**：`api.go` 的 limit / offset / query 长度三条 400 路径此前零承重——把三条判定改成恒假后全仓后端测试仍全绿。新增用例经 `buildMux` 真实接缝驱动，并配「合法边界不被误伤」的对照片段，防止「一律拒绝」这类同样能过测试的错误实现。
- **展示派生收进 `present` 单点**：`App.tsx` 此前必须同时 import `resultSectionOf` / `shouldScrollToResults` / `deriveStatusHint` 三个派生并另调 `useSearchController` 取 state，界面知识横跨两个 module；既有测试分别打三个函数，无任何断言锁住它们对同一状态给出一致的组合。现收成一次派生，一致性从「同一次调用」这一接缝可验证。

### 清理

- 内联一行实现的 `newStaticCache`（唯一调用点、零测试），修正 `web.go` 指向错误行号的 Vary 引用，删除 `src/**.tsx` 中零引用的 `.result-card.no-anim` 规则。
- 工作流步骤名「安装 Node.js 20」与实际 `node-version: 24` 不符，已改正；CI 的「名单 JSON 校验」job 名与架构文档描述的校验能力与实现不符（实际只做零数据守卫），已改为陈述实际能力。

### 测试

- 前端 135 → **142 用例**（新增 `present` 三条、`gradeDomain` 三条、别名连写对拍语料一条）；后端新增年段扩展性回归锁与三条 wire 参数用例。
- 变异实验六次：Go 端 `parseGrade` 跳过 `knownGrades` 循环、正则退回硬编码，前端 `parseGrade` 跳过规范名循环、`present` 的滚动与提示色各自改坏，api 三条参数判定改恒假——全部精确翻红。
- 诚实记录一次未翻红：前端把 `gradeClassToken` 退回硬编码正则时 54 条用例全绿——对已声明年段而言派生与硬编码行为等价，契约语料无法区分。故补充 `gradeDomain` 一组断言锁「声明本身完整且自洽」，而非试图锁「正则长什么样」。


## [v0.9.3] - 2026-09-26

### 修复

- **数据目录缺失时冷却机制失效**：`dataStamps` 自身失败（目录不可读等）时 `recordFailure(nil, err)` 使 `lastFailStamps` 为 nil，而 `sameStamps(nil, stamps)` 因长度不等恒为 false，导致 `cooling` 判定永不成立。结果是目录缺失场景下每个请求都重试读盘并写出错误日志——正是冷却机制要防的 IO 与日志放大。代码注释承诺「缺失目录同样需要冷却」，实现与承诺相反。现以 `lastFailStampKnown` 显式区分「指纹采集阶段失败」与「指纹已知」，冷却窗口如实生效。

### 架构深化

- **时钟注入窄化**：`newRateLimiter` 与 `newStudentStore` 均增加 `now` 构造参数，`now` 不再是可写字段暴露在生产 struct 上。此前测试通过直接改写 `store.now` / `limiter.now` 推进时间，共 11 处跨 3 个测试文件——测试装置泄漏进了生产结构。
- **自恢复时机单点判定**：探测节流（1 秒窗口）与失败冷却（2 秒窗口）此前拆在 `probeThrottled` 与 `cooling` 两个方法里各自取样时钟，「同一条时间线」只是注释承诺。现收敛为 `recoveryDue(now)` 单点，两条窗口都消费它——改动一个窗口不会绕开另一条轴。
- **限流回补改为纯函数**：新增 `fillTokens(bucket, now, capacity, interval)`，不读时间源、不加锁。回补速率、容量钳制、时钟回拨防护与「令牌是连续量不取整」全部可直接单测，无需假时钟推进。删除生产零调用点的公开 `sweep`。
- **静态资源存在性判定与内容读取合一**：删除 `assetExists` 预检——缓存命中即证明资源存在，未命中才读盘一次。修复前每次请求都执行 `Open`+`Stat`，缓存命中时仍有 2 次文件系统调用。
- **竞态骨架收敛**：新增私有 `perform(id, q, limit, offset)`，收敛「监听中止 → 发起请求 → 判定当前性 → 清理监听」骨架。`submit` 传新会话 id、`loadMore` 传当前会话 id，两方法体不再逐行同构（净减 11 行），判别联合构造点单点化。
- **交互语义收口**：新增 `useSearchInput`，输入法组合守卫、Enter 提交、Escape 清空从 `App.tsx` 的 JSX 事件回调收进一处。该逻辑此前内联在组件里且零测试覆盖；现由 7 条挂载测试锁住运行时行为，页面接线由 TypeScript 在编译期锁住。

### 测试

- 前端从 114 增至 **121 用例**：新增 `useSearchInput.test.tsx`（6 条交互语义 + 1 条 hook 组合链路）。
- 后端新增 `fillTokens` 四条纯函数测试、探测节流与冷却窗口的行为测试、`countingFS` 静态缓存零触碰测试。
- **变异测试剔除零承重的推测性防御**：`useSearchInput` 初版额外加入的本地 `composing` ref 经变异验证无任何测试承重（原 `App.tsx` 也没有该装置），已删除。同批修正三处自身写错的测试断言（`recoveryDue` 是纯判定不推进状态、`retry` 布尔语义方向、`cooling` 返回值方向），均由变异验证暴露。

## [v0.7.1] - 2026-09-26

### 修复

- **`Search` 负 offset 崩溃**：分页前置约定（`offset ≥ 0`）此前只由 `buildMux` 的 HTTP 层校验兜底，`Search` 自身未声明也未自守——直接调用 `Search(students, q, 10, -3)` 触发 `panic: slice bounds out of range [-3:]`。现按既有越界钳制的同一形状在 `Search` 内把 offset 归一到 `[0, len(matches)]`，越界按最近有效边界处理。归一逻辑零分配，分配次数保持 6/7/8（预算 12）。HTTP 层 400 契约不变。

### 架构深化

- **状态派生归位 reducer**：`App.tsx` 的 `submit` 每次手工串联 `getState → hasNameCondition → statusTextFor → dispatch`，失败分支再串一次 `errorMessage`；派生规则无测试覆盖，把 `statusTextFor` 传错位置不会有任何测试失败。现 `submit-success` 载荷收窄为 `{ items, total, hasMore, query }`、`submit-error` 收窄为 `{ cause }`，状态与文案由 reducer 内部依既有纯函数算出。`App.tsx` 删除随之孤立的四个导入，`submit` 由 15 行缩为 9 行。
- **API 路由独立成模块**：新增 `server/api.go` 承载 `buildMux` 与 `searchHandler`，把 HTTP 翻译（参数取值、错误码映射、JSON 写出）与查询语义（`search.go` 独占）分处两个文件。`main.go` 只留启动自举、日志、中间件与装配四个薄角色，281 → 222 行。
- **数据层 fixture 归位**：`newTestStore` 从 `main_test.go` 迁至 `data_test.go`——学生数据构造归属数据模块，`chain_test`/`version_test`/`main_test` 跨文件复用不受影响。`validGradeOne`/`validGradeTwo` 留在 `main_test.go`：它们是 API 响应语料而非数据层 fixture。

### 测试

- 前端从 87 增至 **91 用例**：新增 4 个编排行为测试（success / empty / F36 纯年段提示 / 429 错误分类），首次直接锁定"响应 → 状态与文案"的完整派生链。
- 删除 `TestStatusRecorderImplementsFlusher` 与 `TestStatusRecorderImplementsReaderFrom`——二者只断言实现满足 interface，从不调用 `Flush` 或验证字节搬运，interface 约等于 implementation。替换为经 `accessLog` 真实路径的行为断言：`Flush` 透传至下游且 `rec.Flushed` 为真、`ReadFrom` 搬运 10 字节且下游正文完整一致。
- 新增 `TestSearchNegativeOffsetTreatedAsFirstPage` 锁定负 offset 行为。

### 决策

- **ADR-0001：Student 不拆分为内部模型与 wire DTO**。隐私已由 `json:"-"` 标签类型强制（`NameKey`/`ClassNo`/`GradeIdx`），并由 `TestSearchResponseKeys` 逐一断言六个内部字段名不出现在响应中；`newStudent` 纪律无绕过路径。实测 DTO 转换每请求固定新增 1 次分配，收益为重复保证已有测试所保证的事项。附明确重启条件，见 `docs/adr/0001-student-type-not-split.md`。

## [v0.7.0] - 2026-09-25

### 架构深化（五候选一次性落地）

- **删除生产死代码**：移除唯一调用方为测试的 `snapshot()` 与纯死代码 `probeThrottledAt()`；测试改走零拷贝 `view()`，热重载/并发/错误恢复断言等价，只读视图不变量由 `TestStoreViewNoCopy` 接管。
- **收敛跨端契约常量**：后端新增 `server/config.go`（端口、分页默认/上限、查询长度上限、限流参数、缓存头）、前端新增 `src/config.ts`（`PAGE_SIZE`/`MAX_QUERY_LENGTH`/`REQUEST_TIMEOUT_MS`），双端各一份、改动须同步两侧。
- **统一客户端 IP 解析**：新增 `server/ip.go` 作为 IP 解析唯一入口，日志脱敏与限流共享同一 host 视图；`maskedIP` 对 IPv6 从 `unknown` 改善为保留前两组脱敏，并用归一化 IP 做分割（IPv4-mapped 不再混入映射前缀、畸形输入保守降级 `unknown`，绝不泄露完整地址）。
- **查询语义第三拷贝归零**：`App.tsx` 内嵌的 `hasNameCondition` 正则移入 `query.ts` 复用 `parseQuery`，F36 纯年段/班级提示改由解析结果驱动——查询语义只剩 `query.ts` 与 `search.go` 两份镜像实现。
- **拆解 App 状态机与竞态编排**：新增 `src/lib/searchReducer.ts`（纯 reducer，11 个 action 覆盖状态派生/错误文案/F36 提示/IME 组合）与 `src/lib/searchSession.ts`（竞态编排：requestId 递增 + 真实 abort 传导 + `invalidate` 使 clear/onChange/IME 失效在途请求）；`App.tsx` 从 204 行降至 123 行，错误文案补齐 code 维度。逐 Task 审查发现并修复 clear/输入修改后在途响应被应用的竞态回归。

### 测试

- 前端测试从 25 增至 **52 用例**（query 18 / api 13 / searchReducer 11 / searchSession 10），覆盖状态机转换、竞态丢弃、真实 abort 传导、错误分类；零新依赖、不引入 jsdom。
- 后端新增 IP 解析边界测试（IPv4-mapped 归一、畸形输入降级）。

## [v0.6.0] - 2026-09-24

### 性能优化

- **搜索核心链路重构（F73）**：预计算班级号与年段序（`ClassNo`/`GradeIdx`），排序热路径从正则 + 线性扫描降级为纯整数比较；`normalizeName` 增加零分配快路径（姓名已归一化时复用原串），归一化查询表提为包级常量；结果切片按预估容量预分配，排序改用无反射泛型实现。整年段查询从 819µs/1762 次分配降至 34.8µs/7 次（约 23 倍）；单姓名查询从 25.5µs/77 次分配降至 15.9µs/8 次。
- **文件指纹探测节流与零拷贝视图（F72）**：文件指纹探测引入 1 秒节流窗口（热重载真实场景为管理员手工替换名单，1 秒延迟无感），消除每请求 3 次 `os.Stat` 系统调用；新增 `view()` 零拷贝只读视图（reload 整体替换切片、从不就地修改，旧切片读者不受影响），每次请求 134KB 的全量拷贝归零（443µs → 17.9ns、0 分配）。
- **移除写死的 1000ms 最短搜索延迟**：搜索不再等人为设置的 1 秒窗口，响应返回即渲染；`searchTiming.ts` 及其测试整体删除，`StatusOrb` 加载反馈保留（网络真实慢时仍显示）。
- **基准与回归防线（F74）**：新增 `server/bench_test.go`（单姓名 / 整年段 / 组合查询基准 + 视图路径）与 `src/lib/query.bench.ts`（前端镜像基准）；分配次数硬预算断言（`TestSearchAllocsBudget`，预算 12 次）作为机器无关的确定性回归锁——耗时随负载抖动，分配次数不会，优化一旦退化测试立即失败。

### 修复

- **Docker 镜像版本号恒为 dev**：`release.yml` 的 Docker job 此前未向 `docker/build-push-action` 传入 `build-args`，`Dockerfile` 中 `ARG VERSION=dev` 永远使用兜底值，导致容器内 `/api/health` 的 version 恒为 dev（页面页脚随之显示 dev）。现已补传 `build-args: VERSION=${{ github.ref_name }}`，tag 触发构建时镜像内版本与发布 tag 一致。

## [v0.5.5] - 2026-09-11

### 新增

- **页脚显示版本号**：页脚新增"版本"行，展示运行时版本（由 Go 二进制 ldflags 注入、`/api/health` 返回的同一版本，本地构建显示 dev）。版本获取失败时静默隐藏，不影响页面。

## [v0.5.4] - 2026-09-11

### 修复

- **年级+班级连写查询不精确**：此前「高二一班」「高二1班」「高二三班」等输入只解析出年段（高二），返回整个高二年段而非对应班级的人。现在由 gradeClassToken 优先精确解析为年段+班级组合，前后端（src/lib/query.ts 与 server/search.go）同步修复。

## [v0.5.3] - 2026-09-10

### 新增

- **高三年段支持**：系统支持的年段改为按数据目录实际存在的文件动态探测（高一/高二/高三自由组合）。只有 `data/高一.json`、`data/高二.json`、`data/高三.json` 中存在的文件会被加载，缺失文件自动跳过不报错；三者为空时给出明确指引。前后端查询逻辑、排序权重、口语化解析（"高三三班"）均已同步对齐。

### 修复与体验优化

- **全员果冻动画**：移除结果列表中 `index < 10` 的动画限制，所有卡片（含"继续加载"追加的条目）均启用 `Liquid.Item` morph 形变动画，视觉效果全场一致。
- **宽度坍塌修复**：`liquid-gooey` 库在无 morph 时内部容器强制 `display: inline-block`，导致第 11 条起卡片宽度缩减至约一半。通过全员启用 morph（容器切换为 `display: contents`）并在 `.liquid-result-item` 补充 `width: 100%` 双重防御，彻底根除该缺陷。
- **页脚文案精简**：移除隐私说明与页脚中的"联系方式：学校教务处"字样。
- **站点信息可配置**：新增 `src/site.config.ts`，数据来源、运营团队、数据处理方等页脚展示文案可通过配置文件修改，默认值为福清一中信息社。

## [v0.5.2] - 2026-09-07

### 修复与兼容性加固

- **前端跨端兼容**：为 `src/lib/api.ts` 添加轻量 `combineSignals` 兼容逻辑，彻底解决 Safari < 17.4（iOS 17.3 及以下）与旧版移动端 WebView 缺少 `AbortSignal.any` 导致的 `TypeError` 崩溃问题。
- **限流器内存防泄漏**：`server/ratelimit.go` 新增自驱动惰性周期清理机制，在请求并发时自动淘汰超过空闲寿命（10分钟）的过期 IP 令牌桶，根除内存单调递增风险。
- **数据热重载防惊群**：`server/data.go` 引入 `reloadMu` 双重检查互斥锁（Double-Checked Locking），高并发请求下热重载只触发一次磁盘读取与 JSON 反序列化，彻底消除重载击穿与资源峰值。
- **静态资源 MIME 精确映射**：`server/web.go` 针对自托管 `.woff2` 字体显式返回 `font/woff2`，避免精简容器环境回退到嗅探器将其误识别为 `text/plain` 或 `application/octet-stream`。
- **样式与 CI 规范对齐**：修复 `src/styles.css` 中 `--ease-move` 变量自我循环引用问题；修复 `.github/workflows/ci.yml` 的 Node.js 24 描述文案，并在后端测试中补齐 `-race` 数据竞争检测。

## [v0.5.1] - 2026-09-05

### 修复

- 修复 Go 自定义静态资源缓存路径遗漏 MIME 类型的问题：JavaScript 资源返回 `application/javascript`，CSS 资源返回 `text/css; charset=utf-8`，保留 gzip、ETag、304 与长期缓存。

## [v0.5.0] - 2026-09-02

### 重大变更（隐私与发布策略）

- **发布包零数据**：真实名单（data/*.json）不再随仓库与发布包分发（F42 落实），校内部署时自行放置；CI 增加零数据守卫（仓库中出现名单文件即失败）
- 真实名单移出 git 跟踪并加入 .gitignore（data/*.json），git 历史将不再包含任何名单数据

### 安全与加固

- 修复 CI 数据契约 job 的变量作用域 bug（A3 班级人数检测移入循环内），班级人数区间调整为 10-60（适配 15-19 人选科小班）
- 修复 5 个 Go 文件的 gofmt 格式问题（CI gofmt 检查恢复全绿）
- 新增单实例边界文档（ARCHITECTURE + README）：限流桶与数据快照为进程内状态，不支持多副本

### 修复

- F13：移动端胶囊高度变量化（--control-size/--track-pad），消除 48px 按钮与 72px 轨道的 8px 错位
- F41：仓库卫生清理（go.work、release-body.md、design 源图、评审文档、%TEMP% 等全部移出仓库）
- F49：单实例边界文档化

### 变更

- .gitignore 深度完善（16 类规则）、.dockerignore 同步
- README 更新：发布零数据说明、班级人数区间、数据维护流程
- 移除评审基线文档（.superpowers/），评审已完成全部落实

## [v0.4.0] - 2026-08-30

### 性能优化

- 静态资源缓存：/assets/ 与 /fonts/ 设置 Cache-Control: public, max-age=31536000, immutable（哈希命名文件永不失效，二次访问零下载）
- 动效库按需加载：liquid-gooey 拆为 ResultList chunk（53KB）、thinking-orbs 拆为 StatusOrb chunk（15KB），主包从 287KB 降至 220KB
- vendor chunk 拆分：React 全家拆为 vendor-react（143KB）、border-beam 拆为 vendor-beam（65KB），主包进一步降至 12.6KB
- 渲染热路径：状态球仅 loading 渲染、结果 morph 动画仅首屏 10 条、加载更多禁用入场动画
- 字体自托管：5 个 woff2 本地化（93KB），移除 Google Fonts 外部依赖，CSP 同步收紧
- 修复 vite.config.js 遮蔽 vite.config.ts 的构建配置失效问题

## [v0.3.1] - 2026-08-29

### 变更

- GitHub Actions 升级 Node 24（消除 Node 20 弃用警告，vite 6 引擎要求满足）

## [v0.3.0] - 2026-08-29

### 安全加固

- 完整安全响应头：Content-Security-Policy（default-src 'self'，放行 Google Fonts）、X-Frame-Options: DENY、Referrer-Policy: no-referrer、Permissions-Policy 禁用敏感 API
- 按 IP 令牌桶限流（60 次/秒/IP，超出返回 429 + Retry-After）
- HTTP 超时配置（ReadHeaderTimeout 5s / ReadTimeout 10s / WriteTimeout 15s / IdleTimeout 60s）防慢速攻击
- 升级 vite 至 6.4.3、vitest 至 3.2.7，消除全部已知依赖漏洞（npm audit 0 漏洞）

### 新增

- Docker 镜像发布流水线：release 打 tag 后自动构建并推送 wenxiloveyou/find-my-classmate（含 Docker Hub PAT 配置说明）

### 变更

- 搜索框聚焦滚动位置调整：从视口垂直中心改为中上方（scroll-margin-top 18vh）
- 移除 hero 副标题与无主样式

## [v0.2.0] - 2026-08-29

### 新增

- 启动自举：幂等创建数据目录与日志目录；数据文件缺失时给出明确指引并退出
- 日志系统完善：文件 + stdout 双写、FMC_LOG_LEVEL 分级、请求访问日志（方法/路径/状态/耗时/脱敏 IP）
- Docker：显式创建数据与日志目录、compose 增加 healthcheck
- 架构文档 docs/ARCHITECTURE.md（唯一架构指引，永不失效设计）
- 页脚 GitHub 仓库链接（含 octocat 图标）
- 搜索框聚焦时平滑滚动到视口垂直中心

### 变更

- 浏览器标签页标题只保留品牌名 FindMyClassmate
- 移除顶栏"学生档案检索台 / 2025"与页脚"高一 / 高二最新数据 · Go 单体服务"冗余文案
- hero 区域整体上移（顶部内边距收紧）
- 文案更新：介绍语改为"支持福清一中高一高二名单"，示例改用假名并说明分隔方式

### 修复

- 前端产物递归嵌入（//go:embed all:web，含 assets 子目录）
- CI 后端 job 缺失前端产物导致 embed 失败
- CI 数据契约校验与实际名单 JSON 结构不符
- Release 流水线 workflow 复用导致的 0s 失败

## [v0.1.0] - 2026-08-29

### 新增

- React + TypeScript + Vite 前端与 Go 标准库服务
- 胶囊搜索框 + border-beam 彩色光束、thinking-orbs 思维球、liquid-gooey 液体结果行
- 福清一中高一高二名单查询：姓名 / 班级 / 年段组合
- 分页加载、输入法兼容、Escape 清空、结果自动滚动
- 单二进制内嵌前端，单端口 3078
- Docker 多阶段构建、CI / Release 流水线、MIT License
