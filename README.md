# HomeAgent SDK

HomeAgent 插件开发 SDK，用于构建与 HomeAgent 平台交互的智能插件。

## 版本与兼容性

当前：**SDK 1.2.0**（需内核 **1.2.0+**）。

**版本号跟随内核的中版本，patch 位恒为 `.0`**：

| 内核版本 | 对应 SDK |
|---|---|
| 1.0.0 / 1.0.1 / … / 1.0.4 | 1.0.0 |
| 1.1.0 / 1.1.1 / … / 1.1.N | **1.1.0** |
| 1.2.0 起 | 1.2.0 |

内核的 patch 位专用于 bugfix 与漏洞修复，不碰公开接口，所以 SDK 版本号不跟着动——
否则你要么被迫跟版、要么怀疑自己版本过时，而接口其实一个字都没变。

**1.0.x 插件升到 1.1.x：不需要改代码，也不需要重编。** 1.1.0 的新增全部是
「插件调用、内核实现」方向，不调就不受影响（已用 SDK 0.9.2 编的旧 `plugin.bin`
实测验证：在新内核上直接建链通过，因为握手校验的是 `ProtocolVersion`、不是 SDK 版本）。
想用新字段时重编即可。

**1.1.x 插件升到 1.2.x：接口纯追加，但必须重编。** 公开接口没有签名变更（新增
`InjectOptions` 与六个 `*Opts` 变体、`ChannelDef.ContextPolicy`），不调新能力就不受影响；
但内核的**插件运行协议升到了 2**（统一共享内存区的 fd3 布局改变，**不支持滚动升级**），
所以 `plugin.bin` 必须用配套的 `plugindev` 重编后与内核**同批**安装——否则握手时协议版本
不匹配会被拒绝（错误信息会明确提示用配套 plugindev 重编，不会静默降级）。

## 注入行为与上下文裁剪（1.2.0）

「记不记入记忆」与「要不要据此裁剪上下文」这两件事，原先只有 `ToolDef` 能声明；
1.2.0 起**注入侧也能声明**，并且二者共用同一套语义与取值。

```go
type InjectOptions struct {
	NoMemory      bool   // true = 不参与记忆计算（向量化/关键词提取/蒸馏），原文仍留在上下文
	ContextPolicy string // ""/none = 不裁剪（默认）；prune = 据此裁剪上下文
	CleanerName   string // 计算层过滤函数名：先经 Cleaner 得到实际有效内容，再计算/裁剪
}

const (
	ContextPolicyNone  = "none"
	ContextPolicyPrune = "prune"
)

// 六个变体，与旧的三参数方法一一对应，只多一个 opts
InjectTextOpts(source, channel, text string, opts InjectOptions)
InjectInterruptTextOpts(source, channel, text string, opts InjectOptions)
InjectInputSyncOpts(source, channel, text string, opts InjectOptions) string
InjectInputMediaOpts(source, channel, text string, blocks []ContentBlock, opts InjectOptions)
InjectInputMediaSyncOpts(source, channel, text string, blocks []ContentBlock, opts InjectOptions) string
InjectInterruptMediaOpts(source, channel, text string, blocks []ContentBlock, opts InjectOptions)
```

要点：

- **零值 `InjectOptions{}` 与旧的三参数方法逐键等价**（记入记忆 + 不裁剪）。旧方法保留为
  零值糖（`InjectText` / `InjectInterruptText` / `InjectTextNoMemory` …），存量插件不改一行、
  不需重编即可继续调用。
- **裁剪（`prune`）必须显式声明**：它会归档丢弃低相关事件，是有副作用的行为，故默认关闭。
  内核只放行 `""` / `none` / `prune`（`ValidContextPolicy`），未声明的取值会被拒。
- 裁剪前先经该插件注册的 **`Cleaner`**（由 `CleanerName` 指定）拿到实际有效内容，
  避开「按原文裁剪、按清洗后计算」这种不一致。
- `ChannelDef` 也有同名 `context_policy`（并且 1.2.0 给它补上了 JSON tag——通道定义要跨进程
  传给内核，而 `Cleaner` 是函数必须忽略；无 tag 时新增字段会被静默丢掉）。

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
| 多模态注入 | `InjectInputMedia(source, channel, text, blocks)` / `InjectInputMediaSync(...)` / `InjectInterruptMedia(...)` | 注入带图片/音频的输入（1.1.0 新增） |
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

### 多模态注入（1.1.0 新增）

| 方法 | 说明 |
|------|------|
| `InjectInputMedia(source, channel, text, blocks)` | 注入带媒体的输入，异步 |
| `InjectInputMediaSync(source, channel, text, blocks)` | 注入带媒体的输入并同步等待回复文本 |
| `InjectInterruptMedia(source, channel, text, blocks)` | 注入带媒体的中断，可抢占当前处理 |

`blocks` 是 `[]sdk.ContentBlock`，与 `SetToolBlocks` 用同一类型：

```go
s.InjectInputMedia("myplugin", "webui", "帮我看看这张图", []sdk.ContentBlock{{
    Type:     "image_url",
    ImageURL: &sdk.ImageURL{URL: "data:image/png;base64," + b64, Detail: "auto"},
}})
```

与 `SetToolBlocks` 的区别：`SetToolBlocks` 只能在工具处理函数内部调用，媒体要等到
下一条 tool message 才到模型手上；这三个方法是插件**主动发起一轮带媒体的对话**，
媒体在本轮就随消息发给模型，并自动落进媒体存储、挂上媒体记忆引用。

媒体块里的 `data:` URL 会被内核落盘去重；`http(s)` URL 只透传给模型，不入库
（入库需要内核发起网络请求，涉及超时、鉴权与 SSRF）。

`source` 标识来源，`channel` 指定目标输出通道。

### Triple 扩展字段

Triple 数据结构新增字段：

- `Confidence` — 置信度（0.0~1.0）
- `SubjectType` — 主体类型
- `ObjectType` — 客体类型
- `SentenceText` — 原始句子文本（1.1.0 新增），写入 `sentences` 表；媒体引用挂在句子上
- `MediaDigests` — 关联的媒体 digest 列表（1.1.0 新增）

### 记忆里的媒体（1.1.0 新增）

媒体在纯文本记忆里以**标记**形式存在，格式 `[<mime> <短digest>] <描述>`：

```
[image/png a1b2c3d4e5f6] 一张紫蓝红三色带图
```

描述文本是持久的语义记忆（检索靠它），digest 是回到字节的钥匙（反查靠它）。
标记由内核生成，插件不必自己拼——**填 digest 就够**。

#### 图记忆

```go
s.Memory().Commit([]sdk.Triple{{
    Subject: "配色图", Relation: "包含", Object: "三色带",
    MediaDigests: []string{"a1b2c3d4e5f6"}, // 短 digest 即可，内核补全
}})
```

没给 `SentenceText` 时内核会用标记本身充当句子——媒体必须有句子落点，
否则引用无从挂起。

#### 知识库

```go
s.DocMemory().InsertWithMedia(&sdk.Doc{
    Title:   "带图笔记",
    Content: "正文",
}, []sdk.MediaAttachment{
    {MIME: "image/png", Data: pngBytes, Name: "chart.png"}, // 新内容，落盘去重
    {Digest: "a1b2c3d4e5f6"},                               // 引用已有内容
})
```

`Insert` 保持原签名不变，正文里已有的标记同样会被挂成文档级引用。
`Query` 返回的 `Doc` 带 `MediaDigests` 与 `Attachments`（mime + 描述，
**不含字节**——一次检索可能命中几十份媒体）。删除文档时引用自动释放。

#### 文本记忆

```go
s.TextMemory().Append(sdk.TextEvent{
    Role: "user", Content: "看这张图",
    Attachments: []sdk.MediaAttachment{{MIME: "image/png", Data: pngBytes}},
})
```

`RecentEvents` 读回时正文里的标记会被反解成 `Attachments`。

媒体存储可在内核侧关闭（`core.memory.media.enabled=false`），此时以上接口
全部退化为纯文本行为：不报错、不 panic，与本特性上线前一致。

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

`plugindev` 提供插件开发全流程支持。预编译二进制作为 **release 附件**分发（linux/darwin/windows × amd64/arm64），从
[Releases](https://gitcode.com/JianFeeeee/homeagent-sdk/releases) 下载后加入 PATH 即可：

```bash
# 从 release 附件下载（以 v1.1.0 / linux amd64 为例）
curl -Lo plugindev https://gitcode.com/JianFeeeee/homeagent-sdk/releases/download/v1.1.0/plugindev_linux_amd64
chmod +x plugindev

# 或从源码自己编
cd tools/plugindev && go build -o plugindev .
```

> 二进制不再随仓库分发（旧的 `bin/` 目录已停用）：5 个平台各 26-28MB，
> 每次重编都在 git 历史里再叠一份，而它们本质是可从源码复现的产物。

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
  "entry": "plugin.bin",
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
| `entry` | string | 入口文件（`plugin.bin` / `main.lua`）。v1.0.0 起 Go 插件统一为 `plugin.bin`，不再区分平台后缀 |
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
- `plugin.bin` — Go 编译产物（单平台构建）
- `plugin.bin.<goos>.<goarch>` — 多平台 bundle 模式下每平台一份，
  安装时 pluginmgr 挑当前平台那份重命名为 `plugin.bin`
- `main.lua` — Lua 插件入口（Lua 插件时）

> v1.0.0 起不再使用 `plugin.so`/`plugin.dll`/`plugin.dylib`——进程边界即 ABI 边界，
> 不存在平台特定的动态库区分。旧产物新内核不会加载，会给出明确的重编提示。

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
- `sdk.RegisterStopHandler(fn func())` — 注册停止清理回调。内核（内置插件）或 z_bridge（外部插件）会在调用插件 `Stop()` **之前**统一执行已注册的 handler（后注册先执行，执行后清空、幂等）。适合做持久化落盘、取消后台任务等清理：此时插件内存状态仍然新鲜，避免在 `Stop()` 阶段以陈旧状态写回导致数据复活。

### 删除清理（onRemove）

`Stop`/`RegisterStopHandler` 在插件**停止**（含重载、禁用）时执行；`RegisterOnRemoveHandler` 仅在插件被**卸载（删除）**时执行一次，重载/禁用不触发：

- `sdk.RegisterOnRemoveHandler(fn func())` — 注册删除清理回调。内核在 `RemovePlugin` 流程中、插件 `Stop()` **之后**执行（后注册先执行，执行后清空、幂等）。用于删除插件自身创建的持久化文件（数据/缓存/状态文件）。
- 内核卸载时一并清理：工具注册、`disabled_plugins` 记录、插件配置项定义（`plugin.<name>.*`）与插件配置表（`config_<name>`），卸载后插件配置区完全消失。
- 示例：`example/calendar`（删 events.json）、`example/memo`（删 memos.json）、`example/rss`（删订阅数据目录）、`example/weather`（删缓存目录）；`plugindev` 模板含 onRemove 演示。

```go
sdk.RegisterOnRemoveHandler(func() {
    os.Remove(filepath.Join(dataDir, "events.json"))
})
```

### 自动重启

```go
sdk.SetAutoRestart(true)
// 查询状态
enabled := sdk.AutoRestart()
```

插件崩溃时平台自动拉起，保障服务可用性。

> ⚠️ `SetAutoRestart` 的典型用法是「外部连接建好后再判定能否自动重启」，而连接建立
> 通常在后台 goroutine 里，内核又在另一个 goroutine 读它——这对读写天然并发。
> **SDK 1.1.0 已给这个标志与全部 API 字段加锁**（`-race` 实测 11 处竞态，
> 生产表现是插件重载瞬间偶发 nil 解引用崩溃）。早于 1.1.0 的版本建议升级。

## 插件开发者的并发约定

`PluginSDK` 是**被多个 goroutine 同时使用的共享对象**：你在 `Start()` 里起的轮询、
监听、定时器都拿着同一份 `*PluginSDK` 往里注消息，而内核会在加载/重载时写它的
 API 字段。因此：

- **SDK 侧已保证的**：全部 API 访问器（`Memory()`/`DocMemory()`/…）、全部注入方法、
  `SetAutoRestart`/`AutoRestart`、`RegisterTool`/`RegisterStage`、
  `RunStopHandlers`/`RunOnRemoveHandlers`（幂等，并发调也只执行一次）。
- **你需要自己保证的**：`StageContext` 的字段全部导出，并发读写必须自己持
  `ctx.Lock()`/`ctx.RLock()`。尤其是 `ctx.Extra`——**map 的并发写在 Go 里是直接 fatal，
  `recover` 接不住**。

```go
ctx.Lock()
ctx.Extra["mykey"] = value
ctx.FinalText += "补充说明"
ctx.Unlock()
```

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

**发版时附带预编译示例产物**：SDK 的 release 除 5 平台 `plugindev` 外，还包含各示例插件的
`.hmap` 与 `SHA256SUMS`/`MANIFEST.txt`。原因是插件二进制与内核**协议绑定**（`ProtocolVersion`
+ 共享内存区魔数），只发工具链不发示例产物，很容易拿旧产物去装而握手失败——那看起来像
「插件坏了」而不是「版本不配套」。

## Remote Device SDK

用于开发**远程设备接入适配器**的 C 语言 SDK，零外部依赖，兼容嵌入式平台。

### 架构

```
┌─────────────────────────────────────────────────┐
│            ha_remotedevice (C SDK)              │
│  协议引擎  │  WS 帧  │  JSON  │  状态机  │ 传输抽象  │
└──────────┬──────────────────────────────────────┘
           │  同一份 C 代码，设备端和 App 端共用
    ┌──────┴──────────────────┐
    ▼                         ▼
┌──────────────┐    ┌──────────────────────────┐
│  ESP32 裸机   │    │  Linux 设备上的 App        │
│  纯 C 直调     │    │  (Python ctypes / Go CGo / │
│  简单命令处理   │    │   Node addon / C# P/Invoke) │
└──────────────┘    └──────────────────────────┘
```

### 声明式 API 设计

设备在代码中声明**自己是什么**、**能做什么**、**支持哪些命令**，每个命令对应独立处理函数，SDK 自动分发并回执结果：

```c
#include "ha_remotedevice.h"

/* 声明能力 */
const char *caps[] = {"camera", "status", NULL};

/* 声明式命令处理表：每个命令绑定独立处理函数 */
static ha_status_t handle_camerasue(const char *req_id, const char *args,
                                    ha_cmd_result_t *result, void *userdata) {
    (void)req_id; (void)userdata;
    int duration = args[0] ? atoi(args) : 0;
    // 拍照/录像...
    result->status = 0;
    result->output = "data:image/jpeg;base64,...";  // SDK 自动回执
    return HA_OK;
}

ha_cmd_handler_def_t handlers[] = {
    {.command = "shell",      .handler = handle_shell},
    {.command = "camerasue",  .handler = handle_camerasue},
    {.command = "screensee",  .handler = handle_screensee},
    {.command = "speakeruse", .handler = handle_speakeruse},
    {.command = NULL},  /* 标记结束 */
};

ha_config_t config = {
    .transport = my_transport,     // 用户实现 4 个函数
    .server    = "192.168.1.100:9890",
    .token     = "my-token",
    .device = {
        .device_id = "esp32-cam-1",
        .name      = "门口摄像头",
        .kind      = "camera",
        .caps      = caps,
    },
    .handlers  = handlers,   // 声明式命令处理表
    .on_state  = my_state_handler,
};

ha_client_t *client = ha_client_new(&config);
ha_client_start(client);
while (1) {
    ha_client_process(client);     // 主循环处理
}
```

### 传输层抽象

用户只需实现 4 个函数，适配不同平台：

```c
ha_transport_t my_transport = {
    .connect = my_tcp_connect,   // 建立 TCP 连接
    .send    = my_tcp_send,      // 发送数据
    .recv    = my_tcp_recv,      // 接收数据（阻塞）
    .close   = my_tcp_close,     // 关闭连接
    .ctx     = &my_platform_ctx,
};
```

### 支持的协议

| 功能 | API |
|------|-----|
| WS 连接 + 握手 | `ha_client_start` 自动完成 |
| 设备注册 (hello/bind) | 启动时自动发送 |
| 命令接收 (shell/homeagent) | `handlers` 表声明式注册，SDK 自动分发 |
| 命令回执 | `ha_client_send_result` |
| 二进制分块（录像等） | `ha_client_send_data_chunked` |
| TTS 音频接收 | `on_binary` 回调 |
| 事件上报 | `ha_client_send_event` |
| 状态上报 | `ha_client_send_status` |
| 心跳保持 | 自动 ping/pong |

### 使用方式

通过 `plugindev` 工具链初始化项目：

```bash
plugindev init my-adapter --type remotedevice
```

生成 `main.c` + `CMakeLists.txt`，可直接编译或作为三方库引入：

```cmake
add_subdirectory(path/to/ha_remotedevice)
target_link_libraries(my_app ha_remotedevice)
target_include_directories(my_app PRIVATE ${HA_REMOTEDEVICE_INCLUDE_DIR})
```

### 快速接入指南

以下是从零到设备成功接入 HomeAgent 的完整步骤。

#### 1. 准备工作

在 HomeAgent 平台上创建接入令牌：

```bash
# 在 HomeAgent 服务端创建一个设备接入令牌
curl -X POST http://<homeagent-server>:8080/api/v1/device/token \
  -H "Content-Type: application/json" \
  -d '{"device_id":"esp32-cam-1","name":"门口摄像头","kind":"camera"}'
# 返回: {"token":"ha-dev-token-xxxxx"}
```

记录下返回的 `token`，设备端配置时使用。

#### 2. 实现传输层（4 个函数）

根据你的平台实现 `ha_transport_t` 的 4 个函数指针。以下是几种常见场景：

**场景 A：带 TCP/IP 栈的嵌入式设备（如 ESP32 + lwIP）**

```c
#include "ha_remotedevice.h"
#include "lwip/sockets.h"

static int esp_connect(void *ctx, const char *host, uint16_t port) {
    struct sockaddr_in addr;
    int sock = socket(AF_INET, SOCK_STREAM, 0);
    if (sock < 0) return -1;
    addr.sin_family = AF_INET;
    addr.sin_port = htons(port);
    inet_pton(AF_INET, host, &addr.sin_addr);
    int ret = connect(sock, (struct sockaddr *)&addr, sizeof(addr));
    if (ret < 0) { closesocket(sock); return -1; }
    *(int *)ctx = sock;
    return 0;
}

static int esp_send(void *ctx, const uint8_t *data, int len) {
    int sock = *(int *)ctx;
    return send(sock, (const char *)data, len, 0);
}

static int esp_recv(void *ctx, uint8_t *buf, int len) {
    int sock = *(int *)ctx;
    return recv(sock, (char *)buf, len, 0);
}

static void esp_close(void *ctx) {
    int sock = *(int *)ctx;
    closesocket(sock);
}

int esp_ctx = -1;
ha_transport_t transport = {
    .connect = esp_connect,
    .send    = esp_send,
    .recv    = esp_recv,
    .close   = esp_close,
    .ctx     = &esp_ctx,
};
```

**场景 B：通过串口（UART）连接透传模块**

```c
static int uart_connect(void *ctx, const char *host, uint16_t port) {
    (void)host; (void)port;
    // 初始化 UART，波特率 115200
    return uart_init((uart_ctx_t *)ctx, 115200);
}

static int uart_send(void *ctx, const uint8_t *data, int len) {
    return uart_write((uart_ctx_t *)ctx, data, len);
}

static int uart_recv(void *ctx, uint8_t *buf, int len) {
    return uart_read((uart_ctx_t *)ctx, buf, len);
}

static void uart_close(void *ctx) {
    uart_deinit((uart_ctx_t *)ctx);
}
```

> 注意：UART 透传时，另一端需运行一个 TCP 桥接程序，将串口数据转发到 HomeAgent 的 WebSocket 端口。

#### 3. 声明设备能力和命令处理

```c
#include "ha_remotedevice.h"

/* 声明设备能力 */
const char *caps[] = {"camera", "speaker", "status", NULL};

/* 处理 camerasue 命令（拍照） */
static ha_status_t handle_camera(const char *req_id, const char *args,
                                 ha_cmd_result_t *result, void *userdata) {
    (void)req_id; (void)userdata;
    int duration = args[0] ? atoi(args) : 0;  // 参数：录像时长

    // 拍照或录像，将结果填入 result
    result->status = 0;
    result->output = "data:image/jpeg;base64,/9j/4AAQ...";  // base64 图像数据
    return HA_OK;
}

/* 处理 shell 命令 */
static ha_status_t handle_shell(const char *req_id, const char *args,
                                ha_cmd_result_t *result, void *userdata) {
    (void)req_id; (void)userdata;
    // 执行 shell 命令，args 为完整命令字符串
    result->status = 0;
    result->output = "command executed";
    return HA_OK;
}

/* 声明式命令处理表 */
ha_cmd_handler_def_t handlers[] = {
    {.command = "shell",      .handler = handle_shell},
    {.command = "camerasue",  .handler = handle_camera},
    {.command = "screensee",  .handler = handle_camera},
    {.command = "speakeruse", .handler = handle_speaker},
    {.command = NULL},  /* 标记结束 */
};
```

#### 4. 配置并启动客户端

```c
ha_config_t config = {
    .transport = transport,                 // 传输层实现
    .server    = "192.168.1.100:9890",      // HomeAgent 服务端地址
    .token     = "ha-dev-token-xxxxx",      // 第 1 步获取的令牌
    .device = {
        .device_id = "esp32-cam-1",
        .name      = "门口摄像头",
        .kind      = "camera",
        .caps      = caps,
        .info_json = "{\"chip\":\"ESP32-S3\",\"firmware\":\"v1.0\"}",
    },
    .handlers  = handlers,                  // 命令处理表
    .on_binary = on_binary_data,            // 接收 TTS 音频等二进制数据
    .on_state  = on_state_change,           // 连接状态变化回调
    .ping_interval = 30,                    // 心跳间隔秒数
};

ha_client_t *client = ha_client_new(&config);
ha_status_t ret = ha_client_start(client);
if (ret != HA_OK) {
    printf("设备接入失败: %d\n", ret);
    return;
}

/* 主循环 */
while (1) {
    ha_client_process(client);  // 处理协议帧、心跳、命令分发

    /* 可选：设备主动上报事件 */
    ha_client_send_event(client, "motion_detected",
                         "{\"zone\":\"front_door\",\"confidence\":0.95}");

    /* 可选：上报设备状态 */
    ha_client_send_status(client, "online");

    vTaskDelay(100 / portTICK_PERIOD_MS);  // 嵌入式 RTOS 风格延时
}
```

#### 5. 验证连接

在 HomeAgent 服务端检查设备是否在线：

```bash
# 查看已注册设备列表
curl http://<homeagent-server>:8080/api/v1/device/list
# 预期输出包含: {"device_id":"esp32-cam-1","status":"online",...}

# 向设备发送命令（测试 camerasue）
curl -X POST http://<homeagent-server>:8080/api/v1/device/esp32-cam-1/cmd \
  -H "Content-Type: application/json" \
  -d '{"cmd":"camerasue","args":"3"}'
# 预期返回: {"status":"ok","result":"data:image/jpeg;base64,..."}
```

#### 6. 调试技巧

| 问题 | 检查点 |
|------|--------|
| 连接失败 | 确认 `server` 地址和端口可通；检查 `token` 是否正确 |
| WS 握手失败 | 确认 HomeAgent 服务端已开启 WebSocket 支持 |
| 命令无响应 | 确认 `handlers` 表中注册了对应命令名；检查 `on_binary` 是否配置 |
| 断线重连 | `max_reconnect` 控制重连次数，-1 为无限重连 |
| 内存不足（嵌入式） | 定义 `HA_NO_ALLOC` 宏禁用动态内存分配 |

### 位置

- **SDK 源码**: `remotedevice/`
- **plugindev 模板**: `plugindev init --type remotedevice`

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

## 许可

SDK 以 **AGPL-3.0-only** 发布，全文见 [LICENSE](LICENSE)。

**这对插件开发者是实质性约束**：SDK 会随插件一起**静态链接**（其源码进入插件二进制），
插件因此是本 SDK 的衍生作品，**必须以相同许可（AGPL-3.0-only）发布**；并且因为 AGPL §13
覆盖网络交互，通过 HTTP/WebSocket 等向用户提供服务的插件同样要向使用者提供源码。
若你的插件需要闭源，唯一合规路径是另行取得本项目的例外/商业授权——目前不提供。

第三方组件（Go 依赖：go-sqlite3、gojieba、bubbletea 等，均为 MIT / BSD-3 / Apache-2.0）
保持各自原有许可。平台侧的模型与推理运行时（Chinese-CLIP Apache-2.0、ONNX Runtime MIT）
不属于本 SDK，其许可全文随发行包放在 `/usr/share/doc/homeagent/licenses/`。
