# herdr 插件开发参考文档

官方文档原文，版本 **v0.9.0**，来源 [herdrdev/herdr `docs/next/website`](https://github.com/herdrdev/herdr/tree/v0.9.0/docs/next/website/src)。更新：`doc/herdr/sync.sh [版本]`。

## 按用途查

| 要做的事 | 看哪里 |
|---|---|
| 清单 `herdr-plugin.toml` 字段 | [plugins.md](plugins.md) → Manifest |
| build / startup / actions / events / panes / link_handlers | [plugins.md](plugins.md) 对应章节 |
| 插件运行时注入的环境变量 | [plugins.md](plugins.md) → Commands and environment |
| `herdr plugin link/install/action/log/pane` 命令 | [cli-reference.md](cli-reference.md) → Plugins |
| 插件相关的 socket 方法 | [socket-api.md](socket-api.md) → Plugin APIs |
| 可订阅的事件名（`workspace.*`、`worktree.*` 等） | [socket-api.md](socket-api.md) → Event subscriptions |
| 侧栏 token（`workspace/pane report-metadata`，TTL、上限） | [socket-api.md](socket-api.md) → Agent state reporting；[cli-reference.md](cli-reference.md) → Workspaces / Panes |
| 侧栏行里放 `$token` | [configuration.md](configuration.md) → UI and sidebar |
| 给插件动作绑定快捷键（`type = "plugin_action"`） | [configuration.md](configuration.md) → Custom command keybindings；[plugins.md](plugins.md) → Keybindings |
| workspace / worktree / tab / pane 操作命令 | [cli-reference.md](cli-reference.md) |
| 全部配置项、类型和默认值 | [config-reference.json](config-reference.json) |
| socket 请求/响应/事件的 JSON Schema | [api-schema.json](api-schema.json)（本机 `herdr api schema` 导出） |
| workspace、tab、pane、worktree 等概念 | [concepts.md](concepts.md) |
| 发布到插件市场 | [marketplace.md](marketplace.md) |
| 用脚本或 agent 驱动 herdr | [agent-automation.md](agent-automation.md) |
| 重启后哪些状态会保留 | [session-state.md](session-state.md) |
| 日志位置和排错 | [troubleshooting.md](troubleshooting.md) |

## 写 GitLab 插件时要记住的约束

- 整个 herdr CLI 就是插件接口，调用时用 `$HERDR_BIN_PATH`，不要写死 `herdr`。
- 命令按参数数组直接执行，不经过 shell。
- 插件不能自带快捷键，由用户在 `config.toml` 里绑定。
- `plugin link` 不执行 `[[build]]`。`[[startup]]` 只在服务启动恢复会话后执行，link、enable、重载配置时都不执行，所以后台进程要能在事件或动作里补启动。
- token 值最长 80 字符，服务重启后不保留，需要轮询进程持续上报。
- 凭证和用户配置放 `$HERDR_PLUGIN_CONFIG_DIR`，运行时状态放 `$HERDR_PLUGIN_STATE_DIR`。
