# sanitizer · 文本清洗

**不注册任何工具**，只挂三个阶段钩子，在 Agent 全链路上洗掉两类污染：

1. **坏字节**：坏 UTF-8、`U+FFFD`（替换符）、ANSI 转义序列
2. **思维泄漏**：LLM 输出里残留的工具调用标记

## 为什么需要它

坏字节会**被 LLM 复读**。一次工具返回乱码（比如源码里带 ANSI 颜色码、或二进制片段被当文本读出来），
这些字节会进上下文，之后模型每次生成都可能把它抄一遍 —— 越滚越脏。
在每个入口洗掉，比事后清理便宜得多。

思维泄漏则是另一种：模型有时把 `<tool_call>...</tool_call>` 这类内部标记直接写进正文，
用户就看到一堆不该出现的 XML。

## 挂载的三个阶段

| 阶段 | 处理对象 | 作用 |
|---|---|---|
| `on_input` | `ctx.RawMessage` | 洗用户输入，脏字节不进后续链路 |
| `after_toolcall` | `ctx.ToolResults` | 洗工具结果，**坏字节不进 LLM 上下文** |
| `post_action` | `ctx.LLMText` | 洗模型输出：先清思维泄漏，再清乱码 |

每次有改动都打一行日志（`cleaned N bytes`），便于确认它真的在工作而不是静默失败。

## 识别哪些泄漏形态

按正则匹配多种标记写法，覆盖不同模型家族的习惯：

- `<tool_call>…</tool_call>`、`<invoke>…</invoke>`、`<tool>…</tool>`
- `<function>…</function>`
- 上述标记包在 ```xml / ```json 代码块里的形态
- 中文括号变体：`【tool_call】…【/tool_call】`

## 实现要点

- 依赖 **ABI v2 的 stage 写回能力**：插件对 `StageContext` 的修改会同步回内核。
  在 v1 上改了不生效。
- 读写 `StageContext` 时按约定加 `ctx.Lock()`。

## 构建

```bash
go build -buildmode=plugin -o sanitizer.so .
```

或经 `hmapdev build` 打包为 `.hmap`。
