# ai_image · 文生图

按文字提示生成图片，下载到本地并返回**文件路径**。

## 工具

| 工具 | 说明 |
|---|---|
| `ai_image_generate` | 按 prompt 生成图片 |

返回值是**本地文件路径**（永久，不过期）。要把图给用户看，再用导出的通道
以 `type=image`、`payload=<该路径>` 发送。

## 配置项

| 键 | 默认 | 说明 |
|---|---|---|
| `api_key` | 空 | OpenAI / Stable Diffusion 的 API Key |
| `base_url` | 空 | 自定义 OpenAI 兼容网关（**不带 `/v1` 尾缀**，如 `http://127.0.0.1:8081`）。留空走官方 `https://api.openai.com` |
| `provider` | `openai` | 服务方：`openai` / `stability` |
| `model` | `dall-e-3` | 模型名（如 `dall-e-3`、`sd-xl`） |
| `size` | `1024x1024` | 默认尺寸，也可 `1024x1792` / `1792x1024` |

## 实现要点

- **返回本地路径而不是远端 URL**：远端图床链接会过期，写进记忆就成了悬空指针。
  下载到本地后路径稳定，可交给媒体存储做内容寻址。
- 配了 `base_url` 就能指向自建/兼容网关，不必依赖官方接口。

## 构建

```bash
hmapdev build
```
