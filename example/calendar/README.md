# calendar · 日历事件

事件管理：支持**重复事件**与**多档提醒**。

## 工具

| 工具 | 说明 |
|---|---|
| `calendar_event_add` | 添加事件 |
| `calendar_event_list` | 列出即将到来的事件（含日期、时间、重复规则） |
| `calendar_event_update` | 更新事件（**只改传入的字段**；会重置提醒状态） |
| `calendar_event_delete` | 删除事件（连带**该事件及之后的所有重复实例**） |
| `calendar_today` | 今日事件 + 倒计时 |
| `calendar_week` | 本周事件，按天分组 |
| `calendar_month` | 月历网格，带事件标记点 |
| `calendar_search` | 按关键词搜标题 / 地点 / 备注 |

`calendar_event_add` 的时间格式：`YYYY-MM-DD HH:MM`；只给 `YYYY-MM-DD` 表示全天事件。

## 重复规则

`repeat` 取值：

| 值 | 含义 |
|---|---|
| `none` | 不重复 |
| `daily` | 每天 |
| `weekday` | 每个工作日 |
| `weekly` | 每周 |
| `biweekly` | 每两周 |
| `monthly` | 每月 |
| `yearly` | 每年 |
| `lunar_yearly` | **按农历年**（生日、传统节日用） |

`lunar_yearly` 是刻意加的：农历节日按公历写死会逐年偏移。

## 提醒

`remind_before` 单位是**分钟**，可给多个、逗号分隔：

```
15,60,1440      # 提前 15 分钟 + 1 小时 + 1 天
0 或留空        # 不提醒
```

到点通过 `InjectInterruptText` 注入提醒，带 `NoMemory: true` ——
提醒是瞬时信号，不是记忆内容。通道 `calendar` 同样声明为 NoMemory。

## 存储

事件存为 JSON，插件重启后保留。

## 构建

```bash
hmapdev build
```
