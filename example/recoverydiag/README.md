# recoverydiag · 快速检查 / 崩溃取证

给 guard 与 failback 用的**确定性诊断工具集**。

设计基调（源码原话）：**返回结论而非原文，确定性检出，不消耗 LLM token。**
崩溃后最忌讳的是把几万行日志塞进模型上下文让它"看看"，那既慢又不可靠 ——
这里每个工具都在本地算出结论再返回。

## 工具

| 工具 | 说明 |
|---|---|
| `recoverydiag_diag_triage` | 快速分诊：按退出码 / 信号 / 存活状态粗分类别（进程死亡 vs 配置类不可达 vs 正常） |
| `recoverydiag_diag_db` | config.db 完整性（`PRAGMA integrity_check`）+ LLM 源解析校验（`core.llm.sources.*` 必备字段），逐项 ok/fail |
| `recoverydiag_diag_log_scan` | 在日志目录的时间窗内统计已知错误签名（panic / OOM / 网络不可达 / provider 失败 / sql / 致命）出现次数，给出主导结论 |
| `recoverydiag_diag_delta` | 对比 baseline（上次 good 快照/目录）与现状，列出 created / modified / deleted 清单与摘要，判定"改了什么" |
| `recoverydiag_diag_loc` | 综合前四项结论，按**因果强度正交排序**定位根因并给出推荐恢复动作 |

## 用法顺序

```
diag_triage  →  diag_db  →  diag_log_scan  →  diag_delta  →  diag_loc
   （各自独立，可只跑需要的）                        （要传前四项的结论）
```

`diag_loc` 需要你把它余下的结论**作为参数传进去**（`triage` / `db` / `log` / `delta` 四个对象），
它不自己去调 —— 这样它只做归因，不重复执行。

## 配置项

| 键 | 默认 | 说明 |
|---|---|---|
| `db_check_cmd` | `auto` | `diag_db` 用的 `sqlite3` 命令。留空=auto：可用时用 sqlite3，缺失则回退读内核 Settings |
| `recovery_kb_dir` | 空 | `diag_loc` 结论 JSON 的落盘目录。缺省 `<data_dir>/recovery_kb` |

## 不注册通道与钩子

本插件**只提供工具**，不订阅输入、不挂阶段钩子 —— 它是被 guard 或 agent 主动调用的，
不做后台干预。

## 测试

```bash
go test -count=1 ./...
```

`diag_test.go` 覆盖各诊断项的判定逻辑。

## 构建

```bash
hmapdev build
```
