# ocr · 图片文字识别

从图片里提取文字（中英文），基于 [Tesseract](https://github.com/tesseract-ocr/tesseract) OCR 引擎。

## 前置依赖

需要系统里装有 `tesseract` 可执行文件：

```bash
# Debian/Ubuntu
apt install tesseract-ocr tesseract-ocr-chi-sim
```

中文识别需要 `chi_sim` 语言包；缺它时中文会识别成乱码而非报错。

## 工具

| 工具 | 说明 |
|---|---|
| `ocr_ocr_image` | 对图片做 OCR，返回识别文本 |

参数：

| 参数 | 说明 |
|---|---|
| `image_url` | 图片的 HTTP/HTTPS 地址（与 `image_data` 二选一） |
| `image_data` | 图片的 base64 数据，**不含** `data:image/...` 前缀（与 `image_url` 二选一） |
| `language` | 识别语言，默认 `chi_sim+eng`；可选 `chi_sim` / `eng` / `chi_sim+eng` |

## 实现要点

- 传入的图先落到临时目录，OCR 完 `defer os.RemoveAll` 清掉，不残留。
- 调用参数固定 `--psm 3`（全自动页面分割），适合截图与常规排版图片；对单行小图或竖排文本效果会下降。
- **`Cleaner`**：工具返回的是 JSON（含 `text`、`language` 等字段），进记忆计算前只取 `text` 正文 —— 否则 JSON 结构本身会参与向量化。

## 构建

```bash
hmapdev build
```
