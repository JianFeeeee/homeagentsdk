# weather · 天气查询

给 agent 补上天气查询能力（基于 [wttr.in](https://wttr.in)，无需 API Key）。

## 工具

| 工具 | 说明 |
|---|---|
| `weather_current` | 查询某城市当前天气 |
| `weather_forecast` | 查询未来几天预报 |
| `weather_set_location` | 设置默认城市 |

`weather_current` / `weather_forecast` 都接受 `location`（城市名，如 `Beijing`、`Shanghai`）；
省略时用配置里的默认城市。`weather_current` 另有 `units`：`metric`（摄氏，默认）或 `imperial`（华氏）。

## 配置项

| 键 | 默认 | 说明 |
|---|---|---|
| `default_location` | 空 | 默认城市名。留空则每次调用都必须传 `location` |

## 实现要点

- **`NoMemory: true`**：天气是外部实时数据，对记忆计算无长期价值，跳过向量化与关键词提取（原文仍保留在对话里）。
- **`Cleaner`**：输出参与记忆计算前先过滤，只保留摘要行 —— 天气查询会反复出现，全文进记忆会挤占上下文预算，而"上周三北京多少度"通常并不需要召回。

## 构建

```bash
hmapdev build
```

产出 `.hmap` 后经 Plugin Manager API 安装。
