# 插件 SDK 文档站

用 MkDocs Material 构建的 SDK 文档站。**API 参考不是手写的** ——
它从 `sdk/*.go` 的源码注释生成，因为手抄必然与代码漂移。

## 目录结构

```
mkdocs.yml                    站点配置（导航、主题、中文检索）
docs/
├── index.md               ┐
├── versions.md            │
├── guide/*.md             ├─ 手写：指南、边界说明、版本
├── api/index.md           │
├── javascripts/           │
│   └── api-search.js      │  自建 API 检索（按名称/描述/签名）
├── stylesheets/extra.css  ┘
├── api/*.md               ┐ 生成物 —— 勿手改
├── examples/index.md      │ （build 时覆盖）
└── assets/api-index.json  ┘
tools/apidoc/                 生成器（本仓 Go 代码，零外部依赖）
├── extract.go                从源码提取符号、注释、分层
├── tiers.go                  应用能力分层（public / builtin / bridge）
├── tiers.json                **能力边界的事实源**（每条附源码依据）
├── gensite/main.go           渲染 Markdown + 检索索引
├── gensite/usages.go         从 example/ 抽取真实调用点
└── build.sh                  一键生成 + 构建
```

## 构建

```bash
tools/apidoc/build.sh          # 生成 + 构建到 site_build/
tools/apidoc/build.sh serve    # 本地预览（http://127.0.0.1:8000）
```

依赖：Go 1.21+、`mkdocs-material`（`pip install mkdocs-material`）、
`jieba`（中文检索分词，`pip install jieba`）。

## 两条设计原则

**① API 参考从源码生成。** 签名、说明、示例全部来自 `sdk/*.go` 的文档注释。
发现文档不对时，**改的是源码注释**，然后重新生成。生成页首行有「勿手改」标记。

**② 能力边界是可核对的事实，不是印象。** 哪些 API 外部插件拿不到，
逐条记在 `tools/apidoc/tiers.json`，每条都写清**可复核的依据**
（文件:行号、或 `grep` 结论）。判断标准是：

| 依据 | 含义 |
|---|---|
| `tools/hmapdev/templates/proc_main.go.tmpl` 的 `base.Set*` 调用 | 外部插件运行时**实际注入**哪些能力 |
| `internal/sdk` | 内置插件用的完整接口（对照出外部缺什么） |
| `internal/plugin/proc/protocol.go` | 外部插件**能发哪些 RPC** |

文档站上每条「仅内置」告警都带这个依据，读者可自行核对。

### 为什么这个边界值得单独维护

写这个站时，实测发现文档与源码有**三处不符**（现已在站内更正）：

1. `PluginMgr()` 曾被写成「仅内置可用」——实际桥接**显式注入**了它。
   真正的区别是方法数：公开面 3 个，内部面 9 个（两个包里同名不同接口）。
2. `Events()` 曾被当作可用的事件订阅入口——实际桥接**不注入** subscriber，
   外部插件拿到的恒为 nil（`SetEventSubscriber` 全仓无调用点）。
   外部插件的事件订阅实际由生成的运行时走 `events.subscribe` RPC 完成。
3. `UnregisterOutputChannel` 易被当成「可用但会报错」——实际返回 nil，
   **静默无效**（桥不注入 unregister），不报错也不注销。

## 检索

站内有两套检索，互补：

- **MkDocs 内置搜索**（右上角）：全文检索，中文走 jieba 分词。
- **自建 API 检索**（首页与 API 参考页的输入框）：读 `assets/api-index.json`，
  专门解决「**按描述找 API**」——搜「注册工具」能找到 `RegisterTool`，
  搜「崩溃」能找到 `SetAutoRestart`，并可区分公开/仅内置。

自建检索支持四类查询：名称、描述（中英文）、`限定符.方法`
（如 `memory.recall`）、签名片段（如 `(string) error`）。

## 维护提示

- **改了 `sdk/*.go` 的注释或签名** → 重跑 `build.sh`，改动自动进文档。
- **改了能力边界** → 改 `tiers.json`，不要直接改生成的 `.md`。
- **新增示例插件** → 自动出现在「示例插件」页的用法表里（扫 `example/`）。
- `site_build/` 是构建产物，已 gitignore，不要提交。
