# 受限 SDK 与安全

外部插件与内置插件的区别不只是「能不能调某个函数」，还包含一层**安全边界**：
外部插件的进程不共享内核地址空间，能力通过显式注入点交过去。

## 三层隔离

| 层 | 机制 | 防住了什么 |
|---|---|---|
| **进程** | 插件跑在独立子进程 | 插件 panic / 内存越界**不会带崩内核** |
| **能力** | 只注入显式声明的接口 | 插件拿不到未授权的内核内部结构 |
| **权限** | 公开接口是内部接口的**只读子集** | 插件无法改写他人数据 |

第一种是 v1.0.0 从 C ABI 动态库改为子进程 + 共享内存的直接收益：
在此之前，插件 panic 会带崩 `homed`。

## 受限接口是怎么实现的

**按接口裁剪，而不是按方法裁剪。** 同一个概念在公开包与内部包里是**两个不同的
接口声明**，公开的那个只保留安全子集：

```go
// 公开 SDK：6 个只读方法
type SocialAPI interface {
    GetPerson(name string) (*PersonProfile, error)
    GetTrait(name, trait string) (string, bool)
    GetRelations(name string) ([]SocialRelation, error)
    GetNetwork(name string, depth int) ([]*PersonProfile, error)
    ListPersons() ([]string, error)
}
```

写操作只在内核内部接口里。这样外部插件**在类型层面就调不到**，
不是靠运行时检查拦截。

同理，`EventSubscriber` 公开版**刻意只有 `Subscribe`，没有 `Publish`**：

```go
// 插件可以订阅，但由内核决定投递哪些事件
type EventSubscriber interface {
    Subscribe(eventType EventType, handler EventHandler) func()
}
```

## 进程边界带来的约束

事件订阅是理解这层边界的典型例子。公开包里有一个 `Events() EventSubscriber`，
但**外部插件拿到的恒为 nil** —— 桥接运行时不注入它（`SetEventSubscriber`
在全仓没有调用点）。外部插件的事件订阅由生成的运行时通过 `events.subscribe`
RPC 完成，Lua 插件走内部 SDK 的 `Subscribe`。

这不是缺陷，而是进程边界的结果：跨进程无法共享内核的事件发布通道。
详见[能力边界](capability-boundary.md)。

## 共享内存中的数据面

工具调用帧、Cleaner、输入输出通道、媒体块、文档与知识正文**都走共享内存**，
RPC 只传偏移描述符。因此：

- 大对象不经 JSON 序列化，避免了大 payload 的性能与内存放大；
- StageContext 在同一份状态上读改写，消除了副本模型的 lost update
  （实测由 35.8~36.8% 降到 0）。

`SharedRef`（共享内存描述符）是**内部实现细节**，插件开发者看不到它 ——
公开 SDK 只暴露普通字符串与 map。

## 插件作者的实践建议

- **不要在 `Start` 里长时间阻塞** —— 内核在等待它返回。
- **工具处理器要可并发**：模型可能并发发起多个调用；共享状态用锁保护
  （`example/memo` 用 `sync.RWMutex`）。
- **写文件用原子替换**（临时文件 + rename），避免进程被强杀时截断数据。
- **声明 `NoMemory`**：定时提醒、连接状态这类不是对话内容的东西，
  别让它们污染记忆（`InjectOptions{NoMemory: true}`）。
- **在 `-race` 下测**：插件重载瞬间的并发访问是历史高发缺陷。

## 许可与分发

SDK 是 **MIT**，插件可以**闭源分发**，可商用、可私有，无需回馈。
这是刻意的：SDK 随插件静态链接（源码进入插件二进制），用传染性许可会
强迫插件开源。内核本身是 AGPL-3.0-only，但那是内核的许可，与外部插件无关。
