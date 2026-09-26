# 架构评审变更档案

> 本文件是历轮架构评审的完整过程档案（候选、变异实验、纠错记录）。
> **活规范不在此处**——改动代码前请读根目录 `CLAUDE.md` 的规范章节。
> 面向用户的版本变更见 `CHANGELOG.md`。
> 本文件保留原始叙述的价值在于：变异实验的**方法教训**与**自我纠错**不可从最终代码复原。

---

- 批次十八（v0.10.3，2026-09-26，架构评审第六轮：五个候选经变异实验与真实探针逐条核实，全部落地）：
  - 核实方法：沿用既往纪律并加一条——**侦察结论只当线索，file:line 证据与变异实验才当结论**。本轮派四路只读侦察，**推翻四条侦察判断**（详见下方「侦察纠错记录」）。侦察用到的临时探针文件在结论固化后删除。
  - **候选 1（Strong，已落地，用户可见缺陷）**：年段值域四处硬编码致扩展路径彻底断裂。`data.go:77` 的名单标题校验**反过来依赖 `parseGrade`**，而 `parseGrade` 与 `gradeClassToken` 各自硬编码年段字面量（前端另有两处）。**探针实测**：往 knownGrades 追加「高四」后，运维提示正确列出该文件、`gradeOrder` 正确排位，但 `loadStudents` 报「文件名与年级标题不一致」拒绝加载合法名单，查询返回 total=0 且与真不存在的年段完全一致。CLAUDE.md 承诺的「扩展年段只需在 knownGrades 追加」**只在运维提示与排序两处兑现**。现规范名与别名统一从 knownGrades 派生，扩展只需改三处声明。
  - **候选 2（Worth exploring，已落地）**：`api.go` 三条参数 400 路径零承重。变异实验：把 limit / offset / query 长度判定改成恒假后**全仓后端测试仍全绿**。侦察称「完全零覆盖」过头了——`errors_test.go` 确实锁住了错误码到状态的映射表，但那条路径不经参数解析。现新增经 `buildMux` 真实接缝的四条用例，并配「合法边界不被误伤」的对照片段，防止「一律拒绝」这类同样能过测试的错误实现。变异验证三条路径精确翻红。
  - **候选 3（Strong，已落地）**：`App.tsx` 界面知识横跨两个 module。组件必须同时 import 三个派生并另调 `useSearchController` 取 state；既有测试分别打三个函数，**无任何断言锁住它们对同一状态给出一致的组合**。现收成 `present(state)` 一次派生，一致性从「同一次调用」这一接缝可验证。变异验证：scroll 改坏 3 条翻红、tone 恒 muted 1 条翻红。
  - **候选 4（Worth exploring，已落地）**：版本事实源三处不同步，且 CHANGELOG 单向累积缺失三轮。`release.yml` 两处步骤名写「安装 Node.js 20」而实际 24；`ci.yml` job 名「名单 JSON 校验」而实际只做零数据守卫，`ARCHITECTURE.md` 声称校验「字段白名单、去重」——**该能力不存在**。已补齐 CHANGELOG 条目、package.json 跟随、标签与文档改为陈述实际能力。
  - **候选 5（Speculative，已落地）**：deletion test 判定三处零承重——`newStaticCache`（一行实现、唯一调用点、零测试）内联；`web.go` 注释指向的行号与 Vary 实际位置不符，改为按函数名引用不再写行号；`.result-card.no-anim` 在源码中零引用，删除。
  - **侦察纠错记录（三条，均经实测推翻或修正）**：
    一、「api 参数校验完全零覆盖」——**过头**。`errors_test.go` 锁住了错误码→状态映射表，零覆盖的只是参数解析分支本身。
    二、「`tsc --noEmit` 可作类型检查手段」——**错误**。根 tsconfig 是 `files: []` + project references，`npx tsc --noEmit` **什么都不检查**，第一次跑变异实验时因此假绿。正确命令是 `npm run typecheck`（`tsc -b`）。已记入全局约束。
    三、「前端把 `gradeClassToken` 退回硬编码后全部用例翻红」——**错误**。实测全绿：对已声明年段而言，派生与硬编码行为等价，契约语料无法区分二者。**诚实记录该次未翻红**，改补 `gradeDomain` 一组断言锁「声明本身完整且自洽」（别名必须映射回已声明的规范名），而非试图锁「正则长什么样」。
  - 过程中被自身测试拦下的错误：首版把 `gradeClassToken` 改为每次调用重新编译以解决「包级 var 求值过早」，整年段查询分配从 6 涨到 117，被 `TestSearchAllocsBudget` 当场拦下。改回缓存 + `rebuildGradePattern`。**这条断言的价值在本次实施中再次兑现**。
  - 验证：前端 typecheck 干净 + **142** vitest 全绿（135 + present 三条 + gradeDomain 三条 + 别名连写对拍语料一条）+ npm run build 成功；后端 gofmt（本次改动文件全干净）+ go vet + go test 全绿（清 testcache）；**分配次数实测 6/7/8、view 0 次，与基线完全一致**；契约语料 44 → 45 条双端对拍通过；**真实服务冒烟**（2091 条真实名单）验证 health 200、「高一」1047 未误伤、**本轮新增四条 400 路径返回 400 与正确错误码**、合法边界 limit=50 返回 200、已知缺陷「一一班」「高一0班」「、」total=0 未回归、「高二三班」53 与「六班」107 与基线一致、静态资源 immutable + ETag + JS MIME + `gzip;q=0` 返回原始表示且带 Vary 全部正常。gofmt -l 仍报 12 个既有 CRLF 文件（Windows core.autocrlf 既存状况）；本机 CGO_ENABLED=0 无法跑 -race，由 CI 兜底。
  - 领域模型：CONTEXT.md 新增「年段值域」与「展示派生」两组术语。
- 批次十七（v0.10.2，2026-09-26，架构评审第五轮：六个候选经变异实验逐条核实，落地五项、推翻一项、判定不做一项）：
  - 核实方法：沿用既往纪律——侦察结论只当线索，`file:line` 证据与变异实验才当结论。本轮先派四路只读侦察（前端查询管线 / 后端名单搜索 / 后端 HTTP 边缘 / 全仓测试面测绘），再对每条要进报告的结论亲自复核，**共推翻两条侦察判断**（详见下方「侦察纠错记录」）。变异实验一律在清 testcache 后以 `-count=1` 执行。
  - 候选 1（Strong，已落地）：「查询状态 → 界面」此前只有 `resultSectionOf` 一根派生通道，但另有三条支路绕开它直读 `SearchState` 字面量——`App.tsx` 直比 `"loading"` 取提交禁用态与 aria-label、把原始枚举喂给 `data-state` 与 `StatusOrb`、`StatusOrb.tsx` 再直比一次、`styles.css` 第四第五次镜像同一批字面量。**变异实验证明三条支路全部零承重**：禁用态改成 `false`、`data-state` 与 orb 状态硬编码、`StatusOrb` 对任何状态都渲染，132 条用例全部仍然通过。新增 `deriveStatusHint`（searchReducer.ts）单点派生 `busy / sendLabel / tone / showOrb`，`Record` 穷尽性让新增查询状态时编译器强制补齐；`App.tsx` 只消费派生值，`StatusOrb` 改为接收布尔，CSS 改按 `data-tone` 着色。三个变异（busy 恒假、showOrb 恒真、tone 恒 muted）全部翻红。用例 132 → 135。
  - 候选 6（Speculative → 已落地，v0.10.2 唯一行为变更）：静态资源用 `strings.Contains(Accept-Encoding, "gzip")` 判定压缩，把 `gzip;q=0`（客户端明确拒绝）判为接受。响应同时声明 `Vary: Accept-Encoding`，即向共享缓存声明本响应随该头变化——声明了协商维度却不严格协商，接入 CDN 后缓存会忠实地把一种表示复给声明了另一种的请求。抽出 `acceptsGzip` 纯函数逐候选解析 q 值。**变异实验证明此前零覆盖**：判定改成 `== "gzip"` 后全部后端用例无一翻红。新增三条断言，其中两条走 HTTP 接口而非直接调纯函数——直接测纯函数只能锁住实现，实现被换掉时断言不会翻红。变异验证：回退子串匹配 → `TestFrontendAssetsGzipQZeroServesRaw` 翻红；改成严格相等 → `TestFrontendAssetsGzipInCandidateList` 翻红。
  - 候选 2（Strong，已落地）：前端 `parseQuery` 返回的 `tokens` 是分词中间结果（归一后按空白切出的全部 token，在分类为姓名/年段/班级之前就已确定），此前是 JS 独有字段、Go 侧无对应物，契约无法表达——「两端空白集合逐码位对齐」这条不变量只能在单侧断言。44 条语料全部补 `tokens` 字段（值取自 Go 端实测，契约以 Go 行为为准），并抽出 `tokenize` 单点让 `parseQuery` 复用，测试经同一函数对拍而非复制一份切分规则。变异验证：Go 端不再按 NEL 切 → 12 条翻红（含 NEL 语料）；前端空白集合删掉 U+0085 → 2 条翻红。
  - 候选 4（Worth exploring，已落地）：两个最大的测试文件方向相反地放错了对方的测试——改限流器要去 `data_test.go`（名单模块）里找，改名单加载要去 `search_test.go` 里找。搬迁三条用例：`TestRateLimitAutomaticSweep` → `ratelimit_test.go`（它构造 `newRateLimiter`、不触碰任何名单符号；此前直读 `limiter.buckets` 并手动 `limiter.mu.Lock()`，是全仓唯一一处测试操作生产锁的地方，该处淘汰行为无外部可观测面，直读是刻意的，已在注释中说明），两条 `TestLoadStudents*` → `data_test.go`。搬迁后 `os` 与 `path/filepath` 在 `search_test.go` 成为孤儿导入，一并删除。变异验证：破坏空目录报错分支 → `TestLoadStudentsEmptyDirFails` 精确翻红，确认搬迁未削弱断言。
  - 候选 5（Speculative，已落地）：`writeJSON` 定义在 `main.go` 而全部消费点都在 API 侧（api.go 三处成功响应、errors.go 错误体、ratelimit.go 间接经 `writeError`），改响应写出格式要去一个只声称承担四个薄角色的文件里找，而它自身零调用。搬到 `errors.go` 与 `writeError` 同址，`main.go` 随之删除 `encoding/json` 孤儿导入。纯搬迁，零行为变化。
  - 交叉核实（补断言，非候选）：`TestValidImpliesPositiveClassNo` 锁住「Valid ⇒ ClassNo > 0」在接缝处的承重。**该不变量的执行点（`parseClassName` 内部）对下游消费侧零独立承重**——两条变异实验证明：把 `Search` 的匹配条件从 `ClassNo > 0` 改成 `!= 0` 全部仍绿，删掉 `newStudent` 的 `Valid` 守卫全部仍绿。新增断言在 `classCondition` 这道接缝上枚举十个输入，变异验证让 `classCondition` 误把 0 当合法 → 9 条翻红（含本条）。`newStudent` 的 `Valid` 守卫对零值输入**不可观测**（`parseClassName` 对非法输入返回的 `ClassNo` 本就是 0，有无守卫结果相同），已在断言注释中如实记录，不再假装它可测——这是对批次十五「变异验证作为承重判据」纪律的一次自我纠正。
  - **候选 3 被推翻，不做**：侦察报告称「调换中间件链顺序（`securityHeaders` 移到 `rateLimit` 内侧）全部测试仍绿，链顺序零覆盖」。实测**该判断错误**：`TestHandlerChainRateLimitedResponseCarriesSecurityHeaders` 立刻翻红，7 个安全头全部缺失报错。链顺序有测试承重，候选前提不成立，不予实施。
  - 侦察纠错记录（本轮两条，均经实测）：
    一、「Go 端 NEL 分词零覆盖」——错误。`docs/query-contract.json` 第 35 条 `张+U+0085+三` 就是 NEL 样本，其 `classNumber: 3` 正是「Go 按空白切、JS 不切」的差异点，两端都在对拍。原始 `grep` 因该字符不可见而漏检；用脚本按码位解析语料后推翻。该条促成了候选 2（把 `tokens` 纳入契约，让分词本身有直接对拍）。
    二、「`search_test.go` 的 ClassNo=0 断言零承重，原因是生产不可达」——一半对。我第一轮据此推断该断言在承重（变异反转期望值后翻红），但那是**期望值断言**在起作用，不是语义。精确变异（删掉 `newStudent` 的 `Valid` 守卫）证实：全部仍绿，该断言对守卫零承重。承重方向是「实际值恰好为 0」，不是「非法输入经守卫产出 0」。
  - 验证：前端 typecheck 干净 + **135** vitest 全绿（132 + deriveStatusHint 三条）+ npm run build 成功（产物 CSS 已生成 `status-line[data-tone=ink]`/`[dim]`，`data-state` 规则完全消失）；后端 gofmt（本轮改动的 search.go / contract_test.go / main.go / errors.go 全干净）+ go vet + go test 全绿（清 testcache）；**真实服务冒烟**（2091 条真实名单）验证 /api/health 200、`gzip` 返回压缩表示（7884 字节）、`gzip;q=0` 返回原始表示（19807 字节且无 Content-Encoding）、两者均带 `Vary: Accept-Encoding`，「高一0班」「、」total=0 未误伤，错误契约 invalid_limit / not_found / method_not_allowed 准确，woff2 MIME 为 font/woff2，首页带 `no-store`（证明 writeJSON 搬迁后安全头链路完好）；**真实浏览器验证**（Chromium 加载构建产物）确认查询后 `data-tone="ink"`、状态文字正确、144 条命中结果区渲染、提交按钮未禁用、加载 orb 不渲染，Escape 后回到 `data-tone="muted"` 与初始文案。gofmt -l 仍报 13 个既有 CRLF 文件（Windows core.autocrlf 既存状况，未批量转换）；本机 CGO_ENABLED=0 无法跑 -race，由 CI 兜底。
  - 领域模型：CONTEXT.md 新增「界面提示」术语（查询状态到禁用态/提示色/加载指示的派生，与「结果区段」同源）。
- 批次十六（v0.10.1，2026-09-26，架构评审第四轮：发现并修复用户可见缺陷「0班放大为全校检索」，另清理三处死重量）：
  - **缺陷（已实证，非推断）**：输入「高一0班」返回**整个高一年级 1047 条**。`parseClassName` 的 `strconv.Atoi` 路径（classparse.go:74）未校验正数——`Atoi("0")` 语法成功且无错，被原样包成 `Valid:true`，违背本文件声明的「Valid ⇒ ClassNo > 0」不变量。该不变量此前**只约束汉字路径**（chineseNumberToInt 查表未命中返回 0），阿位数字路径从未被覆盖。后果是 `classCondition` 既不产生班级条件也不降级为姓名条件（token 静默消失）；与年级连写时年级条件仍成立而班级条件落空，一次精确查询被放大成整年段全量。这是「条件全部落空 ⇒ 返回全部」不变量的**第三个实例**（v0.9.1 修「无法解析」、v0.10.0 修「纯分隔符」），也是唯一未被契约语料覆盖的路径。
  - **双端分裂（探针实测）**：前端 `classNumber`（query.ts:102-114）本就按 `classNo <= 0` 降级为姓名条件，解释正确；Go 端却接受 0 班号。同一查询词两端解释相反，且分叉方向正是最危险的一侧——前端 `hasNameCondition` 会告诉用户「这是姓名查询」，后端却返回全校 1047 条。**契约对拍本应抓住，但 41 条语料无一条涉及 0 班号**（`grep '"0' docs/query-contract.json` 无匹配）——机制健全，失效在覆盖。
  - **零承重证据（变异实验）**：把实现改为正确行为（`err == nil && number > 0`）后跑完整后端套件，**零条用例翻红**。这条路径在 200+ 条后端用例中零覆盖。
  - 修复：`parseClassName` 的 Atoi 分支改为「成功即证明格式合法（因此不可能溢出），但班号必须为正，`<= 0` 返回零值」。**关键设计点：0 属非法而非溢出**——若让它落入下方 `isAllDigits` 分支会被谎报成 `Overflow:true`，那就是在类型里撒谎（同一零值承担两种语义）。修复后实测：`Search("高一0班")` 从 total=2（合成名单全校）降为 0，`loadStudents` 拒绝「0班」类名（"班级格式异常"），`Search("高一")` 仍正常返回全量（未误伤）。
  - 契约语料 41 → 44 条（新增「0班」「高一0班」「00班」），双端对拍通过。变异验证：把修复回退后三条语料**同时翻红**，精确报出 `nameTokens = []`（期望 `["0班"]`）——证明新语料承重而非恰好通过。
  - 新增三条 Go 端回归锁（classparse_test.go）：`TestZeroClassIsRejectedNotAccepted`（解析层 + 正数班号不被误伤 + 降级层）、`TestSearchZeroClassDoesNotReturnEveryone`（用户可见行为，含「高一」对照组防空结果误判）、`TestLoadStudentsRejectsZeroClass`（数据加载侧）。三条在变异下**全部翻红**，错误信息可直接读出失败原因。
  - 死重量清理（grep 逐个核实，**侦察代理的三条指控中两条被推翻**）：删除 `rateLimit` 薄包装（生产 main.go:137 与测试全走 `rateLimitWith`，其存在让读者误以为生产装配走该入口；连带重写 `rateLimitWith` 注释，否则「rateLimit 的可注入变体」会悬空）；`staleResult` 由导出改为私有（外部零消费）；删除 `ResultSummary.total`（ResultList 只读其余 5 字段，核实 resultSummary.test.ts 从未断言该字段——代理称「测过全部 6 字段」不准确）。**判定不删**：`sweepLocked`（被 `allow` 内部自驱动调用，ratelimit.go:62）、`errDataUnavailable`（data.go:308）、`levelWarn`（FMC_LOG_LEVEL=warn 是对外环境契约）——代理建议删 `sweepLocked` 是错的。
  - **判定不做的候选**：前端 `query.ts` 120 行实现生产只消费 `hasNameCondition` 一个布尔（`parseQuery`/`normalizeName` 生产零调用）。保留理由：契约对拍走 `parseQuery` 的完整结构比对（tokens/nameTokens/grade/classNumber）而非只比布尔，精度显著更高，且跨语言事实源是已定决策。**正确处置不是删，而是补它缺的覆盖**——已在本轮补 3 条语料。状态穷尽性守卫同理：`COPY: Record<SearchState, string>`（searchReducer.ts:31）已由类型强制，真实缺口只在测试里两处手写 `const all: SearchState[]`，收益不足以单独改动。
  - 验证：前端 typecheck 干净 + **132** vitest 全绿（原 129 + 语料派生 3 条）+ npm run build 成功；后端 gofmt（三个改动文件全干净）+ go vet + go test 全绿（清 testcache）；**分配次数实测 6/7/8、view 0 次，与基线完全一致**（修复零新增分配）；契约语料 44 条双端对拍通过；**真实服务冒烟**（2091 条真实名单）验证「高一0班」「0班」「00班」「0」「高一00班」全部 total=0，而「高一」1047、「高二」1044、「高一18班」54、「18班」69、「六班」107、「高二三班」53 全部未误伤；错误契约 400 invalid_limit / 404 not_found / health 200 均正常。gofmt -l 仍报 13 个既有 CRLF 文件（Windows core.autocrlf 既存状况，未批量转换）；本机 CGO_ENABLED=0 无法跑 -race，由 CI 兜底。
- 批次十五（v0.10.0，2026-09-26，架构评审第三轮七候选经逐条读码 + 变异实验核实后落地五项、判定一项不做）：
  - 核实方法：沿用批次十四纪律——侦察结论只当线索，`file:line` 证据与变异实验才当结论。本轮四次变异实验 + 一次真实探针，**推翻三条侦察判断**（详见下方「侦察纠错记录」）。
  - 修复 1（用户可见缺陷·候选 01）：纯分隔符输入退化为全校检索。`Search` 的空查询守卫只剥空白（`TrimSpace(raw) == ""`），而 `querySeparators` 已把中英文逗号、顿号加号替换为空格——两处归一化不一致，纯分隔符输入既非空查询也不产生任何条件，三个筛选条件全不约束。**实测（2112 条合成名单）输入「、」「,」「+」「+++」均返回 total=2112**；纯空白（空格、Tab）被正确拦为 0，缺陷边界精确落在分隔符未被剥除这一侧。真实服务冒烟（2091 条名单）确认修复后六种分隔符/空白输入全部 total=0，而「高一」仍正常返回 total=1047。**与 v0.9.1「无法解析/溢出的班级 token 不得静默丢弃」同源**：v0.9.1 修的是前两个实例，这是第三个。修复：判空依据从「原始串剥空白后是否为空」改为「解析后是否存在任何条件」，`query` 已在上一行算好故**零新增分配**（实测仍 6/7/8）。**不可用 `len(NameTokens)==0` 单条件判空**——契约语料中 27 条合法查询（高1/18班/六班/高二三班等）nameTokens 为空但带年级或班级条件。契约语料 39 → 41 条（「、」与「，,、+」）双端对拍。
  - 修复 2（locality·候选 03）：班级条件降级策略收进 `classCondition(matchPart, rawToken)`。`parseQuery` 此前有四段近乎逐字重复的「追加 normalizeName(token); continue」，而解释为何必须降级、不能静默丢弃的注释**只写在其中一段**——其余三段靠「照抄旁边那段」维持这条安全不变量。`parseQuery` 收敛为两次调用，净减 23 行。两个参数必须分开的理由已写进函数注释：查询「一一班」时班级部分是「一一」，降级后的姓名匹配键必须是「一一班」。**本次重构最初把 matchPart 误用于两处，契约语料 TestParseQueryContractCorpus/一一班 立刻翻红才发现**——跨语言事实源的价值。变异验证：降级改回静默丢弃（v0.9.1 修复前的原始 bug），3 条语料立刻翻红。
  - 修复 3（零保护缺口·候选 02）：结果区渲染映射收进 `resultSectionOf` / `shouldScrollToResults` 两个纯函数。「哪些状态该渲染哪块结果区」此前在 App.tsx 散落四份判断（renderResultBody 四值 if、滚动 effect 四值否定链、hasResultSection 二值否定、StatusOrb 与 styles.css 的 data-state 镜像），**零测试覆盖**：把 duplicate 从列表分支移除（重名同学结果列表完全不渲染）后 121 条用例全部通过。关键事实：滚动 effect 与 hasResultSection **不是复制而是互补**（7 值全集上恰好凑齐），新增第 8 个状态时只改一处编译器不报错，复制工具能发现、互补关系不能。抽出后 App.tsx 改 switch 消费返回值，组件面不变。变异验证：同一变异修复前 121 条全绿、修复后 3 条立刻翻红。
  - 修复 4（单一事实源·候选 05）：「数据目录无任何年段文件」的判定与运维指引收进 `errNoRoster()`。此前 `loadStudents` 与 `dataStamps` 各自独立判定且文案逐字复制，两处都硬编码三个年段。改为从 `knownGrades` 生成清单——**扩展年段只需在 knownGrades 追加，运维提示自动跟随**，不会在新增年段后继续提示放置已不被支持的文件名。新增两条常驻测试（文案逐字不变 / 随 knownGrades 扩展且单年段无悬空连接符），均由本次实现引入的两个真实缺陷驱动：首版把「或」交给 `strings.Join` 产出「高二.json、 或 高三.json」多一个顿号；只处理 len==1 与 len>=2，空 `knownGrades` 时 `names[:-1]` 直接 panic。变异验证：改回多顿号版本两条测试同时翻红。
  - 清理（候选 06）：`static_cache_test.go` 的注释与实现相反——原文描述批次八**修复前**的缓存跨来源泄漏，而同文件断言期望的正是隔离，读者会误以为有未修缺陷，改为陈述不变式 + 保留修复历史。删除零生产调用的 `logWarnf`（全仓仅定义处），与 v0.9.3「删除零调用薄包装」同型；`levelWarn` 常量与 `parseLogLevel` 的 warn 分支**刻意保留**（FMC_LOG_LEVEL=warn 是对外环境契约，删掉会让该配置静默失效），并在常量处注明当前无 warn 级输出点。
  - **判定不做（候选 04：useSearchInput 浅模块）**：侦察以「3 个方法纯透传、interface ≈ implementation」定性，实为误判。`onKeyDown` 持有 5 行真实业务逻辑（Escape 清空、Enter 提交、双 IME 守卫），且本轮变异实验已证明两条守卫都真实承重（拆任一 → 对应断言立刻翻红）——该 module 存在的首要理由就是它。三个透传方法是 `useCallback` 记忆化包装，App.tsx 直接挂在 `<input>` 上，有无 `useCallback` 是运行时差异（每次重渲染新引用会让 React 重新绑定事件）。测试替身写 `as unknown as` 是因为只造 `useSearchInput` 实际调用的 5 个方法（最小替身是正确做法），非 interface 过宽。按「简洁优先」判定 Speculative 不做。
  - 侦察纠错记录（三条，均经实测推翻或修正）：
    一、「useSearchInput 的 IME 双守卫只有 `nativeIsComposing` 侧承重、`isComposing` 侧靠 harness 重渲染偶然有效」——**证伪**。分别拆除两个守卫，对应断言（test:103 / test:108）均立刻翻红，代码注释诚实。
    二、「`cooling` 的 `lastFailStampKnown` 守卫存在零承重缺口，断言打到了 `recoveryDue` 的 retry 布尔」——**证伪**。拆掉守卫后 `TestStoreReloadFailureIsFailClosedThenRecovers` 立刻翻红（修复后的名单无法立即重试，数据停在不可用）。批次十四补的 `TestCoolingHoldsWhenStampsUnknown` 与该用例形成双锁。
    三、「`loadStudents` 的 `!found` 分支生产路径几乎不可达、是需要维护的死重量」——**修正**。`loadStudents` 是导出的包级函数，`search_test.go` 等测试直接调用，不是死代码；它独立保证「无数据时明确报错」而非静默返回空切片，本身合理。成立的部分是「不变量与文案各两份」。
  - 领域模型：CONTEXT.md 新增「结果区段」与「无条件查询」两组术语（前者是候选 02 抽出的映射，后者是候选 01 修复的不变量）。
  - 验证：前端 typecheck 干净 + 129 vitest 全绿 + npm run build 成功；后端 go vet + go test 全绿（清 testcache）；分配次数实测 6/7/8、`view` 0 次（预算上限 12 / 4，均未变）；契约语料 41 条双端对拍通过；**真实服务冒烟**（Windows 原生二进制 + 2091 条真实名单 + FMC_DATA_DIR 指向真实数据目录）验证 /api/health 200、六种纯分隔符/空白输入 total=0、「高一」total=1047 未误伤。gofmt -l 仍报 13 个既有 CRLF 文件，经 `gofmt -d` 确认差异自第 1 行起全文件替换且行数不变，属 Windows core.autocrlf 既存状况，未批量转换；本机 CGO_ENABLED=0 无法跑 -race，由 CI 兜底。
- 批次十四（v0.9.3，2026-09-26，架构评审第二轮七候选经逐条核实后落地五项）：
  - 核实方法：对 improve-codebase-architecture 报告的七个候选逐条读码核实，不照搬侦察结论。据此推翻三条侦察判断：候选 04（web.go 缓存隔离）实为批次八有意决策（实例级隔离，注释明确），不重开，只保留「双读盘」这一真实残留；候选 01 的 orchestrator+reducer 整体合并不做（T0 刚建深接口且 114 测试全绿，收益低风险高），只取「App 交互语义收集」子项；候选 05（query.ts 收窄为最小判定）不做——会削掉 39 行契约语料的对拍锁，风险高于收益。
  - 实施 1（限流时钟注入窄化）：newRateLimiter 增加 now 构造参数，now 不再是可写字段（此前 ratelimit_test.go 3 处、chain_test.go 1 处、data_test.go 1 处跨三文件改写生产字段）。抽出 fillTokens 纯函数——不读时间源、不加锁，回补速率、容量钳制、时钟回拨可直接单测。删除公开 sweep（生产零调用点，自驱动清理已由 allow 内部承担）。变异验证：破坏回补速率 → TestFillTokensMultipleIntervals 翻红；破坏回拨防护 → 3 条回拨断言翻红。修正一处自身错误：初版断言按 5s/2s=2 个令牌写，实为 2.5——令牌是连续量不取整，改断言而非改实现。
  - 实施 2（热重载时钟注入 + 自恢复时机单点，附真实缺陷修复）：newStudentStore 同样改构造注入（此前 data_test.go 5 处 + main_test.go 1 处直写 store.now）。新增 recoveryDue(now) 单点判定，probeThrottled 与 cooling 均消费它——两条时间窗口此前拆在两个方法里各自取样时钟，「同一条时间线」只是注释承诺。**修复真实缺陷**：dataStamps 失败时 recordFailure(nil, err) 使 lastFailStamps 为 nil，而 sameStamps(nil, stamps) 恒为 false，导致 cooling 在目录缺失场景下永不成立——注释承诺「缺失目录同样需要冷却」而实现是每个请求都重试读盘并写错误日志，正是冷却机制要防的 IO 与日志放大。现以 lastFailStampKnown 显式标记。变异验证揭示测试盲区一则：初版新测试断言的是 recoveryDue 的 retry 布尔（不读 lastFailStamps），去掉守卫仍全绿——补 TestCoolingHoldsWhenStampsUnknown 直接锁 cooling 行为后，去掉 lastFailStampKnown 守卫立即翻红。过程中三次写错测试断言（recoveryDue 纯判定不推进状态、retry 语义方向、cooling 返回值方向），均由变异验证暴露后修正。
  - 实施 3（静态资源单次读盘）：删除 assetExists 预检——命中缓存即证明资源存在，未命中才读盘一次；immutable 头移入 serveCachedStatic 在确认存在后设置。修复前每次请求都跑 Open+Stat（缓存命中时仍 2 次 FS 调用）。新增 countingFS 测试锁住缓存命中零触碰，变异验证：加回一次 Open 即翻红。真实服务冒烟确认 304 带 Vary、缺失资源 404 且为 no-store（不继承 immutable）。
  - 实施 4（竞态骨架收敛）：抽出私有 perform(id, q, limit, offset)，收敛 listen→search→isCurrent→finally unlisten 骨架；submit 传 begin() 的新会话 id、loadMore 传 currentId，两方法体不再逐行同构（净减 11 行）。判别联合构造点单点化。变异验证：破坏 stale 判定 → 4 条竞态断言翻红。
  - 实施 5（交互语义收口）：新增 useSearchInput，IME 组合守卫、Enter 提交、Escape 清空从 App.tsx 的 JSX 回调收进一处——该逻辑此前内联在组件里且零测试覆盖。7 条挂载测试锁住运行时行为。**设计修正**：初版额外加的本地 composing ref 经变异验证零承重（纯推测性防御，原 App.tsx 也没有），已删除——不为测试存在的装置与无承重的防御都不该留。覆盖边界如实记录：挂载测试用 harness 串 hook，不加载 App.tsx，因此不覆盖 JSX 接线；接线由 TypeScript 编译期锁（实测少传 isComposing 参数即报 TS2554）。
  - 领域模型：惰性创建 CONTEXT.md（此前不存在），收录名单域、数据可用性、查询会话、契约四组术语，其中「自恢复时机」「时钟注入」「查询控制器」「交互语义」为本轮落定的新概念。
  - 验证：前端 typecheck 干净 + 121 vitest 全绿 + npm run build 成功；后端 go vet + go test 全绿（清 testcache）；分配次数实测 6/7/8（**预算上限 12**，断言为 allocs > budget 才失败，非预算等于 6/7/8）；真实服务冒烟（2091 条名单）验证 /api/health 200、搜索与年级班级连写解析正确（"高二十八班" total=15）、错误契约 400/404、静态资源 ETag/304/缺失资源缓存头。gofmt -l 仍报 13 个既有 CRLF 文件，属 Windows core.autocrlf 既存状况未批量转换；本机 CGO_ENABLED=0 无法跑 -race，由 CI 兜底。
  - 环境事实：沙箱内 http_proxy 会把本地 curl 请求也代理掉（502），冒烟须加 --noproxy '*'；持久化进程与 WSL relay 不可用，服务须用 cmd /c "start /b" 启动并用 taskkill /F /IM 终止。
- 批次十三（v0.9.2，2026-09-26，架构评审八项候选经变异测试核实后修复三项）：
  - 核实方法：八条候选逐条做变异测试（破坏实现看测试是否翻红），不以读码推断为依据。三条侦察结论被证伪或需修正。
  - 修复 1（分页失效零覆盖）：useSearchController 的 loadMore 用例只断言 loadingMore 为 false，把 loadMore 改成「永不追加」后 4 条测试仍全绿——分页功能整体失效而无任何警报。追加语义仅在 searchReducer 层被单独测，编排层从不断言 items 变化。新增两条从编排入口断言 items 增长的用例（追加语义 + 无后续页时不发请求），变异验证两条锁均承重。
  - 修复 2（跨端空白集合分歧，U+FEFF）：JS \s 含 U+FEFF 而 Go unicode.IsSpace 不含（自 Unicode 4.0.1 起它不在 White_Space 内），两端在 U+FEFF 上静默分叉。实测「␣18班」：Go 保留整体为一个姓名 token，前端因 trim() 也会移除首尾 U+FEFF 而削成「18班」判为班级条件——翻转前端唯一消费的 hasNameCondition 布尔，提示文案因此分叉。修复：src/lib/query.ts 引入 goSpaceChars 显式枚举 Go 的空白集合（不用 \s 近似），分词与姓名归一化共用该集合；同时把 toLocaleUpperCase 改为 toUpperCase（前者随宿主 locale 变化，tr/az 下 "i" 映射为 U+0130，而 Go strings.ToUpper locale 无关）。语料补两行 U+FEFF 样本（39 行），变异验证：退回 \s 立即 2 条失败。
  - 修复 3（分页自守有分支未覆盖）：Search 的空查询提前返回位于两个钳制之前，负 limit/offset 原样回显进响应，「Search 自守分页前置约定」只在非空路径成立。实测 Search(students,"",-3,-5) 返回 Limit:-3 Offset:-5，非空同样入参返回 0/0。修复：负向钳制上移到任何提前返回之前，offset 上界钳制（依赖匹配结果长度）留在匹配循环之后。分配预算 6/7/8 保持不变。
  - 核实后不做的三项：状态接口存派生事实（破坏 input-change 与 submit-success 两处同步各 2 条测试失败，批次九已加锁）；错误分类「后端新增码静默退化」（现有 7 个码全落在 400/404/405/429/500，errStatus 有尺寸断言兜底；但「invalid-response 带 status=200 落空四段判断」成立，属窄缺口）；状态枚举五处副本（成立但需新增 App 挂载测试，收益独立）。
  - 推迟两项：数据缝取具体类型（main_test.go 两处直触 store.now/store.dir；改为窄接口使降级路径可用两行 double 覆盖）；零调用薄包装（rateLimit 与 limiter.sweep 全仓零生产调用点，删 sweep 仅 data_test.go:302 编译失败且与 TestRateLimitAutomaticSweep 重复）。
  - 侦察结论的三处事实性更正：一、U+0085 在 normalizeName 中不承重（删掉后 46 条全绿），只有分词器承重（删掉即 2 条失败）——与侦察「删掉即翻红」的表述相反。二、编排层 stale 用例对「stale 被误当成功」这一真实变异有效（移除判别即失败），并非零杠杆。三、对拍机制本身健全：注入 U+FEFF 语料行后 Go 侧通过、TS 侧翻红，说明失效点在语料覆盖而非机制，无需更换机制。
  - 沿用既有纪律：变异测试作为断言有效性的判据（本批次三次变异均先观察失败再恢复）；过程性计划落盘到 git 忽略的 docs/superpowers/；本机 CGO_ENABLED=0 无法跑 -race，由 CI 兜底；go test 缓存会显示假绿，核实变异时必须 go clean -testcache 配 -count=1。
- 批次十二（v0.9.1，2026-09-26，架构评审七项修复，全部经实证与变异测试验证）：
  - 修复 1（用户可见失效）：useSearchController 末行用 useMemo([controller]) 缓存 state，而 controller 引用恒定（依赖 [force]，force 来自 useReducer 的 dispatch，引用永不改变），使 useMemo 永久命中缓存——force() 重渲染后组件仍拿到首帧快照，查询状态对用户停留在 idle，整条搜索链路不可用。改为每次渲染直读 controller.getState()。该缺陷长期存活的结构性原因：vitest 只收集 .test.ts 且无任何 React 挂载测试，hook 壳从未被验证过。新增 useSearchController.mount.test.tsx（5 用例真实挂载）+ vitest.config.ts 放开 .tsx 收集。实证：修复前 4 用例失败（success/empty/error 均停在 idle），修复后全绿。
  - 修复 2（信息越权）：汉字数字走 classDigits 查表，未命中时 Go 返回零值 0，parseClassName 把 0 无条件包成 Valid=true；parseQuery 两条班级分支在 Valid 为假时仍 continue，token 无声消失、条件全部落空，Search 退化成与输入无关的全校第一页。真实名单下输入「一一班」曾把 1047 条高一记录全部返回给任意普通用户。数据侧同型：loadStudents 只检 Valid，畸形类名以 ClassNo=0 排到所有正常班级之前。修复：chineseNumberToInt 返回 (int,bool)；!Valid 与 Overflow 同策略降级为姓名条件；年级+班级连写且年级可解析时保留年级并按姓名处理整个 token。契约语料新增两条（一一班 / 高一一一班）由两端对拍共同消费。冒烟验证：修复前 total=1047，修复后 total=0。
  - 修复 3（接口契约违约）：Search 注释声明自守分页约定却只钳制 offset，负 limit 使 end=offset+limit 变负下界触发 panic（slice bounds out of range [:-1]）。生产 HTTP 路径被 api.go 的 parsed<1 挡住，故非线上故障而是导出的 Search 未声明前置条件。补齐 limit 侧钳制为负归零，零分配，TestSearchAllocsBudget 的 6/7/8 保持不变。
  - 修复 4（测试有效性）：newHandlerChain 自称链顺序唯一事实源，但 newTestChain 用字面量重写了一遍嵌套，生产链零测试调用点，四个组合政策测试全走测试链——改生产链顺序它们仍全绿。抽出 newHandlerChainWith(mux, limiter) 作为唯一实现，两侧均委托它。变异验证：把 accessLog 移到 rateLimit 外层后 TestHandlerChainRateLimitedRequestIsNotAccessLogged 立即失败，证明断言真正覆盖生产装配（首次尝试的「整段摘掉 accessLog」是无效变异——两个断言都是「不写访问日志」）。
  - 修复 5（虚假安全感）：api.ts combineSignals 的 AbortSignal.any 降级仅 fetchVersion 经过（searchApi 裸用 AbortSignal.timeout），而唯一的「Safari < 17.4」用例调用的是 searchApi，整段降级逻辑零覆盖。删除该用例，新增 4 条按行为断言的测试，api.ts 零改动。变异验证：删掉降级分支中 b.addEventListener 后「timeout 侧触发」用例立即失败。设计该用例时三次落空（摘监听未被发现、queueMicrotask 造成游离异常、未传调用方 signal 时 combineSignals 在 !a 处直接返回 b 组合器根本不参与），最终形态须同时提供调用方 signal 并单独触发 timeout 侧。
  - 修复 6（维护摩擦）：contract_constants_test.go 用 HasPrefix 匹配「export const NAME = 」，把 TypeScript 排版（空格、类型标注、分号）变成隐式契约，纯风格改动触发指错方向的「前端常量未找到」。改用正则识别，容忍空白/类型标注/尾随逗号/行尾注释，失败信息区分「未找到声明行」与「字面量无法解析」。变异验证：排版变异仍通过、值漂移（10→20）精确失败（首轮因 go test 缓存显示假绿，清 testcache 后才暴露）。
  - 修复 7（死代码）：删除 classparse.go 的 classNumber 薄包装——生产零调用点，批次十归位时迁移未走完，注释仍写「供查询路径过渡使用」而该路径早已改用 parseClassName。其存在让 TestClassNumberOverflowSafe 看起来在测生产行为，实则只测薄包装自己。两处测试改走 parseClassName 断言三态（汉字多位数用例顺带增强为同时断言 Valid 与 Overflow）。
  - 环境事实：Windows core.autocrlf=true 且无 .gitattributes，工作区 13 个 .go 文件检出即 CRLF，gofmt -l 必报；git 存储层统一为 LF，diff 只显示实际改动行。属既存状况，由 CI（Linux）兜底，勿擅自批量转换行尾。
  - 验证：前端 typecheck 干净 + 110 vitest 全绿 + npm run build 成功；后端 go vet + go test 全绿（清 testcache）；真实服务冒烟（Windows 原生二进制 + 真实名单）验证 /api/health 200、一一班 total=0、非法 limit 400、未知路径 404 JSON。本机 CGO_ENABLED=0 无法跑 -race，由 CI 兜底。
- 批次十一（v0.9.0，2026-09-26，架构深化六项 + 冒烟验证）：
  - T0 查询会话深模块：新增 src/lib/useSearchController.ts（createSearchOrchestrator 纯逻辑 + useSearchController 薄 hook 壳），把「发起并展示一次查询」的编排（守卫/判别联合/竞态同步）收进一根接口；App.tsx 从理解 reducer+session+resultSummary+api 五个接口面降为只消费 state+controller 两面，编排守卫与 stale 判别从此可脱离 React 单测（4 个 orchestrator 行为测试）。
  - C1 load-more 单 action：searchReducer 删 load-more-append/error/settle 三个结果 action，收成 { type:"load-more-result"; result: SearchResult }，reducer 依 reason 分派追加/置错/静默复位——stale 的 loadingMore 复位此前依赖 App 无条件 settle 兜底（delete settle 测试全绿、stale 会卡死），现收进 reducer 显式化并有回归测试。
  - C2 SearchSession 接口收窄：公开面 6 方法 → 4 方法，begin/isCurrent 与 caller signal 收归内部闭包；searchApi 移除 signal 参数（timeout 单一信号源，Safari 降级组合保留给 fetchVersion）；两级信号组合收敛为 session 内部 controller → api 一层。
  - C3 classNumber 三态化：新增 parseClassName 返回 ClassParseResult{ClassNo,Valid,Overflow}，替换 0/-1 双哨兵返回码；修复溢出类名此前被 data.go 的 ==0 校验静默放行、以 ClassNo=-1 流入排序比较（非法班级排最前）的真实缺陷；parseQuery 的溢出「按姓名处理」语义用 Overflow 显式表达。新增 TestParseClassNameStates + TestLoadStudentsRejectsOverflowClass 回归锁。
  - C4 名单加载解析一次三处消费：loadStudents 内同一 className 从 3 次正则+Atoi（校验/排序/newStudent 派生）降为 1 次，classNos map 缓存三态结果，newStudent 签名改为接收解析结果。
  - C5 Search 收窄单值返回：Search(students, raw, limit, offset) 返回 SearchResponse 单值，生产零消费的 Query 第二返回值删除；解析断言（TestGradeSubstringBehavior/TestGradeClassCompoundPrecise）改走 parseQuery 独立暴露。
  - C6 错误码映射表：新增 server/errors.go 的 errStatus map[string]int 单表 + writeError 统一查表，api.go 6 处与 ratelimit.go 1 处手写 status+code 配对全部收敛；未知码回落 500，TestErrStatusMapping 尺寸断言锁每码一行。
  - 死字段与失真注释清理：ResultSummary.loaded 删除（接口定义+derive 回显，零读取）；statusTextFor 注释修正（批次九归位后 hasName 由 reducer 内部计算，不再由调用方传入）。
  - 验证：前端 typecheck + 100 vitest 全绿 + npm run build 成功；后端 go vet + go test 全绿；契约对拍 37 条 PASS；TestSearchAllocsBudget 分配预算 6/7/8 保持；真实服务冒烟（Windows 原生二进制）验证 /api/health 200、/api/search?q=张三 200 正确 JSON、非法 limit 400 {"error":"invalid_limit"}、未知路径 404 {"error":"not_found"}。
  - 明确不做：C7（前端查询解析复制归位）与批次八「契约语料唯一事实源」决策相悖且 Speculative，跳过；不重开 ADR-0001；不引入新依赖。

- 批次九（v0.7.1，架构评审核实后落地）：
  - 修复 Search 负 offset 崩溃：分页前置约定（offset ≥ 0）此前只由 buildMux 的 HTTP 层校验兜底，Search 自身未声明也未自守——实测 Search(students, q, 10, -3) 触发 panic: slice bounds out of range [-3:]。现按既有越界钳制的同一形状在 Search 内把 offset 归一到 [0, len(matches)]，零分配，TestSearchAllocsBudget 保持 6/7/8（预算 12）。HTTP 层 400 契约不变。
  - 状态派生归位 reducer：App.tsx 的 submit 每次手工串联 getState → hasNameCondition → statusTextFor → dispatch，失败分支再串一次 errorMessage；派生规则无测试覆盖。submit-success 载荷收窄为 { items, total, hasMore, query }、submit-error 收窄为 { cause }，派生由 reducer 内部依既有纯函数完成。App.tsx 删除四个随之孤立的导入，submit 由 15 行缩为 9 行。新增 4 个编排行为测试（success/empty/F36/429）。
  - API 路由独立成模块：新增 server/api.go 承载 buildMux 与 searchHandler，HTTP 翻译与查询语义分处两个文件；main.go 只留启动自举/日志/中间件/装配四个薄角色，281→222 行。
  - 数据层 fixture 归位：newTestStore 从 main_test.go 迁至 data_test.go；validGradeOne/Two 留在 main_test.go（属 API 响应语料而非数据层 fixture）。
  - 删除纯形状测试：TestStatusRecorderImplementsFlusher/ImplementsReaderFrom 只断言实现满足 interface，从不调用 Flush 或验证字节搬运——替换为经 accessLog 真实路径的行为断言（Flush 透传且 rec.Flushed 为真；ReadFrom 搬运 10 字节且下游正文完整一致）。
  - ADR-0001（docs/adr/0001-student-type-not-split.md）：Student 不拆分为内部模型与 wire DTO。隐私已由 json:"-" 标签类型强制（NameKey/ClassNo/GradeIdx），并由 TestSearchResponseKeys 逐一断言六个内部字段名不出现在响应中；newStudent 纪律无绕过路径。实测 DTO 转换每请求固定新增 1 次分配，收益为重复保证已有测试所保证的事项。附重启条件。
  - 评审纠错记录：初版报告误称"上限常量在校验处被两次引用"（config.go 已是唯一事实源，实际引用命名常量）、误称"SearchSession.begin/isCurrent 无生产调用者"（invalidate 内部即调用 begin，且三处测试直接驱动），两项均已撤回。


- 批次十（v0.8.0，2026-09-26，核实后架构深化十一项）：
  - NEL 空白语义统一：实测确认 Go strings.Fields / unicode.IsSpace 与 JS \s 在 U+0085 上分歧（Go 切、JS 不切）。tokenizer 与 normalizeName 双端补齐 NEL，corpus 新增对拍用例锁定"张\u0085三 → 姓名张 + 班级3"行为（"三"是汉字数字）。
  - 只读视图强制化：新增 studentStore.Size()，main.go 自举日志不再直接读 store.items 字段。
  - SearchSession 判别联合：submit/loadMore 返回 SearchResult（{ok:true,response} | {ok:false,reason:"stale"} | {ok:false,reason:"error",cause}），App.tsx 不再依赖 undefined 约定。
  - 跨语言常量对拍：server/contract_constants_test.go 读 src/config.ts 断言 PAGE_SIZE/MAX_QUERY_LENGTH/MAX_LIMIT 与 config.go 一致；前端新增 MAX_LIMIT=50；错误文案"80 字以内"改模板插值。
  - 统一 error 码：新增 server/errors.go 常量表（7 个错误码），api.go/ratelimit.go 发射点引用常量，删除字面量。
  - 中间件链重构：securityHeaders 移居链最外（securityHeaders → rateLimit → accessLog → mux），429 自动继承安全头，writeRateLimited 删除手工重放；newTestChain 同构更新。
  - buildMux 注入版本号：buildMux(store, version)，API 层不再读包级 version 变量。
  - 班级解析归位：classToken/classDigits/chineseNumberToInt/classNumber/isAllDigits 迁至 server/classparse.go，search.go 与 data.go 共享，查询语义改动不再静默影响数据文件校验。
  - ResultSummary 单点派生：新增 src/lib/resultSummary.ts（进度/计数/剩余文案），App.tsx 与 ResultList.tsx 四处重复计算归零。
  - 删除 className 兼容别名：后端 json:"class" 保证只输出 class，decodeItem 只认 canonical 单字段，删两个别名测试。
  - ErrorBoundary 文案收窄：覆盖 lazy 结果块，文案改为"查询结果未能加载"，与真实覆盖一致。
- 批次八（v0.7.0 候选，架构深化五候选）：
  - F：删除无生产调用者的前端本地搜索——searchStudents 实现、专属测试与 src/lib/query.bench.ts 一并删除，package.json 移除 bench 脚本；运行时搜索（匹配/排序/分页）唯一归属 server/search.go，前端 query.ts 只保留查询解释（parseQuery/hasNameCondition/normalizeName）。query.test.ts 改为纯解析断言。
  - G：建立可执行跨语言解析契约——docs/query-contract.json 为唯一事实源，src/lib/query.test.ts 与 server/contract_test.go 各自消费；实测发现并修复前端班级号溢出阈值（曾用 > 1000000）与 Go 端 Atoi 不一致的问题，前端改用 Number.isSafeInteger 对齐。语料 34 条，变异验证：破坏任一侧解析即在对拍中失败。
  - H：静态资源缓存所有权收归来源——staticCache 从包级 sync.Map 改为 handler 实例内的 staticCache，实测确认原实现会跨 fs.FS 来源污染（第二个来源拿到第一个来源的内容与 ETag）；新增 assetExists 使 immutable 只给存在资源（缺失 assets/fonts 原本会继承一年 immutable）；Vary: Accept-Encoding 在 raw 200 与 304 上一致补齐（原仅 gzip 200 分支设置）。新增 server/static_cache_test.go 三个不变式测试。
  - A：删除生产死代码 snapshot() 与 probeThrottledAt()（测试改走 view()，契约锁由 TestStoreViewNoCopy 接管）。
  - B：收敛跨端契约常量——server/config.go（端口/分页/上限/限流/缓存头）与 src/config.ts（PAGE_SIZE/MAX_QUERY_LENGTH/REQUEST_TIMEOUT_MS），双端各一份需同步。
  - C：统一客户端 IP 解析——server/ip.go 为唯一入口，maskedIP 复用 clientIP（IPv6 从 unknown 改善为保留前两组 + :::*，隐私红线不落完整 IP）。
  - D：查询语义第三拷贝归零——App.tsx 内嵌 hasNameCondition 正则移入 query.ts 导出（复用 parseQuery），F36 提示由解析结果驱动。
  - E：拆 App 状态机与竞态——src/lib/searchReducer.ts（纯 reducer，11 个 action）+ src/lib/searchSession.ts（请求竞态编排，invalidate 使 clear/onChange/IME 失效在途请求）；App.tsx 204→123 行；错误文案补 code 维度。
- 批次七（v0.6.0，2026-09-24）：
  - F72：server/data.go 文件指纹探测 1 秒节流（probeThrottled 原子 CAS）+ view() 零拷贝只读视图，每请求 3 次 os.Stat 与 134KB 全量拷贝归零（443µs → 17.9ns、0 分配）。
  - F73：server/search.go 预计算 ClassNo/GradeIdx 派生字段（newStudent 统一填充，测试 fixture 同步改造），排序热路径正则清零改纯整数比较；normalizeName 零分配快路径（归一化姓名 Name/NameKey 共享内存）；NewReplacer 提为包级常量；结果切片预分配 + slices.SortStableFunc。整年段查询 819µs/1762 allocs → 34.8µs/7 allocs（23 倍）。
  - F74：server/bench_test.go（4 个基准 + TestSearchAllocsBudget 分配次数预算 12 + TestStoreViewAllocsBudget 预算 4）+ src/lib/query.bench.ts + package.json bench 脚本；分配次数断言是机器无关的确定性回归锁（耗时随负载抖动、分配次数不会）。
  - F75：删除 src/lib/searchTiming.ts 与 searchTiming.test.ts（3 用例），App.tsx 移除 wait()/startedAt 与两处 await wait(1000ms)；StatusOrb 加载反馈保留。
  - F76：修复 Docker 镜像版本恒为 dev——release.yml 的 Docker job 补传 build-args VERSION=${{ github.ref_name }}（此前缺失，Dockerfile ARG VERSION=dev 兜底生效）。
  - 本机 CGO_ENABLED=0 无法跑 -race，由 CI（ci.yml 与 release.yml test job）在 Linux 兜底；web.go/web_test.go 为既有 CRLF 行尾（gofmt -l 会报），非本次变更不触碰。
- 批次六（v0.5.5，2026-09-11）：页脚显示运行时版本——新增 src/lib/api.ts fetchVersion 读取 /api/health 的 version（200/503 degraded 均携带），App.tsx 页脚渲染版本行（失败静默隐藏）；版本事实源仍为 server/main.go ldflags 注入。
- 批次一至三（v0.5.0 ~ v0.5.1）：F1-F62 系列加固，涵盖错误边界、429/503 规范 JSON、CSP 加固、MIME 嗅探根因修复（.js/.mjs 强制类型）、班级汉字解析与子串匹配对齐。
- 批次五（v0.5.3，2026-09-10）：
  - F68：移除结果列表 index<10 动画限制，所有卡片全员启用 Liquid.Item morph 动画；液态库无 morph 时容器为 inline-block 导致第 11 条起宽度坍塌，通过全员 morph + .liquid-result-item width:100% 双重防御彻底修复。
  - F69：移除页面隐私说明与页脚中"联系方式：学校教务处"字样；新增 src/site.config.ts 集中配置数据来源/运营团队/数据处理方（默认福清一中信息社）。
  - F70：支持高三年段，服务端按数据目录实际存在的年段文件探测（空目录明确报错），查询/排序/口语化解析前后端同步镜像，四年级起仅需在 knownGrades 追加。
- 批次四（v0.5.2，2026-09-07）：
  - F63：修复 src/lib/api.ts 中使用 ECMAScript 2024 新特性 AbortSignal.any 导致 Safari < 17.4 及老旧移动端设备白屏的缺陷，引入兼容事件监听回退。
  - F64：修复 server/web.go 静态服务缺少 .woff2 显式 MIME 映射的问题，强制指定为 font/woff2，防止在精简容器环境中回退至 application/octet-stream 或纯文本。
  - F65：修复 server/data.go 名单热重载时的并发惊群风险，引入 reloadMu 双重检查锁定机制，确保高并发重载下 I/O 与 JSON 反序列化唯一执行。
  - F66：修复 server/ratelimit.go 令牌桶集合未自动清理的内存泄漏问题，实现请求驱动的周期性淘汰策略，自动清理超期未活跃桶。
  - F67：修复 src/styles.css 中 --ease-move 自我循环引用的无效声明；对齐 .github/workflows/ci.yml 中 Node 24 文案标注及后端单元测试 -race 并发竞态检测参数。
