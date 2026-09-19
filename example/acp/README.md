# acp · Agent Client Protocol 通信

[ACP](https://agentclientprotocol.com/) 桥接：本 Agent 既能**当服务端**接别人的任务，也能**当客户端**去调别的 ACP Agent。

## 两个方向

| 角色 | 行为 |
|---|---|
| **服务端** | 在本机起 HTTP 服务，处理 `session/new` / `session/update`，接受其他 Agent 的任务请求 |
| **客户端** | 通过 `acp_query` 向远程 ACP Agent 发 `session/new` 并读回复 |

## 协议端点

- `POST /api/session` —— JSON-RPC，支持 `session/new` 与 `session/update`
- 客户端侧同时兼容**两种服务端**：SSE 型（流式 `session/reply`）与同步 JSON 型

## 工具

| 工具 | 说明 |
|---|---|
| `acp_acp_query` | 向远程 ACP Agent 发起会话并等待回复，返回最终回答文本 |
| `acp_acp_status` | 查看运行状态与**当前活跃会话数** |
| `acp_acp_configure` | 改监听配置并重启 HTTP 服务 |

> 工具名前缀取自插件名（`tp`），按默认 `acp_` 列出。

`acp_query` 可指向的远端举例（源码注释给的）：

- opencode：`http://127.0.0.1:13000`
- pi bridge：`http://127.0.0.1:12011`
- 回环到自身：`http://127.0.0.1:12001`

## 配置项

| 键 | 默认 | 说明 |
|---|---|---|
| `listen` | `127.0.0.1:12001` | 服务端监听地址。**设为空可禁用 HTTP 服务**（只出站） |

## 与 a2a 的区别

| | a2a | acp |
|---|---|---|
| 面向 | Agent ↔ Agent 对等通信 | 客户端 → Agent 会话（每次一个 session） |
| 会话 | 一问一答 | 有 session 生命周期，可续 |
| 发现 | `/agent-card` | 无（需已知地址） |

## 构建

```bash
hmapdev build
```
