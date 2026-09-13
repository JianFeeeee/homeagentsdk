# luademo

Lua 插件全功能示例，展示 Lua SDK 的完整能力面（对齐 SDK 1.3.0）：

- **工具注册**：`no_memory` + `context_policy` + `cleaner`（记忆计算层过滤）
- **阶段钩子**：`register_stage(stage, handler, scope)`，`own_tools` 与全局作用域
- **通道**：`register_output_channel` / `register_input_channel` / `unregister_output_channel`（def 支持 no_memory/context_policy/cleaner）
- **注入**：`inject_text` / `inject_interrupt` / `inject_text_no_memory`、`*_opts`（no_memory/context_policy/cleaner_name/priority）、`inject_input_sync`、`inject_*_media`、`set_tool_blocks`
- **数据类 API**：`sdk.memory.*`（含 sentence_text/media_digests）、`sdk.doc.*`（含 insert_with_media）、`sdk.knowledge.*`、`sdk.text_memory.*`（含 attachments）、`sdk.llm.*`、`sdk.settings.*`、`sdk.social.*`、`sdk.events.*`、`sdk.plugin_mgr.*`
- **其他**：`register_api`、`set_auto_restart`

> `luademo_probe_v2` 巡检 1.1/1.2/1.3 新增面。它**故意不调用** `inject_input_sync`：工具 handler 在 LLM 回合内运行，同步注入会自己等自己（死锁）。

## 本地独立测试

```bash
lua main.lua   # 使用 sdk.lua mock，不依赖内核
```

## 构建

```bash
hmapdev build
```

## 安装

通过插件管理 HTTP API 上传 `.hmap` 包，或解压到 `<data>/plugins/luademo/` 后重启内核。
