<!-- 本页由 tools/apidoc/gensite 从源码生成，请勿手改；要改文档就改 sdk/*.go 的注释。 -->

# 桥接装配点（Bridge）

以下方法**不是给插件业务代码调的**——它们由 `hmapdev` 生成的运行时在启动时调用，用来把内核能力注入到 SDK 实例。列在这里是为了让「公开 API 面」完整，并说明每个注入点对应什么能力。

### `APIRegistrar`

```go
type APIRegistrar func(name string) error
```

APIRegistrar registers a plugin API for external access.

<small>`plugin.go:410`</small>

### `InputChannelRegistrar`

```go
type InputChannelRegistrar func(name string, def ChannelDef) error
```

InputChannelRegistrar registers an input channel with its memory behavior.

<small>`plugin.go:413`</small>

### `OutputChannelRegistrar`

```go
type OutputChannelRegistrar func(name string, caps int, desc string, def ChannelDef, handler ToolHandler) error
```

OutputChannelRegistrar registers an output channel that the output_send tool can use.

<small>`plugin.go:416`</small>

### `OutputChannelUnregistrar`

```go
type OutputChannelUnregistrar func(name string) error
```

OutputChannelUnregistrar 注销一个输出通道。

为什么需要它：输出通道不止有"启动时注册一次"的静态通道，还有**随外部资源生灭**的
动态通道 —— 典型是远程设备：`device/<id>` 只在设备在线期间存在，设备掉线后
必须注销，否则 output_list_channels 会一直列着它、模型会往一个死通道发消息。

<small>`plugin.go:423`</small>

### `PluginSDK.SetDocMemoryAPI`

!!! info "桥接装配点"
    桥接运行时注入点（由 hmapdev 生成的 proc_main 调用，插件业务代码不调用）

```go
func (s *PluginSDK) SetDocMemoryAPI(dm DocMemoryAPI)
```

<small>`plugin.go:718`</small>

### `PluginSDK.SetEventSubscriber`

!!! warning "仅内核内置插件可用"
    同上：无调用点。标 builtin 而非 bridge，因为连桥接运行时都不注入它——外部插件无法用它获得事件订阅能力。

```go
func (s *PluginSDK) SetEventSubscriber(es EventSubscriber)
```

<small>`plugin.go:742`</small>

### `PluginSDK.SetIOInjector`

!!! info "桥接装配点"
    桥接运行时注入点（由 hmapdev 生成的 proc_main 调用，插件业务代码不调用）

```go
func (s *PluginSDK) SetIOInjector(io IOInjector)
```

SetIOInjector sets the IO injector (called by the core at startup).

<small>`plugin.go:699`</small>

### `PluginSDK.SetInputChannelRegistrar`

!!! info "桥接装配点"
    桥接运行时注入点（由 hmapdev 生成的 proc_main 调用，插件业务代码不调用）

```go
func (s *PluginSDK) SetInputChannelRegistrar(r InputChannelRegistrar)
```

SetInputChannelRegistrar sets the input channel registrar (called by the core at startup).

<small>`plugin.go:692`</small>

### `PluginSDK.SetKnowledgeAPI`

!!! info "桥接装配点"
    桥接运行时注入点（由 hmapdev 生成的 proc_main 调用，插件业务代码不调用）

```go
func (s *PluginSDK) SetKnowledgeAPI(kn KnowledgeAPI)
```

<small>`plugin.go:724`</small>

### `PluginSDK.SetLLMAPI`

!!! info "桥接装配点"
    桥接运行时注入点（由 hmapdev 生成的 proc_main 调用，插件业务代码不调用）

```go
func (s *PluginSDK) SetLLMAPI(llm LLMAPI)
```

<small>`plugin.go:730`</small>

### `PluginSDK.SetMemoryAPI`

!!! info "桥接装配点"
    桥接运行时注入点（由 hmapdev 生成的 proc_main 调用，插件业务代码不调用）

```go
func (s *PluginSDK) SetMemoryAPI(mem MemoryAPI)
```

SetMemoryAPI sets the memory API (called by the core at startup).

<small>`plugin.go:706`</small>

### `PluginSDK.SetOutputChannelRegistrar`

!!! info "桥接装配点"
    桥接运行时注入点（由 hmapdev 生成的 proc_main 调用，插件业务代码不调用）

```go
func (s *PluginSDK) SetOutputChannelRegistrar(r OutputChannelRegistrar)
```

SetOutputChannelRegistrar sets the output channel registrar (called by the core at startup).

<small>`plugin.go:678`</small>

### `PluginSDK.SetOutputChannelUnregistrar`

!!! warning "仅内核内置插件可用"
    同上，桥接模板不注入。

```go
func (s *PluginSDK) SetOutputChannelUnregistrar(r OutputChannelUnregistrar)
```

SetOutputChannelUnregistrar sets the output channel unregistrar (called by the core at startup).

<small>`plugin.go:685`</small>

### `PluginSDK.SetPluginMgrAPI`

!!! info "桥接装配点"
    桥接运行时注入点（由 hmapdev 生成的 proc_main 调用，插件业务代码不调用）

```go
func (s *PluginSDK) SetPluginMgrAPI(pm PluginMgrAPI)
```

SetPluginMgrAPI sets the plugin manager API (called by the bridge at startup).

<small>`plugin.go:749`</small>

### `PluginSDK.SetSocialAPI`

!!! info "桥接装配点"
    桥接运行时注入点（由 hmapdev 生成的 proc_main 调用，插件业务代码不调用）

```go
func (s *PluginSDK) SetSocialAPI(social SocialAPI)
```

<small>`plugin.go:736`</small>

### `PluginSDK.SetTextMemoryAPI`

!!! info "桥接装配点"
    桥接运行时注入点（由 hmapdev 生成的 proc_main 调用，插件业务代码不调用）

```go
func (s *PluginSDK) SetTextMemoryAPI(tm TextMemoryAPI)
```

<small>`plugin.go:712`</small>

### `ToolRegistrar`

```go
type ToolRegistrar func(name string, def ToolDef, handler ToolHandler) error
```

ToolRegistrar registers a tool dynamically.

<small>`plugin.go:404`</small>

