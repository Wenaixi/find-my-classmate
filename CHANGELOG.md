# 更新记录

本文件记录 FindMyClassmate 的版本变更。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

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
