# Task 011: Session Management — Design Document

## Package Structure

```
internal/session/
├── storage.go          # JSONL read/write, header, entry serialization
├── session.go          # Session lifecycle (Create, Open, Append, Delete, Resume)
├── naming.go           # Auto-naming strategy
├── compaction.go       # Compaction pure functions
├── migrate.go          # Version migration scaffolding
├── types.go            # Typed payload structs and marshal helpers
├── storage_test.go     # Storage round-trip, corruption recovery
├── session_test.go     # Lifecycle, resume, usage tracking, delete
├── naming_test.go      # Auto-naming edge cases
├── compaction_test.go  # Token estimation, cut points, turn boundaries
└── migrate_test.go     # Version validation
```

**Dependencies**: Only `internal/types`, `internal/testutil` (tests), and stdlib.

---

## 1. types.go — Typed Payload Structs

```go
package session

// MessageData wraps an AgentMessage for JSONL persistence.
type MessageData struct {
    Message types.AgentMessage `json:"message"`
}

// ModelChangeData records a model switch.
type ModelChangeData struct {
    ModelID string `json:"model_id"`
}

// ThinkingLevelChangeData records a thinking level adjustment.
type ThinkingLevelChangeData struct {
    Level types.ThinkingLevel `json:"level"`
}

// CompactionData records a compaction summary.
type CompactionData struct {
    FirstKeptEntryID string `json:"first_kept_entry_id"`
    TokensBefore     int    `json:"tokens_before"`
    Summary          string `json:"summary"`
    Details          string `json:"details,omitempty"`
}

// CustomEntryData for internal metadata (non-LLM-visible).
type CustomEntryData struct {
    Key   string `json:"key"`
    Value string `json:"value"`
}

// CustomMessageData for extension messages (LLM-visible).
type CustomMessageData struct {
    Source  string `json:"source"`
    Content string `json:"content"`
}

// SessionInfoData for session metadata.
type SessionInfoData struct {
    DisplayName string `json:"display_name"`
}

// MarshalEntryData converts a typed payload to json.RawMessage for SessionEntry.Data.
func MarshalEntryData(entryType types.EntryType, data any) (json.RawMessage, error)

// UnmarshalEntryData converts SessionEntry.Data back to the appropriate typed struct.
func UnmarshalEntryData(entryType types.EntryType, raw json.RawMessage) (any, error)
```

---

## 2. storage.go — JSONL Read/Write

### JSONLWriter — Append-only writer
```go
type JSONLWriter struct {
    f   *os.File      // opened O_APPEND|O_CREATE|O_WRONLY
    buf *bufio.Writer // buffered for performance
}

func NewJSONLWriter(path string) (*JSONLWriter, error)
func (w *JSONLWriter) WriteHeader(h types.SessionHeader) error
func (w *JSONLWriter) WriteEntry(e types.SessionEntry) error
func (w *JSONLWriter) Close() error
func (w *JSONLWriter) Sync() error
```

### ReadEntries — Read all entries with corruption recovery
```go
// ReadEntries opens the file, parses the header, and returns all valid entries.
// If the last line is incomplete (corruption), it is silently discarded.
func ReadEntries(path string) (*types.SessionHeader, []types.SessionEntry, error)
```

**Corruption recovery logic**:
1. Read line by line using `bufio.Scanner`
2. First line → parse as `SessionHeader`, validate version
3. Subsequent lines → parse as `SessionEntry`
4. If any line fails to parse as JSON → stop, return what we have (discard incomplete last line)

### File Naming
```go
// GenerateFilename creates a session filename: <timestamp>_<8-char-hex>.jsonl
func GenerateFilename(ts time.Time, id string) string
// Example: "20260503T120000_a3f7b2c1.jsonl"
```

### CWD Encoding
```go
// EncodeCWD replaces "/" with "-" for directory-safe filenames.
func EncodeCWD(cwd string) string
// Example: "/home/adam/Projects/tau" → "-home-adam-Projects-tau"
```

---

## 3. session.go — Session Lifecycle

```go
type Session struct {
    mu            sync.RWMutex
    file          string           // Full path to JSONL file
    header        types.SessionHeader
    messages      []types.AgentMessage  // Rebuilt from JSONL
    entries       []types.SessionEntry  // All entries (for compaction reference)
    usage         types.Usage           // Cumulative usage accumulator
    writer        *JSONLWriter          // nil if opened read-only
    currentModel  types.Model           // Track current model (from model_change entries)
    thinkingLevel types.ThinkingLevel   // Track current thinking level
}
```

### CreateSession
```go
func CreateSession(dirPath string, cwd string, name string, id string) (*Session, error)
```
- Generates unique ID if empty (using `crypto/rand`)
- Creates `types.SessionHeader` with version=1
- Creates JSONL file in `dirPath` with generated filename
- Writes header line
- Returns initialized `Session`

### OpenSession (Resume)
```go
func OpenSession(filePath string) (*Session, error)
```
- Reads all entries via `ReadEntries()`
- Rebuilds message list from entries (per ARCHITECTURE.md §7.2a):
  - `message` → append to messages
  - `model_change` → update `currentModel`
  - `thinking_level_change` → update `thinkingLevel`
  - `compaction` → note for context rebuild
  - `custom_entry` → store in metadata (not messages)
  - `custom_message` → append to messages
  - `session_info` → update header name
- Opens file in append mode for writing
- Returns `Session` ready for use

### Append
```go
func (s *Session) Append(entryType types.EntryType, data any) error
func (s *Session) AppendWithUsage(entryType types.EntryType, data any, usage *types.Usage) error
```
- Creates `SessionEntry` with ID, timestamp
- Marshals data to `json.RawMessage`
- Appends to JSONL via writer
- Updates in-memory state (messages, usage, etc.)

### Usage
```go
func (s *Session) Usage() types.Usage
```
- Returns cumulative usage accumulator

### Messages
```go
func (s *Session) Messages() []types.AgentMessage
```
- Returns deep copy of message list

### Delete
```go
func (s *Session) Delete() error
```
- Closes writer
- Removes session file from disk

### Close
```go
func (s *Session) Close() error
```
- Flushes and closes writer

### Accessors
```go
func (s *Session) ID() string
func (s *Session) Name() string
func (s *Session) Cwd() string
func (s *Session) File() string
func (s *Session) CurrentModel() types.Model
func (s *Session) CurrentThinkingLevel() types.ThinkingLevel
```

---

## 4. naming.go — Auto-Naming

```go
// AutoName generates a session name from the first user message.
// - Strips special characters (keeps alphanumeric, spaces, hyphens)
// - Truncates to 50 characters
// - Converts to lowercase
// - Collapses multiple spaces/hyphens to single
// - Trims leading/trailing whitespace and hyphens
// Returns fallback "YYYY-MM-DD-HHMMSS" if input is empty.
func AutoName(firstUserMessage string, ts time.Time) string
```

**Examples**:
- `"Hello world"` → `"hello-world"`
- `"Let's build a REST API in Go!!"` → `"lets-build-a-rest-api-in-go"`
- 60-char message → truncated to 50 chars
- `""` → `"2026-05-03-120000"`

---

## 5. compaction.go — Pure Functions

**No I/O. No provider calls. Pure functions only.**

### Token Estimation
```go
// EstimateTokens estimates token count for a message using per-type heuristics.
// - Text: chars / 4
// - Tool results: chars / 3
// - Thinking: chars / 3.5
func EstimateTokens(msg types.AgentMessage) int
```

### EstimateMessageTokens — helper for single message
```go
func EstimateMessageTokens(msg types.AgentMessage) int
```

### Cut Point Finding
```go
// FindCutPoint walks backwards from the end of messages, accumulating token
// estimates. Returns the index where tokens exceed budget.
// Returns -1 if all messages fit within budget.
func FindCutPoint(messages []types.AgentMessage, budget int) int
```

### Turn Boundary Constraint
```go
// AdjustToTurnBoundary adjusts cutIndex to never split a tool call from its result.
// A turn boundary is at:
//   - After a user message (start of new turn)
//   - After a complete turn (user → assistant → tool results)
// Never cuts mid-tool-call-batch or between tool call and result.
// Returns adjusted index, or -1 if no valid cut point exists.
func AdjustToTurnBoundary(messages []types.AgentMessage, cutIndex int) int
```

**Turn detection logic**:
- Walk forward from cutIndex to find the next "safe" boundary
- A safe boundary is: after a `tool_result` message that follows an `assistant` message with tool calls, OR before a `user` message
- If cutIndex falls between a tool call and its result, advance past all results

### Compaction Entry Builder
```go
// BuildCompactionEntry creates a compaction SessionEntry.
// - firstKeptEntryID: ID of first message to keep
// - tokensBefore: token count before compaction
// - summary: structured summary text (provided by caller/SDK)
func BuildCompactionEntry(firstKeptEntryID string, tokensBefore int, summary string) types.SessionEntry
```

### ShouldCompact
```go
// ShouldCompact checks if total estimated tokens exceed the threshold.
// threshold = contextWindow - reserveTokens
func ShouldCompact(messages []types.AgentMessage, contextWindow int, reserveTokens int) bool
```

---

## 6. migrate.go — Version Migration

```go
// ValidateVersion checks the session header version.
// Returns nil if version is supported, error otherwise.
func ValidateVersion(version int) error

// migrateV1ToV2 is a stub for future version migration.
// Currently returns the entries unchanged (v1 is the only version).
func migrateV1ToV2(header *types.SessionHeader, entries []types.SessionEntry) (*types.SessionHeader, []types.SessionEntry, error)
```

---

## Testing Strategy (TDD)

### storage_test.go
1. **Round-trip**: Create writer → write header → write entry → close → read → verify
2. **Multiple entries**: Write 10 entries → read → verify order preserved
3. **Corruption**: Write 3 valid entries + partial line → read → verify 3 entries recovered, no error
4. **Empty file**: Read empty file → error (no header)
5. **Invalid header**: File with invalid JSON on first line → error

### session_test.go
1. **Create**: New session → verify file exists, header written, ID set
2. **Append**: Append message → verify file has 2 lines (header + entry)
3. **Resume**: Create session → append messages → close → reopen → verify messages match
4. **Usage tracking**: Append entries with usage → verify cumulative total
5. **Delete**: Create → delete → verify file removed
6. **Concurrency**: Multiple goroutines append → verify no data loss

### naming_test.go
1. Simple message → lowercase with hyphens
2. Message with special chars → stripped
3. Long message → truncated to 50
4. Empty message → timestamp fallback
5. Edge cases: all special chars, Unicode, numbers only

### compaction_test.go
1. **Token estimation**: text 4 chars → 1 token; tool 3 chars → 1 token; thinking 3.5 chars → 1 token
2. **Cut point**: Messages exceeding budget → correct cut index
3. **Turn boundary**: Cut point mid-tool-call → adjusted past all results
4. **No compaction needed**: Messages within budget → return -1
5. **Single turn exceeds budget**: Edge case handling
6. **ShouldCompact**: Above threshold → true; below → false

### migrate_test.go
1. Version 1 → valid
2. Version 0 → error
3. Version 99 → error
4. migrateV1ToV2 → returns unchanged

---

## Implementation Order (TDD)

1. **types.go + naming.go** + tests — Simplest, no dependencies
2. **storage.go** + tests — Foundation for session lifecycle
3. **session.go** + tests — Depends on storage
4. **compaction.go** + tests — Pure functions, independent
5. **migrate.go** + tests — Simple validation
