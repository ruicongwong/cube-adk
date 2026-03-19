# Cube ADK

> 用 Go 构建生产级 LLM Agent 系统的开发框架

---

## 设计理念

**Cube ADK 的核心主张：Agent 系统的复杂度应该在编排层，而不是胶水代码里。**

三条设计原则驱动所有决策：

1. **协议优先（Protocol First）**
   所有层之间只通过显式定义的数据类型交互，消除隐式耦合。`Message`、`Signal`、`ToolCall` 是框架内部唯一的通信货币，任何实现都可以替换，只要满足协议契约。

2. **接口即边界（Interface as Boundary）**
   `Model`、`Tool`、`Vault`、`Embedder`、`Retriever`、`Gate`、`Tracer` 均为纯接口。框架不依赖任何具体实现。你可以在不修改框架代码的情况下，替换任意底层能力。

3. **Context 即管道（Context as Pipeline）**
   可观测性（Tracer）、生命周期回调（Callback）、截止时间（Deadline）全部通过 `context.Context` 传递。Agent 之间无全局状态，调用链天然可组合、可测试。

---

## 架构总览

```
┌─────────────────────────────────────────────────────────────┐
│                        Examples                             │
│              solo  /  team  /  artifact                     │
├─────────────────────────────────────────────────────────────┤
│                   Engine（Agent 引擎）                       │
│                                                             │
│   SoloAgent          ChainAgent        ParallelAgent        │
│   ReAct 单循环        顺序编排           并发 + 结果合并      │
│                                                             │
│   DeepAgent          Conductor                              │
│   递归规划分解         多 Agent 调度 + handoff               │
├─────────────────────────────────────────────────────────────┤
│                   Core（核心抽象）                            │
│                                                             │
│   Agent   State   Signal   Vault   Shelf                    │
│   Gate    Policy  Tracer   Embedder  Retriever              │
├──────────────────────┬──────────────────────────────────────┤
│  Callback            │  Component          Option           │
│  Handler · RunInfo   │  Model · Tool        ModelOption     │
│  TimingGuard         │                     ToolOption       │
├──────────────────────┴──────────────────────────────────────┤
│                   Protocol（协议层）                          │
│                                                             │
│   Message · ContentPart · ToolCall · ToolSpec · ToolResult  │
│   StreamReader[T] · StreamWriter[T] · Document              │
└─────────────────────────────────────────────────────────────┘
```

### 数据流向

```
用户输入
  │
  ▼
State（消息历史 + KV 上下文）
  │
  ▼
Agent.Execute(ctx, state)
  │                        ┌─────────────────────────┐
  ├── Recall(Vault) ──────►│ 记忆检索（keyword/vector）│
  │                        └─────────────────────────┘
  │
  ├── Model.Generate ─────► LLM 推理
  │        │
  │        ├── 无工具调用 ──► Signal{Reply} ──► StreamReader
  │        │
  │        └── 有工具调用 ──► Gate.Check（可选人机审批）
  │                  │
  │                  └── Tool.Run ──► Signal{Invoke/Yield}
  │
  └── Vault.Append ──────► 写入记忆
```

### Signal 事件类型

Agent 通过 `StreamReader[Signal]` 向调用方推送事件，调用方按需消费：

| Signal 类型 | 含义 |
|------------|------|
| `Think`    | Agent 内部推理文本 |
| `Invoke`   | 即将调用工具 |
| `Yield`    | 工具调用结果 |
| `Reply`    | 最终回复 |
| `Recall`   | 从 Vault 检索到的记忆 |
| `Plan`     | DeepAgent 生成的任务分解计划 |
| `Synth`    | DeepAgent 综合子任务结果 |
| `Handoff`  | Conductor Agent 间移交 |
| `Gate`     | 人机审批检查点 |
| `Artifact` | 产物输出 |
| `Fault`    | 错误事件 |

---

## 快速开始

```bash
go get cube-adk
```

### 最简示例：单 Agent 对话

```go
package main

import (
    "context"
    "fmt"

    "cube-adk/pkg/brain"
    "cube-adk/pkg/core"
    "cube-adk/pkg/engine"
    "cube-adk/pkg/protocol"
    "cube-adk/pkg/runtime"
)

func main() {
    model := brain.NewOpenAIModel(
        "https://api.openai.com/v1",
        "sk-xxx",
        "gpt-4o",
    )

    agent := &engine.SoloAgent{
        Name:    "assistant",
        Persona: "你是一个有帮助的助手。",
        Model:   model,
    }

    state := runtime.NewState("demo")
    state.Append(protocol.NewTextMessage("user", "你好"))

    r, _ := agent.Execute(context.Background(), state)
    for {
        sig, err := r.Recv()
        if err != nil { break }
        if sig.Kind == core.SignalReply {
            fmt.Println(sig.Text)
        }
    }
}
```

---

## 核心抽象（pkg/core）

### Agent — 统一执行接口

```go
type Agent interface {
    Identity() string
    Execute(ctx context.Context, state *State) (*protocol.StreamReader[Signal], error)
}
```

所有 Agent 实现（SoloAgent、ChainAgent、DeepAgent、Conductor、ParallelAgent）均满足此接口，可以任意嵌套和组合。

### State — 会话上下文

```go
state := core.NewState("session-id",
    core.WithVault(mv),   // 注入记忆
    core.WithShelf(sh),   // 注入产物存储
)
state.Append(protocol.NewTextMessage("user", "分析这份报告"))
state.Set("user_id", "u123")     // 任意 KV
history := state.History()        // []*protocol.Message
```

### Vault — 可插拔记忆

`Vault` 是 Agent 的长期记忆存储，`Recall` 在每次推理前自动检索相关片段注入上下文。

```go
type Vault interface {
    Append(ctx context.Context, entry Entry) error
    Recall(ctx context.Context, query string, topK int) ([]Entry, error)
    Delete(ctx context.Context, id string) error
}
```

检索策略可插拔，通过 `Retriever` 接口解耦：

```go
// 默认：关键词匹配（无依赖）
mv := vault.NewMemVault()

// 向量检索（注入 Embedder）
mv := vault.NewMemVault(
    vault.WithRetriever(vault.NewVectorRetriever(myEmbedder)),
)

// 混合检索：RRF 融合关键词 + 向量
mv := vault.NewMemVault(
    vault.WithRetriever(vault.NewHybridRetriever(
        vault.NewKeywordRetriever(),
        vault.NewVectorRetriever(myEmbedder),
    )),
)

// 持久化到磁盘
fv, _ := vault.NewFileVault("./memory",
    vault.WithFileRetriever(vault.NewVectorRetriever(myEmbedder)),
)
```

实现自定义 Embedder 只需一个方法：

```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float64, error)
}
```

### Gate + Policy — 人机审批

```go
type Gate interface {
    Check(ctx context.Context, cp Checkpoint) (Review, error)
}
type Policy interface {
    NeedsReview(cp Checkpoint) bool
}
```

`Policy` 决定哪些操作需要审批，`Gate` 执行审批动作（CLI 交互、远程审批系统等）。Agent 在工具调用前自动触发检查，支持 `Approve`、`Reject`、`Modify` 三种裁决。

---

## 协议层（pkg/protocol）

协议层是框架的地基，所有层共享这组类型，不含任何业务逻辑。

### 多模态消息

```go
// 纯文本
msg := protocol.NewTextMessage("user", "描述这张图片")

// 多模态
msg := protocol.NewUserParts(
    protocol.ContentPart{Kind: protocol.PartText, Text: "这是什么？"},
    protocol.ContentPart{Kind: protocol.PartImage,
        PartMeta: protocol.PartMeta{URL: "https://example.com/img.png"}},
)

text := msg.TextOf() // 提取纯文本
```

支持：`PartText` · `PartImage` · `PartAudio` · `PartVideo` · `PartFile` · `PartReasoning`

### 泛型流（StreamReader / StreamWriter）

```go
// 创建管道
reader, writer := protocol.Pipe[core.Signal](32)

// 生产端（goroutine）
go func() {
    writer.Send(sig)
    writer.Finish(nil) // nil = 正常结束，err = 异常
}()

// 消费端
for {
    sig, err := reader.Recv()
    if err != nil { break }
    // ...
}

// 工具函数
all, _  := protocol.CollectAll(reader)          // 收集全部
mapped  := protocol.MapReader(reader, fn)        // 映射变换
copies  := reader.Copy(3)                        // 扇出复制
```

---

## 组件接口（pkg/component）

### Model — LLM 推理

```go
type Model interface {
    Generate(ctx context.Context, msgs []*protocol.Message,
        opts ...option.ModelOption) (*protocol.Message, error)
    Stream(ctx context.Context, msgs []*protocol.Message,
        opts ...option.ModelOption) (*protocol.StreamReader[*protocol.Message], error)
}
```

### Tool — 结构化工具

```go
type Tool interface {
    Identity() string
    Brief()    string
    Spec()     protocol.ToolSpec
    Run(ctx context.Context, call protocol.ToolCall,
        opts ...option.ToolOption) (protocol.ToolResult, error)
}
```

快捷创建：

```go
calc := &tool.QuickTool{
    Name: "calculator",
    Desc: "计算数学表达式",
    Params: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "expr": map[string]any{"type": "string"},
        },
    },
    Fn: func(ctx context.Context, args string) (string, error) {
        // args 是 JSON 字符串
        return "42", nil
    },
}
```

---

## 两层函数式选项（pkg/option）

通用选项和实现特定选项在同一变参中共存，框架代码无需感知具体实现：

```go
// 通用 ModelOption
resp, _ := model.Generate(ctx, msgs,
    option.WithTemperature(0.7),
    option.WithMaxTokens(2048),
    option.WithToolSpecs(specs...),
)

// 混入实现特定选项（vLLM guided decoding）
resp, _ := model.Generate(ctx, msgs,
    option.WithTemperature(0.7),
    brain.WithGuidedJSON(schema),
    brain.WithRepetitionPenalty(1.1),
)

// ToolOption：构建时传入，控制超时和重试
ddg := tool.NewDuckDuckGoTool(
    option.WithTimeout(5 * time.Second),
    option.WithRetryCount(2),
)
```

---

## 回调系统（pkg/callback）

AOP 风格的生命周期钩子，通过 context 传递，**无 handler 时零开销**（TimingGuard 短路）。

```go
handler := callback.NewHandler().
    Start(func(ctx context.Context, info callback.RunInfo, input any) context.Context {
        fmt.Printf("[%s/%s] started\n", info.Kind, info.Name)
        return ctx
    }).
    End(func(ctx context.Context, info callback.RunInfo, output any) context.Context {
        fmt.Printf("[%s/%s] finished\n", info.Kind, info.Name)
        return ctx
    }).
    Error(func(ctx context.Context, info callback.RunInfo, err error) context.Context {
        fmt.Printf("[%s/%s] error: %v\n", info.Kind, info.Name, err)
        return ctx
    }).
    Build()

ctx = callback.Inject(ctx, handler)
// 之后所有 Agent 的生命周期事件自动分发
```

---

## Agent 引擎（pkg/engine）

### SoloAgent — ReAct 单循环

推理 → 工具调用 → 观察 → 循环，直到无工具调用时输出最终回复。

```go
agent := &engine.SoloAgent{
    Name:      "researcher",
    Persona:   "你是一个研究助手。",
    Model:     model,
    Tools:     []component.Tool{searchTool, calcTool},
    Vault:     memVault,
    StepLimit: 10,
    Gate:      cliGate,
    Policy:    toolPolicy,
    Tracer:    memTracer,
}
```

### ChainAgent — 顺序编排

```go
chain := &engine.ChainAgent{
    Name:   "research-then-write",
    Agents: []core.Agent{researcher, writer},
}
```

### DeepAgent — 递归规划分解

复杂任务自动分解为子任务，递归执行后综合结果。支持 Vault Recall 辅助规划，退化时自动切换为 ReAct 循环。

```go
deep := &engine.DeepAgent{
    Name:     "analyst",
    Persona:  "你是一个深度分析师。",
    Model:    model,
    Tools:    tools,
    Vault:    memVault,
    MaxDepth: 3,
}
```

执行流程：
```
Recall → Plan → Gate(可选) → 子任务递归执行 → Synth → Reply
```

### Conductor — 多 Agent 调度

自动为子 Agent 注入 `handoff` 工具，Agent 间通过工具调用完成对话移交：

```go
conductor := engine.NewConductor("team", "researcher",
    researcher,
    writer,
    analyst,
)

// 将 Agent 包装为 Tool 供其他 Agent 调用
delegateTool := engine.AsTool(writer)
```

### ParallelAgent — 并发执行

```go
parallel := &engine.ParallelAgent{
    Name:   "multi-search",
    Agents: []core.Agent{agent1, agent2, agent3},
    Merge: func(results map[string][]core.Signal) string {
        // nil 时默认拼接所有 Reply
        return merged
    },
}
```

---

## 内置实现

### Model（pkg/brain）

```go
m := brain.NewOpenAIModel("https://api.openai.com/v1", "sk-xxx", "gpt-4o")
m := brain.NewVLLMModel("http://localhost:8000/v1", "Qwen/Qwen2.5-7B")
```

### Tools（pkg/tool）

```go
ddg := tool.NewDuckDuckGoTool(option.WithTimeout(5 * time.Second))

weather := tool.NewRESTTool(tool.RESTSpec{
    Name:       "get_weather",
    Desc:       "查询实时天气",
    Method:     "GET",
    URL:        "https://api.weather.com/v1/{city}",
    ResultPath: "current.temperature",
}, option.WithTimeout(10*time.Second))
```

### Tracer（pkg/trace）

```go
t := trace.NewMemTracer()
model := trace.WrapModel(rawModel, t)
tools := trace.WrapTools(rawTools, t)

for _, sp := range t.Spans() {
    fmt.Printf("%s  %v  tokens=%d\n", sp.Name, sp.Duration(), sp.TokenUsage())
}
```

---

## 示例

| 示例 | 说明 |
|------|------|
| `examples/solo/` | 单 Agent + 工具调用 |
| `examples/team/` | 多 Agent 编排（Chain、Conductor、DeepAgent）|
| `examples/artifact/` | 产物生成与 Shelf 存储 |

```bash
export OPENAI_API_KEY=sk-xxx
export OPENAI_ENDPOINT=https://api.openai.com/v1
go run examples/solo/main.go
```

---

## License

Apache-2.0
