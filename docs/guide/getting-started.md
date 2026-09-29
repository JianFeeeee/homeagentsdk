# 环境与工具链

`hmapdev` 是 SDK 仓提供的统一插件开发工具链，Go 与 Lua 两种插件都用它，
最终产出 `.hmap` 插件包（工具名即取自这个包格式）。

!!! note "改名说明"
    1.2.0 起工具链由 `plugindev` 更名为 `hmapdev`；SDK 存储目录同时由
    `~/.homeagent/plugindev/sdk` 迁到 `~/.homeagent/hmapdev/sdk`
    （旧目录会自动继续沿用）。

## 安装

从源码构建：

```bash
git clone https://github.com/JianFeeeee/homeagentsdk
cd homeagentsdk/tools/hmapdev
go build -o hmapdev
# 把 hmapdev 放进 PATH，或直接用 ./hmapdev
```

也可以从 SDK 的 release 附件下载预编译二进制（`hmapdev_linux_amd64` 等，
共 5 个平台：linux/darwin/windows × amd64/arm64）。

> 仓已迁到 GitHub；gitcode 仅作国内镜像（源码同步，**release 附件暂时仍在那里**）：
> `https://gitcode.com/JianFeeeee/homeagent-sdk/releases`。
> Go 模块路径仍是 `gitcode.com/JianFeeeee/homeagent-sdk` —— 这是有意保留的，
> 改模块路径会让现有插件的 `go.mod` 全面失效。

## SDK 版本管理

`hmapdev` 会维护一份本地 SDK 存储，`init` 时按 `plg.json` 里的 `sdk` 字段
选择版本。两者**必须**一致，否则编译出的插件与内核协议可能错配。

```bash
hmapdev sdk list            # 已安装的 SDK 版本
hmapdev sdk current         # 当前使用的版本
hmapdev sdk latest          # 最新可用版本
hmapdev sdk install v1.2.0  # 安装指定版本
hmapdev sdk use v1.2.0      # 切换版本
hmapdev sdk path            # 当前 SDK 路径
```

存储在 `~/.homeagent/hmapdev/sdk/<version>/`。

!!! warning "版本未命中会**明确报错**"
    `plg.json` 声明的 `sdk` 版本若不在本地存储里，`hmapdev` 不会退回某个默认版本，
    而是报错并让你先 `hmapdev sdk install`。这是有意的：静默降级会产出与内核
    协议不匹配的插件，那种失败要到运行时才暴露。

## 源码调试

不编译直接跑插件源码，输出调用轨迹：

```bash
hmapdev debug [dir]   # dir 默认当前目录
```

写 Lua 插件时更简单——`sdk.lua` 是 SDK 模拟层，可以直接用解释器跑：

```bash
lua main.lua
```

## 下一步

- [第一个 Go 插件](first-plugin.md)
- [第一个 Lua 插件](first-lua-plugin.md)
