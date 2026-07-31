# luademo

Lua 插件全功能示例，展示 v0.8.0 Lua SDK 的完整能力面：

- **工具注册**：`no_memory` + `cleaner`（记忆计算层过滤）
- **阶段钩子**：`register_stage(stage, handler, scope)`，`own_tools` 与全局作用域
- **通道**：`register_output_channel` / `register_input_channel`（def 支持 no_memory/cleaner）
- **数据类 API**：`sdk.memory.*`、`sdk.doc.*`、`sdk.knowledge.*`、`sdk.text_memory.*`、`sdk.llm.*`、`sdk.settings.*`、`sdk.social.*`
- **其他**：`register_api`、`set_auto_restart`

## 本地独立测试

```bash
lua main.lua   # 使用 sdk.lua mock，不依赖内核
```

## 构建

```bash
plugindev build
```

## 安装

通过插件管理 HTTP API 上传 `.hmap` 包，或解压到 `<data>/plugins/luademo/` 后重启内核。
