# Task 030: Message Queuing

## Why
Allow users to type messages and issue slash commands while the model is streaming. Queued messages auto-execute (FIFO) after the current turn ends.

## Implementation

### SDK Changes (`internal/sdk/sdk.go`)
- Added `messageQueue []string` and `overflowCount int` fields to `Session`
- Added methods: `EnqueueMessage()`, `DequeueMessage()`, `PendingCount()`, `OverflowCount()`, `ResetOverflow()`
- Max queue size: 10, FIFO, drops oldest on overflow
- Thread-safe via `s.mu` mutex

### TUI Changes
- `update.go`: `handleKeyPress` enqueues during streaming on Enter
- `model.go`: `handlePromptDone` drains queue, shows overflow warning
- `model.go`: `processSlashCommandStreaming` handles non-blocking commands immediately
- `view.go`: Footer shows queue count
- `render.go`: `blockQueuedMessage` type + rendering
- `styles.go`: `queuedMessageStyle`

### Slash Command Classification (during streaming)
- **Immediate**: `/help`, `/clear`, `/skills`, `/session`
- **Queued**: `/quit`, `/exit`, `/name`, `/compact`, `/model`, `/skill:<name>`, regular text

## Tests
- SDK: 4 tests (enqueue/dequeue, overflow, concurrency, reset)
- TUI: 8 tests (enqueue during streaming, drain queue, empty queue, footer count, overflow warning, slash commands)

## Acceptance Criteria
- [x] User can type messages while model is streaming
- [x] Queue is FIFO, max 10 messages, drops oldest on overflow
- [x] Footer shows queued message count
- [x] Non-blocking slash commands execute immediately during streaming
- [x] Blocking slash commands queue and execute after turn ends
- [x] Overflow warning displayed in viewport
- [x] All existing tests pass
- [x] go vet/build clean
