# Task 024: Thinking/Reasoning Rendering Investigation

## Why

When using Ollama with `gemma4:26b` (a thinking model), the chat rendering shows thinking blocks but the actual model response text is missing for some inputs.

**Observed behavior (facts only):**
- Polish greeting ("cześć"): thinking block with structured reasoning, cut off mid-sentence, **no response text follows**
- English greeting ("hi"): thinking block completes, **response text appears correctly**
- Both inputs are in the same conversation, 256K context window, fresh session
- Both inputs are short greetings (2-3 tokens)

## What Was Checked and Failed

| # | Change | File(s) | Result |
|---|--------|---------|--------|
| 1 | Added "· thinking" header to thinking blocks | `render.go` | Visual improvement only, response still missing |
| 2 | Set `max_tokens: 8192` in OpenAI-compat provider | `openai_compat.go` | No change |
| 3 | Created native Ollama provider (`/api/chat`) | `ollama.go` | Separation works for English, Polish still broken |
| 4 | Set `num_predict: -1` (unlimited) | `ollama.go` | No change |
| 5 | Set `num_predict: 32768` | `ollama.go` | No change |
| 6 | Added `done_reason` field + debug logging | `ollama.go` | Instrumentation in place, awaiting data |

## Real Code Comparison

### PI (PI Coding Agent)

**Repo:** https://github.com/mariozechner/pi-coding-agent
**Source:** `node_modules/@mariozechner/pi-ai/dist/providers/openai-completions.js`

PI uses an `AssistantMessageEventStream` with explicit block lifecycle events:
- `thinking_start`, `thinking_delta`, `thinking_end`
- `text_start`, `text_delta`, `text_end`
- `toolcall_start`, `toolcall_delta`, `toolcall_end`

Block switching logic (real code from `openai-completions.js`):
```javascript
let currentBlock = null;
const blocks = output.content;

// Text content
if (choice.delta.content !== null && choice.delta.content !== undefined && choice.delta.content.length > 0) {
    if (!currentBlock || currentBlock.type !== "text") {
        finishCurrentBlock(currentBlock);  // emits text_end on old block
        currentBlock = { type: "text", text: "" };
        blocks.push(currentBlock);
        stream.push({ type: "text_start", contentIndex: currentContentIndex(), partial: output });
    }
    currentBlock.text += choice.delta.content;
    stream.push({ type: "text_delta", contentIndex: currentContentIndex(), delta: choice.delta.content, partial: output });
}

// Reasoning: checks reasoning_content, reasoning, reasoning_text
const reasoningFields = ["reasoning_content", "reasoning", "reasoning_text"];
for (const field of reasoningFields) {
    if (choice.delta[field] !== null && choice.delta[field] !== undefined && choice.delta[field].length > 0) {
        if (!currentBlock || currentBlock.type !== "thinking") {
            finishCurrentBlock(currentBlock);
            currentBlock = { type: "thinking", thinking: "", thinkingSignature: field };
            blocks.push(currentBlock);
            stream.push({ type: "thinking_start", contentIndex: currentContentIndex(), partial: output });
        }
        currentBlock.thinking += choice.delta[field];
        stream.push({ type: "thinking_delta", contentIndex: currentContentIndex(), delta: choice.delta[field], partial: output });
    }
}
```

Agent loop (real code from `agent-loop.js`):
```javascript
case "thinking_start":
case "thinking_delta":
case "thinking_end":
case "text_start":
case "text_delta":
case "text_end":
    if (partialMessage) {
        partialMessage = event.partial;  // FULL accumulated message on every event
        context.messages[context.messages.length - 1] = partialMessage;
        await emit({ type: "message_update", assistantMessageEvent: event, message: { ...partialMessage } });
    }
```

**Key observation:** Every delta event carries `event.partial` — the full accumulated message. The UI always has complete state.

### OpenCode (anomalyco/opencode)

**Repo:** https://github.com/anomalyco/opencode
**Source:** `packages/opencode/src/provider/sdk/copilot/chat/openai-compatible-chat-language-model.ts`

OpenCode uses AI SDK V3 event stream format. Key streaming code (real):
```typescript
// reasoning-start fires when reasoning_text first appears
const reasoningContent = delta.reasoning_text
if (reasoningContent) {
    if (!isActiveReasoning) {
        controller.enqueue({ type: "reasoning-start", id: "reasoning-0" })
        isActiveReasoning = true
    }
    controller.enqueue({ type: "reasoning-delta", id: "reasoning-0", delta: reasoningContent })
}

// When content appears after reasoning, reasoning-end fires first
if (delta.content) {
    if (isActiveReasoning && !isActiveText) {
        controller.enqueue({ type: "reasoning-end", id: "reasoning-0",
            providerMetadata: reasoningOpaque ? { copilot: { reasoningOpaque } } : undefined })
        isActiveReasoning = false
    }
    if (!isActiveText) {
        controller.enqueue({ type: "text-start", id: "text-0" })
        isActiveText = true
    }
    controller.enqueue({ type: "text-delta", id: "text-0", text: delta.content })
}
```

Request parameters (real code):
```typescript
thinking_budget: compatibleOptions.thinking_budget,  // separate budget for thinking
```

Part types (real code from `packages/sdk/js/src/v2/gen/types.gen.ts`):
```typescript
export type ReasoningPart = {
    id: string; sessionID: string; messageID: string;
    type: "reasoning"; text: string; metadata?: { [key: string]: unknown };
    time: { created: number; first: number; last: number };
}
export type Part = TextPart | ReasoningPart | ToolPart | FilePart | StepStartPart | ...
```

UI rendering (real code from `packages/ui/src/components/message-part.tsx`):
```typescript
PART_MAPPING["reasoning"] = function ReasoningPartDisplay(props) {
    const part = () => props.part as ReasoningPart
    const streaming = () => props.message.role === "assistant" && typeof props.message.time.completed !== "number"
    const text = () => part().text.trim()
    return (
        <Show when={text()}>
            <div data-component="reasoning-part">
                <Show when={streaming()} fallback={<Markdown text={text()} />}>
                    <PacedMarkdown text={text()} streaming={streaming()} />
                </Show>
            </div>
        </Show>
    )
}
```

**Key observations:**
- OpenCode uses `thinking_budget` parameter (separate from `max_tokens`)
- OpenCode checks `reasoning_text` and `reasoning_opaque` fields (Copilot-specific)
- Uses AI SDK V3 typed event stream with explicit start/end lifecycle
- Part-based rendering (each part type has its own component)

### OpenCode (opencode-ai/opencode) — Go implementation

**Repo:** https://github.com/opencode-ai/opencode (different from anomalyco)
**Source:** `/tmp/opencode/`

Provider events (`internal/llm/provider/provider.go`):
```go
const (
    EventContentStart  EventType = "content_start"
    EventContentDelta  EventType = "content_delta"
    EventThinkingDelta EventType = "thinking_delta"
    EventContentStop   EventType = "content_stop"
)

type ProviderEvent struct {
    Type     EventType
    Content  string    // for content_delta
    Thinking string    // for thinking_delta
    Response *ProviderResponse
}
```

Message model (`internal/message/content.go`):
```go
type ReasoningContent struct { Thinking string `json:"thinking"` }
type TextContent struct { Text string `json:"text"` }

func (m *Message) IsThinking() bool {
    return m.ReasoningContent().Thinking != "" && m.Content().Text == "" && !m.IsFinished()
}
```

### Tau (current)

**Source:** `internal/provider/ollama.go`, `internal/provider/openai_compat.go`, `internal/agent/loop.go`, `internal/tui/model.go`, `internal/tui/render.go`

Event flow:
```
Provider StreamEvent → Agent AgentEvent → TUI processEvent → messageBlock[] → renderBlocks
```

Block switching (real code from `model.go`):
```go
case types.AgentEventThinkingDelta:
    m.ensurePending(blockThinking)      // flushes previous kind if different
    m.pendingBuilder.WriteString(text)

case types.AgentEventTextDelta:
    m.ensurePending(blockAssistantText) // flushes previous kind if different
    m.pendingBuilder.WriteString(text)
```

### Comparison Summary

| Aspect | PI | OpenCode (anomalyco) | OpenCode (Go) | Tau |
|--------|---|----------|--------|---|
| Event model | thinking_start/delta/end, text_start/delta/end | reasoning-start/delta/end, text-start/delta | EventContentDelta, EventThinkingDelta | AgentEventThinkingDelta, AgentEventTextDelta |
| start/end events | Yes | Yes | Partial (no start) | No |
| State on delta | `event.partial` (full message) | Accumulated via controller | Message.Parts array | pendingBuilder + blocks slice |
| Thinking budget | max_completion_tokens | thinking_budget | num_predict | num_predict (native), max_tokens (compat) |
| Reasoning fields | reasoning_content, reasoning, reasoning_text | reasoning_text, reasoning_opaque | thinking (Ollama native) | reasoning_content, reasoning, reasoning_text |
| Rendering | Partial message replacement | Part-based with components | IsThinking() check | Block-based with header |

## What Needs Investigation

1. **Raw Ollama SSE response** — capture `/api/chat` output for both Polish and English inputs to see which fields contain what data
2. **Thinking budget** — OpenCode uses `thinking_budget` parameter; PI uses `max_completion_tokens`; we use `num_predict`. Is there an Ollama-native parameter we're missing?
3. **Field mapping** — Does Ollama's `/api/chat` response use `thinking` field or something else? Does it use `content` for response text?
4. **Event lifecycle** — Both PI and OpenCode use explicit start/end events. Tau doesn't. Does this matter?

## Acceptance Criteria

- [x] Raw Ollama `/api/chat` SSE response captured for both Polish and English inputs
- [x] Field-level comparison: which fields contain what data for each input
- [x] Root cause identified with evidence — TUI event channel drops events during long thinking
- [x] Fix implemented and verified with both Polish and English inputs — blocking send + larger buffer
- [x] Test added covering thinking + text separation for thinking models

## Subtasks

- [x] **024.1** — Capture raw Ollama `/api/chat` SSE response
- [x] **024.2** — Identify root cause — event channel drops during long thinking blocks
- [x] **024.3** — Implement fix — blocking send + 256→2048 buffer in `model.go`
- [x] **024.4** — Add regression tests — provider + TUI tests with Polish Unicode content
