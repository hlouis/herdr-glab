# herdr-glab 设计文档

herdr 的 GitLab 插件：在 overlay 面板里查看和我相关的 MR，对 MR 做 review / checkout 等操作，并在 workspace 侧栏显示当前分支的 MR 状态。

herdr 插件机制见 [herdr/README.md](herdr/README.md)。本文所有 GitLab 字段和 herdr 命令均已在 herdr 0.9.0、GitLab 18.11 EE（自建）、glab 1.117 上实测。

## 1. 范围

**已实现**

1. MR 面板（overlay）：按「请我 review / 指派给我 / 我创建 / 提到我」分组，显示流水线、审批人、合并状态、thread 计数。
2. MR 操作：tuicr review、checkout 到 worktree、跳到已有 workspace、浏览器打开、复制链接、刷新。
3. 侧栏 `$mr` token：workspace 当前分支有 opened MR 时显示状态，不限于和我相关的 MR。
4. tab 行计数：总数与需要我处理的条数。
5. 单 MR 浮层：Ctrl+点击任意 MR 链接，或用选中文本、剪贴板里的链接打开。
6. 讨论抽屉：查看讨论、解决与重开、把讨论交给 agent（最后一项尚未实测）。

**不做**

- **回复讨论**：待做，需要文本输入框和草稿持久化。
- **自建 diff 视图**：tuicr 已经内联显示 MR 讨论并支持提交行内评论，重复造收益低。它缺的「回复 + 解决」已有 [PR #599](https://github.com/agavra/tuicr/pull/599) 实现但作者想自己设计交互，先观望。
- 多个 GitLab 实例、自动 clone 本机没有的仓库、Windows（见 README 的平台说明）。

## 2. 依赖

| 依赖 | 用途 |
|---|---|
| herdr ≥ 0.9.0 | 插件宿主 |
| `glab`，已对目标实例登录 | 所有 GitLab 请求走 `glab api graphql --hostname <host>`，插件不接触 token |
| `git` | 读 remote / 当前分支，fetch MR 分支 |
| `tuicr` | review 操作；未安装时该操作提示缺失 |
| Go 工具链 | 可选。`[[build]]` 执行 `install.sh`：有 Go 就编译源码，没有就下载对应平台的 Release 二进制 |

## 3. 架构

一个 Go 二进制 `bin/herdr-glab`，按子命令区分入口。数据流：

```
               glab api graphql
                     │
 poller（后台常驻）──┴─→ cache.json ──→ panel（overlay 面板，只读缓存）
     │                       │
     └──→ herdr workspace report-metadata  ←── tokens（事件钩子，只读缓存）
```

- **poller**：定时拉取 GitLab 数据写缓存，并刷新所有 workspace 的 token。
- **panel**：读缓存渲染，`R` 手动刷新时调用和 poller 相同的拉取逻辑。
- **tokens**：事件钩子入口，只用缓存和本地 git 重算指定 workspace 的 token，不发网络请求。

缓存写入用「临时文件 + rename」，poller 和 panel 并发写入都不会产生半截文件。

### poller 生命周期

`[[startup]]` 只在服务启动恢复会话后执行，link/enable/重载配置不执行（[plugins.md](herdr/plugins.md) → Startup hooks），所以每个事件钩子和 `refresh` 动作都先执行 `ensure`：

- PID 记录在 `$HERDR_PLUGIN_STATE_DIR/poller.json`，内容含 `pid`、`socket_path`、`binary_mtime`。
- 进程不存在、`socket_path` 不同（换了 herdr server）或二进制已更新时，重新拉起一个脱离父进程的 poller。
- `stop-poller` 动作写入停止标记并结束进程；下次 `refresh` 清除标记并重新拉起。

## 4. GitLab 数据

### 4.1 和我相关的 MR

一次 GraphQL 请求取三类列表，每类 `first: 50`，按 `pageInfo.hasNextPage` 翻页：

```graphql
query($authoredAfter: String, $reviewAfter: String, $assignedAfter: String) {
  currentUser {
    username
    authoredMergeRequests(state: opened, first: 50, after: $authoredAfter) { pageInfo { hasNextPage endCursor } nodes { ...mr } }
    reviewRequestedMergeRequests(state: opened, first: 50, after: $reviewAfter) { pageInfo { hasNextPage endCursor } nodes { ...mr } }
    assignedMergeRequests(state: opened, first: 50, after: $assignedAfter) { pageInfo { hasNextPage endCursor } nodes { ...mr } }
  }
}

fragment mr on MergeRequest {
  iid title webUrl draft updatedAt
  sourceBranch targetBranch diffHeadSha
  detailedMergeStatus
  approved approvalsLeft
  resolvableDiscussionsCount resolvedDiscussionsCount userNotesCount
  author { username }
  project { fullPath sshUrlToRepo httpUrlToRepo }
  sourceProject { fullPath }
  headPipeline { status }
  approvedBy { nodes { username } }
  reviewers { nodes { username mergeRequestInteraction { reviewState approved } } }
}
```

「提到我的」没有对应的 MR 列表接口，只能读 todo：

```graphql
query($after: String) {
  currentUser {
    todos(action: [mentioned, directly_addressed], type: [MERGEREQUEST], state: [pending], first: 50, after: $after) {
      pageInfo { hasNextPage endCursor }
      nodes { target { ... on MergeRequest { ...mr } } }
    }
  }
}
```

MR 合并或关闭后 todo 仍在，所以只保留 `state == "opened"` 的。

同一个 MR 可能出现在多个列表里，按 `webUrl` 合并，`roles` 记录它属于哪几类（`author` / `reviewer` / `assignee` / `mentioned`）。

### 4.2 workspace 分支对应的 MR

侧栏要覆盖不属于「和我相关」的 MR。poller 每轮收集所有 workspace 的 `(project fullPath, 当前分支)`，排除已在 4.1 结果里的，再用别名批量查询，每个项目一个别名：

```graphql
query {
  p0: project(fullPath: "group/repo") {
    mergeRequests(sourceBranches: ["feat/a", "fix/b"], state: opened, first: 20) { nodes { ...mr } }
  }
}
```

这类 MR 存入缓存的 `branch_mrs`，不出现在面板里。

GitLab 限制单次查询复杂度 300（`queryComplexity { score limit }` 可查）。实测 4.1 的三个列表合计 194；4.2 每个项目别名在 `first: 20` 时约 48，所以每次请求最多 4 个项目，超过就分批。

### 4.3 派生字段

| 字段 | 计算 |
|---|---|
| 未解决 thread 数 | `resolvableDiscussionsCount - resolvedDiscussionsCount` |
| 我的 review 状态 | `reviewers` 中 `username == currentUser.username` 的 `mergeRequestInteraction.reviewState` |
| 是否 fork MR | `sourceProject.fullPath != project.fullPath` |

### 4.4 讨论

抽屉按需拉取，不进轮询：

```graphql
discussions(first: 20, after: $after) {
  pageInfo { hasNextPage endCursor }
  nodes { id resolved resolvable notes(first: 20) { nodes { system createdAt body author { username } position { filePath oldLine newLine } } } }
}
```

复杂度 24。只保留 `resolvable` 的讨论，丢掉系统事件（「requested review from …」）与不可解决的评论；`position` 给出文件与行号，行号优先取 `newLine`。

解决与重开走 `glab mr note resolve|reopen <讨论id> <iid> --repo <项目>`，host 由 `GITLAB_HOST` 传入。讨论 id 取 `gid://gitlab/Discussion/<hex>` 的最后一段。

## 5. 缓存格式

`$HERDR_PLUGIN_STATE_DIR/cache.json`：

```json
{
  "version": 1,
  "host": "gitlab.hlouis.com",
  "username": "louis",
  "fetched_at": "2026-09-15T16:00:00+08:00",
  "error": "",
  "mine": [
    {
      "roles": ["assignee"],
      "iid": 412,
      "title": "feat(customer-service): 推荐回复链路每日快照导出",
      "web_url": "https://gitlab.hlouis.com/project-immt/atlas/atlas-server/-/merge_requests/412",
      "project": "project-immt/atlas/atlas-server",
      "source_project": "project-immt/atlas/atlas-server",
      "source_branch": "feat/export-rec-pipeline",
      "target_branch": "main",
      "head_sha": "…",
      "draft": false,
      "author": "kkdy",
      "updated_at": "2026-09-15T14:37:33+08:00",
      "merge_status": "UNCHECKED",
      "pipeline": "SUCCESS",
      "approved": true,
      "approvals_left": 0,
      "approved_by": ["codebuddy"],
      "threads_total": 10,
      "threads_unresolved": 1,
      "notes": 33,
      "reviewers": [{ "username": "codebuddy", "state": "APPROVED", "approved": true }],
      "my_review_state": ""
    }
  ],
  "branch_mrs": []
}
```

`error` 记录最近一次拉取失败的原因（glab 未登录、网络错误等），面板顶部显示；失败时保留上一次成功的数据。

## 6. workspace 与 MR 的匹配

### 6.1 取 workspace 的仓库

1. `herdr workspace list` 中有 `worktree.checkout_path` 的，直接用它。
2. 没有 `worktree` 字段的，取该 workspace 第一个 pane 的 `cwd`（`herdr pane list --workspace <id>`），执行 `git -C <cwd> rev-parse --show-toplevel`，失败则视为非仓库。

### 6.2 仓库对应的 GitLab 项目

`git -C <repo> remote -v` 的每个 URL 规范化为 `host/fullPath`：去掉协议、用户名、端口、`.git` 后缀，SCP 形式 `git@host:path` 转为 `host/path`，统一小写。host 等于配置的 GitLab host 的 remote 才算匹配，并记下 remote 名，checkout 时 fetch 用。

### 6.3 workspace 当前 MR

`git -C <checkout> branch --show-current` 得到分支，按以下顺序匹配缓存：

1. 分支形如 `mr/<iid>`（fork MR 的本地分支，见 8.2）：同项目 `iid` 相同的 MR。
2. `project == 项目 fullPath` 且 `source_branch == 分支`，先查 `mine`，再查 `branch_mrs`。

## 7. 面板

`[[panes]]` 入口 `panel`，`placement = "overlay"`。`panel-open` 动作调用 `herdr plugin pane open --plugin hlouis.glab --entrypoint panel` 打开。面板用 bubbletea 实现，只读缓存，每 5 秒检查缓存 mtime 自动重载。

### 7.1 列表

按分组展示，每个 MR 占两行：第一行标题，第二行状态。MR 之间空一行，组之间有标题和分隔线。

```
Review requested (1)
────────────────────────────────────────────────────────
  atlas-client !318  fix(customer-service): 邮件回复页搜索后自动选中匹配会话第一条
    ✔ success · ✓ approved · ✎0/3 threads · rebase · fix/264-… → main · kkdy · 2d ago · ○

Assigned to me (2)
────────────────────────────────────────────────────────
▌ atlas-server !412  feat(customer-service): 推荐回复链路每日快照导出
    ↻ running · +1 approvals · ✎1/10 threads · feat/export-… → main · kkdy · 1h ago · ●
```

第二行的内容依次是：流水线、审批、thread、需要处理的合并状态、源分支 → 目标分支、作者、更新时间、本地状态。

审批显示具体的人：`✓ codebuddy` 是已批准的人，`+N approvals` 是还差几个，`⧗ ai.tan` 是指派了但还没批准的 reviewer。不能用 `approved` 字段判断，项目不要求审批时它恒为 true。

thread 显示 `✎已解决/总数`，还有未解决时标黄。侧栏 token 用同样的写法。

`?` 帮助页列出全部图例：选中标记、流水线、审批、thread、合并状态、本地状态，以及侧栏 token 的格式。取值规则见第 9 节的符号表；`●` 表示已有 workspace 检出该分支，`○` 表示仓库在本机但分支未检出，两者都没有表示本机没有这个仓库。

选中的 MR 整块加背景色（复用侧栏选中行的 `#45475a`）并在行首显示 `▌`。选中块内不上色，因为颜色重置会把背景冲掉。

内容宽度取终端宽度，但限制在 40～120 列之间：终端很宽时拉满一行的分隔线和标题都难读。

### 7.2 分组与排序

分组顺序固定，一个 MR 只出现在第一个命中的组里，其余角色在标题行末尾以 `also …` 标出（标题按剩余宽度截断，标记始终可见）：

1. Review requested — 请我 review
2. Assigned to me — 指派给我
3. Authored by me — 我创建的
4. Mentioning me — 提到我的

组内按 `updated_at` 倒序，draft 沉到组尾。

### 7.3 按键

| 键 | 操作 |
|---|---|
| `j` / `k`、方向键 | 移动 |
| `Tab` | 切换筛选：全部 / 待我 review / 指派给我 / 我创建 / 提到我 |
| `/` | 按标题、项目、分支过滤 |
| `Enter` | 跳到已有 workspace（8.3） |
| `c` | checkout 到 worktree（8.2） |
| `t` | 打开讨论抽屉（7.5） |
| `r` | tuicr review（8.1） |
| `o` / `b` | 默认浏览器打开 |
| `y` | 复制 MR 链接 |
| `R` | 立即刷新 |
| `?` | 帮助页：功能介绍、快捷键、列和符号的含义；按任意键关闭 |
| `q` / `Esc` | 关闭 |

执行 `c` / `r` / `Enter` 成功后面板退出，焦点落到目标位置；失败时在面板底部显示错误，不退出。

### 7.4 单 MR 浮层

在任意 pane 里 Ctrl+点击 MR 链接，打开只显示这一个 MR 的 overlay。它不受「和我相关」的限制，同事贴出来的链接同样可用。

- 先查缓存，命中则秒开；否则按「项目 + iid」实时查一次（`project(fullPath).mergeRequest(iid)`，复杂度 45），已合并或已关闭的 MR 也查得到并标出状态。
- 按键：`c` / `r` / `o` / `b` / `y` / `Enter` 与面板一致，另有 `p` 打开完整面板、`R` 重新拉取、`q` 关闭。
- 清单里的正则无法按配置注入 host，所以匹配任意 host，运行时再比对；不是当前实例时浮层提示，不做其他操作。
- herdr 只把 `HERDR_PLUGIN_CLICKED_URL` 传给动作，不传给窗格。所以 `link-open` 先把 URL 写进 `$HERDR_PLUGIN_STATE_DIR/clicked-url`，窗格启动后再读。
- Ctrl+点击在部分终端里到不了 herdr（Ghostty + macOS 有多个报告，见 herdrdev/herdr#307、#2284）。所以另有键盘入口 `url` 动作：优先取上下文里的 `selected_text`，其次取剪贴板，解析成功后打开同一个浮层。

### 7.5 讨论抽屉

按 `t` 在**同一个窗格内**左右分栏：左边 MR 列表压缩成每条一行，右边是选中 MR 的讨论。

```
Review requested (2)                           │ atlas-server !412  10 threads, 1 unresolved
  !1148  fix(tms): correct DPD pickup recipie… │
  !420 ✔ chore(infra): WhereInIfNotEmpty 统一… │ Unresolved (1)
                                               │ ──────────────────────────
Assigned to me (3)                             │  · overall  louis  @kkdy 我希望将配置…
▌ !412 ✔ ✎1 feat(customer-service): 推荐回复…  │
```

内容宽度上限放宽到 170 列，左栏 46 列。讨论按未解决/已解决分组，未解决在前；`enter` 展开全文，按显示宽度换行（`ansi.Wrap` 而非 `Wordwrap`，中文没有空格可断）。

| 键 | 操作 |
|---|---|
| `h` / `l`、方向键 | 左右切换焦点 |
| `j` / `k` | 在当前栏移动；在左栏移动会重新加载右栏 |
| `enter` | 展开或收起讨论全文 |
| `space` | 多选讨论 |
| `R` | 解决或重开当前讨论 |
| `a` | 把选中的讨论交给 agent（8.4） |
| `esc` | 收起抽屉 |

**为什么必须在同一个窗格内**：herdr 的 overlay 关闭时会恢复「打开前的焦点和 zoom」。面板本身就是 overlay，若它再打开一个 overlay 窗格然后自己退出，被恢复的目标已经不存在，会留下一个 zoom 状态的空 shell。所以窗格不开窗格，多视图在一个进程里切换。

选中块的配色由终端背景决定：启动时发 `tea.RequestBackgroundColor`，收到 `BackgroundColorMsg` 后按明暗选择前景与背景。只设背景不设前景会在浅色主题下变成深底深字。

## 8. 操作

### 8.1 review（tuicr）

`tuicr mr <web_url>` 通过 glab 访问 GitLab，在任意目录都能打开自建实例的 MR（已实测），cwd 只影响 tuicr 自身行为。

1. 找本机仓库：6.2 中项目匹配的 workspace；有多个时优先 `worktree.is_linked_worktree == false` 的主仓库。
2. 目标 workspace 取该仓库的 workspace；找不到仓库时取打开面板时的 workspace（`HERDR_PLUGIN_CONTEXT_JSON`）。
3. `herdr tab create --workspace <ws> --cwd <repo 或 $HOME> --label "review !<iid>" --focus`，取返回的 `.result.root_pane.pane_id`。
4. `herdr pane run <pane_id> "tuicr mr <web_url>"`。

### 8.2 checkout 到 worktree

1. 找本机仓库（同 8.1 第 1 步）。找不到时提示「本机没有 <project> 的仓库」，结束。
2. 确定本地分支名：同项目 MR 用 `source_branch`；fork MR 用 `mr/<iid>`。
3. `herdr worktree list --cwd <repo>` 中已有该分支的 worktree：
   - 有 `open_workspace_id`：`herdr workspace focus <id>`，结束。
   - 没打开：`herdr worktree open --cwd <repo> --path <path> --label "!<iid> <repo_name>" --focus`，结束。
4. fetch：
   - 同项目：`git -C <repo> fetch <remote> <source_branch>`
   - fork：`git -C <repo> fetch <remote> +refs/merge-requests/<iid>/head:refs/heads/mr/<iid>`
5. 创建：
   - 同项目：`herdr worktree create --cwd <repo> --branch <source_branch> --base <remote>/<source_branch> --label "!<iid> <repo_name>" --focus`
   - fork：`herdr worktree create --cwd <repo> --branch mr/<iid> --label "!<iid> <repo_name>" --focus`

本地已有同名分支时 herdr 直接 checkout 它（[cli-reference.md](herdr/cli-reference.md) → Worktrees），不会自动对齐远端，由用户自己 pull。

### 8.3 跳到已有 workspace

用 6.3 反查：任一 workspace 的当前 MR 是选中的 MR，就 `herdr workspace focus <id>`；否则提示用 `c` checkout。

### 8.4 把讨论交给 agent

1. 用 6.3 反查该 MR 已检出的 workspace，没有就提示先按 `c`。
2. `herdr agent list` 中取同一 workspace 的 agent pane。
3. 拼提示词后 `herdr agent prompt <pane> <文本>`。

提示词开头声明 MR、项目与分支，随后逐条列出讨论的位置与全部回复，单条正文超过 1200 字符截断。其中两条约束是刻意的：不让 agent 去 GitLab 上解决讨论（它判断不了评审人是否满意），也不让它 push。

此路径尚未实测。

## 9. 侧栏 token

- 命令：`herdr workspace report-metadata <ws> --source plugin:hlouis.glab --token mr=<label> --ttl-ms <ttl>`；无 MR 时 `--clear-token mr`。
- TTL 为拉取间隔的 2 倍。poller 停止后 token 自动消失，不会显示过期状态。
- 用户需在 `config.toml` 中把 `$mr` 放进 `[ui.sidebar.spaces] rows`（[configuration.md](herdr/configuration.md) → UI and sidebar）。

标签格式：`!<iid>[ draft][ <合并状态>][ <流水线>][ ✎<已解决>/<总数>]`，总长不超过 80 字符。

| 流水线 `status` | 符号 |
|---|---|
| `SUCCESS` | `✔` |
| `FAILED` | `✖` |
| `RUNNING` | `↻` |
| `PENDING` / `CREATED` / `WAITING_FOR_RESOURCE` / `PREPARING` / `SCHEDULED` | `⋯` |
| `CANCELED` / `SKIPPED` | `⊘` |
| `MANUAL` | `⚙` |

合并状态只显示需要处理的：`NEED_REBASE` → `rebase`，`CONFLICT` → `conflict`。

示例：`!412 ✔ ✎9/10`、`!67 draft ↻`、`!318 rebase ✔`。

### tab 行状态区

herdr 的 `[ui] tab_bar_right` 支持定时执行命令并显示输出的最后一行，这是唯一一个和 workspace 无关、始终可见的位置。用户自行配置：

```toml
[ui]
tab_bar_right = [
  { type = "command", command = "<插件目录>/bin/herdr-glab status", interval_seconds = 30, timeout_seconds = 2 },
]
```

输出形如 `MR 4 · 1 todo`：总数，以及其中需要我处理的条数（请我 review 但我还没批准的，或我发起且流水线失败、有未解决 thread、需要 rebase 或有冲突的；draft 不算）。拉取失败时追加 `⚠`，还没有缓存时显示 `MR –`。

herdr 执行状态区命令时不注入插件环境变量，所以程序在缺少 `HERDR_PLUGIN_STATE_DIR` 时回落到 `~/.local/state/herdr/plugins/<id>`。该命令只读缓存，不加载配置、不访问网络，也不会因为出错而清空状态区。

### 刷新时机

| 时机 | 做法 |
|---|---|
| poller 每轮拉取后 | 刷新所有 workspace |
| `workspace.created` / `workspace.focused` / `worktree.created` / `worktree.opened` | 钩子执行 `tokens --workspace $HERDR_WORKSPACE_ID`，只重算该 workspace |
| 面板 `R` 刷新后 | 刷新所有 workspace |

切换分支后、下次拉取前，钩子用本地分支匹配缓存即可更新；新分支的 MR 不在缓存里时，要等下一轮拉取。

## 10. 配置

`$HERDR_PLUGIN_CONFIG_DIR/config.toml`，全部可选：

```toml
host = "gitlab.hlouis.com"   # 默认：glab 配置 hosts 中唯一的 host；有多个时必须填写
fetch_interval = "5m"         # 最小 1m
glab_path = ""                # 默认从 PATH 和 /opt/homebrew/bin、/usr/local/bin 查找
tuicr_path = ""
token_name = "mr"             # 与其他插件的 token 名冲突时修改
```

glab 全局 `host` 默认是 `gitlab.com`，而插件在非仓库目录下运行，所以每次调用都显式传 `--hostname`。

## 11. 清单

完整内容见仓库根目录的 [herdr-plugin.toml](../herdr-plugin.toml)，这里只记结构：

| 段 | 内容 |
|---|---|
| `[[build]]` | `sh install.sh`：有 Go 编译源码，否则下载 Release 二进制 |
| `[[startup]]` | `ensure`，拉起 poller |
| `[[events]]` | `workspace.created/focused`、`worktree.created/opened` → `tokens` |
| `[[actions]]` | `panel`、`refresh`、`stop-poller`、`link`（链接处理器用）、`url`（选中文本或剪贴板） |
| `[[link_handlers]]` | 匹配任意 host 的 `/-/merge_requests/N`，运行时再比对配置的 host |
| `[[panes]]` | `panel`（面板，讨论抽屉在其中）、`mr`（单 MR 浮层） |

插件不能自带快捷键，用户侧示例：

```toml
[[keys.command]]
key = "prefix+m"          # prefix+g 是 herdr 默认的 goto
type = "plugin_action"
command = "hlouis.glab.panel"
description = "GitLab MR panel"
```

## 12. 子命令

| 子命令 | 调用方 | 作用 |
|---|---|---|
| `ensure` | startup | 确保 poller 在运行 |
| `poller` | `ensure` 拉起 | 循环：拉取 → 写缓存 → 刷新 token |
| `tokens` | 事件钩子 | `ensure` 后重算 `HERDR_WORKSPACE_ID` 的 token |
| `refresh` | 动作 | 清除停止标记，`ensure`，立即拉取并刷新全部 token |
| `stop` | 动作 | 停止 poller |
| `panel-open` | 动作 | 打开面板窗格 |
| `panel` | 窗格 | 运行 bubbletea 面板 |
| `status` | tab 行状态区 | 打印一行汇总，只读缓存 |
| `link-open` | 链接处理器 | 记下被点击的 URL 并打开单 MR 浮层 |
| `url-open` | 动作 | 用选中文本或剪贴板里的 URL 打开单 MR 浮层 |
| `detail` | 窗格 | 单 MR 浮层 |

## 13. 目录结构

```
cmd/herdr-glab/        子命令分发
internal/config/       读取 config.toml，解析 glab host
internal/gitlab/       glab graphql 调用、查询、分页、模型转换
internal/cache/        缓存读写（原子写）
internal/herdr/        herdr CLI 封装（workspace/pane/tab/worktree/report-metadata）
internal/repo/         remote 规范化、workspace → 仓库/项目/分支、MR 匹配
internal/token/        标签格式化与上报
internal/poller/       PID 记录、拉起、循环
internal/action/       review / checkout / focus / open
internal/ui/           bubbletea 面板
herdr-plugin.toml
```

## 14. 日志与错误

- 钩子和动作：错误写 stderr，`herdr plugin log list --plugin hlouis.glab` 可查；正常运行不输出。
- poller：herdr 不采集脱离进程的输出，写 `$HERDR_PLUGIN_STATE_DIR/poller.log`，超过 1 MB 轮转。
- glab 未安装或未登录：写入缓存 `error`，清除所有 token，poller 按正常间隔重试。
- 网络等临时错误：保留缓存和 token，等下一轮。

## 15. 发布

公开仓库 [hlouis/herdr-glab](https://github.com/hlouis/herdr-glab)，安装命令 `herdr plugin install hlouis/herdr-glab`。

平台限于 macOS 和 Linux。Windows 不在范围内：poller 依靠信号、`setsid` 和 `flock` 保证单实例，剪贴板与浏览器动作也依赖 Unix 命令，`GOOS=windows` 直接编译失败，而且没有可验证的环境。四个发行组合中只有 darwin/arm64 实际使用过，其余仅通过编译和测试，README 里已注明。打 `v*` tag 触发 goreleaser 产出 darwin/linux × amd64/arm64 的二进制，供没有 Go 工具链的机器在 `install.sh` 里下载。打上 GitHub topic `herdr-plugin` 后约 30 分钟进入插件市场。
