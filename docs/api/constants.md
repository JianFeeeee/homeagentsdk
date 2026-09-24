<!-- 本页由 tools/apidoc/gensite 从源码生成，请勿手改；要改文档就改 sdk/*.go 的注释。 -->

# 常量与枚举

SDK 里的取值枚举。其中带「仅内置」标注的取值在内核侧会被夹到较低级别。

## StageOnInput 等

| 名称 | 说明 |
|---|---|
| `StageOnInput` |  |
| `StagePreAction` |  |
| `StagePostAction` |  |
| `StageBeforeToolcall` |  |
| `StageAfterToolcall` |  |
| `StageBeforeOutput` |  |
| `StageAfterOutput` |  |

## ContextPolicyNone 等

| 名称 | 说明 |
|---|---|
| `ContextPolicyNone` |  |
| `ContextPolicyPrune` |  |

## RecallPolicyNone 等

| 名称 | 说明 |
|---|---|
| `RecallPolicyNone` |  |
| `RecallPolicyAuto` |  |

## PriorityL1 等

| 名称 | 说明 |
|---|---|
| `PriorityL1` |  |
| `PriorityL2` |  |
| `PriorityL3` |  |
| `PriorityL4` | PriorityL4 仅内核级（内置）插件可用；外部插件声明会被夹到 L3。 |

## EventRawInput 等

| 名称 | 说明 |
|---|---|
| `EventRawInput` |  |
| `EventAgentOutput` |  |
| `EventAgentLLMChain` |  |
| `EventToolCall` |  |
| `EventReasoning` |  |
| `EventStage` |  |
| `EventSystem` |  |
| `EventReasoningDelta` | 流式增量事件（token 级）：核心 process() 流式化后每收到一个增量块发布。 |
| `EventContentDelta` |  |

## StageScopeGlobal 等

| 名称 | 说明 |
|---|---|
| `StageScopeGlobal` | StageScopeGlobal receives all stage events (default). |
| `StageScopeOwnTools` | StageScopeOwnTools only receives events for this plugin's own tool calls |

## CapText 等

| 名称 | 说明 |
|---|---|
| `CapText` |  |
| `CapFile` |  |
| `CapImage` |  |
| `CapAudio` |  |
| `CapStructured` |  |

