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
| Media inject | `InjectInputMedia(source, channel, text, blocks)` / `InjectInputMediaSync(...)` / `InjectInterruptMedia(...)` | Inject input carrying images/audio (added in 1.1.0) |
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

### Multimodal Injection (added in 1.1.0)

| Method | Description |
|--------|-------------|
| `InjectInputMedia(source, channel, text, blocks)` | Inject media-bearing input, asynchronous |
| `InjectInputMediaSync(source, channel, text, blocks)` | Inject media-bearing input and wait for the reply text |
| `InjectInterruptMedia(source, channel, text, blocks)` | Inject a media-bearing interrupt that can preempt current processing |

`blocks` is `[]sdk.ContentBlock`, the same type `SetToolBlocks` takes:

```go
s.InjectInputMedia("myplugin", "webui", "take a look at this", []sdk.ContentBlock{{
    Type:     "image_url",
    ImageURL: &sdk.ImageURL{URL: "data:image/png;base64," + b64, Detail: "auto"},
}})
```

How this differs from `SetToolBlocks`: that one is only callable inside a tool handler and
its media reaches the model with the *next* tool message. These three let a plugin
**initiate a turn that carries media** — the media goes out with this turn's message and is
automatically stored in the media store with a memory reference attached.

`data:` URLs in the blocks are stored and deduplicated by the kernel; `http(s)` URLs are
passed to the model only and never stored (storing them would require the kernel to make
network requests, bringing timeouts, auth and SSRF into scope).

`source` identifies the origin, `channel` specifies the target output channel.

### Triple Extended Fields

The Triple data structure includes additional fields:

- `Confidence` — confidence score (0.0–1.0)
- `SubjectType` — subject type
- `ObjectType` — object type
- `SentenceText` — the original sentence (added in 1.1.0), written to the `sentences` table; media references hang off the sentence
- `MediaDigests` — associated media digests (added in 1.1.0)

### Media in Memory (added in 1.1.0)

Inside plain-text memory, media is represented as a **marker** of the form
`[<mime> <short digest>] <description>`:

```
[image/png a1b2c3d4e5f6] a purple-blue-red three-band chart
```

The description is the durable semantic memory (retrieval uses it); the digest is the key
back to the bytes (reverse lookup uses it). Markers are generated by the kernel — a plugin
never has to assemble one, it just **supplies the digest**.

#### Graph memory

```go
s.Memory().Commit([]sdk.Triple{{
    Subject: "palette", Relation: "contains", Object: "three-band",
    MediaDigests: []string{"a1b2c3d4e5f6"}, // short digest is fine, the kernel resolves it
}})
```

With no `SentenceText`, the kernel uses the marker itself as the sentence — media must have
a sentence to hang off, otherwise the reference has nowhere to attach.

#### Knowledge base

```go
s.DocMemory().InsertWithMedia(&sdk.Doc{
    Title:   "illustrated note",
    Content: "body",
}, []sdk.MediaAttachment{
    {MIME: "image/png", Data: pngBytes, Name: "chart.png"}, // new content, stored and deduped
    {Digest: "a1b2c3d4e5f6"},                               // reference existing content
})
```

`Insert` keeps its original signature; markers already present in the body are bound as
document-level references too. `Query` fills `MediaDigests` and `Attachments` (mime plus
description, **no bytes** — one query can match dozens of media items). Removing a document
releases its references.

#### Text memory

```go
s.TextMemory().Append(sdk.TextEvent{
    Role: "user", Content: "look at this",
    Attachments: []sdk.MediaAttachment{{MIME: "image/png", Data: pngBytes}},
})
```

`RecentEvents` decodes markers in the body back into `Attachments`.

The media store can be disabled kernel-side (`core.memory.media.enabled=false`); all of the
above then degrades to plain-text behaviour — no errors, no panics, identical to how it
behaved before this feature shipped.

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

`plugindev` provides full development workflow support. Prebuilt binaries ship as **release assets**
(linux/darwin/windows × amd64/arm64); download from
[Releases](https://gitcode.com/JianFeeeee/homeagent-sdk/releases) and put it on your PATH:

```bash
# From release assets (v1.0.0 / linux amd64 shown)
curl -Lo plugindev https://gitcode.com/JianFeeeee/homeagent-sdk/releases/download/v1.0.0/plugindev_linux_amd64
chmod +x plugindev

# Or build from source
cd tools/plugindev && go build -o plugindev .
```

> Binaries no longer ship inside the repository (the old `bin/` directory is retired): five
> platforms at 26-28MB each piled another copy into git history on every rebuild, and they are
> reproducible from source anyway.

| Command | Description |
|---------|-------------|
| `plugindev init <name> [--lua]` | Initialize plugin project (generates plg.json, plugin.go or main.lua, go.mod, README.md) |
| `plugindev build [flags]` | Build and package into a `.hmap` (supports cross-compilation and bundle mode) |
| `plugindev clean` | Clean `build/` and `dist/` plus generated files |
| `plugindev debug [dir]` | Load plugin source through the Yaegi Go interpreter and start an interactive REPL |
| `plugindev sdk <command>` | SDK version management (list/install/use/path/current/latest) |

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

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Plugin identifier |
| `name_zh` | string | Chinese name |
| `name_en` | string | English name |
| `version` | string | Version |
| `description` | string | Plugin description |
| `author` | string | Author |
| `entry` | string | Entry file (`plugin.bin` / `main.lua`). Since v1.0.0 Go plugins uniformly build to `plugin.bin`—no per-platform suffix |
| `tags` | string[] | Tags |
| `targets` | string | Build targets, comma-separated (e.g. `linux/amd64,windows/amd64`) |
| `outdir` | string | Output directory (default `dist`) |
| `bundle` | bool | Bundle mode (build all platforms at once) |
| `replaces` | object | Go module replacements, key=module path, value=local path |
| `source_dirs` | string[] | Additional source search paths (auto-imported at build time) |

### .hmap Package Format

`.hmap` is a ZIP archive containing:

- `plugin.json` — plugin metadata
- `plugin.bin` — Go compiled artifact (single-platform build)
- `plugin.bin.<goos>.<goarch>` — one per platform in bundle mode; on install pluginmgr picks
  the one matching the current platform and renames it to `plugin.bin`
- `main.lua` — Lua plugin entry (for Lua plugins)

> Since v1.0.0 `plugin.so`/`plugin.dll`/`plugin.dylib` are no longer used—the process boundary
> *is* the ABI boundary, so there is no platform-specific shared-library distinction. The new
> kernel will not load old artifacts; it emits an explicit rebuild hint instead.

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

## Remote Device SDK

A C language SDK for developing **remote device access adapters** with zero external dependencies, compatible with embedded platforms.

### Architecture

```
┌─────────────────────────────────────────────────┐
│            ha_remotedevice (C SDK)              │
│  Protocol Engine │ WS Frames │ JSON │ State     │
│  Machine │ Transport Abstraction                │
└──────────┬──────────────────────────────────────┘
           │  Same C code, shared by device & app
    ┌──────┴──────────────────┐
    ▼                         ▼
┌──────────────┐    ┌──────────────────────────┐
│  ESP32 Bare   │    │  Linux App                │
│  Pure C       │    │  (Python ctypes / Go CGo /│
│  Simple Cmd   │    │   Node addon / C# P/Invoke)│
└──────────────┘    └──────────────────────────┘
```

### Declarative API Design

The device declares **what it is** and **what it can do** in code. The SDK handles all protocol details automatically:

```c
#include "ha_remotedevice.h"

/* Declare capabilities */
const char *caps[] = {"camera", "status", NULL};

ha_config_t config = {
    .transport = my_transport,     // User implements 4 functions
    .server    = "192.168.1.100:9890",
    .token     = "my-token",
    .device = {
        .device_id = "esp32-cam-1",
        .name      = "Front Door Camera",
        .kind      = "camera",
        .caps      = caps,
    },
    .on_cmd    = my_cmd_handler,   // Called when receiving commands
    .on_binary = my_data_handler,  // Called on binary data (TTS audio, etc.)
    .on_state  = my_state_handler, // Connection state changes
};

ha_client_t *client = ha_client_new(&config);
ha_client_start(client);
while (1) {
    ha_client_process(client);     // Main loop processing
}
```

### Transport Layer Abstraction

Users only need to implement 4 functions to adapt to different platforms:

```c
ha_transport_t my_transport = {
    .connect = my_tcp_connect,   // Establish TCP connection
    .send    = my_tcp_send,      // Send data
    .recv    = my_tcp_recv,      // Receive data (blocking)
    .close   = my_tcp_close,     // Close connection
    .ctx     = &my_platform_ctx,
};
```

### Protocol Support

| Feature | API |
|---------|-----|
| WS connection + handshake | Automatic via `ha_client_start` |
| Device registration (hello/bind) | Automatic on startup |
| Command receive (shell/homeagent) | `on_cmd` callback |
| Command result | `ha_client_send_result` |
| Binary chunked transfer (video) | `ha_client_send_data_chunked` |
| TTS audio receive | `on_binary` callback |
| Event reporting | `ha_client_send_event` |
| Status reporting | `ha_client_send_status` |
| Heartbeat keepalive | Automatic ping/pong |

### Usage

Initialize a project via the `plugindev` toolchain:

```bash
plugindev init my-adapter --type remotedevice
```

Generates `main.c` + `CMakeLists.txt`, can be built directly or used as a third-party library:

```cmake
add_subdirectory(path/to/ha_remotedevice)
target_link_libraries(my_app ha_remotedevice)
target_include_directories(my_app PRIVATE ${HA_REMOTEDEVICE_INCLUDE_DIR})
```

### Quick Start Guide

A complete step-by-step guide from zero to a device successfully connected to HomeAgent.

#### Step 1: Preparation

Create an access token on the HomeAgent platform:

```bash
# Create a device access token on the HomeAgent server
curl -X POST http://<homeagent-server>:8080/api/v1/device/token \
  -H "Content-Type: application/json" \
  -d '{"device_id":"esp32-cam-1","name":"Front Door Camera","kind":"camera"}'
# Returns: {"token":"ha-dev-token-xxxxx"}
```

Save the returned `token` — you'll need it in the device configuration.

#### Step 2: Implement the Transport Layer (4 functions)

Implement the 4 function pointers of `ha_transport_t` for your platform. Here are common scenarios:

**Scenario A: Embedded device with TCP/IP stack (e.g., ESP32 + lwIP)**

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

**Scenario B: Serial (UART) passthrough module**

```c
static int uart_connect(void *ctx, const char *host, uint16_t port) {
    (void)host; (void)port;
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

> Note: For UART passthrough, a TCP bridge program must run on the other end to forward serial data to the HomeAgent WebSocket port.

#### Step 3: Declare Device Capabilities and Command Handlers

```c
#include "ha_remotedevice.h"

/* Declare device capabilities */
const char *caps[] = {"camera", "speaker", "status", NULL};

/* Handle camerasue command (take photo) */
static ha_status_t handle_camera(const char *req_id, const char *args,
                                 ha_cmd_result_t *result, void *userdata) {
    (void)req_id; (void)userdata;
    int duration = args[0] ? atoi(args) : 0;

    // Capture image, fill the result
    result->status = 0;
    result->output = "data:image/jpeg;base64,/9j/4AAQ...";  // base64 image data
    return HA_OK;
}

/* Handle shell command */
static ha_status_t handle_shell(const char *req_id, const char *args,
                                ha_cmd_result_t *result, void *userdata) {
    (void)req_id; (void)userdata;
    result->status = 0;
    result->output = "command executed";
    return HA_OK;
}

/* Declarative command handler table */
ha_cmd_handler_def_t handlers[] = {
    {.command = "shell",      .handler = handle_shell},
    {.command = "camerasue",  .handler = handle_camera},
    {.command = "screensee",  .handler = handle_camera},
    {.command = "speakeruse", .handler = handle_speaker},
    {.command = NULL},  /* terminator */
};
```

#### Step 4: Configure and Start the Client

```c
ha_config_t config = {
    .transport = transport,                 // Transport layer implementation
    .server    = "192.168.1.100:9890",      // HomeAgent server address
    .token     = "ha-dev-token-xxxxx",      // Token from Step 1
    .device = {
        .device_id = "esp32-cam-1",
        .name      = "Front Door Camera",
        .kind      = "camera",
        .caps      = caps,
        .info_json = "{\"chip\":\"ESP32-S3\",\"firmware\":\"v1.0\"}",
    },
    .handlers  = handlers,                  // Command handler table
    .on_binary = on_binary_data,            // Receive TTS audio etc.
    .on_state  = on_state_change,           // Connection state callback
    .ping_interval = 30,
};

ha_client_t *client = ha_client_new(&config);
ha_status_t ret = ha_client_start(client);
if (ret != HA_OK) {
    printf("Device connection failed: %d\n", ret);
    return;
}

/* Main loop */
while (1) {
    ha_client_process(client);  // Process protocol frames, heartbeats, commands

    /* Optional: device-initiated event reporting */
    ha_client_send_event(client, "motion_detected",
                         "{\"zone\":\"front_door\",\"confidence\":0.95}");

    /* Optional: report device status */
    ha_client_send_status(client, "online");

    vTaskDelay(100 / portTICK_PERIOD_MS);  // RTOS-style delay
}
```

#### Step 5: Verify the Connection

Check if the device is online on the HomeAgent server:

```bash
# List registered devices
curl http://<homeagent-server>:8080/api/v1/device/list
# Expected output includes: {"device_id":"esp32-cam-1","status":"online",...}

# Send a command to the device (test camerasue)
curl -X POST http://<homeagent-server>:8080/api/v1/device/esp32-cam-1/cmd \
  -H "Content-Type: application/json" \
  -d '{"cmd":"camerasue","args":"3"}'
# Expected: {"status":"ok","result":"data:image/jpeg;base64,..."}
```

#### Step 6: Debugging Tips

| Issue | Check |
|-------|-------|
| Connection failed | Verify `server` address and port are reachable; check `token` |
| WS handshake failed | Verify HomeAgent server WebSocket support is enabled |
| Command not responding | Confirm the command name is registered in `handlers` table; check `on_binary` |
| Reconnection issues | `max_reconnect` controls retry count; -1 = infinite |
| Low memory (embedded) | Define `HA_NO_ALLOC` to disable dynamic memory allocation |

### Location

- **SDK Source**: `remotedevice/`
- **plugindev template**: `plugindev init --type remotedevice`

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
