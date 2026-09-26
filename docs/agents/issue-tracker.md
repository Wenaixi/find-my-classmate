# Issue 追踪器：GitHub

本仓库的 issue 与规格文档存放于 GitHub Issues。所有操作使用 `gh` CLI 完成。

## 约定

- **创建 issue**：`gh issue create --title "..." --body "..."`。多行正文使用 heredoc。
- **读取 issue**：`gh issue view <number> --comments`，配合 `jq` 过滤评论，并同时获取标签。
- **列出 issue**：`gh issue list --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'`，按需附加 `--label` 与 `--state` 过滤。
- **评论**：`gh issue comment <number> --body "..."`
- **增删标签**：`gh issue edit <number> --add-label "..."` / `--remove-label "..."`
- **关闭**：`gh issue close <number> --comment "..."`

仓库归属由 `git remote -v` 推断；在克隆目录内运行时 `gh` 会自动处理。

## Pull Request 作为 triage 入口

**PRs as a request surface: no。**（若本仓库把外部 PR 视作功能请求，改为 `yes`；`/triage` 会读取此标志。）

设为 `yes` 时，PR 走与 issue 相同的标签与状态机，使用对应的 `gh pr` 命令：

- **读取 PR**：`gh pr view <number> --comments` 查看详情，`gh pr diff <number>` 查看差异。
- **列出待 triage 的外部 PR**：`gh pr list --state open --json number,title,body,labels,author,authorAssociation,comments`，仅保留 `authorAssociation` 为 `CONTRIBUTOR`、`FIRST_TIME_CONTRIBUTOR` 或 `NONE` 的条目（丢弃 `OWNER`/`MEMBER`/`COLLABORATOR`）。
- **评论 / 打标签 / 关闭**：`gh pr comment`、`gh pr edit --add-label`/`--remove-label`、`gh pr close`。

GitHub 的 issue 与 PR 共享同一套编号空间，因此裸写的 `#42` 可能是其中任意一种：先执行 `gh pr view 42`，失败则回退到 `gh issue view 42`。

## 当技能要求「发布到 issue 追踪器」

创建一个 GitHub issue。

## 当技能要求「获取相关 ticket」

执行 `gh issue view <number> --comments`。

## Wayfinding 操作

供 `/wayfinder` 使用。**map** 是单个 issue，其**子 issue** 作为 ticket。

- **Map**：一个带 `wayfinder:map` 标签的 issue，承载 Notes / Decisions-so-far / Fog 正文。`gh issue create --label wayfinder:map`。
- **子 ticket**：作为 GitHub sub-issue 链接到 map 的 issue（`gh api` 调用 sub-issues 端点）。若未启用 sub-issues，则把子 ticket 加入 map 正文的 task list，并在子 ticket 正文顶部标注 `Part of #<map>`。标签为 `wayfinder:<type>`（`research`/`prototype`/`grilling`/`task`）。被认领后，ticket 指派给主导开发者。
- **阻塞关系**：使用 GitHub **原生 issue 依赖**，即规范化、可在 UI 中直接查看的表达。添加边的命令为 `gh api --method POST repos/<owner>/<repo>/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>`，其中 `<blocker-db-id>` 是阻塞项的**数据库 id**（用 `gh api repos/<owner>/<repo>/issues/<n> --jq .id` 获取，**不是** `#number` 或 `node_id`）。GitHub 通过 `issue_dependencies_summary.blocked_by` 报告未关闭的阻塞项（即实时门禁）。若依赖功能不可用，回退为在子 ticket 正文顶部写 `Blocked by: #<n>, #<n>`。当所有阻塞项关闭后，ticket 即被解除阻塞。
- **前沿查询**：列出 map 下未关闭的子 ticket（`gh issue list --state open`，限定在 map 的 sub-issues / task list 范围内），剔除存在未关闭阻塞项（`issue_dependencies_summary.blocked_by > 0`，或 `Blocked by` 行中仍有未关闭 issue）或已有指派人的 ticket；按 map 中的顺序取第一个。
- **认领**：`gh issue edit <n> --add-assignee @me`，作为本次会话的首次写操作。
- **解决**：`gh issue comment <n> --body "<answer>"`，然后 `gh issue close <n>`，最后向 map 的 Decisions-so-far 追加一条上下文指针（gist + 链接）。
