# 更新记录

本文件记录 FindMyClassmate 的版本变更。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

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
