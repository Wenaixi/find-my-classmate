# 更新记录

本文件记录 FindMyClassmate 的版本变更。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

## [v0.11.0] - 2026-10-01

三批架构评审的合并版本。两条用户能看见的缺陷（输入法组合期卡死、状态行 chunk 失败白屏），一处名存实亡的对拍，一处测量环境错配。每条都做过变异实验：改坏实现，测试必须翻红。

### 修复

**输入法组合期输入变化让界面永久卡死。** `onInput` 无条件调 `session.invalidate()` 作废在途请求，fetch 被 abort，查询会话归为 stale。而 `submit` 只对真实错误写回状态，stale 直接 return，于是 state 永远停在 `submit-start` 写入的 loading。submit 的守卫恰好是「loading 中不发起新请求」，按 Enter 毫无反应，得再敲一个非组合字符或按 Escape 才脱困。挂载探针实测渲染序列是 `idle → editing → loading → loading → loading`，删掉 `invalidate()` 后终态变 `error`。

`searchReducer` 早就为 loadMore 侧修过同族问题（stale 的 `loadingMore` 复位在 reducer 内显式完成，注释里写着），submit 侧只是少一个分支。现由 `submit-result` 单 action 收编 ok / error / stale 三分支，与 `load-more-result` 对称；stale 落到 `editing`，因为组合期的输入已写进 query 而针对它的查询还没发。`submit-success` 与 `submit-error` 随即成了生产零派发的死分支，一并删除。

**整套加载界面从没渲染过。** hook 壳对 Promise 型动作只在落定后触发重渲染，而 `submit` 的函数体在第一个 `await` 之前就同步跑完 `submit-start` 然后挂起，整段请求期间零次重渲染。探针实测序列 `idle → editing → success`，loading 帧一次都没出现。结果区的 loading 区段、加载指示、提交禁用、按钮文案、加载更多禁用态全都不可达，而状态机单测为这些派生值写了完整双向断言且全绿——断言的对象在生产中一次都不会被渲染。现对 Promise 型动作分两段渲染。

两条互相放大：loading 不上屏反而盖住了上一条的死角。只修一条会把死角变成显眼的永久 spinner。

**姓名含完整年段串被读成年段条件。** 输入真实人名「高一鸣」，`parseQuery` 返回 `{NameTokens:[], Grade:高一}`，姓名条件整个消失，`Search` 返回整个高一年级。探针实测修复前 `total=2`，返回 `[高一鸣 张伟]`，那个「张伟」跟查询毫无关系。

根因是一份实现服务两种必须不同的判定：`parseGrade` 用子串匹配，被查询侧和名单标题校验共用。标题形如「福清一中2025级高一编班名单」，年段只是标题的一部分，确实需要子串语义；查询侧不需要。现拆为 `parseGradeInToken`（查询侧精确匹配，整个 token 就是年段或年段+「班」）与 `parseGradeInTitle`（标题侧子串匹配），前端镜像同步改精确。

这是同一族缺陷的第四个实例，前三个是无法解析的班级 token 不得静默丢弃、纯分隔符不得退化成全校检索、「高一0班」不得放大成整年段全量。契约语料当时把缺陷行为锁成了期望值——「高一同学」条目断言 nameTokens 为空且 grade 为高一，却没写 `$comment` 说明依据，那是被记录下来的实现行为而不是产品裁决。现改为按姓名处理并补 `$comment`，另补「高一鸣」（缺陷直接样本）、「高1鸣」（别名形态）、「高一班」（纯年段对照组，防止判断退化成一律按姓名）。

**跨语言查询镜像在「年段+溢出班级」形态上分叉。** `query.ts` 的年级+班级连写分支里，班级号溢出时直接 `continue`，`parsed.grade` 从未赋值；Go 端同一形态先写 Grade 再降级为姓名条件。同一输入「高二99999999999999999999班」，Go 得「高二」，修复前的前端是 undefined。

这个分叉此前没有用户可见症状——前端解析结果在生产中从不上传，唯一消费点只读 nameTokens 长度——但它是同一条规则的两个镜像实现对不上的实证。修复后两端五组输入逐行一致。

**名单加载的四个失败出口不带年段。** 标题不一致、名单结构异常、班级格式异常、学生记录格式异常此前都是裸 `errors.New`。数据目录通常同时放多份年段文件，运维看到「data unavailable: 班级格式异常」无从判断该看哪一份，三选一的猜测，且随年段数增长线性变差。更硬的一层：`recordFailure` 的注释声称比较单位是「同一年段的同一种解析失败」，而这四个出口的文本里根本没有年段。

标识带上之后，一次持续不可用期间修好一份年段、同时弄坏另一份且成因相同，指纹变化会越过冷却、重试、在新文件处失败，文本与上次不同因而补记一条；此前两种情形文本相同、零条新日志，而运维看到的是一条没有指向的成因。班级格式异常额外带上班名。

**数据不可用的根因在健康端点完全不可见。** `/api/health` 用 `_` 丢弃 `view()` 的错误且不产出任何日志，`recordFailure` 只更新状态不记录。探针实测：5 次健康探针（每 30 秒）→ 站点持续 503 degraded → 日志字节数为 0。配上 compose healthcheck 与零用户流量，真实故障链是「管理员写坏名单 → 无人查询 → 站点持续 503 → 运维侧零痕迹」。

现由 `recordFailure` 在故障状态转折时单点记录，判据是此前无失败记录或根因文本变化，`/api/search` 不再重复记。同一探针序列修复后恰好 1 条。记转折而非每次请求，是因为健康探针每天约 2880 次，逐次记会把根因淹没在噪音里。根因按错误文本比较而非 `errors.Is`，`loadStudents` 每次都新建错误值，`errors.Is` 永不匹配。响应契约没动：health 仍 503 degraded，search 仍 500 data_unavailable。

**错误区段在未知错误形态下重复同一句文案。** 区段标题由组件硬编码为「查询没有完成」，而未知错误形态下的状态文案走错误分类兜底分支，返回「查询没有完成，请稍后重试」，同一句前缀在同屏出现两次。分类文案明确时不重复，恰好掩盖了它。现把切分收进展示派生，使这条跨两个模块的不变量在派生处即被排除。

**inert 修复只覆盖了两个装饰 SVG 中的一个。** 动效库渲染 `data-gooey-svg` 与 `data-gooey-overlay` 两层，库自己的 MutationObserver 用 `closest` 把二者当同类，而 overlay 的 z-index 是 9999、库注释自陈「Above the content layer by design」，内含与 svg 层结构对称的 `g`/`defs`/`mask`。两层的键盘焦点陷落来源相同。现覆盖两层，断言改为逐层检查——断总数的话只覆盖一层的实现同样能过。

**服务器初始化日志失败时无可行动指引。** `resolveLogDir` 的回退是数据目录下的 log 子目录，而容器里数据目录通常只读挂载，这条回退在它注释自称的场景里必然失败。两个真实部署都显式设了 `FMC_LOG_DIR` 规避，所以今天不触发；但删掉那行环境变量的镜像变体会静默落进死路，运维只看到「初始化日志失败」。

### 状态行的 chunk 失败会带走整页

页面组装有两个 `lazy` chunk，结果区那条包在 `ErrorBoundary` 里，状态行那条没有，而它无条件挂载。`StatusOrb` 一旦抛错（部署切换后旧 chunk 404、网络抖动导致的模块求值失败），错误冒泡到 React 根，连有自己边界的 ResultList 一起消失，正是 `ErrorBoundary` 注释自陈要防的整页白屏。现给状态行补上同一个边界，边界放在 `Suspense` 之外才能捕获 chunk 拒绝。

回归用例单独一个文件：`vi.mock` 是文件作用域且在模块求值期生效，同一文件里既要走正常模块又要走抛错模块需要 `resetModules` 加动态 import，在 vitest 3 下会触发 hoisting 限制。

### 跨语言对拍能被一行注释关掉

年段值域对拍用 Go 正则从 `src/lib/query.ts` 抽取 `gradeValues` 与 `gradeAliases` 声明，这两条抽取器原先都没有行首锚定（`(?s)` 而非 `(?ms)^\s*`），上方注释里一行被注释掉的同名声明会被当成真声明。

把真实声明改成别的年段、同时在上一行留一条带正确清单的注释，对拍报告 PASS，而生产跑着一份完全不同的年段清单。四场景实测，修复前后对照：未改动 PASS→PASS，真实声明漂移 FAIL→FAIL，漂移加注释诱饵 PASS→FAIL，仅诱饵而真声明完好 PASS→PASS。

这是本仓唯一一处「保护机制报告通过却什么都没在保护」的地方。年段值域是最贵的跨语言不变量，漏改一端的后果是合法名单被判「文件名与年级标题不一致」而拒绝加载、全站 503。已知上限：`(?m)^` 只挡行注释，块注释内部的行首同样是行首。

### 分配预算与竞态插桩的环境错配

推送后 CI 首次变红，`TestSearchAllocsBudget` 的「汉字数字班级」实测 11.0、预算 10。这不是改动引入的分配退化，而是预算与判定环境错配：CI 跑 `go test -race`，竞态插桩会抬高每条路径的分配数，而本仓历来的预算都是在无插桩的本机按实测值收紧的。

用一次性的诊断 workflow 在同一 runner 上取「唯一变量是 `-race`」的两组数据定案：

| 样本 | 不带 race | 带 race |
| --- | --- | --- |
| 单姓名查询 | 6 | 7 |
| 整年段查询 | 7 | 8 |
| 组合查询 | 8 | 12（正好触顶） |
| 汉字数字班级 | 9 | 11（越过预算 10） |

那种红测的是竞态检测器的插桩开销，当成分配退化去「修」，会为了迁就测量工具而改动零分配热路径。现按构建标记在插桩下跳过，`-race` 不暴露运行时标志，构建标记是唯一可靠判据。代价是预算对「插桩下的分配」从此完全失明，而这是有意的取舍：竞态覆盖仍在，只是分配数不再在插桩环境里判定。

跳过逻辑本身也做了变异确认：把 `isRaceEnabled` 硬编码为 `false`，CI 上两条样本同时翻红（组合查询 13.0/预算 12、汉字数字班级 11.0/预算 10），既证明跳过真的在起作用，也说明插桩影响每条路径而非单点。该变异已回滚。

本仓此前把「分配预算」与「CI 全绿」当作彼此的推论，实际上它们量的是两个环境。

### 变更

**年段匹配拆成查询侧与标题侧两个语义。** 见上面「姓名含完整年段串」那条。

**删除一条恒不发作的编译期检查。** `classDigits` 的键集自洽检查以 `Exclude<元组键, keyof typeof classDigits>` 表达，而 `classDigits` 经 `Object.fromEntries` 构造后键类型被拓宽为 `string`，`Exclude<X, string>` 对任何 X 都是 `never`，永不发作。实测把值表整个换成 `{ "十": "10" }`，`tsc -b` 仍 CLEAN。

拓宽有两个来源，`Record<string, string>` 标注只是其一，`Object.fromEntries` 自身的签名也会拓宽键类型，所以删标注无济于事——我最初判断根因是那个标注，是错的。

删除而非修复：要让它咬合就得显式手写十字值域作为被检查对象，那是 `chineseDigitOnes` 之外的第二份手抄声明，而它要防的漏字在派生写法下本就结构性不可能。

**前端把降级策略收成 `classCondition`，班级判定改用显式三态。** 「无法解析 / 溢出 → 降级为姓名条件」此前内联在查询解释器的四处，两对逐字等价、部分理由只存在于其中一处靠「照抄旁边那段」维持。后端同名函数已收拢该策略，其注释自陈经历过同样阶段（「此前 parseQuery 有四段近乎逐字重复」），而前端至今停在那里。同一条规则的两个镜像实现分叉过一次，修复手段是手工逐点补一行赋值。形状不变，裂缝会再开。

班级判定此前用 `-1`（溢出）与 `0`（非法）两个哨兵，两者行为完全相同、零信息量。改掉后 Overflow 一值在两端都零生产消费方，三态化的收益是类型层而非行为层。

**加载区的三条界面知识收进展示派生单点。** 是否有后续页、是否正在加载、上次是否失败此前在 `ResultList` 里各判一次，而展示派生自称折出「视图需要的全部派生」。同一形状的知识一分为二：查询的忙碌态归状态提示派生，加载更多的却归组件。现折成 `loadMoreZone: { show, busy, error }`。

`show` 取代单独下传的 `hasMore` 是因为结果摘要内部已把同一个布尔编码过两次（工具栏状态、加载区文案），再传一份独立的等于让同一事实跨接缝两次，组件因此能渲染出「工具栏说已全部加载、按钮却还在」的自相矛盾界面。改后 ResultList 的 interface 从六个 props 收到四个。

**查询骨架收成单一实现。** `submit` 与 `loadMore` 此前各手写「取查询词 → 守卫 → 启动态 → 发起 → 派发结果」，start/result 这对 action 的配对与「三分支必须一次派发完毕」这条不变量在两处各写一份而互不引用。后者是「作废的在途查询让界面永久卡死」换来的。现收进私有 `run`，配对由类型承载，传错即编译报错。

守卫刻意留在各自动作里：两者的守卫集合本就不同，合并会掩盖差异。

**日志与中间件各自独立成文件。** `logging.go` 约 54 行此前住在 `main.go`，却被热重载、数据不可用、访问日志三个 module 消费，改日志行为必须先读懂整个装配流程。`middleware.go` 的 76 行同理，而测试侧早已按关注点归位成四个文件——实现欠了测试侧的债。`main.go` 从 227 行降到 88 行。

两处都没引入 interface：当前只有标准库一种实现，凭空造接缝是负债而非加深。链顺序的唯一实现 `newHandlerChainWith` 留在 `main.go`，跨文件的顺序知识仍只有一处。

**年段值域的反向完整性改由编译期强制。** 此前该方向由测试里手抄的 `gradeUnion` 数组承担，而手抄副本在 `Grade` 联合追加成员时不会跟着长——实测加「高四」后 `tsc -b` 干净、全部前端用例通过，而 `parseQuery` 已不再识别该年段。现由 `query.ts` 的编译期检查承担，报错直接点名缺失年段。

前提是 `gradeValues` 不能写成 `ReadonlyArray<Grade>`：显式标注会把字面量拓宽回 `Grade`，使 `Exclude` 恒为 `never`。改用 `as const satisfies`，`as const` 保留字面量类型供反向检查，`satisfies` 保住原有的正向强制，两者缺一不可。检查刻意落在生产文件，容器构建排除测试文件，放进测试则镜像内的类型检查不再检查它。

随之删掉手抄的 `gradeUnion` 与那两条断言：它们有判别力但方向反直觉（`gradeValues` 增长而副本没跟上时反而翻红，惩罚一次正确的扩展），在真正关心的裂缝上恒绿。

**状态全集成为可被测试消费的声明。** 它此前是模块内 `const` 而非 `export`，测试里的四份手抄数组一个都没被收走。给状态联合加一个成员后，生产侧两处受强制，那四份自称「穷尽性断言」的用例全部保持绿色。

**分配预算的样本集扩到解析分支的取值域。** 三条样本全部走阿拉伯数字班级，汉字数字路径与降级路径一次都没进入预算，而落在那两条路径上的分配退化不会翻红。补四条样本并按实测收紧。预算是上限不是实测值，且对行为退化是失明的——降级路径若被错误实现改回「静默丢弃 token」，分配反而下降而预算全绿。

**服务器超时提为具名常量。** 实现与测试此前各写一份字面量，两处可以各改一处而不冲突，断言看似承重却与实现无共同事实源。没进 `config.go`，因为那个文件的边界是「能与前端对拍的跨语言契约常量」，而这四个值只有 Go 一侧。

**挂载测试的 act 环境声明收敛为共享模块。** 没抽 vitest `setupFiles`：全局 setup 会让默认 node 环境的纯逻辑测试也带上该标志，使「纯逻辑测试零 DOM 依赖」这条纪律失效。导入行为本身就是一次显式声明，漏写时会在收集期直接报「无法解析该模块」。

**修正一处形同虚设的测试垫片。** canvas 的 `getContext` 垫片改的是二维上下文原型，而该方法是 canvas 元素的方法，整个条件分支从不进入。

**隐私说明里去掉重复的机构署名。** 原文案在隐私边界里写「数据处理：福清一中信息社」，而页脚已有「运营团队：福清一中信息社」，同一个机构名在页面上说了两遍。现只保留页脚那处。

### 测试

前端 198 → 206 用例（11 个测试文件）。下面几条都是修复前全量绿、修完精确翻红的：

- **加载更多进行中此前零断言。** 全仓唯一的 `loadingMore` 取值是 `false`，两条 `hasMore` 用例都在它为假时渲染按钮，因此按钮禁用态与「正在加载」文案从无一次被真正检查。
- **加载更多的 offset 此前零断言。** 既有用例全部在 `await` 之后取样，只断条目增长或请求次数，从不看请求参数。offset 改成常量 0（永远重新请求第一页并原地追加）修复前全量通过。新断言双向：正向断 offset 等于已加载条数，反向断两次请求取样点不同。
- **提交守卫的「加载更多进行中」一项此前零断言。** 删掉它，加载更多进行中按 Enter 会并发发起新查询，而新查询的启动态会清空条目。
- **错误区段文案的三条不变量此前只在端到端层被守住。** 标题去前缀、去掉后为空则保留原句、分类文案原样保留，而这是展示派生里唯一跨两个模块的不变量。三条断言取三个给出不同结果的输入，互不重复。
- **名单加载的年段标识有 3/4 零断言。** 从三个出口去掉年段标识，Go 全量仍绿。现三条断言各自对应一个出口，剥离后恰好 3 条翻红。
- **排序第一键「匹配度」补判别性 fixture。** 全仓四条顺序断言的取样点此前全部落在同分格，变异把第一键置为恒返 0 时全量零翻红，而探针证实顺序确实翻转。新 fixture 让匹配度键与班级号键给出相反顺序，任一键失效都会使顺序翻转。
- **「过期响应被丢弃」的断言改为不依赖时序的双向断言。** 原用例断言 state 不是 loading，声称的对象是「过期响应不会到达状态机」，实际只排除七个取值中的一个。取样点改为过期响应抵达后、后续查询覆盖之前。
- **失败根因「变化」半支补双向用例。** `recordFailure` 的判据有两个析取项，既有用例只覆盖「此前无失败记录」，而它的第二轮是「先恢复再损坏」，恢复会清空记录根因，走的是前一个半支。
- **查询状态补编译期穷尽性检查。** 给状态联合加一个新成员后类型检查报两处。
- **前端「是否含姓名条件」补断言。** 该布尔是前端唯一消费的解析输出，解析层虽有契约语料对拍，布尔本身此前零断言。只断含年段串那一侧的话，把判断退化成「一律按姓名」同样能过。
- **名单加载两道 fail-closed 守门补承重断言。** 七个失败出口中「名单结构异常」与「学生记录格式异常」此前全仓一次未触发，删掉守门后全量零翻红，而行为确实坏了。附带记一条边界：`Roster == nil` 拦得住显式 `null`，拦不住 `"名单": {}`，守的实为「字段缺失或为 null」而非「名单为空」，空名单是可接受的部署状态，不作为缺陷。

### 撤销的候选

记录在此以免后续评审重复提出。

- **「每个 token 至少产出一个条件无保证」推翻。** 探针走 8477 条非零 token 输入零蒸发，残留的「未来分支静默 continue」风险属推测性。真缺陷是另一件事：`Query` 的年段与班号是单槽位字段，后写 token 会无条件覆盖先写 token（「高一 1班 11班」得班号 11）。该行为零断言，而契约语料的 6 条多 token 条目无一含同槽位冲突。已记为规格未定等产品裁定，裁定前不把当前行为写进断言。
- **安全头与资源缓存头的 `Cache-Control` 冲突，降级为缺一条组合断言。** 变异零翻红，但真实 `net/http` 服务器探针证明当前行为正确。`httptest.NewRecorder` 不做 header 快照，用它做这项实验会得到假象。
- **「数据不可用」需要统一 owner 撤销。** health 的 503 degraded 与 search 的 500 data_unavailable 是刻意差异：探针要存活视图，搜索要业务错误码。
- **客户端 IP 需要 XFF adapter 撤销。** 部署形态为容器直接暴露 3078，不存在第二个 adapter。
- **前端展示 module 应收掉撤销，改判加深。** `useSearchInput` 形态浅，但两条 IME 守卫变异实验证明双双承重。删除测试的答案不是「复杂度消失」而是「不变量失守」。
- **静态资源协商需从传输中拆出撤销。** raw 200 / gzip 200 / 304 三处的 `Vary` 已逐一断言。
- **「signal 归属跨 module 分裂」为误报。** `searchApi` 不接受调用方 signal 是正确归属。

### 文档

- 五处陈述与代码不符的注释与文档收敛为事实：状态机把「穷尽性由类型强制」归错了地方、错误码测试声称「前端按 code 分文案」而前端只读 HTTP 状态码、网络适配层称「扩展年段只需改一处」而实际四处、站点配置称「所有字段均为可选」实际三项皆必填、前后端常量文件各自自称「唯一事实源」实为双写同步。
- 架构文档另更正三处：年段派生的 owner 由两处更正为四处、标题校验的依赖方向、排序规则补上多 token 求和语义。这三处最初只落在 `ARCHITECTURE.md`，`CONTEXT.md` 与 `search.go` 的注释漏改，而 `CONTEXT.md` 曾把前端 `parseGrade` 描述为「标题子串匹配」，与该函数的实际语义恰好相反。
- `gradeClassToken` 与 `rebuildGradePattern` 已在早前删除，活文档仍按它们陈述规则。「编译结果必须缓存、分配 6 涨到 117」改为仍然有效的分配纪律，教训保留，对象不再虚构。
- 跨语言声明覆盖表两行订正：汉字数字值表行记的「前端编译期键集检查」不存在；空白集合行的「三个差集码位」实为两处。
- 记忆文档自身也有失真并已更正：「96 行中间件实现」实为 76 行、「当前交付」段复制了已被推翻的结论。记忆文档被引用得越多，被错误结论污染得越深。

### 验证

前端 206 用例、`npm run typecheck`、`npm run build`、后端 `go clean -testcache` + `go test ./...`、`go vet` 全绿，`gofmt` 归一化行尾后无输出。分配预算 6/7/8 与 `view()` 的 0 分配在本版全部改动后未变（基准实跑核对，非引自文档）——控制器骨架与展示派生的收拢都在前端，不触及 Go 热路径。

冒烟用合成名单，覆盖阿拉伯班名、汉字班名、无「班」字三种写法与年级+班级连写：四条已知不变量（纯分隔符、降级「一一班」、「高一0班」、空 q）均 total=0 未退化成全校检索；「高一鸣」「高一同学」按姓名处理并各命中本人，对照组「高一班」「高一」返回整个年段 6 条；响应条目键恰为 name/grade/class；400/404/405 三条错误码正确；静态资源 raw 200 带 immutable + ETag + `Vary`，`gzip;q=0` 正确不压缩，`If-None-Match` 命中回 304。

发布产物已核：三平台二进制与归档包齐全，包内 `data/` 是空目录无任何名单 JSON，Linux 二进制里 `v0.11.0` 出现 1 次而 `v0.10.4` 为 0 次。## [v0.10.4] - 2026-09-27

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
