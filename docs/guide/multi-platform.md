# 多平台构建

## 默认就是多平台

`hmapdev build` 默认 bundle 模式，一次产出含三个平台的单个 `.hmap`：

```
dist/myplugin_bundle.hmap
  └── plugin.bin.linux.amd64
  └── plugin.bin.darwin.amd64
  └── plugin.bin.windows.amd64
```

安装时内核挑当前平台那份，重命名为 `plugin.bin`。

## 逐平台构建

```bash
hmapdev build --no-bundle        # 按 plg.json 的 targets 构建
hmapdev build --target linux/arm64   # 追加一个目标
```

`plg.json` 里声明目标：

```json
{
  "name": "myplugin",
  "version": "1.0.0",
  "targets": "linux/amd64,windows/amd64"
}
```

单平台输出文件名：`{name}_{os}_{arch}.hmap`。

## 交叉编译

子进程插件**不再需要 cgo**，所以交叉编译不需要目标平台的 C 工具链 ——
这是 v1.0.0 的收益之一。

!!! note "bundle 模式忽略 `targets`"
    固定构建 linux/amd64、darwin/amd64、windows/amd64。如果你只需要其中一个，
    用 `--no-bundle` 更快。

## 平台能力差异

历史上有过一处真实的平台断层，现已消除：

- **v1.0.0 之前**：Windows 上插件只看到 **3 个 stage 字段、且无法写回**。
- **v1.0.0 起**：Windows 与其他平台**共用同一套 RPC 实现**，16 字段全可见 + 写回。

因此**不必**为 Windows 写条件分支 —— 除非你的插件自己用了平台专有的外部命令。

## 下一步

- [打包与发布](packaging.md)
- [环境与工具链](getting-started.md)
