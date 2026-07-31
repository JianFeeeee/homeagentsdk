# HomeAgent SDK

HomeAgent 插件开发 SDK，用于构建与 HomeAgent 平台交互的智能插件。

## SDK API 接口

### Plugin 接口

插件需实现 `Plugin` 接口：

```go
type Plugin interface {
    Name() string
    Start(sdk *PluginSDK) error
    Stop() error
}
```

### PluginSDK 方法

通过 `Start(sdk *PluginSDK)` 注入的 SDK 实例提供以下方法：

| 分类 | 方法 | 说明 |
|------|------|------|
| 阶段钩子 | `RegisterStage(stage, handler, scope...)` | 注册阶段回调，scope 可选：`StageScopeGlobal`（全局，默认）或 `StageScopeOwnTools`（仅自己工具） |
| 输入通道 | `RegisterInputChannel(name, def)` | 注册输入通道，def 为 `ChannelDef`（NoMemory/Cleaner） |
| 输出通道 | `RegisterOutputChannel(name, caps, desc, def, handler)` | 注册输出通道，def 为 `ChannelDef`，caps 为能力位掩码 |
| 工具注册 | `RegisterTool(name, def, handler)` | 注册工具供 LLM 调用 |
| 插件 API | `RegisterPluginAPI(name)` | 注册插件 API 供其他插件访问 |
| 图记忆 | `Memory()` | 访问图记忆 API（实体-关系存储） |
| 文本记忆 | `TextMemory()` | 访问文本记忆 API（时序事件） |
| 文档记忆 | `DocMemory()` | 访问文档记忆 API（向量存储） |
| 社交图谱 | `Social()` | 访问社交图谱 API（外部插件只读） |
| 知识库 | `Knowledge()` | 访问知识库 API |
| LLM | `LLM()` | 访问 LLM 提供商管理 API |
| 设置 | `Settings()` | 访问设置 API |
| 事件 | `Events()` | 访问事件订阅器（外部插件仅订阅） |
| 注入 | `InjectText(source, channel, text)` / `InjectInterruptText(source, channel, text)` / `InjectTextNoMemory(source, channel, text)` | 向管道注入文本 |
| 自动重启 | `SetAutoRestart(enabled)` / `AutoRestart()` | 控制崩溃自动重启 |

### 阶段钩子

```go
// 全局监听所有插件的阶段事件
sdk.RegisterStage(StagePreAction, func(ctx *StageContext) error { return nil })

// 仅监听自己注册的工具的 before_toolcall / after_toolcall
sdk.RegisterStage(StageBeforeToolcall, myHandler, StageScopeOwnTools)
```

### ChannelDef

```go
type ChannelDef struct {
    NoMemory bool              // 通道输入/输出不参与记忆计算（向量/关键词/蒸馏），原文保留
    Cleaner  func(string) string // 可选：计算层过滤函数（不改原文）
}
```

`ChannelDef` 控制通道在记忆计算层的行为，与 `ToolDef` 的 `NoMemory`/`Cleaner` 语义一致。

### 输入通道

```go
sdk.RegisterInputChannel("qq", ChannelDef{
    NoMemory: true,
    Cleaner:  func(text string) string { return strings.TrimSpace(text) },
})
```

### 输出通道

```go
sdk.RegisterOutputChannel("my-channel", CapText|CapFile, "通道描述", ChannelDef{}, handler)
```

handler 接收三个参数：
- `payload` (string) — 消息载荷。`type=text` 时直接填文字，`type=file/image` 时填 URL
- `meta` (string) — 可选的 JSON 路由元数据（如 `{"group_id":123,"user_id":456}`）
- `type` (string) — 载荷类型，枚举值见下

能力标志位：

| 标志 | 值 | 说明 |
|------|----|------|
| `CapText` | 1 | 纯文本输出 |
| `CapFile` | 2 | 文件输出 |
| `CapImage` | 4 | 图片输出 |
| `CapAudio` | 8 | 音频输出 |
| `CapStructured` | 16 | 结构化数据输出 |

type 枚举值：

| 值 | 说明 |
|----|------|
| `text` | 纯文本 |
| `voice` / `audio` | 语音 |
| `image` | 图片 |
| `file` | 文件 |

### IOInjector 通道路由

| 方法 | 说明 |
|------|------|
| `InjectText(source, channel, text)` | 注入文本，记入内存，路由到指定通道 |
| `InjectInterruptText(source, channel, text)` | 注入中断文本，打断当前处理，路由到指定通道 |
| `InjectTextNoMemory(source, channel, text)` | 注入文本，不记入内存，路由到指定通道 |

`source` 标识来源，`channel` 指定目标输出通道。

### Triple 扩展字段

Triple 数据结构新增字段：

- `Confidence` — 置信度（0.0~1.0）
- `SubjectType` — 主体类型
- `ObjectType` — 客体类型

### ToolDef 字段说明

`RegisterTool` 的 `def` 参数类型为 `sdk.ToolDef`，包含以下字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `Name` | `string` | 工具名，建议插件名前缀避免冲突 |
| `Description` | `string` | 工具描述，LLM 据此选择调用 |
| `Parameters` | `map[string]interface{}` | JSON Schema 格式参数定义 |
| `NoMemory` | `bool` | 默认为 `false`；设为 `true` 时输出不参与向量/jieba/蒸馏计算（原文保留） |
| `Cleaner` | `func(string) string` | 可选，输出进入计算层前的清洗函数（如 JSON 提取 `.content`） |

`NoMemory` 和 `Cleaner` 的详细设计意图参见核心仓 `docs/zh/PLUGIN_DEV.md`。

### New 构造函数

`New()` 由内核在加载插件时调用，插件开发者无需手动构造 PluginSDK：

```go
func New(name string, sett SettingsAPI, regTool ToolRegistrar, regStage StageRegistrar, regAPI APIRegistrar, regOutput OutputChannelRegistrar) *PluginSDK
```

插件开发者只需实现 `Plugin` 接口并导出 `NewPluginFactory()` 入口函数。

## plugindev 工具链

`plugindev` 提供插件开发全流程支持。仓库 `bin/` 提供各平台预制二进制（linux/darwin/windows × amd64/arm64），下载后直接加入 PATH 即可：

```bash
curl -o plugindev https://gitcode.com/JianFeeeee/homeagent-sdk/-/raw/main/bin/plugindev_linux_amd64
chmod +x plugindev
```

| 命令 | 说明 |
|------|------|
| `plugindev init <name> [--lua]` | 初始化插件项目（生成 plg.json、plugin.go 或 main.lua、go.mod、README.md） |
| `plugindev build [flags]` | 编译并打包为 `.hmap` 包（支持跨平台编译和 bundle 模式） |
| `plugindev clean` | 清理 `build/`、`dist/` 目录及生成文件（plugin.json、z_bridge_gen.go） |
| `plugindev debug [dir]` | 通过 Yaegi Go 解释器加载插件源码，启动交互式 REPL 调试 |
| `plugindev sdk <command>` | SDK 版本管理（子命令：list/install/use/path/current/latest） |

支持 **Go** 和 **Lua** 两种插件语言。

### build 命令 flags

| Flag | 说明 |
|------|------|
| `--outdir <dir>` | 输出目录（默认 `dist`，可覆盖 plg.json 中的 `outdir`） |
| `--target <os/arch>` | 构建目标（如 `linux/amd64`），可重复指定（追加到 plg.json 中的 targets） |
| `--bundle` | 强制 bundle 模式（同时编译 linux/amd64, darwin/amd64, windows/amd64） |
| `--no-bundle` | 关闭 bundle 模式，仅按 targets 逐个编译 |
| `--sdk-path <path>` | 指定 SDK 源码路径（覆盖 plg.json 中的 `sdk_path`） |
| `--replace <from=to>` / `-R` | Go 模块替换（追加到 plg.json 中的 replaces），`from` 为模块路径，`to` 为本地路径 |

### plg.json 清单格式

```json
{
  "name": "weather",
  "name_zh": "天气查询",
  "name_en": "Weather",
  "version": "1.0.0",
  "description": "天气查询插件",
  "author": "HomeAgent",
  "entry": "plugin.so",
  "tags": ["weather", "forecast"],
  "targets": "linux/amd64,windows/amd64",
  "outdir": "dist",
  "bundle": true,
  "replaces": {
    "github.com/example/pkg": "../local/pkg"
  },
  "source_dirs": [
    "../shared-lib"
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | 插件标识名 |
| `name_zh` | string | 中文名 |
| `name_en` | string | 英文名 |
| `version` | string | 版本号 |
| `description` | string | 插件描述 |
| `author` | string | 作者 |
| `entry` | string | 入口文件（`plugin.so` / `plugin.dll` / `main.lua`） |
| `tags` | string[] | 标签 |
| `targets` | string | 构建目标，逗号分隔（如 `linux/amd64,windows/amd64`，Lua 插件为 `lua`） |
| `outdir` | string | 输出目录（默认 `dist`） |
| `bundle` | bool | 是否 bundle 模式（同时编译多平台，默认 `true`） |
| `sdk_path` | string | SDK 源码路径（覆盖自动检测的 SDK 路径） |
| `go_version` | string | Go 版本（如 `1.21`，默认从 SDK 的 go.mod 读取） |
| `replaces` | object | Go 模块替换，key=模块路径，value=本地路径 |
| `source_dirs` | string[] | 额外源码搜索路径（编译时自动导入，用于引入 `thirdpart/` 外部的共享代码） |

### .hmap 包格式

`.hmap` 为 ZIP 归档，包含：

- `plugin.json` — 插件元数据
- `plugin.so` — Go 编译产物（Linux）
- `plugin.dll` — Go 编译产物（Windows）
- `plugin.dylib` — Go 编译产物（macOS，bundle 模式）
- `main.lua` — Lua 插件入口（Lua 插件时）

## 插件生命周期

### 入口函数

插件必须导出 `NewPluginFactory` 入口函数（Go）或 `start()` 函数（Lua）：

**Go 插件** — 实现 `Plugin` 接口并导出工厂函数：

```go
func NewPluginFactory(name string, config map[string]interface{}) (sdk.Plugin, error) {
    return &Plugin{name: name}, nil
}
```

该函数由内核在加载插件时调用，`name` 为插件名，`config` 为 `skill.json` 中的配置（如有）。

**Lua 插件** — 返回包含 `start(sdk)` 和 `stop()` 方法的 table：

```lua
local plugin = { name = "my-plugin" }
function plugin.start(sdk) -- 注册工具等 end
function plugin.stop() end
return plugin
```

### 启动与停止

- `Start(sdk *PluginSDK) error` — 插件启动，接收 SDK 实例
- `Stop() error` — 插件停止，释放资源

### 自动重启

```go
sdk.SetAutoRestart(true)
// 查询状态
enabled := sdk.AutoRestart()
```

插件崩溃时平台自动拉起，保障服务可用性。

## 受限 SDK vs 完整 SDK

外部插件（第三方分发）使用**受限 SDK**，仅暴露安全子集：

| 受限 API | 允许操作 |
|----------|----------|
| `SocialAPI` | 只读：`GetPerson`、`GetTrait`、`GetRelations`、`GetNetwork`、`ListPersons` |
| `EventSubscriber` | 仅订阅：`Subscribe`（无 `Publish`） |

内部插件（平台内置）拥有完整 SDK 访问权限，包括 SocialAPI 写操作和 EventPublisher。

## 示例插件

| 插件 | 类型 | 说明 |
|------|------|------|
| [weather](example/weather) | Go | 天气查询（wttr.in），演示 NoMemory/Cleaner/阶段钩子/通道/文本记忆 |
| [luademo](example/luademo) | Lua | Lua 全功能示例，覆盖 v0.8.0 Lua SDK 全部 API 面 |
| [qq](example/qq) | Go | QQ 消息集成（NapCat），17 个工具，输入/输出通道完整对接 |
| [a2a](example/a2a) | Go | Agent-to-Agent 协议通信 |
| [ai_image](example/ai_image) | Go | AI 图片生成 |
| [bili](example/bili) | Go | Bilibili 视频下载 |
| [browser](example/browser) | Go | 网络搜索、网页抓取、浏览器渲染 |
| [calendar](example/calendar) | Go | 日历管理 |
| [editdoc](example/editdoc) | Go | 文档编辑 |
| [files](example/files) | Go | 文件管理 |
| [memo](example/memo) | Go | 备忘录（PreAction 注入 + 定时提醒） |
| [music](example/music) | Go | 音乐播放 |
| [ocr](example/ocr) | Go | 光学字符识别 |
| [rss](example/rss) | Go | RSS 订阅 |
| [sanitizer](example/sanitizer) | Go | 内容清洗/安全过滤 |

## 构建与安装

### 构建

```bash
plugindev build
```

输出 `.hmap` 包到 `dist/` 目录（默认 bundle 多平台合集；单平台构建使用 `plugindev build --no-bundle`）。

### 安装

通过 pluginmgr HTTP API 安装（端口默认 9876，仅监听 127.0.0.1，无鉴权）：

```bash
# 本地路径
curl -X POST http://127.0.0.1:9876/plugins \
  -H "Content-Type: application/json" \
  -d '{"path": "/path/to/my-plugin.hmap"}'

# 直接上传二进制
curl -X POST http://127.0.0.1:9876/plugins \
  --data-binary @dist/my-plugin.hmap
```

或通过 WebUI 插件管理页面上传，也可手动将 `.hmap` 放入插件目录后重启平台。
