# 打包与发布

`hmapdev build` 一次完成编译与打包，产出 `.hmap` 分发包（zip 格式，内含
`plugin.json` 清单 + 二进制）。

## 命令

```bash
hmapdev build                      # 默认 bundle（多平台合集）
hmapdev build --no-bundle          # 只构建 plg.json targets 里的平台
hmapdev build --target linux/arm64 # 在 targets 基础上追加目标
hmapdev build --outdir out         # 指定输出目录（默认 dist）
hmapdev build --sdk-path <path>    # 覆盖 go.mod 的 replace 指向的 SDK
hmapdev build --replace <mod@path> # 追加 go.mod replace（可多次）
```

执行流程：

1. 读 `plg.json` 的 `targets` / `bundle` 决定构建目标
2. 生成子进程运行时代码（`z_proc_gen.go`、`z_proc_shm_*.go`）
3. **Go 插件**：`go build`（普通可执行文件，`CGO_ENABLED=0`）
   **Lua 插件**：直接打包源码，不编译
4. 生成 `plugin.json` 输出清单
5. 打成 `.hmap`

## 两个 JSON 的区别

这一点经常混淆：

| 文件 | 谁维护 | 作用 | 关键字段 |
|---|---|---|---|
| `plg.json` | **你** | 项目元信息，构建输入 | `targets`、`bundle` |
| `plugin.json` | `hmapdev` 自动生成 | 构建产物清单 | `entry`、`platforms` |

`plg.json` 里的 `sdk` 字段声明**本插件针对的 SDK 版本**；未命中本地 SDK 存储
会明确报错（见[环境与工具链](getting-started.md)）。

## 多平台（bundle）

`build` 默认就是 bundle 模式：一次编译 linux/amd64、darwin/amd64、windows/amd64，
产出一个含全部平台二进制的 `.hmap`；安装时内核挑当前平台那份。

```bash
hmapdev build              # → dist/myplugin_bundle.hmap
hmapdev build --no-bundle  # → dist/myplugin_linux_amd64.hmap 等
```

!!! note "bundle 模式会忽略 `plg.json` 的 `targets`"
    固定构建上述三个平台。交叉编译需要对应工具链（如 Linux 上构建 darwin 需要
    clang / macOS SDK），缺工具链时会失败 —— 此时用 `--no-bundle` 只构建当前平台。

bundle 包内按 `plugin.bin.<goos>.<goarch>` 区分，安装时重命名为 `plugin.bin`。

## 产物形态

子进程插件是**普通可执行文件**，不分平台后缀：

| 平台 | 二进制 |
|---|---|
| Linux / macOS / Windows | `plugin.bin` |

!!! warning "v1.0.0 破坏性变更：不再加载 `.so` / `.dll`"
    外部插件从 C ABI 动态库改为**子进程 + 共享内存**。

    - `plugin.so` / `plugin.dylib` / `plugin.dll` **不再被加载**。
      新内核遇到旧产物会跳过并报可操作错误，不崩溃。
    - **业务代码不用改一行** —— 公开 SDK 接口零改动，用新版 `hmapdev`
      （原 `plugindev`）重编即可。
    - `plg.json` 的 `entry` 字段对 Go 插件**已无意义**（写 `plugin.so` 也无妨），
      现在只用于区分 Lua 插件。
    - 产物不再需要 cgo，交叉编译无需目标平台 C 工具链。

## 安装

三种方式（`9876` 是 pluginmgr 的本地端口，默认只监听 `127.0.0.1`、无鉴权）：

```bash
# 从 URL 安装（仅 http/https，流式下载不落盘）
curl -X POST http://127.0.0.1:9876/plugins \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/myplugin.hmap"}'

# 从本地路径安装（读取文件，不移动原文件）
curl -X POST http://127.0.0.1:9876/plugins \
  -H "Content-Type: application/json" \
  -d '{"path": "/path/to/myplugin.hmap"}'

# 直接上传二进制
curl -X POST http://127.0.0.1:9876/plugins \
  --data-binary @dist/myplugin_bundle.hmap
```

安装后调用 `/api/v1/plugins/reload` 或重启内核生效。

走 WebUI 的 HTTP API（默认 `8080`，需 `api_key` 鉴权，内部代理到 pluginmgr）：

```bash
curl -X POST http://127.0.0.1:8080/api/v1/plugins \
  -H "Authorization: Bearer <api_key>" \
  -H "Content-Type: application/json" \
  -d '{"path": "/path/to/myplugin.hmap"}'
```

也可以在 WebUI 的插件管理页面上传。

## 发布前自查

- [ ] `plg.json` 的 `sdk` 版本与目标内核匹配
- [ ] `version` 已递增（内核按版本判断是否需要重装）
- [ ] 若插件有外部状态，`SetAutoRestart(false)` 或在 `Start` 里重建连接
      （崩溃重启是**线性退避** 1s→2s→3s，5 分钟内第 4 次崩溃即停止，
      见[生命周期](../api/lifecycle.md#pluginsdksetautorestart)）
- [ ] `RegisterOnRemoveHandler` 里清理自己写下的数据文件
- [ ] 在 `-race` 下跑一遍：插件的 `Start` 与工具的并发访问是最常见的竞态来源
