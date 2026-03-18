# Cube ADK

Cube ADK 是一个用 Go 编写的 Agent 开发框架，用于构建基于 LLM 的智能体系统。支持单 Agent、多 Agent 编排、工具调用、记忆管理、人机审批、可观测性追踪等能力。

## 架构总览

```
┌──────────────────────────────────────────────────────┐
│                   Examples                           │
│           solo  /  team  /  artifact                 │
├──────────────────────────────────────────────────────┤
│                 Engine（Agent 实现）                  │
│    SoloAgent / ChainAgent / DeepAgent / Conductor    │
├──────────────────────────────────────────────────────┤
│                 Core（接口定义）                      │
│  Agent · Brain · Tool · Vault · Shelf                │
│  Bus · Gate · Policy · Tracer                        │
├──────────────────────────────────────────────────────┤
│                 Implementations                      │
│  OAI Brain · REST/DDG Tools · Mem/File Vault/Shelf   │
│  MemBus · CLI/Callback Gate · MemTracer              │
└──────────────────────────────────────────────────────┘
```

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
    "cube-adk/pkg/runtime"
)

func main() {
    b := &brain.OAI{
        Endpoint: "https://api.openai.com/v1",
        Secret:   "sk-xxx",
        ModelID:  "gpt-4o",
    }

    agent := &engine.SoloAgent{
        Name:    "assistant",
        Persona: "你是一个有帮助的助手。",
        Brain:   b,
    }

    conv := runtime.NewConversation()
    conv.Append(core.Dialogue{Role: "user", Text: "你好"})

    for sig := range agent.Execute(context.Background(), conv) {
        if sig.Kind == core.SigReply {
            fmt.Println(sig.Text)
        }
    }
}
```

## 核心接口（pkg/core）

所有组件面向接口编程，可自由替换实现。

### Agent

```go
type Agent interface {
    Identity() string
    Execute(ctx context.Context, conv *Conversation) <-chan Signal
}
```

Agent 通过 channel 返回 `Signal` 事件流（Think / Invoke / Yield / Reply / Handoff / Fault 等），调用方可实时消费。

### Brain

```go
type Brain interface {
    Think(ctx context.Context, dialogue []Dialogue, tools []Tool) (*Dialogue, error)
}
```

LLM 推理抽象。输入对话历史和可用工具，返回 assistant 消息（可能包含工具调用请求）。返回的 `Dialogue` 携带 `Usage`（token 用量）和 `TTFT`（首 token 耗时）。

### Tool

```go
type Tool interface {
    Identity() string
    Brief() string
    Schema() map[string]any
    Perform(ctx context.Context, input string) (string, error)
}
```

工具能力抽象。`Schema()` 返回 JSON Schema 供 LLM 理解参数格式。实现 `ArtifactTool` 接口的工具还可以产出富媒体产物。

快捷创建方式：

```go
calc := &core.QuickTool{
    Name: "calculator",
    Desc: "计算数学表达式",
    Params: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "expression": map[string]any{"type": "string"},
        },
    },
    Fn: func(ctx context.Context, input string) (string, error) {
        // 解析并计算 expression
        return result, nil
    },
}
```

### Vault（记忆系统）

```go
type Vault interface {
    Append(ctx context.Context, entry Entry) error
    Recall(ctx context.Context, query string, limit int) ([]Fragment, error)
    Forget(ctx context.Context, filter Filter) error
}
```

三级记忆作用域：`Working`（工作记忆）、`Short`（短期）、`Long`（长期）。Agent 每轮推理前会自动 Recall 相关记忆注入上下文。

### Shelf（产物存储）

```go
type Shelf interface {
    Store(ctx context.Context, artifact ArtifactDetail) error
    Fetch(ctx context.Context, id string) (*ArtifactDetail, error)
    List(ctx context.Context, mime string, limit int) ([]ArtifactDetail, error)
    Discard(ctx context.Context, id string) error
}
```

存储 Agent 产出的富媒体内容（HTML、图片、JSON 等）。

### Gate & Policy（人机审批）

```go
type Gate interface {
    Check(ctx context.Context, cp Checkpoint) (*Review, error)
}

type Policy interface {
    NeedsReview(cp Checkpoint) bool
}
```

Gate 负责获取人类审批结果（批准 / 拒绝 / 修改），Policy 决定哪些操作需要审批。

### Bus（消息总线）

```go
type Bus interface {
    Publish(ctx context.Context, topic string, sig Signal) error
    Subscribe(topic string) (<-chan Signal, error)
    Close() error
}
```

Agent 间的发布/订阅通信机制。

### Tracer（可观测性）

```go
type Tracer interface {
    Start(ctx context.Context, name string, kind SpanKind) (context.Context, Span)
}
```

分布式追踪抽象，支持 Agent / Brain / Tool 三种 SpanKind。Span 自动记录 token 用量、TTFT、耗时等指标。

## Agent 引擎（pkg/engine）

### SoloAgent — 单 Agent ReAct 循环

最常用的 Agent 类型。执行 Think → Invoke → Yield → Reply 循环，直到 LLM 给出最终回复或达到步数上限。

```go
agent := &engine.SoloAgent{
    Name:      "researcher",
    Persona:   "你是一个研究助手。",
    Brain:     oaiBrain,
    Tools:     []core.Tool{searchTool, calcTool},
    Vault:     memVault,       // 可选：记忆
    StepLimit: 10,             // 可选：最大循环步数
    Gate:      cliGate,        // 可选：人机审批
    Policy:    toolPolicy,     // 可选：审批策略
    Tracer:    memTracer,      // 可选：追踪
}
```

### ChainAgent — 顺序编排

将多个 Agent 串联执行，前一个 Agent 的回复作为下一个的用户输入。

```go
chain := &engine.ChainAgent{
    Name:   "research-then-write",
    Agents: []core.Agent{researcher, writer},
}
```

### DeepAgent — 递归分解

对复杂任务进行 Plan → Execute → Synthesize 递归分解。LLM 先将任务拆分为子任务，递归执行后综合结果。

```go
deep := &engine.DeepAgent{
    Name:     "analyst",
    Persona:  "你是一个深度分析师。",
    Brain:    oaiBrain,
    Tools:    tools,
    MaxDepth: 3,       // 最大递归深度
}
```

### Conductor — 多 Agent 调度

管理多个 Agent，支持 Handoff（移交）机制。Agent 可以通过返回 `SigHandoff` 信号将对话移交给其他 Agent。

```go
conductor := &engine.Conductor{
    Name:       "orchestrator",
    Agents:     map[string]core.Agent{"researcher": r, "writer": w},
    EntryAgent: "researcher",
}
```

也可以将 Agent 包装为 Tool 供其他 Agent 调用：

```go
delegateTool := conductor.AsTool("writer", "让写作 Agent 撰写内容")
```

## 内置实现

### Brain — OAI（pkg/brain）

兼容 OpenAI Chat Completions API 的 Brain 实现，支持任意兼容端点（OpenAI、Azure、本地模型等）。

```go
b := &brain.OAI{
    Endpoint:   "https://api.openai.com/v1",
    Secret:     "sk-xxx",
    ModelID:    "gpt-4o",
    HTTPClient: http.DefaultClient, // 可选：自定义 HTTP 客户端
}
```

自动解析 API 返回的 `usage` 字段，填充 `Dialogue.Usage` 和 `Dialogue.TTFT`。

### Tools（pkg/tool）

**DuckDuckGo 搜索** — 无需 API Key 的网页搜索：

```go
ddg := &tool.DuckDuckGoTool{}
```

**REST 工具** — 声明式 REST API 封装：

```go
weather := core.RESTSpec{
    Name:       "get_weather",
    Desc:       "查询天气",
    Method:     "GET",
    URL:        "https://api.weather.com/v1/{city}",
    Headers:    map[string]string{"Authorization": "Bearer xxx"},
    ResultPath: "current.temperature",
}
weatherTool := tool.FromREST(weather)
```

### Vault 实现（pkg/vault）

| 实现 | 说明 |
|---|---|
| `MemVault` | 内存存储，适合开发测试 |
| `FileVault` | 文件持久化，按 working/short/long 目录分类存储 JSON |

```go
mv := vault.NewMemVault()
fv, _ := vault.NewFileVault("/path/to/memory")
```

### Shelf 实现（pkg/shelf）

| 实现 | 说明 |
|---|---|
| `MemShelf` | 内存存储 |
| `FileShelf` | 文件持久化，每个 artifact 存为 .meta.json + .data |

```go
ms := shelf.NewMemShelf()
fs, _ := shelf.NewFileShelf("/path/to/artifacts")
```

### Gate 实现（pkg/gate）

| 实现 | 说明 |
|---|---|
| `CLIGate` | 命令行交互审批 |
| `CallbackGate` | 自定义回调函数 |
| `ChannelGate` | 基于 channel，适合集成 Web/Slack |

```go
// CLI 审批
g := &gate.CLIGate{}

// 自定义回调
g := &gate.CallbackGate{Fn: func(ctx context.Context, cp core.Checkpoint) (*core.Review, error) {
    return &core.Review{Verdict: core.Approve}, nil
}}

// Channel 审批（适合 Web 集成）
g := gate.NewChannelGate()
go func() {
    for cp := range g.Pending() {
        g.Respond() <- &core.Review{Verdict: core.Approve}
    }
}()
```

### Policy 实现（pkg/gate）

```go
gate.AllowAll{}                           // 全部放行
gate.ToolPolicy{Names: []string{"rm"}}    // 仅审批指定工具
gate.KindPolicy{Kinds: []string{"tool"}}  // 按类型审批
gate.CompositePolicy{Policies: [...]}     // 组合策略（OR 逻辑）
```

### Bus 实现（pkg/bus）

```go
b := bus.NewMemBus()
ch, _ := b.Subscribe("events")
b.Publish(ctx, "events", signal)
```

### Trace 实现（pkg/trace）

```go
// 内存追踪（开发调试）
t := trace.NewMemTracer()

// 无操作追踪（生产环境关闭追踪）
t := trace.Nop

// 包装 Brain/Tool 自动注入追踪
brain := trace.WrapBrain(oaiBrain, t)
tools := trace.WrapTools(rawTools, t)

// 查看追踪结果
for _, sp := range t.Spans() {
    fmt.Printf("%s %s %v\n", sp.Name, sp.Duration(), sp.TokenUsage())
}

// 聚合统计
summary := t.Summary()
fmt.Printf("总耗时: %v, Brain调用: %d, Token: %d\n",
    summary.TotalDuration, summary.BrainCalls, summary.TotalTokens)
```

## Runtime 工具（pkg/runtime）

Signal 流处理工具函数：

```go
// 收集所有信号
signals := runtime.Collect(agent.Execute(ctx, conv))

// 旁路监听
ch := runtime.Tap(agent.Execute(ctx, conv), func(sig core.Signal) {
    log.Println(sig.Kind, sig.Text)
})

// 按类型过滤
replies := runtime.FilterKind(signals, core.SigReply)

// 提取产物
artifacts := runtime.CollectArtifacts(signals)
```

## 示例

项目包含三个完整示例：

- `examples/solo/` — 单 Agent + 工具调用（计算器、天气、搜索）
- `examples/team/` — 多 Agent 编排（Chain、Conductor、DeepAgent）
- `examples/artifact/` — 产物生成与存储

运行示例：

```bash
export OAI_KEY=sk-xxx
export OAI_ENDPOINT=https://api.openai.com/v1
go run examples/solo/main.go
```

## License

Apache-2.0 license
