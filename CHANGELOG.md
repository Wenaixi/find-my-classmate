# Changelog

本项目所有值得记录的变更都记录在这个文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，本项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Added

- 多校数据模型：`data/*.json` 按「区县 > 学校 > 年级 > 班级 > 学生」加载，学校身份取自文件内 `校名`。
- `Student` 新增 `school` 字段，`/api/search` 支持 `school` 过滤，排序加入学校层。
- 新增 `GET /api/schools`，只列含高中年级的学校。
- 前端学校筛选胶囊下拉。

### Changed

- 不再支持旧格式 `{"标题":...,"名单":...}`（不兼容），旧数据移至 `data/_legacy/`。
- API 返回字段为 `name/school/grade/class`。
- 前端依赖升级：React 19.3.1、TypeScript 7.0.2、Vite 8.3.8、Vitest 5.1.0、@vitejs/plugin-react 6.3.0、jsdom 31.0.1、动效库 1.4.1 / 0.2.2 / 0.3.2。
- Vite 8 换用 Rolldown：`rollupOptions` 改为 `rolldownOptions`，`manualChunks` 改为 `output.codeSplitting.groups`。
- 运行前置版本提升（不兼容）：Node 26 LTS、Go 1.27.4。
- Dockerfile：node:26-alpine、golang:1.27-alpine、alpine:3.24.4。
- CI/Release：Node 26、Go 1.27.4，actions 升至当前主版本，顶层权限收为 `contents: read`。
- CI 增加 `npm audit --audit-level=high` 门禁。
- README 更新技术栈、徽章与「部署安全边界」章节。
- Cloudflare 迁移设计稿同步当前基线。

### Security

- `/api/health` 补方法限定：非 GET 返回 405 + `Allow: GET`。
- 日志文件权限 0644 → 0600。
- 依赖漏洞清零：tinypool 与 launch-editor 漏洞随大版本升级消失，`npm audit` 报 0。

## [v0.11.0] - 2026-10-01

### Changed

- 前端降级策略收进 `classCondition`，班级判定改显式三态。
- `ResultList` 的 `hasMore`/`loadingMore`/`loadMoreError` 收进 `present` 的 `loadMoreZone`，props 从六个减到四个。
- `submit` 与 `loadMore` 共用私有 `run` 骨架，start/result 配对由类型强制。
- 删除 `classDigits` 恒不发作的键集检查。
- `logging.go` 与 `middleware.go` 从 `main.go` 独立，`main.go` 227 → 88 行。
- 年段值域反向完整性改由编译期检查承担（`as const satisfies`）。
- 状态全集导出为 `searchStateValues`，供测试消费。
- 分配预算补四条解析分支样本。
- 服务器超时提为具名常量，实现与测试共用。
- act 环境声明收敛为共享模块。
- 文档更正：五处代码注释失真、三处架构文档失真、两处记忆文档失真。
- 测试：前端 198 → 206 用例。

### Removed

- `submit-success` 与 `submit-error` 两个零派发的死分支。
- 以下候选经实测撤销并记录，避免重复提出：token 蒸发、安全头 `Cache-Control` 冲突、XFF adapter、展示 module 拆分、静态资源协商拆分、`searchApi` signal 归属。

### Fixed

- 输入法组合期的输入变化让界面永久卡死：stale 响应没有分支释放 loading。
- 加载界面从未渲染：Promise 型动作没有分两段重渲染。
- 「高一鸣」这类含年段串的姓名被读成年段，精确检索放大成整年段全量。
- 跨语言镜像在「年段+溢出班级」形态上分叉。
- 名单加载四个失败出口不带年段标识，运维无法定位文件。
- 数据不可用的根因在 `/api/health` 完全不可见（日志 0 条）。
- 错误区段在未知错误形态下重复标题文案。
- inert 修复只覆盖 gooey 两层装饰 SVG 中的一层。
- 服务器初始化日志失败时没有可行动指引。
- 状态行 chunk 加载失败带走整页，缺 `ErrorBoundary`。
- 年段值域对拍的抽取器缺行首锚定，注释可冒充声明。
- 分配预算与 `-race` 插桩环境错配，现按构建标记跳过。

## [v0.10.4] - 2026-09-27

### Changed

- 架构核实：`studentStore` 保持 deep；不引入没有第二实现的日志 interface；`MAX_LIMIT` 与 `rebuildGradePattern` 保留。

### Fixed

- `TestSearchDataUnavailable` 不再读取 `studentStore` 私有字段，改由 fixture 返回目录。

## [v0.10.3] - 2026-09-26

### Changed

- wire 参数三条 400 路径进入受测接缝，并配合法边界对照。
- 展示派生收进 `present` 单点。
- 清理 `newStaticCache`、错误的 Vary 行号引用、零引用 CSS 规则；修正两处工作流步骤名。
- 测试：前端 135 → 142 用例；变异六次精确翻红；一次未翻红已记录原因。

### Fixed

- 年段扩展路径断裂：标题校验与查询各自硬编码年段，新增年段后合法名单被拒。
- `gradeClassToken` 每次调用重编译正则，整年段查询分配 6 → 117。
- CI 格式化检查失败：11 个文件的 gofmt 差异（对齐与行尾两类）已规范化。
- 镜像构建报 TS2307：`.dockerignore` 排除测试文件。

## [v0.9.3] - 2026-09-26

未以 tag 发布，随 v0.10.3 发布。

### Changed

- 时钟改为构造注入，`now` 不再作为可写字段暴露。
- 探测节流与失败冷却收敛为 `recoveryDue(now)` 单点。
- 限流回补改为纯函数 `fillTokens`，删除生产零调用的 `sweep`。
- 删除 `assetExists` 预检，缓存命中即证明资源存在。
- 竞态骨架收敛为私有 `perform`，净减 11 行。
- 新增 `useSearchInput`，IME 守卫、Enter 提交、Escape 清空收进一处。
- 测试：前端 114 → 121 用例；剔除零承重的 `composing` ref；修正三处写错的断言。

### Fixed

- 数据目录缺失时冷却永不成立：`lastFailStamps` 为 nil 使判定恒 false，现以 `lastFailStampKnown` 区分。

## [v0.7.1] - 2026-09-26

未以 tag 发布，随 v0.10.3 发布。

### Changed

- 状态派生归位 reducer，`submit` 由 15 行缩为 9 行。
- 新增 `server/api.go`，`main.go` 281 → 222 行。
- `newTestStore` 迁至 `data_test.go`。
- 测试：前端 87 → 91 用例；`statusRecorder` 两条 interface 断言换成行为断言。
- ADR-0001：Student 不拆 wire DTO，附重启条件。

### Fixed

- `Search` 负 offset 触发 `slice bounds out of range`，现归一到 `[0, len(matches)]`。

## [v0.7.0] - 2026-09-25

### Changed

- 删除 `snapshot()` 与 `probeThrottledAt()` 两处死代码。
- 新增 `server/config.go` 与 `src/config.ts` 收敛跨端常量。
- 新增 `server/ip.go` 作为 IP 解析唯一入口，IPv6 脱敏保留前两组。
- `hasNameCondition` 从 `App.tsx` 移入 `query.ts`。
- 拆出 `searchReducer.ts`（11 个 action）与 `searchSession.ts`（requestId + abort），`App.tsx` 204 → 123 行。
- 测试：前端 25 → 52 用例，零新依赖。

## [v0.6.0] - 2026-09-24

### Changed

- 搜索热路径预计算 `ClassNo`/`GradeIdx`，整年段查询 819µs/1762 次分配降至 34.8µs/7 次，单姓名 25.5µs/77 次降至 15.9µs/8 次。
- 文件指纹探测加 1 秒节流，新增 `view()` 零拷贝视图，134KB 拷贝归零（443µs → 17.9ns）。
- 移除写死的 1000ms 最短搜索延迟，删除 `searchTiming.ts`。
- 新增 `bench_test.go`、`query.bench.ts` 与分配预算断言（预算 12）。

### Fixed

- Docker 镜像版本号恒为 dev：Docker job 补传 `build-args: VERSION=${{ github.ref_name }}`。

## [v0.5.5] - 2026-09-11

### Added

- 页脚显示运行时版本号，获取失败时静默隐藏。

## [v0.5.4] - 2026-09-11

### Fixed

- 「高二一班」这类年级+班级连写只解析出年段，现由 `gradeClassToken` 精确解析，前后端同步。

## [v0.5.3] - 2026-09-10

### Added

- 高三年段支持：年段按数据目录实际存在的文件动态探测。

### Fixed

- 移除结果列表 `index < 10` 的动画限制。
- `liquid-gooey` 无 morph 时卡片宽度坍塌，补 `width: 100%`。
- 移除「联系方式：学校教务处」文案。
- 新增 `src/site.config.ts`，页脚文案可配置。

## [v0.5.2] - 2026-09-07

### Fixed

- `api.ts` 补 `combineSignals`，修复 Safari < 17.4 缺少 `AbortSignal.any` 的崩溃。
- 限流器增加惰性清理，淘汰 10 分钟无请求的桶。
- 热重载加 `reloadMu` 双重检查锁，防惊群。
- `.woff2` 显式返回 `font/woff2`。
- 修复 `--ease-move` 自我循环引用；修正 CI 的 Node 版本描述；后端测试补 `-race`。

## [v0.5.1] - 2026-09-05

### Fixed

- 静态资源缓存路径遗漏 MIME 类型：JS 返回 `application/javascript`，CSS 返回 `text/css; charset=utf-8`。

## [v0.5.0] - 2026-09-02

### Changed

- 新增单实例边界文档：限流桶与数据快照是进程内状态。
- 完善 `.gitignore` 与 `.dockerignore`；README 补发布零数据与数据维护说明。
- 移除评审基线文档。

### Fixed

- CI 数据契约 job 变量作用域 bug；班级人数区间调整为 10-60。
- 修复 5 个 Go 文件的 gofmt 问题。
- 移动端胶囊高度变量化，消除 8px 错位。
- 仓库卫生：`go.work`、`release-body.md`、design 源图、`%TEMP%` 等移出仓库。

### Security

- 真实名单（`data/*.json`）不再随仓库与发布包分发，CI 增加零数据守卫。

## [v0.4.0] - 2026-08-30

### Changed

- `/assets/` 与 `/fonts/` 设 immutable 长缓存。
- 动效库与 React 拆 chunk，主包 287KB → 12.6KB。
- 渲染热路径收敛：状态球仅 loading 渲染，morph 动画仅首屏 10 条。
- 字体自托管，移除 Google Fonts 依赖，CSP 同步收紧。
- 修复 `vite.config.js` 遮蔽 `vite.config.ts`。

## [v0.3.1] - 2026-08-29

### Changed

- GitHub Actions 升级 Node 24。

## [v0.3.0] - 2026-08-29

### Added

- Docker 镜像发布流水线：打 tag 后自动构建并推送。

### Changed

- 搜索框聚焦滚动位置改为中上方（`scroll-margin-top: 18vh`）。
- 移除 hero 副标题与无主样式。

### Security

- 完整安全响应头：CSP、`X-Frame-Options`、`Referrer-Policy`、`Permissions-Policy`。
- 按 IP 令牌桶限流，60 次/秒/IP，超出返回 429 + `Retry-After`。
- HTTP 读写超时配置，防慢速攻击。
- 升级 vite 6.4.3、vitest 3.2.7，`npm audit` 0 漏洞。

## [v0.2.0] - 2026-08-29

### Added

- 启动自举：幂等创建数据与日志目录，数据缺失时给出指引。
- 日志系统：文件 + stdout 双写、`FMC_LOG_LEVEL` 分级、访问日志（脱敏 IP）。
- Docker 补 healthcheck 与目录创建。
- `docs/ARCHITECTURE.md`。
- 页脚 GitHub 链接；搜索框聚焦滚动。

### Changed

- 标签页标题只保留品牌名。
- 移除顶栏与页脚冗余文案，hero 上移，示例改用假名。

### Fixed

- 前端产物递归嵌入（`//go:embed all:web`）。
- CI 后端 job 缺前端产物导致 embed 失败。
- CI 数据契约校验与实际 JSON 结构不符。
- Release workflow 复用导致的 0s 失败。

## [v0.1.0] - 2026-08-29

### Added

- React + TypeScript + Vite 前端与 Go 标准库服务。
- 胶囊搜索框、border-beam、thinking-orbs、liquid-gooey 动效。
- 福清一中高一高二名单查询：姓名 / 班级 / 年段组合。
- 分页加载、输入法兼容、Escape 清空、结果自动滚动。
- 单二进制内嵌前端，单端口 3078。
- Docker 多阶段构建、CI / Release 流水线、MIT License。

[Unreleased]: https://github.com/Wenaixi/find-my-classmate/compare/v0.11.0...HEAD
[v0.11.0]: https://github.com/Wenaixi/find-my-classmate/compare/v0.10.4...v0.11.0
[v0.10.4]: https://github.com/Wenaixi/find-my-classmate/compare/v0.10.3...v0.10.4
[v0.10.3]: https://github.com/Wenaixi/find-my-classmate/compare/v0.6.0...v0.10.3
[v0.6.0]: https://github.com/Wenaixi/find-my-classmate/compare/v0.5.5...v0.6.0
[v0.5.5]: https://github.com/Wenaixi/find-my-classmate/compare/v0.5.4...v0.5.5
[v0.5.4]: https://github.com/Wenaixi/find-my-classmate/compare/v0.5.3...v0.5.4
[v0.5.3]: https://github.com/Wenaixi/find-my-classmate/compare/v0.5.2...v0.5.3
[v0.5.2]: https://github.com/Wenaixi/find-my-classmate/compare/v0.5.1...v0.5.2
[v0.5.1]: https://github.com/Wenaixi/find-my-classmate/compare/v0.5.0...v0.5.1
[v0.5.0]: https://github.com/Wenaixi/find-my-classmate/compare/v0.4.0...v0.5.0
[v0.4.0]: https://github.com/Wenaixi/find-my-classmate/compare/v0.3.1...v0.4.0
[v0.3.1]: https://github.com/Wenaixi/find-my-classmate/compare/v0.3.0...v0.3.1
[v0.3.0]: https://github.com/Wenaixi/find-my-classmate/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/Wenaixi/find-my-classmate/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/Wenaixi/find-my-classmate/releases/tag/v0.1.0