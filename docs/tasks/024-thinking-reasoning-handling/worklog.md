# Task 024 Worklog

## Session 1: Investigation & Task Definition (prior session)

### What was attempted (failed)
1. Added "· thinking" header to `renderThinkingBlock()` — visual improvement only
2. Set `max_tokens: 8192` in OpenAI-compat provider — no change
3. Created native Ollama provider (`/api/chat`) — separation works for English, Polish still broken
4. Set `num_predict: -1` — no change
5. Set `num_predict: 32768` — no change
6. Added `done_reason` field + debug logging — instrumentation in place

### Real code research
- **PI** uses `AssistantMessageEventStream` with thinking_start/delta/end + text_start/delta/end lifecycle
- **OpenCode (anomalyco)** uses AI SDK V3 event format with `thinking_budget` parameter
- **OpenCode (Go)** uses `EventThinkingDelta` + `EventContentDelta`

### Task document created
- `docs/tasks/024-thinking-reasoning-handling/task.md` — full comparison and acceptance criteria
- `docs/TRACKING.md` — updated task 024 entry

## Session 2: Root Cause Analysis & Verification (2026-05-04)

### Subtask 024.1 — Raw Ollama SSE capture

Captured raw `/api/chat` responses for both Polish and English inputs using `gemma4:26b`.

**Polish input ("cześć"):**
- 166 total SSE chunks, 152 thinking chunks, 13 content chunks, 0 overlap
- Response text: "Cześć! W czym mogę Ci dzisiaj pomóc?"

**English input ("hi"):**
- 90 total SSE chunks, 80 thinking chunks, 9 content chunks, 0 overlap
- Response text: "Hello! How can I help you today?"

**Key finding:** Ollama IS sending response text for both inputs. The raw API works correctly.

### Subtask 024.2 — Root cause identification

Tested all layers:

| Layer | Polish text? | English text? |
|-------|-------------|---------------|
| Raw Ollama SSE | ✅ Yes | ✅ Yes |
| Native Ollama provider | ✅ Yes | ✅ Yes |
| Full SDK path | ✅ Yes | ✅ Yes |
| OpenAI-compat provider | ✅ Yes | ✅ Yes |
| **Print mode e2e** | ✅ Yes | ✅ Yes |
| **Interactive TUI (first attempt)** | ❌ No | — |

The provider/SDK layers work correctly. The bug is in the **TUI event delivery**.

### Session 3: TUI Event Delivery Fix (2026-05-04)

### Root cause found

In `internal/tui/model.go`, the SDK event subscription used a **non-blocking send**:

```go
select {
case m.eventCh <- AgentEventMsg{Event: e}:
default:
    // Channel full — drop (prefer liveness)
}
```

Combined with a **256-event buffer**, long thinking responses (300-500+ events) caused `text_delta` events to be silently dropped. The TUI viewport re-rendering is slower than event production during streaming, so the channel fills up and events are discarded.

### Fix applied

**`internal/tui/model.go`** — two changes:
1. Event channel buffer: `256 → 2048`
2. Event delivery: non-blocking `select` → **blocking send**

```go
// Before (drops events when buffer full)
unsub := m.session.Subscribe(func(e types.AgentEvent) {
    select {
    case m.eventCh <- AgentEventMsg{Event: e}:
    default:
    }
})

// After (blocks if TUI can't keep up — no event loss)
unsub := m.session.Subscribe(func(e types.AgentEvent) {
    m.eventCh <- AgentEventMsg{Event: e}
})
```

### Subtask 024.4 — Regression tests

Added two regression tests:
1. **`TestOllamaProvider_ParseStream_PolishInput_ThinkingThenText`** (provider) — uses real Polish SSE data, verifies thinking→text event sequence
2. **`TestModel_ProcessEvent_PolishInput_ThinkingThenText`** (TUI) — verifies TUI model accumulates Polish Unicode text correctly in both thinking and text blocks

### E2E Verification
| Verification | Status |
|---|---|
| Raw Ollama SSE capture | ✅ |
| Native Ollama provider | ✅ |
| Full SDK path | ✅ |
| Print mode e2e | ✅ "Cześć! W czym mogę Ci dzisiaj pomóc?" |
| JSON mode e2e | ✅ `text_delta` events after `thinking_delta` |
| Interactive TUI e2e | ✅ Fixed — blocking send + larger buffer |

### Cleanup
- Removed temporary test commands
- All tests pass, `go vet`/`go build`/`-race` clean
