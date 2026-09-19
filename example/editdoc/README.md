# editdoc · Office 文档编辑

编辑 `.docx` / `.xlsx` / `.pptx` 内容：查找替换、改单元格、插行。

> ⚠️ **版本说明**：本目录是 **v1.0.0**，只有 `edit_document` 一个工具。
> 线上部署的 v2.0.0（全能办公版，支持新建/读取/转换 docx·xlsx·pptx·md·csv·txt）
> **源码尚未公开**，本文档不描述那些能力。参见 `plugin.json` 的 `version`。

## 工具

| 工具 | 说明 |
|---|---|
| `edit_document` | 编辑文档内容，**编辑后原文件被覆盖** |

参数：

| 参数 | 说明 |
|---|---|
| `file` | 文档路径（必填） |
| `operation` | `replace_text`（查找替换）/ `set_cell`（设置单元格）/ `insert_row`（插入行）（必填） |
| `target` | 要查找的文本（`replace_text` 用） |
| `replacement` | 替换为的文本（`replace_text` 用） |
| `sheet` | 工作表名（xlsx 可选） |
| `row` | 行号（`set_cell` / `insert_row` 用） |
| `col` | 列号（`set_cell` 用） |
| `value` | 单元格值（`set_cell` 用） |

编辑前建议先读一遍内容确认目标文本 —— 查找替换是**全文件覆盖写**，没有撤销。

## 配置项

| 键 | 默认 | 说明 |
|---|---|---|
| `script_path` | 空 | `edit_doc.py` 的绝对路径。留空则用插件可执行文件同目录下的 `edit_doc.py` |
| `venv_python` | 空 | 执行 `edit_doc.py` 的 Python 解释器（建议用 venv 里的）。**必须配置，留空会报错** |

## 工作原理

本插件是 Go 写的薄壳：把参数序列化成 JSON，交给 Python 脚本 `edit_doc.py` 执行实际文档操作。
文档解析依赖 Python 侧的库（python-docx / openpyxl / python-pptx 之类），所以：

- **需要自备 `edit_doc.py`**：它不在本目录里。
- 用 `venv_python` 指向装了这些库的解释器，避免污染系统 Python。

## 构建

```bash
hmapdev build
```
