# 版本与兼容

## SDK 版本语义

**SDK 版本跟随内核的中版本，patch 位恒为 `.0`。**

整条内核 `1.1.x` 线（1.1.0、1.1.1、1.1.7…）共用 **SDK 1.1.0**；
只有内核进入 `1.2.0` 这种中版本跃迁时，SDK 才升到 1.2.0。

这样插件作者只需关心「我在为哪个中版本写插件」，不必跟着内核的每个 bugfix 换依赖。
当前内核声明的兼容上限是 **SDK 1.3.0**。

## 版本历史

| SDK | 内核 | 变化 | 需要重编？ |
|---|---|---|---|
| **1.3.0** | 1.4.0+ | 驻留子 agent、`RecallPolicy` 等 | 想用新 API 才需要 |
| **1.2.0** | 1.2.0 / 1.3.x | `InjectOptions{NoMemory, ContextPolicy}`、六个 `*Opts` 变体、`ChannelDef.ContextPolicy` | 不需要 |
| **1.1.0** | 1.1.x | 多模态贯通：`Triple.SentenceText`、`Doc.Attachments`、`MediaAttachment`、`InsertWithMedia`、媒体注入方法 | 不需要 |
| **1.0.0** | 1.0.0+ | **运行模型变更**：C ABI 动态库 → 子进程 + 共享内存 | **需要** |

### 1.0.0 是唯一一次破坏性变更

- `.so` / `.dylib` / `.dll` **不再被加载**，遇到旧产物会跳过并报可操作错误（不崩溃）。
- **业务代码不用改一行** —— 公开 SDK 接口零改动，用新版 `hmapdev` 重编即可。
- 产物从 `plugin.so` 变为 `plugin.bin`；不再需要 cgo。

### 1.1.0 / 1.2.0 是纯追加

两次都是**新增方法由插件调用、内核实现**，不调就不受影响。
零值 `InjectOptions` 与旧的三参数方法完全等价，因此存量插件**不需要改、也不需要重编**；
想用新字段的重编即可。

!!! tip "什么时候必须重编"
    只有两种情况：① 内核跨了中版本（如 1.1 → 1.2）且你用了新 API；
    ② 内核的 RPC 协议版本变了（`.hmap` 里的 `protocol` 字段与内核不匹配）。
    后者的错配**不会静默失效** —— 握手时会显式拦下。

## RPC 协议版本

插件包里带 `protocol` 字段，必须等于内核的 `ProtocolVersion`（当前 **2**）。

协议 v2 引入了调用帧（tool / cleaner / output）与 `blocks_ref` 媒体块。
v1 插件遇上 v2 内核会拿到空参数，反过来 v2 插件发 `blocks_ref` 会被 v1 内核静默忽略 ——
**两边错配都不报错、只是静默失效**，所以协议版本在握手上显式校验。

## 怎么确认自己在用什么

装的 SDK 版本：

```bash
hmapdev sdk current
hmapdev sdk list
```

插件声明的目标版本在 `plg.json` 的 `sdk` 字段。若该版本不在本地存储里，
`hmapdev` 会**明确报错**，不静默降级 —— 静默降级会产出与内核协议不匹配的包，
那种失败要到运行时才暴露。

## 文档站对应的版本

本页与 [API 参考](api/index.md) 由 `tools/apidoc` 从源码生成，
内容随源码一起演进。发现文档与代码不一致时，**改的是源码注释**，
`go run ./tools/apidoc` 重新生成即可（见下方「维护」）。

## 维护（给 SDK 维护者）

```bash
cd homeagent-sdk
go run ./tools/apidoc -pkgdir ./sdk -out /tmp/api.json
go run ./tools/apidoc/gensite -api /tmp/api.json -out ./docs -examples ./example
mkdocs serve     # 本地预览
mkdocs build     # 产出 site_build/
```

API 面的**能力分层**（哪些 API 仅内置可用）记在 `tools/apidoc/tiers.json`，
每条裁定都附源码依据 —— 改这里而不是改生成物。
