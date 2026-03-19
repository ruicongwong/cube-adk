# Cube ADK

Cube ADK 是一个用 Go 编写的 Agent 开发框架，用于构建基于 LLM 的智能体系统。采用分层架构设计，提供完整的协议层、组件层、回调系统，支持多模态、流式推理、动态选项、多 Agent 编排、工具调用、记忆管理、人机审批、可观测性追踪等能力。

## 架构总览

```
┌──────────────────────────────────────────────────────────┐
│                      Examples                            │
│            solo  /  team  /  artifact                    │
├──────────────────────────────────────────────────────────┤
│                Engine（Agent 实现）                       │
│  SoloAgent / ChainAgent / DeepAgent / Conductor          │
│  ParallelAgent                                           │
├──────────────────────────────────────────────────────────┤
│                 Core（核心类型）                           │
│  Agent · State · Signal · Vault · Shelf                │
│  Gate · Policy · Tracer · Embedder · Retriever           │
├──────────────────────────────────────────────────────────┤
│              Callback（生命周期回调）                      │
│  Handler · RunInfo · TimingGuard · Inject/Extract        │
├──────────────────────────────────────────────────────────┤
│              Component（组件接口）                         │
│  Model · Tool                                            │
├──────────────────────────────────────────────────────────┤
│              Option（两层函数式选项）                      │
│  ModelOption · ToolOption                                │
├──────────────────────────────────────────────────────────┤
│              Protocol（协议层）                            │
│  Message · ContentPart · ToolCall · ToolSpec · ToolResult │
│  StreamReader · StreamWriter · Document                   │
└──────────────────────────────────────────────────────────┘
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
    "cube-adk/pkg/protocol"
    "cube-adk/pkg/runtime"
)

func main() {
    m := brain.NewOpenAIModel(
        "https://api.openai.com/v1",
        "sk-xxx",
        "gpt-4o",
    )

    agent := &engine.SoloAgent{
        Name:    "assistant",
        Persona: "你是一个有帮助的助手。",
        Model:   m,
    }

    sess := runtime.NewState("demo")
    sess.Append(protocol.NewTextMessage("user", "你好"))

    r, _ := agent.Execute(context.Background(), sess)
    for {
        sig, err := r.Recv()
        if err != nil { break }
        if sig.Kind == core.SignalReply {
            fmt.Println(sig.Text)
        }
    }
}
```

## 协议层（pkg/protocol）

协议层定义了框架中所有共享的数据类型，支持多模态内容。

### Message — 多模态消息

```go
msg := protocol.NewTextMessage("user", "描述这张图片")

// 多模态消息
msg := protocol.NewUserParts(
    protocol.ContentPart{Kind: protocol.PartText, Text: "这是什么？"},
    protocol.ContentPart{Kind: protocol.PartImage, PartMeta: protocol.PartMeta{URL: "https://example.com/img.png"}},
)

// 提取纯文本
text := msg.TextOf()
```

支持的内容类型：`PartText`、`PartImage`、`PartAudio`、`PartVideo`、`PartFile`、`PartReasoning`。

### ToolCall / ToolSpec / ToolResult — 工具调用协议

```go
// 工具规格（供 LLM 理解）
spec := protocol.ToolSpec{
    Name:   "calculator",
    Desc:   "计算数学表达式",
    Schema: map[string]any{"type": "object", "properties": map[string]any{...}},
}

// 工具调用结果
result := protocol.NewTextResult(callID, "42")
errResult := protocol.NewErrorResult(callID, err)
```

### StreamReader / StreamWriter — 泛型流式读取

```go
reader, writer := protocol.Pipe[*protocol.Message](8)

// 生产端
go func() {
    writer.Send(msg)
    writer.Finish(nil)
}()

// 消费端
for {
    msg, err := reader.Recv()
    if err != nil { break }
    fmt.Println(msg.TextOf())
}

// 工具函数
items, _ := protocol.CollectAll(reader)
mapped := protocol.MapReader(reader, transformFn)
copies := reader.Copy(3) // 扇出
```

## 组件接口（pkg/component）

### Model — LLM 推理

```go
type Model interface {
    Generate(ctx context.Context, msgs []*protocol.Message, opts ...option.ModelOption) (*protocol.Message, error)
    Stream(ctx context.Context, msgs []*protocol.Message, opts ...option.ModelOption) (*protocol.StreamReader[*protocol.Message], error)
}
```

Tool 绑定在 Agent 层，通过 `option.WithToolSpecs()` 传给 Model。

### Tool — 结构化工具

```go
type Tool interface {
    Identity() string
    Brief() string
    Spec() protocol.ToolSpec
    Run(ctx context.Context, call protocol.ToolCall, opts ...option.ToolOption) (protocol.ToolResult, error)
}
```

快捷创建：

```go
calc := &tool.QuickTool{
    Name: "calculator",
    Desc: "计算数学表达式",
    Params: map[string]any{...},
    Fn: func(ctx context.Context, args string) (string, error) {
        return result, nil
    },
}
```

构建时可通过 `ToolOption` 配置超时和重试，每个 tool 独立控制：

```go
// DuckDuckGo 搜索：5s 超时，失败重试 2 次
ddg := tool.NewDuckDuckGoTool(
    option.WithTimeout(5 * time.Second),
    option.WithRetryCount(2),
)

// REST API：10s 超时
weather := tool.NewRESTTool(spec,
    option.WithTimeout(10 * time.Second),
)
```

## 两层函数式选项（pkg/option）

支持通用选项和实现特定选项在同一变参中混用：

```go
// ModelOption — 通用选项
resp, _ := model.Generate(ctx, msgs,
    option.WithTemperature(0.7),
    option.WithMaxTokens(1024),
    option.WithToolSpecs(specs...),
)

// ModelOption — vLLM 特有选项（通过 impl-specific 层传递）
resp, _ := model.Generate(ctx, msgs,
    option.WithTemperature(0.7),
    brain.WithGuidedJSON(schema),    // vLLM guided decoding
    brain.WithRepetitionPenalty(1.1),
)

// ToolOption — 构建 tool 时传入，控制超时和重试
ddg := tool.NewDuckDuckGoTool(
    option.WithTimeout(5 * time.Second),
    option.WithRetryCount(2),
)
```

## 回调系统（pkg/callback）

AOP 风格的生命周期钩子，通过 context 注入，无 handler 时零开销。

```go
// 构建 handler
handler := callback.NewHandler().
    Start(func(ctx context.Context, info callback.RunInfo, input any) context.Context {
        fmt.Printf("[%s] started\n", info.Name)
        return ctx
    }).
    End(func(ctx context.Context, info callback.RunInfo, output any) context.Context {
        fmt.Printf("[%s] finished\n", info.Name)
        return ctx
    }).
    Error(func(ctx context.Context, info callback.RunInfo, err error) context.Context {
        fmt.Printf("[%s] error: %v\n", info.Name, err)
        return ctx
    }).
    Build()

// 注入到 context
ctx = callback.Inject(ctx, handler)

// Agent 内部自动分发 OnStart/OnEnd/OnError
// TimingGuard 确保无 handler 时零开销
```

## 核心类型（pkg/core）

### Agent

```go
type Agent interface {
    Identity() string
    Execute(ctx context.Context, state *State) (*protocol.StreamReader[Signal], error)
}
```

Agent 通过 `StreamReader[Signal]` 返回事件流（Think / Invoke / Yield / Reply / Handoff / Fault / Recall / Plan / Synth / Artifact / Gate），调用方可实时消费。

### State

替代旧的 Session，基于 `protocol.Message`：

```go
state := core.NewState("demo", core.WithVault(mv), core.WithShelf(sh))
state.Append(protocol.NewTextMessage("user", "你好"))
history := state.History() // []*protocol.Message
state.Set("key", value)
```

### Vault / Shelf / Gate / Tracer

接口定义详见 `pkg/core/` 下各文件。

### Vault 检索策略（pkg/vault）

Vault 的 `Recall` 支持可插拔检索策略，默认关键词匹配，可替换为向量或混合检索：

```go
// 默认：关键词匹配
mv := vault.NewMemVault()

// 向量检索
mv := vault.NewMemVault(
    vault.WithRetriever(vault.NewVectorRetriever(myEmbedder)),
)

// 混合检索（RRF 融合）
mv := vault.NewMemVault(
    vault.WithRetriever(vault.NewHybridRetriever(
        vault.NewKeywordRetriever(),
        vault.NewVectorRetriever(myEmbedder),
    )),
)

// FileVault 同理
fv, _ := vault.NewFileVault("./data",
    vault.WithFileRetriever(vault.NewVectorRetriever(myEmbedder)),
)
```

`Embedder` 接口只需实现 `Embed(ctx, texts) ([][]float64, error)`，可对接任意嵌入模型。

## Agent 引擎（pkg/engine）

### SoloAgent — 单 Agent ReAct 循环

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

### DeepAgent — 递归分解

```go
deep := &engine.DeepAgent{
    Name:     "analyst",
    Persona:  "你是一个深度分析师。",
    Model:    model,
    Tools:    tools,
    MaxDepth: 3,
}
```

### Conductor — 多 Agent 调度

自动为子 Agent 注入 handoff 工具，支持 Agent 间移交对话：

```go
conductor := engine.NewConductor("team", "researcher", researcher, writer)
```

将 Agent 包装为 Tool：

```go
delegateTool := engine.AsTool(writer)
```

### ParallelAgent — 并发执行

多个 Agent 并发执行，结果合并：

```go
parallel := &engine.ParallelAgent{
    Name:   "multi-search",
    Agents: []core.Agent{agent1, agent2, agent3},
    Merge: func(results map[string][]core.Signal) string {
        // 自定义合并逻辑，nil 时默认拼接所有 Reply
        return merged
    },
}
```

## 内置实现

### Model — OpenAIModel（pkg/brain）

兼容 OpenAI / vLLM / 任意兼容端点，支持多模态消息和 SSE 流式推理：

```go
// OpenAI
m := brain.NewOpenAIModel("https://api.openai.com/v1", "sk-xxx", "gpt-4o")

// vLLM（通常无需 API Key）
m := brain.NewVLLMModel("http://localhost:8000/v1", "Qwen/Qwen2-7B")
```

### Tools（pkg/tool）

```go
// DuckDuckGo 搜索（可选超时和重试）
ddg := tool.NewDuckDuckGoTool(
    option.WithTimeout(5 * time.Second),
    option.WithRetryCount(2),
)

// 声明式 REST API 封装
weather := tool.NewRESTTool(tool.RESTSpec{
    Name:       "get_weather",
    Desc:       "查询天气",
    Method:     "GET",
    URL:        "https://api.weather.com/v1/{city}",
    Headers:    map[string]string{"Authorization": "Bearer xxx"},
    ResultPath: "current.temperature",
}, option.WithTimeout(10 * time.Second))
```

### Trace（pkg/trace）

```go
t := trace.NewMemTracer()

// 包装 Model/Tool 自动注入追踪
model := trace.WrapModel(rawModel, t)
tools := trace.WrapTools(rawTools, t)

// 查看追踪结果
for _, sp := range t.Spans() {
    fmt.Printf("%s %s %v\n", sp.Name, sp.Duration(), sp.TokenUsage())
}
summary := t.Summary()
```

## 示例

- `examples/solo/` — 单 Agent + 工具调用（计算器、天气、搜索）
- `examples/team/` — 多 Agent 编排（Chain、Conductor、DeepAgent）
- `examples/artifact/` — 产物生成与存储

```bash
export OPENAI_API_KEY=sk-xxx
export OPENAI_ENDPOINT=https://api.openai.com/v1
go run examples/solo/main.go
```

## License

Apache-2.0 license
