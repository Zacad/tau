# Task 006 Design Review — Critical Analysis

## Summary

The proposed design is **mostly sound** but has several gaps that will cause rework in downstream tasks. The biggest issues are: missing shared types that Task 012 (Agent Loop) and Task 013 (SDK) will need, untyped string enums that will cause bugs, and a TDD strategy that underestimates what "testing pure data types" actually means.

---

## 1. Missing Types and Fields

### 1.1 `AgentEvent` — Critical Gap

Task 012.2 requires `AgentEvent` types for the event subscription system. The agent loop produces events (`message_start`, `text_delta`, `tool_execution_start`, etc.) and the SDK subscribes to them. This is a **shared type** — produced by `agent/`, consumed by `sdk/` and `cmd/tau/`. It must live in `types/`.

**Missing from proposed design:**
```go
// AgentEvent is emitted by the agent loop at each state transition.
type AgentEvent struct {
    Type    string       // event type (see constants below)
    Data    any          // event-specific payload
    SubAgent *string     // subagent ID if event originated from subagent
}
```

Task 013's `Subscribe(listener func(AgentEvent))` depends on this. Without it, Tasks 012 and 013 will need to retrofit the type into `types/`, forcing a refactor of all event-handling code.

### 1.2 Role, StreamEventType, ContentBlockType — Untyped String Fields

The design uses bare `string` for `AgentMessage.Role`, `StreamEvent.Type`, and `ContentBlock.Type`. This is a bug magnet. Downstream tasks will compare these with raw strings (`"user"`, `"text_delta"`, etc.), creating typo bugs that the compiler won't catch.

**Should be:**
```go
type MessageRole string
const (
    RoleUser      MessageRole = "user"
    RoleAssistant MessageRole = "assistant"
    RoleTool      MessageRole = "tool"
)

type StreamEventType string
const (
    EventStart        StreamEventType = "start"
    EventTextDelta    StreamEventType = "text_delta"
    EventThinkingDelta StreamEventType = "thinking_delta"
    EventToolCallStart StreamEventType = "toolcall_start"
    EventToolCallEnd  StreamEventType = "toolcall_end"
    EventDone         StreamEventType = "done"
    EventError        StreamEventType = "error"
)

type ContentBlockType string
const (
    BlockText      ContentBlockType = "text"
    BlockThinking  ContentBlockType = "thinking"
    BlockToolCall  ContentBlockType = "tool_call"
    BlockImage     ContentBlockType = "image"
)
```

ARCHITECTURE.md §8.1 already documents `ContentBlock.Type` as `"text" | "thinking" | "tool_call" | "image"`. The `"thinking"` type is **missing from the proposed file structure** — only `Text`, `ToolCall`, and `Image` are listed.

### 1.3 `AgentMessage.Role` and `AgentMessage.Model` Type Mismatch

ARCHITECTURE.md §8.1 shows `AgentMessage.Model` as `string` (model ID). The proposed design has `Model` as a field but doesn't clarify whether it's the model ID string or the full `Model` struct. For session persistence (Task 011), storing the full `Model` struct in each message is wasteful — it should be the model ID string. **Confirm it's a `string` field.**

### 1.4 `SessionHeader` Type

ARCHITECTURE.md §7.1 defines a session header with distinct fields (`version`, `id`, `cwd`, `name`) separate from `SessionEntry`. The header is the first line of the JSONL file; everything else is a `SessionEntry`. The proposed design only has `SessionEntry`. Task 011 (Session Management) will need a dedicated header type:

```go
type SessionHeader struct {
    Type      string    `json:"type"` // always "session"
    Version   int       `json:"version"`
    ID        string    `json:"id"`
    Timestamp time.Time `json:"timestamp"`
    Cwd       string    `json:"cwd"`
    Name      string    `json:"name"`
}
```

### 1.5 `BeforeToolCallContext` / `AfterToolCallContext`

ARCHITECTURE.md §3.2 shows the Agent struct has `beforeToolCall` and `afterToolCall` hooks with context/result types. These are referenced by Task 012.7. They should be in `types/` since the hook signatures are part of the Agent struct's public API.

### 1.6 `SubAgentResult` Struct

ARCHITECTURE.md §5.5 defines `SubAgentResult` with `Success`, `Output`, `Artifacts`, `Error`, `Duration`, `Usage`. Task 010 needs this. It depends on `types.Usage`. While the struct itself lives in `subagent/`, the **type must be usable by `types/`** for result injection into the message list. Consider whether result injection needs a dedicated message type in `types/`.

### 1.7 `SessionEntry.Type` Constants

ARCHITECTURE.md §7.1 lists 7 entry types: `message`, `model_change`, `thinking_level_change`, `compaction`, `custom_entry`, `custom_message`, `session_info`, plus `session` for the header. These should be constants, not magic strings.

### 1.8 `MessageRole` for `tool_result`

ARCHITECTURE.md §8.1 shows `Role` as `"user" | "assistant" | "tool_result"`. The proposed design doesn't enumerate the role values. Task 011's resume algorithm needs to distinguish these roles when rebuilding the message list.

---

## 2. API Design Flaws

### 2.1 `StreamEvent.Error` Cannot Be JSON-Serialized

```go
type StreamEvent struct {
    // ...
    Error error `json:"error"` // BUG: error interface doesn't serialize to JSON
}
```

When `StreamEvent` is persisted (Task 011 session storage) or transmitted across goroutines for logging/debugging, the `error` field will serialize to `null`. Either:
- Use `Error string` and construct errors at the call site, OR
- Use a custom `type ErrorDetail struct { Code string; Message string }` with proper JSON marshaling

**Recommendation:** `Error string` — simpler, serializable, and `fmt.Errorf("provider error: %s", e.Error)` reconstructs the error easily.

### 2.2 Missing JSON Tags on Persisted Types

Types that cross the JSONL serialization boundary need explicit `json:"..."` tags:

| Type | Needs Tags | Reason |
|---|---|---|
| `SessionEntry` | YES | Written to JSONL (Task 011) |
| `SessionHeader` | YES | Written to JSONL (Task 011) |
| `AgentMessage` | YES | Serialized in `SessionEntry.Data` for `message` entries |
| `ToolResult` | YES | Serialized in `SessionEntry.Data` for tool result entries |
| `Usage` | YES | Serialized in session entries |
| `Model` | Partially | Only if persisted to config; may not need all fields tagged |
| `StreamEvent` | YES | If used for JSON-mode CLI output (ARCHITECTURE.md §10.5) |

**Critical omission:** `SessionEntry.Data` as `json.RawMessage` is correct for the generic envelope, but the nested types (AgentMessage, ToolResult, etc.) that populate it need tags too.

### 2.3 `StreamEvent.Message` — Naming Collision

ARCHITECTURE.md §6.5 shows `Message *AssistantMessage`. The types list uses `AgentMessage`. These are different concepts:
- `AgentMessage` = a complete persisted message in the transcript
- `AssistantMessage` in `StreamEvent` = the **accumulating** message being built during streaming

The `StreamEvent.Message` field should be `*AgentMessage` (reusing the type), but its semantic meaning is "message so far during this streaming response." The naming is confusing. Consider `StreamEvent.Partial *AgentMessage` or keep `Message` but document that it's the accumulating result, not a completed message.

### 2.4 `ToolDefinition.Parameters` as `*jsonschema.Schema`

Placing `ToolDefinition` in `types/` is **correct** — it avoids import cycles because both `provider/` and `tools/` need it. However, this means `types/` imports `github.com/invopop/jsonschema`, making it not a pure-stdlib package. This is acceptable given the constraint of 3 external deps, but it should be documented as an intentional exception.

**Alternative considered:** Define `ToolDefinition` with `Parameters any` and let `provider/` cast to `*jsonschema.Schema`. This avoids the import but loses compile-time safety. **Stick with the current approach** — the import is justified.

### 2.5 `ContentBlock` Uses Pointers for Optional Fields

```go
type ContentBlock struct {
    Type     string
    Text     string
    ToolCall *ToolCallBlock  // pointer — correct, only set when Type=="tool_call"
    Image    *ImageBlock     // pointer — correct, only set when Type=="image"
}
```

This is correct Go idiomatic design. But note: if `ContentBlockType` becomes a typed constant (see §2.2), the discriminator field becomes safer.

### 2.6 `Model` Struct Is Overloaded

`Model` carries both runtime data (ID, Name, Provider, BaseURL) and configuration data (Cost, Headers, Compat). Task 013's `SetModel()` with pattern matching needs the full struct, but config loading (Task 006.4) might only need a subset. Consider whether `Model` should have a `NewModel()` constructor that validates required fields (ID, Name, Provider).

### 2.7 `BashExecution` Is Not Used by `types/` Directly

`BashExecution` is defined in ARCHITECTURE.md §8.1 but is a **tool-internal detail**. It's not part of any persisted message or session entry. The bash tool (Task 008.8) creates it internally, but it's never exposed through `types/`. Consider whether it belongs in `tools/` instead. If it's only used to populate `ToolResult.Content` (as a text `ContentBlock`), it doesn't need to be in `types/`.

**However**, if future tasks need to serialize bash execution details separately from the tool result, keeping it in `types/` makes sense. **Recommendation:** Keep it in `types/` but add a comment explaining it's for structured tool result metadata.

---

## 3. Testing Gaps

### 3.1 JSON Serialization Round-Trip Tests Are Insufficient

The task.md mentions JSON round-trip tests for `SessionEntry`, `AgentMessage`, `ToolResult`. This is good, but needs expansion:

- **`json.RawMessage` preservation:** When `SessionEntry.Data` contains nested JSON, verify that unmarshal → marshal preserves byte-exact equality (or at least semantic equality). This matters for corruption recovery in Task 011.
- **Omit empty fields:** Test that `json:",omitempty"` is applied correctly so zero-value fields don't bloat JSONL files.
- **Time serialization:** `time.Time` serializes as RFC 3339 by default. Verify this works correctly for resume/reconstruct scenarios.
- **Nil pointer fields:** Verify that `nil` `ToolCall`/`Image` pointers in `ContentBlock` serialize correctly (as absent fields, not `null`).

### 3.2 Config Loading Tests Miss Key Scenarios

| Scenario | Covered? | Risk |
|---|---|---|
| Missing `~/.tau/` directory entirely | No | LoadConfig() might error instead of returning defaults |
| `~/.tau/config.json` exists but is empty `{}` | Partially | Defaults should apply |
| `~/.tau/config.json` has partial config (only `default_model` set) | No | Unset fields should merge with defaults |
| `~/.tau/config.json` is not valid JSON | Partially | Error should be descriptive |
| `~/.tau/config.json` has unknown fields | No | Should be silently ignored (JSON default behavior) |
| `auth.json` has `0777` permissions | No | Security issue — should warn or reject |
| `HOME` environment variable not set | No | `os.UserHomeDir()` returns error |
| `~/.tau/` is a symlink | No | Follow or reject? |

### 3.3 CWD Encoding Tests Are Incomplete

| Input | Expected | Edge Case |
|---|---|---|
| `/` | `-` or `--`? | Root path ambiguity |
| `/a` | `-a-` | Single directory |
| `/a/b/c` | `-a-b-c-` | Normal case |
| `/home/user/my project` | `-home-user-my project-` | Spaces in path |
| `/home/user/café` | `-home-user-café-` | Unicode |
| `` (empty string) | `--` or error? | Empty input |
| `/home/user/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p` | Very long encoded name | Filesystem name length limits (255 bytes on ext4) |

**The root path `/` is the most important edge case.** If the algorithm is: strip leading `/`, replace remaining `/` with `-`, wrap with `-`, then:
- `/` → strip leading → `` → replace → `` → wrap → `--`
- `/home/adam` → strip → `home/adam` → replace → `home-adam` → wrap → `-home-adam-`

This is inconsistent: root becomes `--` while everything else has meaningful content. Consider special-casing root to `root` or `_root_`.

### 3.4 MockProvider Race Condition Risk

If `MockProvider.Stream()` returns a pre-created channel and tests write to it from multiple goroutines, there's a race condition unless the channel is closed properly. The test strategy should specify:

```go
// MockProvider should:
// 1. Accept a []StreamEvent or func() []StreamEvent
// 2. Create a buffered channel (buffer >= len(events))
// 3. Send all events, then close the channel
// 4. Never block on send
```

### 3.5 MockTool Thread Safety

Task 008 requires concurrent tool execution (parallel tools run via `errgroup`). `MockTool` stores calls for assertion. If called concurrently, the call log must be protected by a mutex:

```go
type MockTool struct {
    mu      sync.Mutex
    Calls   []MockToolCall
    Result  *types.ToolResult
    Err     error
}
```

Without this, `go test -race` will fail.

### 3.6 `testutil` Self-Testing

The task.md says "testutil: MockProvider streams events on channel, MockTool executes with params and returns result." But `testutil` depends on `types/`. This means `testutil` tests can't use `testutil` (circular). `testutil` tests must be self-contained, testing only the temp filesystem helpers and verifying that mocks satisfy the expected interfaces (if interfaces exist). Since `Tool` interface isn't in `types/` (correctly deferred to Task 008), MockTool can't verify it implements anything yet. This means MockTool testing is limited to "does it return what I configured?"

---

## 4. Config Loading / Path Resolution Edge Cases

### 4.1 `~` Resolution

Go's `os.UserHomeDir()` is the right choice. But what if `HOME` is unset? In containers or CI environments, this is common. `LoadConfig()` should:
1. Try `os.UserHomeDir()`
2. If it fails, return zero-value defaults (not an error)
3. Log a warning

### 4.2 Config File Permissions

`auth.json` must have `0600` permissions (ARCHITECTURE.md §6.3). Task 006's config loading should:
- Check permissions of `auth.json` if it exists
- Warn (via slog) if permissions are too open
- Not refuse to load (user might have set it up correctly but filesystem doesn't enforce)

### 4.3 `ContextFileSearchList()` — Path Walking

Walking up from cwd through parent directories:
- What if cwd doesn't exist? → Return empty list, log warning
- What if a parent directory is not readable? → Stop walking at that point, return what was found so far
- What about symlinks in the path? → Resolve symlinks to real paths before encoding, to ensure session directory names are consistent regardless of how the user arrived at the directory
- What if cwd is `/`? → The loop terminates at root immediately

### 4.4 Config Default Merging Strategy

When `config.json` is missing, all defaults apply. When it exists but has only some fields, the defaults for missing fields should be preserved. This requires either:
1. Load into a struct with zero values, then apply defaults for zero-valued fields (problematic if zero is a valid value), OR
2. Pre-populate struct with defaults, then unmarshal JSON on top (preferred — `json.Unmarshal` overwrites only present fields)

**Recommendation:** Option 2. This is idiomatic Go and handles partial configs correctly.

### 4.5 Path Constants

The proposed design doesn't specify where path constants live. Consider:

```go
const (
    TauDir    = ".tau"
    SkillsDir    = "skills"
    AgentsDir    = ".agents"
    ConfigFile   = "config.json"
    AuthFile     = "auth.json"
    SessionsDir  = "sessions"
)
```

These should be in `config/paths.go` so they're not hardcoded throughout the codebase.

---

## 5. Deviation from Go Idioms

### 5.1 String-Typed Discriminators Without Constants

Using bare `string` for `AgentMessage.Role`, `ContentBlock.Type`, `StreamEvent.Type`, and `SessionEntry.Type` is the **most significant idiomatic deviation**. Go idioms prefer typed constants for closed sets of string values. This isn't just style — it prevents bugs:

```go
// Bug-prone (current design):
if msg.Role == "tool_reslut" { ... }  // typo, compiles fine

// Type-safe (recommended):
if msg.Role == RoleToolResult { ... }  // typo won't compile
```

This is especially critical because these values are used in switch statements across multiple packages (agent, session, sdk, cli).

### 5.2 `map[string]any` in `ToolCallBlock.Arguments` and `Model.Compat`

`ToolCallBlock.Arguments` as `map[string]any` is correct — LLM tool arguments are dynamic and unknown at compile time. `Model.Compat` as `map[string]any` is also acceptable for compatibility overrides. These are idiomatic Go for "structured data of unknown shape."

### 5.3 `ToolResult.Details` as `any`

ARCHITECTURE.md §8.1 shows `Details any`. This is a catch-all for tool-specific metadata. While flexible, it makes JSON serialization unpredictable. Consider documenting what each tool puts here, or using `json.RawMessage` to defer unmarshaling.

### 5.4 No Constructors for Complex Types

Go doesn't require constructors, but `Model` is complex enough that a `NewModel()` function would help ensure required fields (ID, Name, Provider) are set. Similarly, `AgentMessage` benefits from a constructor to set `ID` (UUID) and `Timestamp` (now).

### 5.5 `log/slog` Package-Level Loggers

The task.md mentions "Each package creates a package-level logger." This is idiomatic:

```go
var logger = slog.Default().With(slog.String("pkg", "config"))
```

But for testability, consider making the logger configurable (e.g., via a package-level `SetLogger()` or dependency injection). Task 006's testutil should provide a way to capture log output.

---

## 6. TDD Approach Issues

### 6.1 Testing Pure Data Types Is Low-Value

The TDD strategy says "Types: zero-value initialization, JSON serialization round-trip." For pure data types with no behavior, this is correct — but it means **most of the test effort should go to config/ and testutil/**, not types/. The types package is ~80% struct definitions. Don't over-test structs.

### 6.2 Test-First for `config/` Requires Mocking the Filesystem

To test `LoadConfig()` TDD-style:
1. Write test that expects defaults when file doesn't exist
2. Write test that parses valid JSON
3. Write test that handles malformed JSON
4. Write test that merges partial config with defaults

This requires creating temp files in `t.TempDir()`, setting up `~/.tau/` structure, or using a path parameter to `LoadConfig()` for testability. **Critical design decision:** Should `LoadConfig()` accept an optional path parameter, or should tests manipulate the real `~/.tau/` directory?

**Recommendation:** `LoadConfig(path string) (*Config, error)` — if `path` is empty, use the default path. This makes testing trivial without filesystem manipulation.

### 6.3 `testutil` Depends on `types` — Testing Order

Since `testutil/` imports `types/`, the TDD order must be:
1. Define types (no tests needed yet)
2. Define testutil mocks (test them minimally)
3. Use testutil to test types (JSON round-trips)
4. Use testutil to test config

This means types must be defined **before** they can be tested, which violates pure TDD. Pragmatic solution: define the types first (they're just structs), then TDD everything else.

### 6.4 Acceptance Criteria Include `go mod tidy` Clean — But readline Is Deferred

The task includes readline as a dependency (subtask 006.1), but readline is deferred to Task 014 (CLI). Adding it now means `go.mod` has an unused dependency until Task 014. `go mod tidy` will **remove** it since nothing imports it.

**Options:**
1. Add a blank import `_ "github.com/chzyer/readline"` in a stub file (hacky)
2. Skip readline until Task 014 (cleaner)
3. Add it to `go.mod` but accept that `go mod tidy` removes it until used

**Recommendation:** Option 2. Don't add readline until Task 014. The constraint says "3 external deps" but the actual imports should only include what's used.

---

## Answers to Key Questions

### Is `ToolDefinition` in `types/` the right place?

**Yes.** It's needed by both `provider/` (for API calls) and `tools/` (for parameter definitions). Placing it in either package would create an import cycle. The `*jsonschema.Schema` import in `types/` is the only external dependency in types/, and it's justified.

### Root path `/` encoding edge case

With the proposed algorithm (replace `/` with `-`, wrap with `-`):
- `/` → `--` (empty string wrapped with hyphens)

This is ambiguous and ugly. **Recommendation:** Special-case root to `root` or `_`. Full algorithm:
1. If path is `/`, return `root`
2. Strip leading `/`
3. Replace remaining `/` with `-`
4. If trailing `-` exists, keep it (for consistency)

| Path | Encoded |
|---|---|
| `/` | `root` |
| `/home/adam/Projects/tau` | `-home-adam-Projects-tau-` |
| `/a` | `-a-` |

### Should MockProvider use a pre-populated slice + buffered channel?

**Yes.** This avoids all blocking and race conditions:

```go
func (m *MockProvider) Stream(ctx context.Context, ...) <-chan types.StreamEvent {
    ch := make(chan types.StreamEvent, len(m.Events)+1)
    for _, e := range m.Events {
        ch <- e
    }
    close(ch)
    return ch
}
```

If dynamic event injection is needed during tests, provide a separate `MockProvider` variant with a `Send()` method.

### Should SessionEntry use `json.RawMessage` or a typed approach?

**`json.RawMessage` is correct for Task 006.** The session storage layer (Task 011) needs to read/write entries without knowing their concrete types — it's a generic envelope. Typed approaches would require a type switch on every read, which is what Task 011's resume algorithm does anyway. The `json.RawMessage` approach defers unmarshaling until the entry type is known.

**However**, consider adding a helper:
```go
func (e *SessionEntry) UnmarshalData(v any) error {
    return json.Unmarshal(e.Data, v)
}
```

---

## Priority Recommendations

| Priority | Issue | Impact | Effort |
|---|---|---|---|
| 🔴 **P0** | Add `AgentEvent` to types/ | Blocks Tasks 012, 013 | 30 min |
| 🔴 **P0** | Add typed constants for Role, StreamEventType, ContentBlockType | Prevents bugs across all tasks | 1 hour |
| 🔴 **P0** | Add JSON tags to all serialized types | Required for Task 011 correctness | 30 min |
| 🔴 **P0** | Fix `StreamEvent.Error` to be `string`, not `error` | JSON serialization correctness | 5 min |
| 🟡 **P1** | Add `SessionHeader` type | Needed by Task 011 | 20 min |
| 🟡 **P1** | Add `BeforeToolCallContext` / `AfterToolCallContext` | Needed by Task 012 | 20 min |
| 🟡 **P1** | Define CWD encoding root path behavior | Session directory naming | 10 min |
| 🟡 **P1** | Make `LoadConfig()` accept optional path | Testability | 10 min |
| 🟡 **P1** | Defer readline dependency until Task 014 | Clean go.mod | 5 min |
| 🟢 **P2** | Add `SessionEntry.UnmarshalData()` helper | Convenience | 10 min |
| 🟢 **P2** | Add `NewModel()` constructor | Validation | 20 min |
| 🟢 **P2** | Move `BashExecution` to `tools/` (Task 008) | Clean separation | 15 min |
