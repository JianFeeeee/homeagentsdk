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

<small>`plugin.go:279`</small>

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

<small>`plugin.go:230`</small>

### `IOInjector.InjectInputMediaOpts`

```go
InjectInputMediaOpts(source, channel, text string, blocks []ContentBlock, opts InjectOptions)
```

<small>`plugin.go:241`</small>

### `IOInjector.InjectInputMediaSync`

```go
InjectInputMediaSync(source, channel, text string, blocks []ContentBlock) string
```

<small>`plugin.go:231`</small>

### `IOInjector.InjectInputMediaSyncOpts`

```go
InjectInputMediaSyncOpts(source, channel, text string, blocks []ContentBlock, opts InjectOptions) string
```

<small>`plugin.go:242`</small>

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

<small>`plugin.go:226`</small>

### `IOInjector.InjectInputSyncOpts`

```go
InjectInputSyncOpts(source, channel, text string, opts InjectOptions) string
```

<small>`plugin.go:240`</small>

### `IOInjector.InjectInterruptMedia`

```go
InjectInterruptMedia(source, channel, text string, blocks []ContentBlock)
```

<small>`plugin.go:232`</small>

### `IOInjector.InjectInterruptMediaOpts`

```go
InjectInterruptMediaOpts(source, channel, text string, blocks []ContentBlock, opts InjectOptions)
```

<small>`plugin.go:243`</small>

### `IOInjector.InjectInterruptText`

```go
InjectInterruptText(source, channel, text string)
```

<small>`plugin.go:221`</small>

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

<small>`plugin.go:239`</small>

### `IOInjector.InjectText`

```go
InjectText(source, channel, text string)
```

<small>`plugin.go:222`</small>

### `IOInjector.InjectTextNoMemory`

```go
InjectTextNoMemory(source, channel, text string)
```

**示例插件里的真实用法**

| 插件 | 位置 | 代码 |
|---|---|---|
| [`browser`](../examples/index.md#browser) | `example/browser/plugin.go:1114` | `p.sdk.InjectTextNoMemory(p.name, p.name, fmt.Sprintf("[浏览器 %s 已导航到 %s]", id, rawURL))` |

<small>`plugin.go:223`</small>

### `IOInjector.InjectTextOpts`

```go
InjectTextOpts(source, channel, text string, opts InjectOptions)
```

以下 Opts 变体让调用点在**这一次注入**上声明记忆与裁剪行为。

上面那些不带 opts 的方法等价于传零值 InjectOptions（记入记忆 + 不裁剪），
保留它们是为了不破坏已有插件；新代码应当用 Opts 变体把意图写清楚。

<small>`plugin.go:238`</small>

### `IOInjector.SetToolBlocks`

```go
SetToolBlocks(blocks []ContentBlock)
```

SetToolBlocks 插件工具注入多模态内容块（image_url/audio_url），内核在下一条
tool message 的 content 数组里带上这些块，让模型在后续轮次看到图/听到音频。

<small>`plugin.go:229`</small>

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

<small>`plugin.go:290`</small>

### `PluginMgrAPI.ListLoadedPlugins`

```go
ListLoadedPlugins() []string
```

ListLoadedPlugins 列出已加载插件。

<small>`plugin.go:288`</small>

### `PluginMgrAPI.ReloadOne`

```go
ReloadOne(name string) error
```

ReloadOne 重载单个插件（停止后重新加载）。

<small>`plugin.go:286`</small>

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

<small>`plugin.go:862`</small>

### `ImageURL`

```go
type ImageURL struct { URL string `json:"url"` Detail string `json:"detail,omitempty"` }
```

<small>`plugin.go:857`</small>

### `MemItem`

```go
type MemItem struct { Role string `json:"role"` Content string `json:"content"` Score float64 `json:"score"` }
```

MemItem represents a memory item in stage context.

<small>`plugin.go:179`</small>

### `PluginSDK`

```go
type PluginSDK struct { name string regTool ToolRegistrar regStage StageRegistrar regAPI APIRegistrar regOutput OutputChannelRegistrar …
```

PluginSDK is the main API surface provided to plugins at runtime.
It wraps tool registration, settings, memory, knowledge, LLM, and IO injection.

<small>`plugin.go:337`</small>

### `SDKVersion`

```go
var SDKVersion
```

SDKVersion 是对外暴露的 SDK 版本号。

<small>`plugin.go:10`</small>

### `PluginSDK.UnregisterOutputChannel`

!!! warning "仅内核内置插件可用"
    外部插件的桥接模板只注入 SetOutputChannelRegistrar（proc_main.go.tmpl:693-705），**未**注入 SetOutputChannelUnregistrar（grep Unregistrar 在 templates/ 下无命中）。因此外部插件的 regOutputUnreg 为 nil，UnregisterOutputChannel 会命中 `if reg != nil` 的 else 分支**直接返回 nil**（sdk/plugin.go:539-549）——即**静默无效**：不报错、通道也没注销。内置插件由 internal/sdk/plugin.go:330 注入 cfg.RegOutputUnreg，真正生效。外部插件要让通道下线，只能重载插件。

```go
func (s *PluginSDK) UnregisterOutputChannel(name string) error
```

UnregisterOutputChannel 注销一个输出通道（动态通道随资源生灭时必须调用）。

<small>`plugin.go:539`</small>

