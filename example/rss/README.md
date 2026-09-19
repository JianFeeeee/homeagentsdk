# rss · RSS/Atom 订阅监控

订阅 RSS/Atom 源，**有新文章时主动通知** agent（不必每轮去问）。

## 工具

| 工具 | 说明 |
|---|---|
| `rss_subscribe` | 订阅一个 RSS/Atom 源 |
| `rss_unsubscribe` | 取消订阅 |
| `rss_list` | 列出全部订阅 |
| `rss_check_now` | 立即检查所有源（不等轮询周期） |

## 配置项

| 键 | 默认 | 说明 |
|---|---|---|
| `poll_interval` | `30` | 默认轮询间隔（**分钟**） |

订阅时可对单个源覆盖间隔。

## 通知机制

- 后台按各自间隔轮询（默认 30 分钟）。
- 发现新条目时通过 `InjectInterruptText` 注入，格式形如
  `📡 <源标题> (<URL>) — N 篇新文章:` 后跟条目。
- 注入带 **`NoMemory: true`**，通道 `rss` 也声明为 `NoMemory` ——
  订阅推送是信号不是知识，不该进向量化挤掉别的记忆。

## 实现要点

- **订阅时就记下全部已有 GUID**：`handleSubscribe` 会把抓取到的历史条目
  一次性标为 `seenGUIDs`，所以**订阅一个源不会把它的历史文章全部推送一遍**。
  只有订阅之后新出现的条目才通知。这是避免刷屏的关键。
- **去重按「源 URL + GUID」**：不同源可能用相同 GUID，只用 GUID 会互相误判。
  GUID 缺失时回退用 `link`；两者都缺则跳过该条。
- `seenGUIDs` 有清理逻辑，不会无限增长。
- 解析用 [gofeed](https://github.com/mmcdole/gofeed)（`v1.4.0`）。

## 构建

```bash
hmapdev build
```
