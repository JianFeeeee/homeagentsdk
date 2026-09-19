# a2a · Agent-to-Agent 通信

让本 Agent 与其他 Agent **双向互调**：既能对外暴露自己的能力，也能去问别的 Agent。

## 两个方向

| 方向 | 怎么实现 |
|---|---|
| **入站**（别人问我） | 插件起一个 HTTP 服务端，暴露 `/agent-card`（能力描述）与 `/a2a`（JSON-RPC 入口） |
| **出站**（我问别人） | 提供 `a2a_query` / `a2a_discover` 工具，主动向远端 A2A Agent 发起请求 |

## HTTP 端点

| 路径 | 作用 |
|---|---|
| `GET /agent-card` | 返回 Agent Card：本 Agent 的能力描述，供对方发现 |
| `POST /a2a` | JSON-RPC 2.0 入口，接收对方的任务请求 |

## 工具

| 工具 | 说明 |
|---|---|
| `a2a_a2a_query` | 向另一个 A2A Agent 发查询并取回复 |
| `a2a_a2a_discover` | 取对方的 Agent Card（能力描述） |
| `a2a_a2a_status` | 看本插件运行状态（监听地址、当前配置） |
| `a2a_a2a_configure` | 改配置并自动重启服务（可动态改监听地址） |
| `a2a_a2a_restart` | 重启 HTTP 服务端（连接异常或改配置后用） |

> 工具名前缀取自插件名（`tp`），按默认 `a2a_` 列出。

## 配置项

| 键 | 默认 | 说明 |
|---|---|---|
| `listen` | `127.0.0.1:12000` | 服务端监听地址。**设为空可禁用 HTTP 服务**（只出站、不入站） |

## 典型用法

1. **先发现再调用**：`a2a_discover` 拿对方能力 → 决定要不要发、发什么 → `a2a_query`。
   跳过 discovery 直接问，容易问出对方不支持的东西。
2. **只出站**：把 `listen` 设为空，本 Agent 不外露端口，但仍能主动联系别人。

## 构建

```bash
hmapdev build
```
