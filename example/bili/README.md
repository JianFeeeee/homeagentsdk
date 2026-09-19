# bili · B站视频下载

用 [yt-dlp](https://github.com/yt-dlp/yt-dlp) 把 B 站视频下载到本地。

## 前置依赖

需要系统里装有 `yt-dlp`：

```bash
pip install -U yt-dlp     # 或 apt install yt-dlp
```

## 工具

| 工具 | 说明 |
|---|---|
| `bili_video` | 下载 B 站视频；不指定 `format` 时先返回可用清晰度列表，指定后真正下载并返回文件路径 |

参数：

| 参数 | 说明 |
|---|---|
| `url` | 视频地址 |
| `format` | 格式 ID。常用：`30112`/`30080`=1080P、`30064`=720P、`30032`=480P、`30016`=360P。不指定则自动选最优 |

## 配置项

| 键 | 默认 | 说明 |
|---|---|---|
| `output_dir` | `/tmp/bili_videos` | 下载目录 |
| `proxy` | 空 | yt-dlp 使用的 HTTP 代理（如 `http://127.0.0.1:7890`）。留空则不设代理 |

## 实现要点

- **`output_dir` 有安全校验**：它是配置项，但会拒绝被配成系统目录，避免 yt-dlp 往任意位置写文件。
- 两阶段用法：先不传 `format` 拿到清晰度清单（`format_id` + `format_note`），再带上选定的 ID 下载。这样模型不会盲选一个不存在的格式。
- B 站在部分网络环境下需要代理，见上面的 `proxy`。

## 构建

```bash
hmapdev build
```
