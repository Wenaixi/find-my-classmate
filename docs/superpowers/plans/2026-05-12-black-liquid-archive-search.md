# 黑白液体档案检索界面实施计划

> **面向 Agent 执行者：** 必需子技能：使用 superpower-executing-plans 按任务逐项执行本计划。步骤使用复选框（`- [ ]`）语法进行跟踪。

**目标：** 基于 DESIGN.md 将 FindMyClassmate 重构为黑白、高级艺术化的档案检索界面，并深度接入 border-beam、thinking-orbs、liquid-gooey，完善回车搜索、立即请求、至少 500ms 搜索反馈、结果自动定位、连续分页、移动端适配与可访问性。

**架构：** 保留现有 React + TypeScript + Vite + Go 查询服务和 SearchResponse 契约。新增一个可测试的最短加载时序纯函数；React 组件负责请求竞态、输入法状态、滚动、清空和动效状态映射；CSS 负责黑白设计系统、Liquid 稳定尺寸、弹性进入、边界光束和响应式布局。Liquid.Item 只包装真实结果行，确保液体轮廓与清晰 DOM 内容分层。

**技术栈：** React 18、TypeScript、Vite、Vitest、border-beam 1.3.0、liquid-gooey 0.2.1、thinking-orbs 0.3.1、CSS @font-face / transitions / keyframes；不增加运行时依赖，不引入 WebGL 或外部图片。

**规格：** 根目录 `DESIGN.md`；项目长期约束记录在根目录 `CLAUDE.md`。

## 全局约束

- 画布采用纯黑层级、光谱白和灰阶，不延续原有米灰、橙色和薄荷色主视觉。
- 标题使用 Mona Sans，正文和中文混排使用 Instrument Sans，检索字段与状态标签使用 IBM Plex Mono；字体不可用时使用系统回退。
- border-beam 只包裹搜索轨道，使用 `size="line"`、`colorVariant="mono"`、`theme="dark"`，请求期间提高强度并启用 active。
- thinking-orbs 在请求期间使用 searching，成功后短暂使用 solving 并暂停，尺寸固定为 20，并提供 aria-label。
- liquid-gooey 必须使用 Liquid.Item；结果文字保持清晰，液体只负责轮廓、桥接、阴影和形变。
- 回车和搜索按钮立即启动请求；loading 视觉状态从请求开始至少持续 500ms。
- 输入法组合期间不提交；Escape 清空；空查询不请求；请求和加载更多期间不能重复触发同一操作。
- 首次查询成功后平滑滚动到结果区；加载更多只追加内容，不打断当前阅读位置。
- 移动端触控目标至少 44px；结果信息不依赖 hover；长姓名和状态文案必须换行而不是溢出。
- 不使用 localStorage，不把查询词或名单写入 URL，不修改 Go API、名单数据和隐私边界。
- 所有循环动效尊重 prefers-reduced-motion: reduce。

---

### 任务 1：建立搜索时序纯函数

**文件：** 新建 `src/lib/searchTiming.ts`、`src/lib/searchTiming.test.ts`。

**接口：** `MIN_SEARCH_DURATION: number`；`getRemainingSearchDelay(startedAt: number, now: number, minimum?: number): number`。

- [ ] 编写测试，覆盖 500ms 未到、恰好到达、已经超时、时钟倒退和自定义最短时长。
- [ ] 实现 `Math.max(0, Math.ceil(startedAt + minimum - now))`，避免返回负延时。
- [ ] 运行 `npm test -- src/lib/searchTiming.test.ts`，确认测试通过。

### 任务 2：重构搜索交互与真实动效接入

**文件：** 修改 `src/App.tsx`、`src/styles.css`。

**接口：** 保留 `searchApi(query, limit, offset, signal)` 和现有 SearchState；在 App 内增加 composition 状态、results ref、首屏自动滚动和 500ms 最短等待；用 `Liquid.Item` 包裹每条结果行，使用 `morph={{ shape: true, speed: 0.85, bounce: 0.3, contentBlur: 0 }}`。

- [ ] 增加 onCompositionStart / onCompositionEnd，并在 Enter 事件中同时检查 nativeEvent.isComposing 和本地 composition 状态。
- [ ] 让 submit 立即创建 AbortController、设置 loading、并发起 fetch；请求完成后等待剩余最短时间再提交结果状态。
- [ ] 对成功响应、失败响应和取消响应分别检查 requestId，防止旧请求覆盖新请求。
- [ ] 首屏成功结果在下一帧调用 resultsRef.scrollIntoView({ behavior: "smooth", block: "start" })；loadMore 不滚动。
- [ ] 状态区使用 searching / solving；结果列表使用 Liquid 根节点和 Liquid.Item；保持 data-od-id 唯一。
- [ ] 为搜索输入提供 aria-describedby、autocomplete off、type search、Enter 和 Escape 行为；loading、empty、error、load-more 错误保留 aria-live 或 role alert。
- [ ] 运行 `npm run typecheck`、`npm test` 和 `npm run build`。

### 任务 3：重做黑白艺术视觉与响应式细节

**文件：** 修改 `src/styles.css`。

**设计产出：** 黑曜画布、浅黑层级、光谱白、灰阶边线、微弱网格、扫描线；Mona Sans 标题、Instrument Sans 正文、IBM Plex Mono 数据标签；搜索轨道的 monochrome beam；结果行的柔和液体轮廓；数字与状态的轻量进入；焦点态、键盘态、触控态和 reduced-motion 降级。

- [ ] 重写颜色 token，删除米灰、橙色、薄荷色作为主视觉的使用。
- [ ] 增加字体加载策略：通过 `@import` 引入 Google Fonts 的 Mona Sans、Instrument Sans、IBM Plex Mono，提供本地系统回退；不增加外部图片。
- [ ] 为 `BorderBeam` wrapper、搜索输入、结果 Liquid 组建立稳定尺寸、层级和 overflow 规则。
- [ ] 为桌面结果表格和 700px 以下移动结果堆叠分别设置清晰的网格轨道，移动端保留 44px 最小触控高度。
- [ ] 加入 hover、focus-visible、active、loading、empty、error 和追加结果的状态动画，所有动画在 reduced-motion 下归零。
- [ ] 运行 `npm run build`，检查 CSS 编译和无资源错误。

### 任务 4：维护项目记忆和文档

**文件：** 修改 `CLAUDE.md`、`README.md`。

- [ ] 在 CLAUDE.md 记录三库的真实接入位置、props、状态映射、500ms 时序、输入法行为和移动端断点。
- [ ] 在 README.md 更新字体、交互快捷键、动效职责、运行与验证命令。
- [ ] 使用 grep 检查不存在 localStorage、外部图片热链、重复 data-od-id 和旧橙色主视觉 token。

### 任务 5：最终验证

**文件：** `src/App.tsx`、`src/styles.css`、`src/lib/searchTiming.ts`、`src/lib/searchTiming.test.ts`、`CLAUDE.md`、`README.md`。

- [ ] 运行 `npm run typecheck`，退出码为 0。
- [ ] 运行 `npm test`，所有测试通过。
- [ ] 运行 `npm run build`，退出码为 0。
- [ ] 运行 Go 校验：在 `server` 中执行 `gofmt -w *.go`、`go test ./...`、`go vet ./...`。
- [ ] 复查 Git diff，只包含本计划范围内的文件。

## 规格自检

- 视觉：黑白、DESIGN.md 对齐、字体角色已定义。
- 动效：border-beam、thinking-orbs、liquid-gooey 均有实际组件接入点和明确状态。
- 交互：立即搜索、最短 500ms、Enter、Escape、输入法、自动滚动、分页竞态均有任务覆盖。
- 响应式：桌面与移动结果结构、44px 触控目标、文本换行均有任务覆盖。
- 数据：API、名单、隐私边界未被改变。
- 质量：纯函数测试、类型检查、前端构建、Go 检查均有验证命令。
- 占位符扫描：未使用 TBD、TODO、后续实现或未定义接口名称。