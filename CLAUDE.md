# FindMyClassmate 项目记忆

## 当前交付

- 交付物：React + TypeScript + Vite 前端和 Go 标准库查询服务。
- 前端入口：src/main.tsx；页面组件：src/App.tsx；查询模块：src/lib/query.ts、src/lib/api.ts；搜索时序：src/lib/searchTiming.ts。
- 后端入口：server/main.go；数据归一化：server/data.go；查询模块：server/search.go。
- 设计基准：DESIGN.md。当前页面采用黑曜画布、光谱白、灰阶边线、工业 HUD 标签和克制的单色动效。
- 前端构建后嵌入 Go 服务；Go 服务统一托管页面和 /api，不提供数据源切换。

## 隐私与数据边界

- data/高一.json 和 data/高二.json 是唯一名单数据源，内容按年级、班级和姓名去重。
- API DTO 只返回 name、grade、class，不读取、不存储、不返回学籍辅号。
- 页面不写入 localStorage，不把查询词或结果写入 URL；生产部署仍需配置 IP 限流、严格 CORS 和脱敏日志。

## 查询契约

- 中文逗号、英文逗号、顿号和连续空白均可分隔查询 token。
- 姓名匹配键删除中英文空格、全角空格和制表符，并统一大小写。
- 高一、高二是年段筛选；18 或 18班可作为班级条件；结果按完整、前缀、包含匹配和自然班级顺序排列。
- API 响应固定为分页结构 { items, total, limit, offset, hasMore }；首屏默认 10 条，单次最多 50 条。

## 动效与字体规范

- border-beam 1.3.0：只包裹 src/App.tsx 的搜索轨道，使用 size="md"、borderRadius=999（胶囊全边框）、colorVariant="colorful"（彩色光束）、theme="dark"、brightness=1.6、saturation=2.6、hueRange=180（半彩虹流动）、duration=2.1；辉光半径紧凑（14-26px），色彩浓艳而光雾收敛；光束无条件常转常亮（默认 active=true），strength 恒为 1（拉满）。搜索轨道为胶囊圆角（999px），常驻彩色环境辉光（box-shadow 蓝+粉双层），focus-within 时增强，虚焦后光晕不消失；移动端保持单行布局。
- thinking-orbs 0.3.1：src/App.tsx 的状态区和 loading 结果区使用 size=64（库内最大预设）；loading 使用 searching，完成后使用暂停的 solving，并提供 aria-label。
- liquid-gooey 0.2.1：src/App.tsx 的结果列表使用 Liquid 根节点和 Liquid.Item；结果行采用 morph shape、speed 0.85、bounce 0.3、contentBlur 0，文字保持清晰。
- 视觉字体：Mona Sans 用于标题，Instrument Sans 用于正文和中文混排，IBM Plex Mono 用于字段、编号、状态和数据标签；均通过 CSS 字体栈引入并保留系统回退。
- 搜索由回车或按钮立即触发，src/lib/searchTiming.ts 保证 loading 至少 1000ms；结果状态更新后平滑滚动到结果区域。
- 搜索提交按钮为圆形发送图标（.search-send，内嵌 SVG 纸飞机），无文字文本；输入框为 type="text" 且自身不画焦点 outline（.search-input:focus-visible { outline: none }），焦点指示由 .search-track:focus-within 的整圈胶囊高亮承担，左右两端均为圆角；输入法组合期间 Enter 不提交，Escape 清空；首屏请求取消旧请求，加载更多只追加结果且不改变阅读位置。
- 页脚小字标注数据来源（福清一中公示数据提取）与运营团队（福清一中信息社），footer-stack 纵向 10px mono 小字。
- 三个库与 CSS 动效均受 prefers-reduced-motion: reduce 控制。

## 运行方式

1. npm install
2. npm run build
3. go run ./server
4. 浏览器打开 http://localhost:3078

Go 服务默认从 data/高一.json 和 data/高二.json 读取；可用 FMC_DATA_DIR 指向其他数据目录，数据文件变更后下一次请求自动热重载，日志写入 data/log/server.log。

## 验证命令

- npm run typecheck
- npm test
- npm run build
- 在 server 目录执行 gofmt -w *.go、go test ./...、go vet ./...
- 静态检查不应发现 localStorage、外部图片热链、完整号码或重复 data-od-id。

## 当前文件边界

- design/logo-raw.png：AI 生成的原始 logo 源图（1024px PNG，不入构建）。
- public/logo.webp（512px，7.7KB）与 public/favicon.png（64px）：顶栏标记与浏览器标签图标，Vite 构建时拷贝到 server/web 根目录。
- DESIGN.md：既有视觉参考，不改动。
- src/：React 页面、样式、API 适配和查询测试。
- server/：Go 服务、数据解析和后端测试。
- docs/superpowers/plans/2026-05-12-black-liquid-archive-search.md：本次黑白液体档案检索界面的实施计划。
- package.json / package-lock.json：固定前端依赖和脚本。
- README.md：启动、验证、目录与查询契约。
- CLAUDE.md：本项目长期记忆。

## 变更记录

- 2026-05-12：建立黑白液体档案检索界面实施计划。
- 2026-05-12：接入最短 500ms 搜索反馈、输入法兼容、结果自动滚动、Liquid.Item 结果行与 Mona Sans / Instrument Sans / IBM Plex Mono 字体栈。
- 2026-05-12：从单文件原型升级为完整 React + TypeScript + Go 工程；真实接入 border-beam、thinking-orbs、liquid-gooey。
- 2026-05-12：搜索框重构为胶囊造型：BorderBeam 改用 size="md" 全边框彩色光束（borderRadius=999、brightness=1.6）常转常亮，搜索轨道圆角化，文本按钮替换为圆形发送图标，输入框焦点态去除方框竖线改为整圈胶囊高亮，移动端改单行布局。
