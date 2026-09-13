# plugindev — 插件开发工具链（Agent 可调用）

把 SDK 的 `hmapdev` 封装成插件，让 **Agent 自己**走完「新建插件 → 构建 → 安装」全流程，
不需要人来敲命令行：

```
plugindev_init    生成工程骨架（等价 hmapdev init <name> [--lua]）
      ↓  改 plugin.go
plugindev_build   构建打包（等价在该目录 hmapdev build）→ dist/*.hmap
      ↓
plugin_install    安装（用 path 指向刚构建出的 .hmap，overwrite=true 表示原地更新）
      ↓
plgreload         重载生效
```

## 工具

| 工具 | 参数 | 说明 |
|---|---|---|
| `plugindev_status` | — | hmapdev 是否可用/版本/当前 SDK 版本与路径/工作区；**排查"为什么不能构建"先用它** |
| `plugindev_init` | `name`、`lang`(go/lua)、`dir` | 生成工程骨架；插件名必须 `[a-zA-Z0-9_-]{1,64}` |
| `plugindev_build` | `dir`、`target` | 在工程目录构建打包；产物路径会在返回里给出 |
| `plugindev_sdk` | `action`、`version`、`from` | SDK 版本管理（list/current/path/latest/install/use）；`from` 可指向本地 SDK 源码 |
| `plugindev_projects` | — | 列出工作区里已有工程与产物 |

## 配置

| 键 | 默认 | 说明 |
|---|---|---|
| `hmapdev_path` | 自动查找 | 依次尝试：本配置项 → PATH → `/usr/local/bin/hmapdev` → `/root/go/bin/hmapdev` |
| `workspace_dir` | `<data_dir>/plugindev` | `plugindev_init` 生成工程的默认目录 |
| `build_timeout_sec` | 600 | 单次 hmapdev 调用超时 |

## 前置：装 hmapdev

```bash
cd <sdk-repo>/tools/hmapdev && go build -buildvcs=false -o /usr/local/bin/hmapdev .
hmapdev version
```

## 安全边界（都在实现里，不只写在文档里）

- 只 exec **hmapdev 一个可执行文件**，不接受任意命令、不做 shell 拼接；
- `plugindev_build` 只接受含 `plg.json` 的目录（"看起来是插件工程"才构建），
  避免把这个工具变成对任意目录跑构建；
- 子进程全部带超时，输出**截断**后才返回（构建日志动辄几百 KB，直接回灌会撑爆模型上下文）；
- 工程名约束与内核/上游对"进工具名的标识符"的规则一致（`[a-zA-Z0-9_-]{1,64}`）。
