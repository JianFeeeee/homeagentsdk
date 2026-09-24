# API 参考

本页所有内容**从源码生成**（`tools/apidoc`），签名与说明直接取自 `sdk/*.go` 的
文档注释。因此不存在「文档写了一套、代码是另一套」的情况——发现不一致时，
改的是源码注释，不是这里。

## 怎么找 API

<div id="api-search"></div>

用上面的搜索框可以：

- **按名称搜**：`InjectText`、`RegisterTool`、`memory.recall`
- **按描述搜**：`注册工具`、`注入`、`重载`、`崩溃`
- **按签名搜**：`(string) error`、`[]ContentBlock`
- 带 <span class="api-badge api-badge-builtin">仅内置</span>
  标记的条目在**外部插件里拿不到**，多数情况下你不需要它

## 章节划分

按「你想做什么」组织，不是按 Go 的符号类别：

| 章节 | 内容 |
|---|---|
| [工具（Tools）](tools.md) | 注册 LLM 可调用的工具——插件最常用的能力形态 |
| [阶段钩子（Stages）](stages.md) | 在处理管道的固定点位插入逻辑 |
| [记忆（Memory）](memory.md) | 三层记忆的读写：图 / 文档 / 文本，以及知识库 |
| [输入/输出通道](channels.md) | 与外界交换消息，以及往流水线里注入内容 |
| [配置（Settings）](settings.md) | 声明插件配置项，内核渲染到 WebUI |
| [生命周期（Lifecycle）](lifecycle.md) | 启动、停止、卸载、自动重启 |
| [事件（Events）](events.md) | 订阅内核事件 |
| [LLM 调用](llm.md) | 插件主动调用模型 |
| [常量与枚举](constants.md) | 取值枚举 |
| [桥接装配点](bridge.md) | 由 `hmapdev` 生成的运行时调用，插件业务代码不碰 |
| [仅内置插件可用](builtin-only.md) | 边界汇总——外部插件拿不到的 API 全在这里 |

!!! tip "先看「能力边界」能省很多时间"
    如果你正在设计插件，先读 [能力边界](../guide/capability-boundary.md)：
    它说明哪些能力外部插件有、哪些没有，以及**为什么**。
