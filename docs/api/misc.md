<!-- 本页由 tools/apidoc/gensite 从源码生成，请勿手改；要改文档就改 sdk/*.go 的注释。 -->

# 其他类型

剩余的类型与方法：`PluginSDK` 本体的访问器、`StageContext` 的并发控制，以及多模态辅助类型。没有归入上面任何一个主题，但可能仍会用到。

## `DocMemoryAPI`

DocMemoryAPI provides access to the document vector store.

| 方法 | 说明 |
|---|---|
| [`Insert`](#docmemoryapiinsert) |  |
| [`InsertWithMedia`](#docmemoryapiinsertwithmedia) | InsertWithMedia 写入文档并关联媒体。attachments 里带 Data 的会落进 |
| [`Query`](#docmemoryapiquery) |  |
| [`Remove`](#docmemoryapiremove) |  |
| [`Stats`](#docmemoryapistats) |  |

### `DocMemoryAPI.Insert`

```go
Insert(doc *Doc) error
```

<small>`memory.go:76`</small>

### `DocMemoryAPI.InsertWithMedia`

```go
InsertWithMedia(doc *Doc, attachments []MediaAttachment) error
```

InsertWithMedia 写入文档并关联媒体。attachments 里带 Data 的会落进
内容寻址存储（相同字节只存一份），只带 Digest 的直接引用已有内容。
媒体成为文档直接持有的一等记忆块：文档向量会融合它们的原生向量，
因此图片按自己的向量被召回，不依赖任何生成的描述文本。

<small>`memory.go:81`</small>

### `DocMemoryAPI.Query`

```go
Query(text string, topK int) []*Doc
```

<small>`memory.go:75`</small>

### `DocMemoryAPI.Remove`

```go
Remove(id string)
```

<small>`memory.go:82`</small>

### `DocMemoryAPI.Stats`

```go
Stats() map[string]interface{}
```

<small>`memory.go:83`</small>

## `EventSubscriber`

EventSubscriber allows plugins to subscribe to kernel events.
This is a restricted interface: plugins can subscribe but the kernel
controls which events are delivered.

| 方法 | 说明 |
|---|---|
| [`Subscribe`](#eventsubscribersubscribe) |  |

### `EventSubscriber.Subscribe`

!!! warning "仅内核内置插件可用"

```go
Subscribe(eventType EventType, handler EventHandler) func()
```

<small>`plugin.go:378`</small>

## `IOInjector`

IOInjector provides methods for injecting input and interrupts into the agent pipeline.
All methods accept (source, channel) where channel is the target output channel
for routing the agent's response.

| 方法 | 说明 |
|---|---|
| [`InjectInputMedia`](#ioinjectorinjectinputmedia) |  |
| [`InjectInputMediaOpts`](#ioinjectorinjectinputmediaopts) |  |
| [`InjectInputMediaSync`](#ioinjectorinjectinputmediasync) |  |
| [`InjectInputMediaSyncOpts`](#ioinjectorinjectinputmediasyncopts) |  |
| [`InjectInputSync`](#ioinjectorinjectinputsync) | InjectInputSync 注入输入事件并同步等待 agent 回复，返回回复文本（无回复时返回空串）。 |
| [`InjectInputSyncOpts`](#ioinjectorinjectinputsyncopts) |  |
| [`InjectInterruptMedia`](#ioinjectorinjectinterruptmedia) |  |
| [`InjectInterruptMediaOpts`](#ioinjectorinjectinterruptmediaopts) |  |
| [`InjectInterruptText`](#ioinjectorinjectinterrupttext) |  |
| [`InjectInterruptTextOpts`](#ioinjectorinjectinterrupttextopts) |  |
| [`InjectText`](#ioinjectorinjecttext) |  |
| [`InjectTextNoMemory`](#ioinjectorinjecttextnomemory) |  |
| [`InjectTextOpts`](#ioinjectorinjecttextopts) | 以下 Opts 变体让调用点在**这一次注入**上声明记忆与裁剪行为。 |
| [`SetToolBlocks`](#ioinjectorsettoolblocks) | SetToolBlocks 插件工具注入多模态内容块（image_url/audio_url），内核在下一条 |

### `IOInjector.InjectInputMedia`

```go
InjectInputMedia(source, channel, text string, blocks []ContentBlock)
```

<small>`plugin.go:329`</small>

### `IOInjector.InjectInputMediaOpts`

```go
InjectInputMediaOpts(source, channel, text string, blocks []ContentBlock, opts InjectOptions)
```

<small>`plugin.go:340`</small>

### `IOInjector.InjectInputMediaSync`

```go
InjectInputMediaSync(source, channel, text string, blocks []ContentBlock) string
```

<small>`plugin.go:330`</small>

### `IOInjector.InjectInputMediaSyncOpts`

```go
InjectInputMediaSyncOpts(source, channel, text string, blocks []ContentBlock, opts InjectOptions) string
```

<small>`plugin.go:341`</small>

### `IOInjector.InjectInputSync`

```go
InjectInputSync(source, channel, text string) string
```

InjectInputSync 注入输入事件并同步等待 agent 回复，返回回复文本（无回复时返回空串）。
用于通道消息的完整闭环：收到入站 → agent 处理 → 回复取回 → 送回通道。

**示例插件里的真实用法**

| 插件 | 位置 | 代码 |
|---|---|---|
| [`a2a`](../examples/index.md#a2a) | `example/a2a/plugin.go:345` | `reply := p.sdk.InjectInputSync(p.name, p.name,` |
| [`acp`](../examples/index.md#acp) | `example/acp/plugin.go:237` | `reply = p.sdk.InjectInputSync(p.name, p.name,` |

<small>`plugin.go:325`</small>

### `IOInjector.InjectInputSyncOpts`

```go
InjectInputSyncOpts(source, channel, text string, opts InjectOptions) string
```

<small>`plugin.go:339`</small>

### `IOInjector.InjectInterruptMedia`

```go
InjectInterruptMedia(source, channel, text string, blocks []ContentBlock)
```

<small>`plugin.go:331`</small>

### `IOInjector.InjectInterruptMediaOpts`

```go
InjectInterruptMediaOpts(source, channel, text string, blocks []ContentBlock, opts InjectOptions)
```

<small>`plugin.go:342`</small>

### `IOInjector.InjectInterruptText`

```go
InjectInterruptText(source, channel, text string)
```

<small>`plugin.go:320`</small>

### `IOInjector.InjectInterruptTextOpts`

```go
InjectInterruptTextOpts(source, channel, text string, opts InjectOptions)
```

**示例插件里的真实用法**

| 插件 | 位置 | 代码 |
|---|---|---|
| [`browser`](../examples/index.md#browser) | `example/browser/plugin.go:1319` | `p.sdk.InjectInterruptTextOpts(p.name, p.name,` |
| [`calendar`](../examples/index.md#calendar) | `example/calendar/plugin.go:527` | `p.sdk.InjectInterruptTextOpts("calendar", "calendar", msg, sdk.InjectOptions{NoMemory: true})` |
| [`memo`](../examples/index.md#memo) | `example/memo/plugin.go:299` | `p.sdk.InjectInterruptTextOpts(p.name, p.name,` |
| [`qq`](../examples/index.md#qq) | `example/qq/plugin.go:1493` | `p.sdk.InjectInterruptTextOpts(p.name, p.name, text, sdk.InjectOptions{` |

<small>`plugin.go:338`</small>

### `IOInjector.InjectText`

```go
InjectText(source, channel, text string)
```

<small>`plugin.go:321`</small>

### `IOInjector.InjectTextNoMemory`

```go
InjectTextNoMemory(source, channel, text string)
```

**示例插件里的真实用法**

| 插件 | 位置 | 代码 |
|---|---|---|
| [`browser`](../examples/index.md#browser) | `example/browser/plugin.go:1114` | `p.sdk.InjectTextNoMemory(p.name, p.name, fmt.Sprintf("[浏览器 %s 已导航到 %s]", id, rawURL))` |

<small>`plugin.go:322`</small>

### `IOInjector.InjectTextOpts`

```go
InjectTextOpts(source, channel, text string, opts InjectOptions)
```

以下 Opts 变体让调用点在**这一次注入**上声明记忆与裁剪行为。

上面那些不带 opts 的方法等价于传零值 InjectOptions（记入记忆 + 不裁剪），
保留它们是为了不破坏已有插件；新代码应当用 Opts 变体把意图写清楚。

<small>`plugin.go:337`</small>

### `IOInjector.SetToolBlocks`

```go
SetToolBlocks(blocks []ContentBlock)
```

SetToolBlocks 插件工具注入多模态内容块（image_url/audio_url），内核在下一条
tool message 的 content 数组里带上这些块，让模型在后续轮次看到图/听到音频。

<small>`plugin.go:328`</small>

## `KnowledgeAPI`

KnowledgeAPI provides access to the knowledge store.

| 方法 | 说明 |
|---|---|
| [`Add`](#knowledgeapiadd) |  |
| [`List`](#knowledgeapilist) |  |
| [`Search`](#knowledgeapisearch) |  |

### `KnowledgeAPI.Add`

```go
Add(name, content string) error
```

<small>`knowledge.go:6`</small>

### `KnowledgeAPI.List`

```go
List() ([]string, error)
```

<small>`knowledge.go:7`</small>

### `KnowledgeAPI.Search`

```go
Search(query string, topK int) ([]*Knowledge, error)
```

<small>`knowledge.go:5`</small>

## `LLMAPI`

LLMAPI provides access to the LLM provider manager.

| 方法 | 说明 |
|---|---|
| [`CurrentSource`](#llmapicurrentsource) |  |
| [`ListSources`](#llmapilistsources) |  |
| [`SetSource`](#llmapisetsource) |  |

### `LLMAPI.CurrentSource`

```go
CurrentSource() string
```

<small>`llm.go:7`</small>

### `LLMAPI.ListSources`

```go
ListSources() []string
```

<small>`llm.go:5`</small>

### `LLMAPI.SetSource`

```go
SetSource(name string) error
```

<small>`llm.go:6`</small>

## `MemoryAPI`

MemoryAPI provides access to the graph memory (entity-relation store).

| 方法 | 说明 |
|---|---|
| [`Commit`](#memoryapicommit) |  |
| [`Introspect`](#memoryapiintrospect) |  |
| [`MergeEntities`](#memoryapimergeentities) |  |
| [`Purge`](#memoryapipurge) |  |
| [`Recall`](#memoryapirecall) |  |

### `MemoryAPI.Commit`

```go
Commit(triples []Triple) error
```

<small>`memory.go:6`</small>

### `MemoryAPI.Introspect`

```go
Introspect() (map[string]interface{}, error)
```

<small>`memory.go:7`</small>

### `MemoryAPI.MergeEntities`

```go
MergeEntities(source, target string) (int, error)
```

<small>`memory.go:8`</small>

### `MemoryAPI.Purge`

```go
Purge(criteria map[string]string, mode string) (int, error)
```

<small>`memory.go:9`</small>

### `MemoryAPI.Recall`

```go
Recall(query []string, depth int) ([]Entity, []Relation, error)
```

<small>`memory.go:5`</small>

## `Plugin`

Plugin is the interface every plugin must implement.

| 方法 | 说明 |
|---|---|
| [`Name`](#pluginname) |  |
| [`Start`](#pluginstart) |  |
| [`Stop`](#pluginstop) |  |

### `Plugin.Name`

```go
Name() string
```

<small>`plugin.go:14`</small>

### `Plugin.Start`

```go
Start(sdk *PluginSDK) error
```

<small>`plugin.go:15`</small>

### `Plugin.Stop`

```go
Stop() error
```

<small>`plugin.go:16`</small>

## `PluginMgrAPI`

PluginMgrAPI 提供插件管理能力（外部插件可调用）。
由 bridge 注入 dispatch 实现，走 C ABI CORE_PLUGIN_RELOAD_ONE 等。

| 方法 | 说明 |
|---|---|
| [`IsPluginDisabled`](#pluginmgrapiisplugindisabled) | IsPluginDisabled 查询插件是否被禁用。 |
| [`ListLoadedPlugins`](#pluginmgrapilistloadedplugins) | ListLoadedPlugins 列出已加载插件。 |
| [`ReloadOne`](#pluginmgrapireloadone) | ReloadOne 重载单个插件（停止后重新加载）。 |

### `PluginMgrAPI.IsPluginDisabled`

```go
IsPluginDisabled(name string) bool
```

IsPluginDisabled 查询插件是否被禁用。

<small>`plugin.go:389`</small>

### `PluginMgrAPI.ListLoadedPlugins`

```go
ListLoadedPlugins() []string
```

ListLoadedPlugins 列出已加载插件。

<small>`plugin.go:387`</small>

### `PluginMgrAPI.ReloadOne`

```go
ReloadOne(name string) error
```

ReloadOne 重载单个插件（停止后重新加载）。

<small>`plugin.go:385`</small>

## `SettingsAPI`

| 方法 | 说明 |
|---|---|
| [`DataDir`](#settingsapidatadir) | DataDir returns the plugin-specific data directory (guaranteed to exist): |
| [`Defs`](#settingsapidefs) | Defs returns config definitions matching the prefix. |
| [`Dump`](#settingsapidump) | Dump returns all config values. |
| [`Get`](#settingsapiget) | Get reads the plugin's own config value (config_<name> table). |
| [`GetCore`](#settingsapigetcore) | GetCore reads the core config table. |
| [`GetPlugin`](#settingsapigetplugin) | GetPlugin reads another plugin's config table. |
| [`List`](#settingsapilist) | List returns all keys matching the given prefix. |
| [`ListCore`](#settingsapilistcore) | ListCore lists core config keys matching the prefix. |
| [`ListPlugin`](#settingsapilistplugin) | ListPlugin lists another plugin's config keys matching the prefix. |
| [`Plugins`](#settingsapiplugins) | Plugins returns a list of all plugin config namespaces. |
| [`RegisterDef`](#settingsapiregisterdef) | RegisterDef registers a config definition for UI display. |
| [`Set`](#settingsapiset) | Set writes a config value to the plugin's own config table. |
| [`SetCore`](#settingsapisetcore) | SetCore writes to the core config table. |
| [`SetPlugin`](#settingsapisetplugin) | SetPlugin writes to another plugin's config table. |

### `SettingsAPI.DataDir`

```go
DataDir() string
```

DataDir returns the plugin-specific data directory (guaranteed to exist):
<daemon data>/plugin_data/<plugin_name>. Plugins should persist any
runtime files (generated images, caches, downloads) here.

<small>`settings.go:25`</small>

### `SettingsAPI.Defs`

```go
Defs(prefix string) []*ConfigDef
```

Defs returns config definitions matching the prefix.

<small>`settings.go:40`</small>

### `SettingsAPI.Dump`

```go
Dump() map[string]interface{}
```

Dump returns all config values.

<small>`settings.go:43`</small>

### `SettingsAPI.Get`

```go
Get(key string) (interface{}, error)
```

Get reads the plugin's own config value (config_<name> table).

<small>`settings.go:5`</small>

### `SettingsAPI.GetCore`

```go
GetCore(key string) (interface{}, error)
```

GetCore reads the core config table.

<small>`settings.go:14`</small>

### `SettingsAPI.GetPlugin`

```go
GetPlugin(plugin, key string) (interface{}, error)
```

GetPlugin reads another plugin's config table.

<small>`settings.go:28`</small>

### `SettingsAPI.List`

```go
List(prefix string) ([]string, error)
```

List returns all keys matching the given prefix.

<small>`settings.go:11`</small>

### `SettingsAPI.ListCore`

```go
ListCore(prefix string) ([]string, error)
```

ListCore lists core config keys matching the prefix.

<small>`settings.go:20`</small>

### `SettingsAPI.ListPlugin`

```go
ListPlugin(plugin, prefix string) ([]string, error)
```

ListPlugin lists another plugin's config keys matching the prefix.

<small>`settings.go:34`</small>

### `SettingsAPI.Plugins`

```go
Plugins() []string
```

Plugins returns a list of all plugin config namespaces.

<small>`settings.go:46`</small>

### `SettingsAPI.RegisterDef`

```go
RegisterDef(def ConfigDef)
```

RegisterDef registers a config definition for UI display.

<small>`settings.go:37`</small>

### `SettingsAPI.Set`

```go
Set(key string, value interface{}) error
```

Set writes a config value to the plugin's own config table.

<small>`settings.go:8`</small>

### `SettingsAPI.SetCore`

```go
SetCore(key string, value interface{}) error
```

SetCore writes to the core config table.

<small>`settings.go:17`</small>

### `SettingsAPI.SetPlugin`

```go
SetPlugin(plugin, key string, value interface{}) error
```

SetPlugin writes to another plugin's config table.

<small>`settings.go:31`</small>

## `SocialAPI`

SocialAPI provides read-only access to the social graph (person profiles and relationships).
External plugins can query person traits and social networks but cannot modify them.

| 方法 | 说明 |
|---|---|
| [`GetNetwork`](#socialapigetnetwork) |  |
| [`GetPerson`](#socialapigetperson) |  |
| [`GetRelations`](#socialapigetrelations) |  |
| [`GetTrait`](#socialapigettrait) |  |
| [`ListPersons`](#socialapilistpersons) |  |

### `SocialAPI.GetNetwork`

```go
GetNetwork(name string, depth int) ([]*PersonProfile, error)
```

<small>`memory.go:104`</small>

### `SocialAPI.GetPerson`

```go
GetPerson(name string) (*PersonProfile, error)
```

<small>`memory.go:101`</small>

### `SocialAPI.GetRelations`

```go
GetRelations(name string) ([]SocialRelation, error)
```

<small>`memory.go:103`</small>

### `SocialAPI.GetTrait`

```go
GetTrait(name, trait string) (string, bool)
```

<small>`memory.go:102`</small>

### `SocialAPI.ListPersons`

```go
ListPersons() ([]string, error)
```

<small>`memory.go:105`</small>

## `TextMemoryAPI`

TextMemoryAPI provides access to chronological text event storage.

| 方法 | 说明 |
|---|---|
| [`Append`](#textmemoryapiappend) |  |

### `TextMemoryAPI.Append`

```go
Append(evt TextEvent) error
```

<small>`memory.go:44`</small>

### `AudioURL`

```go
type AudioURL struct { URL string `json:"url"` }
```

<small>`plugin.go:966`</small>

### `EffectiveProxyAuth`

```go
func EffectiveProxyAuth(auth string) string
```

EffectiveProxyAuth 返回生效的鉴权模式（空串归一化为 ProxyAuthHomeAgent）。

<small>`proxy.go:169`</small>

### `ToolError.Error`

```go
func (e *ToolError) Error() string
```

Error 实现 error，便于工具同时走 (ToolError, error) 通道。

**示例插件里的真实用法**

| 插件 | 位置 | 代码 |
|---|---|---|
| [`a2a`](../examples/index.md#a2a) | `example/a2a/plugin.go:317` | `http.Error(w, "query/message.text required", http.StatusBadRequest)` |
| [`acp`](../examples/index.md#acp) | `example/acp/plugin.go:175` | `http.Error(w, "", http.StatusMethodNotAllowed)` |
| [`ai_image`](../examples/index.md#ai_image) | `example/ai_image/plugin.go:259` | `return map[string]interface{}{"isError": true, "content": "Request failed: " + err.Error()}, nil` |
| [`browser`](../examples/index.md#browser) | `example/browser/plugin_test.go:15` | `if err == nil \|\| !strings.Contains(err.Error(), "timeout is required") {` |

<small>`plugin.go:264`</small>

### `ImageURL`

```go
type ImageURL struct { URL string `json:"url"` Detail string `json:"detail,omitempty"` }
```

<small>`plugin.go:961`</small>

### `MemItem`

```go
type MemItem struct { Role string `json:"role"` Content string `json:"content"` Score float64 `json:"score"` }
```

MemItem represents a memory item in stage context.

<small>`plugin.go:220`</small>

### `NormalizeProxyHost`

```go
func NormalizeProxyHost(pluginName string) string
```

NormalizeProxyHost 由插件名派生默认 Host 标签。

下划线转连字符：插件名允许下划线（huawei_smarthome），但 DNS label 不允许，
直接用会导致该子域名无法解析——这里统一转换，避免每个插件各自碰运气。

<small>`proxy.go:202`</small>

### `PluginSDK`

```go
type PluginSDK struct { name string regTool ToolRegistrar regStage StageRegistrar regAPI APIRegistrar regOutput OutputChannelRegistrar …
```

PluginSDK is the main API surface provided to plugins at runtime.
It wraps tool registration, settings, memory, knowledge, LLM, and IO injection.

<small>`plugin.go:436`</small>

### `ProxyAuthHomeAgent`

```go
const ProxyAuthHomeAgent
```

ProxyAuthHomeAgent 表示由 HomeAgent 统一保护：浏览器走门户会话
（homeagent_session cookie），非浏览器客户端走 X-API-Key。
两者都没有时返回 401，而不是把请求透传给上游。

<small>`proxy.go:149`</small>

### `ProxyAuthNone`

```go
const ProxyAuthNone
```

ProxyAuthNone 表示不经 HomeAgent 鉴权，直接把请求转发给上游。

适用场景：上游自己有鉴权且调用方不是浏览器（设备/嵌入式客户端），
或上游是刻意公开的服务。选用它意味着**信任上游自身的鉴权**，
且该服务在网络层可达范围内对所有人开放。

<small>`proxy.go:156`</small>

### `ProxyDef`

```go
type ProxyDef struct { // Name 是同一插件内多条声明的唯一标识（如 "ui"、"api"）。 // 运行期由 RegisterProxy 的第一个参数填入；声明式由 …
```

反向代理声明：插件告诉 HomeAgent「我起了个 HTTP 服务，请把它反代出去」。

为什么需要：插件自带 Web UI / HTTP API 时，监听地址在插件自己的配置里
（如 127.0.0.1:12100），外部无从得知；而 webui 的对外端口通常只有一个
（默认 :8080，且常经 frp 单端口隧道穿透）。没有声明机制时，用户只能
「知道端口 + 自己配转发」，插件换端口就失效。

设计取舍——**声明式而非注册式**：声明写在 plugin.json 里，由 HomeAgent
在加载插件时读取聚合，而不是让插件在运行期调 API 注册。理由：
 1. 静态可发现：未启动/已崩溃的插件，其服务声明依然可见（可给出准确报错
    「插件 X 声明了 ui 但目标 127.0.0.1:12100 不可达」，而不是静默 404）；
 2. 可版本化：声明随插件包一起分发、可 diff、可审计；
 3. 旧内核无害：manifest 解析忽略未知字段，未支持该能力的 HomeAgent 读旧
    插件、或旧 HomeAgent 读新插件都不会报错。

与 ToolDef / ChannelDef / ConfigDef 同族：SDK 定义声明契约，内核实现行为。
声明方式与其它能力一致 —— 在 Start() 里调 RegisterProxy(name, def)，
或写进 plugin.json 的 proxies 字段（外部插件两种都支持）。

安全性：**不声明 = 不被反代**。声明本身就是能力声明，因此不需要在
capabilities 里另外开一个开关——最小权限默认生效。

# 单一入口原则（强制要求）

**一个声明 = 一个入口**。被反代的插件必须让它的全部资源与接口都能从
该入口的一个基准路径出发访问到，不得依赖「入口之外的根路径」。

为什么强制：反代有两种挂载形态，而它们对「根路径」的处理截然不同——

	Host 形态（host）：插件独占 <标签>.<基域名>，根路径就是插件的根。
	                  根绝对路径（fetch('/api/x')）**天然正确**。
	Path 形态（path）：插件挂在门户自身 host 的某个前缀下，根路径属于**门户**。
	                  此时插件里的 fetch('/api/x') 会打到门户自己的 /api/x
	                  —— 静默错路由，页面能开但功能全坏。

于是「同一个插件必须同时支持两种形态」这条要求，等价于：

	**插件内部一律使用相对路径**（或基于 <base>/location 推导的路径），
	绝不硬编码以 / 开头的绝对路径。

这样同一份前端在两种形态下都正确，插件作者也不必知道自己被挂在哪。
反代层据此可以：外部子域可用时给 Host 形态，子域不可用（证书/放行限制）
时给 Path 形态，**无需插件配合改动**。

自检（插件作者在本地就该做）：把页面挂到 <门户>/<任意前缀>/ 下访问，
所有请求都必须仍然打到插件自己。

本项目实测案例：某插件前端写死 fetch('/api/status')，配在
/p/huawei/ 下会打到门户的 /api/status（404 或返回门户数据）；
改成相对路径后两种形态同时可用。
ProxyDef 是一个服务的**反代声明体**。

与 ToolDef 同构：Name 同时出现在字段与 RegisterProxy 的第一个参数里
（ToolDef 也是这么做的 —— 字段供 plugin.json 序列化，参数供运行期调用）。
Name 只用于展示、日志与冲突提示，**不参与路由**（路由键是 Host 与 Path）。

<small>`proxy.go:64`</small>

### `ProxyRegistrar`

```go
type ProxyRegistrar func(name string, def ProxyDef)
```

ProxyRegistrar 由内核注入（与 ToolRegistrar / InputChannelRegistrar 同族）。
插件不直接调它，用 RegisterProxy。

为什么需要运行期注册（明明有 plugin.json 自动发现）：**内置插件**
（编译进内核、没有独立插件目录与 plugin.json，如 remotedevice）扫不到；
而它们恰恰最需要被反代出去（设备网关就是内置的）。两种来源互补：
  - 外部插件 → plugin.json 的 proxies（静态，未启动也可见）
  - 内置插件 → RegisterProxy（运行期，随 Start 注册）

<small>`proxy.go:306`</small>

### `PluginSDK.RegisterProxy`

```go
func (s *PluginSDK) RegisterProxy(name string, def ProxyDef)
```

RegisterProxy 声明一个需要 HomeAgent 反代出去的服务。

与 RegisterTool / RegisterInputChannel / RegisterOutputChannel 同一风格：
显式给名字 + 声明体。名字用于展示、日志与冲突提示（不参与路由 —— 路由键是
def.Host / def.Path）。

用法（通常在 Start 里调用）：

	s.RegisterProxy("ui", sdk.ProxyDef{
	    Host: "myapp", Target: "127.0.0.1:12100",
	})

声明立即生效（反代表在下一次请求时重建）。**不做去重**：同一 Host/Path
被两条声明占用时由反代层判定冲突并明确报错，而不是在这里静默吞掉 ——
插件作者需要看见冲突。

与 plugin.json 的 proxies 字段等价：写哪个都行，两者会合并（同名以本调用为准）。

<small>`proxy.go:335`</small>

### `SDKVersion`

```go
var SDKVersion
```

SDKVersion 是对外暴露的 SDK 版本号。

<small>`plugin.go:10`</small>

### `ScenePolicyAuto`

```go
const ScenePolicyAuto
```

场面策略：决定一次输入是否参与**场面识别**（场景式记忆）。

与前两项再正交一轴：NoMemory 管「进不进记忆计算」、ContextPolicy 管
「裁不裁上下文」、RecallPolicy 管「召不召回记忆」，本项管的是
「这条输入算不算一场戏的一部分」——它决定输入会不会产出现场指纹
（通道/对话对象/工具/话题/时段），进而决定会不会长出、命中、写入场景。

默认（空串或 ScenePolicyAuto）**参与**，保持既有行为：场景式记忆自
v1.3 落地起就对所有通道无条件生效，没有开关。不默认关有两个原因：
 1. 场景只**附加**现有记忆的检索路，不改记忆本体，默认关会让存量
    通道突然失去场景召回；
 2. 「关」是少数意图（内部信噪通道），少数意图不该是默认——
    与 ContextPolicy 刻意相反（同为破坏性操作，那里是默认关）。

该关的典型是纯内部通道：system（内核自循环）、kernel、timer、healthcheck。
但**现网不标任何一个**（2026-09-26 裁定）：实测这些 0-refs 通道合计 70
strength、0 条记忆，场景召回返回空；而 declared 场景不进相似度空间
（loadEmergentScenesLocked 只取 origin='emergent'），多写对聚类零影响。
「多写无影响、少写会缺场景」——默认 auto 保持开，声明项只作为插件
将来确实需要时的闸门。

<small>`plugin.go:98`</small>

### `ScenePolicyNone`

```go
const ScenePolicyNone
```

场面策略：决定一次输入是否参与**场面识别**（场景式记忆）。

与前两项再正交一轴：NoMemory 管「进不进记忆计算」、ContextPolicy 管
「裁不裁上下文」、RecallPolicy 管「召不召回记忆」，本项管的是
「这条输入算不算一场戏的一部分」——它决定输入会不会产出现场指纹
（通道/对话对象/工具/话题/时段），进而决定会不会长出、命中、写入场景。

默认（空串或 ScenePolicyAuto）**参与**，保持既有行为：场景式记忆自
v1.3 落地起就对所有通道无条件生效，没有开关。不默认关有两个原因：
 1. 场景只**附加**现有记忆的检索路，不改记忆本体，默认关会让存量
    通道突然失去场景召回；
 2. 「关」是少数意图（内部信噪通道），少数意图不该是默认——
    与 ContextPolicy 刻意相反（同为破坏性操作，那里是默认关）。

该关的典型是纯内部通道：system（内核自循环）、kernel、timer、healthcheck。
但**现网不标任何一个**（2026-09-26 裁定）：实测这些 0-refs 通道合计 70
strength、0 条记忆，场景召回返回空；而 declared 场景不进相似度空间
（loadEmergentScenesLocked 只取 origin='emergent'），多写对聚类零影响。
「多写无影响、少写会缺场景」——默认 auto 保持开，声明项只作为插件
将来确实需要时的闸门。

<small>`plugin.go:99`</small>

### `PluginSDK.SetProxyRegistrar`

```go
func (s *PluginSDK) SetProxyRegistrar(r ProxyRegistrar)
```

SetProxyRegistrar 由内核注入。插件不直接调它（与 SetInputChannelRegistrar 同族）。

<small>`proxy.go:309`</small>

### `ToolError`

```go
type ToolError struct { // Field 是出错的参数字段名（参数校验失败时填）。 Field string `json:"field,omitempty"` // Reason 是机器可读的原因码： …
```

ToolError 描述一次工具调用的失败原因。

存在的理由：失败若只表达为文本，模型无法定位到字段，只能原样重试
（实测 cmd_run 失败率 34%~48%，全部源于同一个成因：参数被截断或
JSON 写坏，工具却只回报 "command is required" 这类与真因无关的错）。

⚠️ 零值语义：插件**不必**改用本类型。内核的失败识别同时兼容既有三种约定
（{"error":…}、{"isError":true,…}、显式 error 返回），见 core.isToolError。
本类型是给**新写**的工具用的可选项，不是迁移要求。

<small>`plugin.go:252`</small>

### `PluginSDK.UnregisterOutputChannel`

!!! warning "仅内核内置插件可用"
    外部插件的桥接模板只注入 SetOutputChannelRegistrar（proc_main.go.tmpl:693-705），**未**注入 SetOutputChannelUnregistrar（grep Unregistrar 在 templates/ 下无命中）。因此外部插件的 regOutputUnreg 为 nil，UnregisterOutputChannel 会命中 `if reg != nil` 的 else 分支**直接返回 nil**（sdk/plugin.go:539-549）——即**静默无效**：不报错、通道也没注销。内置插件由 internal/sdk/plugin.go:330 注入 cfg.RegOutputUnreg，真正生效。外部插件要让通道下线，只能重载插件。

```go
func (s *PluginSDK) UnregisterOutputChannel(name string) error
```

UnregisterOutputChannel 注销一个输出通道（动态通道随资源生灭时必须调用）。

<small>`plugin.go:643`</small>

### `ValidProxyAuth`

```go
func ValidProxyAuth(auth string) bool
```

ValidProxyAuth 校验 Auth 取值；空串合法（等价 ProxyAuthHomeAgent）。

<small>`proxy.go:160`</small>

### `ValidProxyHostLabel`

```go
func ValidProxyHostLabel(label string) bool
```

ValidProxyHostLabel 校验子域名标签是否合法（DNS label 规则）。

独立成导出函数：插件作者在写声明时、HomeAgent 在加载时、工具链在打包时
都要用同一套规则判定，避免三处各写一份而互相不一致。

<small>`proxy.go:180`</small>

### `ValidScenePolicy`

```go
func ValidScenePolicy(policy string) bool
```

ValidScenePolicy 校验场面策略取值；空串等价于 ScenePolicyAuto。

<small>`plugin.go:103`</small>

### `ValidateProxyDef`

```go
func ValidateProxyDef(d ProxyDef) string
```

ValidateProxyDef 校验一条反代声明，返回人类可读的错误说明（合法时为空）。

为什么要在 SDK 里做校验：HomeAgent 加载插件时必须能明确拒绝坏声明并说明
原因（而不是静默忽略导致用户以为配好了）；插件作者也需要在本地就能查出
拼错的 Target/Host。同一套规则两端共用。

<small>`proxy.go:229`</small>

