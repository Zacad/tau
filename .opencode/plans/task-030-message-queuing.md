# Task 030: Message Queuing

## Why
Currently, the TUI blocks user input while the model is streaming. Users cannot type their next message or issue slash commands until the agent finishes. This creates a poor UX — users must wait idly instead of preparing their next turn.

## Constraints
- `Session.Prompt()` is blocking and holds `s.mu`; TUI cannot call it again until it returns.
- Existing `Steer()` mechanism is for mid-turn interruption (during tool execution), not next-turn queuing.
- Agent loop transitions to `StateDone` after reaching DONE; a new `Prompt()` call is required for a new turn.
- Must follow Bubbletea idiomatic patterns (message-driven, no goroutine direct state mutation).
- Max queue size: 10 messages (FIFO, drop oldest on overflow).

## Design: TUI-Level Queue (Solution A)

### Architecture
Queue state lives entirely in the TUI `Model`. No SDK/Agent changes needed.

```
┌─────────────────────────────────────────┐
│  TUI Model                               │
│                                          │
│  pendingMessages []queuedMessage         │
│  deferredCmds    []deferredCommand       │
│                                          │
│  ┌──────────────┐    ┌────────────────┐  │
│  │ handleKeyPress│───▶│ queue or send  │  │
│  └──────────────┘    └────────────────┘  │
│                                          │
│  ┌──────────────┐    ┌────────────────┐  │
│  │ PromptDoneMsg │───▶│ drain queue    │  │
│  └──────────────┘    └────────────────┘  │
└─────────────────────────────────────────┘
```

### Key Changes

#### 1. Model State (`model.go`)
Add to `Model`:
```go
type queuedMessage struct {
    text string
    isSlashCmd bool
}

// In Model struct:
pendingMessages []queuedMessage  // max 10, FIFO
queueOverflowWarned bool         // track if warning shown
```

#### 2. Key Handling (`update.go` — `handleKeyPress`)
- **Enter while streaming**: Queue the input text instead of ignoring it.
- **Slash commands while streaming**: 
  - Non-blocking (`/help`, `/clear`, `/skills`, `/skill:<name>`): Execute immediately.
  - Blocking (`/quit`, `/name`, `/session`, `/compact`, `/model`): Queue for execution after turn ends.
- **Input behavior**: Don't blur input during streaming; keep it focused so user can type.

#### 3. Prompt Done Handler (`model.go` — `handlePromptDone`)
After returning to idle, check `pendingMessages`:
- If queue has messages, pop the first one and call `submitPrompt()`.
- If queue had overflow, show a one-time warning message in the viewport.

#### 4. Visual Feedback (`view.go` — `renderFooter`)
- Footer shows queued count: `idle (3 queued)` or `working (2 queued)`.
- Queued messages appear in viewport immediately as `[Queued] message preview...`.

#### 5. Queue Overflow
When queue reaches 10 and a new message arrives:
- Drop the oldest message.
- Set `queueOverflowWarned = true`.
- Show warning in footer: `queue full, dropped oldest`.

### Slash Command Classification

| Command | Behavior During Streaming |
|---------|--------------------------|
| `/quit`, `/exit` | Queue (blocking) |
| `/help` | Execute immediately |
| `/clear` | Execute immediately |
| `/skills` | Execute immediately |
| `/skill:<name>` | Queue (submits prompt) |
| `/name <name>` | Queue (blocking) |
| `/session` | Execute immediately |
| `/compact` | Queue (blocking) |
| `/model` | Queue (opens selector) |

### Implementation Steps

1. **Add queue types and fields** to `model.go`
2. **Update `handleKeyPress`** in `update.go` to queue during streaming
3. **Update `handlePromptDone`** to auto-submit queued messages
4. **Update `renderFooter`** to show queue count
5. **Add queued message preview** to viewport rendering
6. **Handle queue overflow** with warning
7. **Write unit tests**
8. **E2E verification** with Ollama

### Testing Strategy

#### Unit Tests
- Queue enqueue/dequeue behavior
- Overflow drops oldest
- Slash command classification (blocking vs immediate)
- `handlePromptDone` drains queue correctly
- Footer shows correct queue count
- Queued message preview renders correctly

#### E2E Tests
- Type message during streaming → auto-submits after turn ends
- Queue multiple messages → all execute in order
- Queue overflow → oldest dropped, warning shown
- Slash command during streaming → correct behavior per classification

## Acceptance Criteria
1. User can type messages while model is streaming; they queue and auto-execute after turn ends.
2. Queue is FIFO, max 10 messages, drops oldest on overflow with visible warning.
3. Footer shows queued message count.
4. Non-blocking slash commands (`/help`, `/clear`, `/skills`) execute immediately during streaming.
5. Blocking slash commands queue and execute after turn ends.
6. All existing functionality continues to work (abort, Ctrl+C, Ctrl+D, session resume, etc.).
7. Unit tests cover queue behavior, overflow, and slash command classification.
8. E2E verification with Ollama confirms queued messages execute correctly.
