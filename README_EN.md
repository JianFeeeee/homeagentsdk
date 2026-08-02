# HomeAgent SDK

Plugin development SDK for building intelligent plugins that interact with the HomeAgent platform.

## SDK API Surface

### Plugin Interface

Plugins implement the `Plugin` interface:

```go
type Plugin interface {
    Name() string
    Start(sdk *PluginSDK) error
    Stop() error
}
```

### PluginSDK Methods

The SDK instance injected via `Start(sdk *PluginSDK)` provides:

| Category | Method | Description |
|----------|--------|-------------|
| Stage Hooks | `RegisterStage(stage, handler, scope...)` | Register stage callback; scope: `StageScopeGlobal` (all, default) or `StageScopeOwnTools` (own tools only) |
| Input Channel | `RegisterInputChannel(name, def)` | Register input channel with `ChannelDef` (NoMemory/Cleaner) |
| Output Channel | `RegisterOutputChannel(name, caps, desc, def, handler)` | Register output channel with `ChannelDef` and capability bitmask |
| Tool Registration | `RegisterTool(name, def, handler)` | Register a tool for LLM invocation |
| Plugin API | `RegisterPluginAPI(name)` | Register plugin API for inter-plugin access |
| Graph Memory | `Memory()` | Access graph memory API (entity-relation store) |
| Text Memory | `TextMemory()` | Access text memory API (chronological events) |
| Doc Memory | `DocMemory()` | Access document memory API (vector store) |
| Social Graph | `Social()` | Access social graph API (read-only for external plugins) |
| Knowledge | `Knowledge()` | Access knowledge base API |
| LLM | `LLM()` | Access LLM provider manager API |
| Settings | `Settings()` | Access settings API |
| Events | `Events()` | Access event subscriber (subscribe-only for external plugins) |
| Inject | `InjectText(source, channel, text)` / `InjectInterruptText(source, channel, text)` / `InjectTextNoMemory(source, channel, text)` | Inject text into the agent pipeline |
| Auto-Restart | `SetAutoRestart(enabled)` / `AutoRestart()` | Control automatic restart on crash |

### Stage Hooks

```go
// Listen to all stage events globally
sdk.RegisterStage(StagePreAction, func(ctx *StageContext) error { return nil })

// Listen only to this plugin's own tool calls (before_toolcall / after_toolcall only)
sdk.RegisterStage(StageBeforeToolcall, myHandler, StageScopeOwnTools)
```

### ChannelDef

```go
type ChannelDef struct {
    NoMemory bool              // Channel input/output skips memory computation (vector/keyword/distill), original text preserved
    Cleaner  func(string) string // Optional: computation layer filter (does not modify original text)
}
```

`ChannelDef` controls channel behavior in the memory computation layer, with the same semantics as `ToolDef.NoMemory`/`Cleaner`.

### Input Channels

```go
sdk.RegisterInputChannel("qq", ChannelDef{
    NoMemory: true,
    Cleaner:  func(text string) string { return strings.TrimSpace(text) },
})
```

### Output Channels

```go
sdk.RegisterOutputChannel("my-channel", CapText|CapFile, "channel description", ChannelDef{}, handler)
```

The handler receives three arguments:
- `payload` (string) — message content. For `type=text` it's plain text, for `type=file/image` it's a URL
- `meta` (string) — optional JSON routing metadata (e.g. `{"group_id":123,"user_id":456}`)
- `type` (string) — content type enum (see below)

Capability flags:

| Flag | Value | Description |
|------|-------|-------------|
| `CapText` | 1 | Plain text output |
| `CapFile` | 2 | File output |
| `CapImage` | 4 | Image output |
| `CapAudio` | 8 | Audio output |
| `CapStructured` | 16 | Structured data output |

Type enum values:

| Value | Description |
|-------|-------------|
| `text` | plain text |
| `voice` / `audio` | audio/voice |
| `image` | image |
| `file` | file |

### IOInjector Channel Routing

| Method | Description |
|--------|-------------|
| `InjectText(source, channel, text)` | Inject text, record to memory, route to specified channel |
| `InjectInterruptText(source, channel, text)` | Inject interrupt text, interrupt current processing, route to specified channel |
| `InjectTextNoMemory(source, channel, text)` | Inject text without memory recording, route to specified channel |

`source` identifies the origin, `channel` specifies the target output channel.

### Triple Extended Fields

The Triple data structure includes additional fields:

- `Confidence` — confidence score (0.0–1.0)
- `SubjectType` — subject type
- `ObjectType` — object type

### ToolDef Field Reference

The `def` parameter of `RegisterTool` is of type `sdk.ToolDef`, with the following fields:

| Field | Type | Description |
|-------|------|-------------|
| `Name` | `string` | Tool name, use plugin name prefix to avoid conflicts |
| `Description` | `string` | Tool description, LLM uses this for tool selection |
| `Parameters` | `map[string]interface{}` | JSON Schema parameter definition |
| `NoMemory` | `bool` | Default `false`; when `true`, output skips vector/jieba/distill computation (original text preserved) |
| `Cleaner` | `func(string) string` | Optional, filters output before computation layer (e.g., extract `.content` from JSON) |

For detailed design rationale of `NoMemory` and `Cleaner`, see `docs/en/PLUGIN_DEV.md` in the core repository.

### New Constructor

`New()` is called by the kernel when loading a plugin. Plugin developers do not need to construct PluginSDK manually:

```go
func New(name string, sett SettingsAPI, regTool ToolRegistrar, regStage StageRegistrar, regAPI APIRegistrar, regOutput OutputChannelRegistrar) *PluginSDK
```

Plugin developers only need to implement the `Plugin` interface and export a `NewPlugin()` entry function.

## plugindev Toolchain

`plugindev` provides full development workflow support:

| Command | Description |
|---------|-------------|
| `plugindev init` | Initialize plugin project (generates plg.json, entry template) |
| `plugindev build` | Build plugin, output .hmap package |
| `plugindev clean` | Clean build artifacts |
| `plugindev debug` | Run plugin in local debug mode |

Supports both **Go** and **Lua** plugin languages.

### plg.json Manifest Format

```json
{
  "name": "weather",
  "name_zh": "天气查询",
  "name_en": "Weather",
  "version": "1.0.0",
  "description": "Weather plugin",
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

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Plugin identifier |
| `name_zh` | string | Chinese name |
| `name_en` | string | English name |
| `version` | string | Version |
| `description` | string | Plugin description |
| `author` | string | Author |
| `entry` | string | Entry file (`plugin.so` / `main.lua`) |
| `tags` | string[] | Tags |
| `targets` | string | Build targets, comma-separated (e.g. `linux/amd64,windows/amd64`) |
| `outdir` | string | Output directory (default `dist`) |
| `bundle` | bool | Bundle mode (build all platforms at once) |
| `replaces` | object | Go module replacements, key=module path, value=local path |
| `source_dirs` | string[] | Additional source search paths (auto-imported at build time) |

### .hmap Package Format

`.hmap` is a ZIP archive containing:

- `plugin.json` — plugin metadata
- `plugin.so` — Go compiled artifact (Linux)
- `plugin.dll` — Go compiled artifact (Windows)
- `main.lua` — Lua plugin entry (for Lua plugins)

## Plugin Lifecycle

### Start & Stop

- `Start(sdk *PluginSDK) error` — Plugin startup, receives SDK instance
- `Stop() error` — Plugin shutdown, release resources
- `sdk.RegisterStopHandler(fn func())` — Register a shutdown cleanup callback. The kernel (for built-in plugins) or z_bridge (for external plugins) runs all registered handlers **before** calling the plugin's `Stop()` (LIFO order, cleared after running — idempotent). Use it for persistence and cancelling background work: plugin memory is still fresh at that point, avoiding stale-state write-backs that resurrect deleted data.

### Remove Cleanup (onRemove)

`Stop` / `RegisterStopHandler` run whenever the plugin **stops** (including reload and disable); `RegisterOnRemoveHandler` runs **only once when the plugin is uninstalled (removed)** — never on reload or disable:

- `sdk.RegisterOnRemoveHandler(fn func())` — Register a remove cleanup callback. The kernel runs it **after** the plugin's `Stop()` in the `RemovePlugin` flow (LIFO order, cleared after running — idempotent). Use it to delete persistent files the plugin created itself (data/cache/state files).
- The kernel also cleans up on uninstall: tool registrations, the `disabled_plugins` record, the plugin's config definitions (`plugin.<name>.*`) and its config table (`config_<name>`) — the plugin's config section disappears completely after removal.
- Examples: `example/calendar` (removes events.json), `example/memo` (removes memos.json), `example/rss` (removes the subscription data dir), `example/weather` (removes the cache dir); the `plugindev` template includes an onRemove demo.

```go
sdk.RegisterOnRemoveHandler(func() {
    os.Remove(filepath.Join(dataDir, "events.json"))
})
```

### Auto-Restart

```go
sdk.SetAutoRestart(true)
// Query state
enabled := sdk.AutoRestart()
```

The platform automatically restarts the plugin on crash, ensuring service availability.

## Restricted SDK vs Full SDK

External plugins (third-party distribution) use a **restricted SDK** that only exposes a safe subset:

| Restricted API | Allowed Operations |
|----------------|-------------------|
| `SocialAPI` | Read-only: `GetPerson`, `GetTrait`, `GetRelations`, `GetNetwork`, `ListPersons` |
| `EventSubscriber` | Subscribe-only: `Subscribe` (no `Publish`) |

Internal plugins (platform built-in) have full SDK access including SocialAPI write operations and EventPublisher.

## Example Plugins

| Plugin | Type | Description |
|--------|------|-------------|
| [weather](example/weather) | Go | Weather queries (wttr.in); demonstrates NoMemory/Cleaner/stage hooks/channels/text memory |
| [luademo](example/luademo) | Lua | Full-featured Lua example covering the whole v0.8.0 Lua SDK surface |
| [qq](example/qq) | Go | QQ messaging integration (NapCat), 17 tools, full input/output channel wiring |
| [a2a](example/a2a) | Go | Agent-to-Agent protocol communication |
| [ai_image](example/ai_image) | Go | AI image generation |
| [bili](example/bili) | Go | Bilibili video downloading |
| [browser](example/browser) | Go | Web search, page fetching, browser rendering |
| [calendar](example/calendar) | Go | Calendar management |
| [editdoc](example/editdoc) | Go | Document editing |
| [files](example/files) | Go | File management |
| [memo](example/memo) | Go | Memos (PreAction injection + scheduled reminders) |
| [music](example/music) | Go | Music playback |
| [ocr](example/ocr) | Go | Optical character recognition |
| [rss](example/rss) | Go | RSS subscriptions |
| [sanitizer](example/sanitizer) | Go | Content sanitization / safety filtering |

## Building & Installing

### Build

```bash
plugindev build
```

Outputs a `.hmap` package to the `dist/` directory (default is the multi-platform bundle; use `plugindev build --no-bundle` for a single-target build).

### Install

Via the pluginmgr HTTP API (default port 9876, listening on 127.0.0.1 only, no auth):

```bash
# Local path
curl -X POST http://127.0.0.1:9876/plugins \
  -H "Content-Type: application/json" \
  -d '{"path": "/path/to/my-plugin.hmap"}'

# Upload binary directly
curl -X POST http://127.0.0.1:9876/plugins \
  --data-binary @dist/my-plugin.hmap
```

Or upload via the WebUI plugin management page, or manually place the `.hmap` in the plugin directory and restart the platform.
