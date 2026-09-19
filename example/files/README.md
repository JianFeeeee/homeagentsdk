# files · 沙箱文件操作

读写与编辑文件，**全部操作限制在沙箱目录内**。

## 工具

| 工具 | 说明 |
|---|---|
| `files_read` | 读文件内容，支持 `offset` / `limit` 读大文件 |
| `files_write` | 写文件，**自动创建父目录** |
| `files_edit` | 按精确字符串替换改文件 |
| `files_ls` | 列目录（目录名带 `/` 后缀） |

`files_edit` 用 `edits[]` 传多组替换，每组 `{old, new}`：

- 每个 `old` 必须在**原文件**中**恰好出现一次** —— 不唯一会报错，避免改错地方。
- 所有替换都针对**原内容**匹配，不要在同一个 `edits` 里写相互重叠的改动。

## 配置项

| 键 | 默认 | 说明 |
|---|---|---|
| `dir` | 空 | 允许访问的根目录。留空用默认沙箱（主数据目录下的 `files_sandbox`）。**不建议设为 `/`** |

## 沙箱实现

路径校验不止一次，是两道：

1. **规范化后判断**：`filepath.Abs` + `filepath.Clean`，再用 `withinSandbox`
   检查结果是否在根目录之下（`/` 作为特例放行）。
2. **解析符号链接后再判断**：`filepath.EvalSymlinks` 求出真实路径，**再查一次**沙箱。

第 2 步是关键：只做第 1 步的话，沙箱内一个指向外部的软链接就能绕过限制
（`.../sandbox/link -> /etc`）。报错文案也区分了这两种情况
（`path outside sandbox` vs `path escapes sandbox via symlink`）。

对不存在的路径（`write` 会用到），求真实路径时只对已存在的部分做 `EvalSymlinks`，
其余保留为未创建的尾部。

## 构建

```bash
hmapdev build
```
