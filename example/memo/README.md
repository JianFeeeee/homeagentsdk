# memo · 待办与备忘录

两类条目，行为**刻意不同**：

| 类型 | 用途 | 是否主动提醒 |
|---|---|---|
| **待办**（todo） | 有截止概念、需要被催的事 | ✅ 会 |
| **备忘录**（memo） | 纯记事，供以后查阅 | ❌ 不会 |

分开的理由：把"提醒我"和"记一下"混成一类，要么备忘录天天弹、要么待办被忘掉。

## 工具

| 工具 | 说明 |
|---|---|
| `memo_todo_add` | 添加待办（会被主动提醒） |
| `memo_todo_complete` | 标记待办完成（不再提醒） |
| `memo_todo_list` | 列出未完成待办（含 ID、内容、创建时间） |
| `memo_todo_delete` | 删除待办（含已完成的） |
| `memo_memo_create` | 创建备忘录（纯记事，不提醒） |
| `memo_memo_list` | 列出全部备忘录 |
| `memo_memo_delete` | 删除备忘录 |

> 工具名前缀取自插件名（`p.tp`），上面按默认的 `memo_` 写法列出。

## 提醒机制

- 后台 **每 5 分钟**检查一次未完成待办数；有则通过 `InjectInterruptText` 注入一条
  「注意，你还有 N 条待办未完成，请检查」。
- 注入带 **`NoMemory: true`** —— 这是定时提醒，不是记忆内容，不该进向量化。
- 通道声明为 **`NoMemory`**（`RegisterInputChannel(p.name, ChannelDef{NoMemory:true})`），
  理由同上：提醒是瞬时信号。
- 另有 `StagePreAction` 钩子，在每轮动作前参与。

## 存储

条目落在数据目录的 `todos.json`，插件重启后仍在。

## 构建

```bash
hmapdev build
```
