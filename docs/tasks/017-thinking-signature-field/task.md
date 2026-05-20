# Task 017: Thinking Signature Field — Replace NUL-Separator

## Why

Task 016 stores Anthropic thinking signatures by embedding them in `ContentBlock.Text` using a NUL byte (`\x00`) separator:

```
"<thinking content>\x00<base64 signature>"
```

This is fragile:
- If model-generated thinking text contains a NUL byte (rare but possible with binary data or adversarial prompts), parsing breaks — everything after the NUL is treated as the signature
- Some JSON serializers may corrupt or strip NUL bytes during round-trip
- The `messageToAnthropic()` split logic scans byte-by-byte for `\x00` — a linear scan on every round-trip
- No way to distinguish between "no signature" and "signature is empty string"

This was accepted as a pragmatic MVP trade-off in Task 016 but needs a proper fix for production reliability.

## Comparison Analysis: PI vs Our Approach

### PI's Approach
PI stores `thinkingSignature` as a **separate field** on the thinking content block:

```typescript
interface ThinkingContent {
    type: "thinking";
    thinking: string;
    thinkingSignature?: string;  // Separate field
    isRedacted?: boolean;
}
```

Round-trip: `{type: "thinking", thinking: "...", signature: "..."}`
Serialization: signature is a first-class property, always preserved.

### Our Current Approach
We store signature embedded in `Text` via NUL separator. The `ContentBlock` struct has no signature field:

```go
type ContentBlock struct {
    Type     ContentBlockType
    Text     string           // Contains "<thinking>\x00<signature>"
    ToolCall *ToolCallBlock
    Image    *ImageBlock
}
```

Round-trip: Must parse `Text` by scanning for `\x00`, split, then emit `Signature` field in JSON.

### Key Differences

| Aspect | PI | Our Current | Our Target |
|--------|----|-------------|------------|
| Signature storage | Dedicated field | NUL in Text | Dedicated field |
| Data integrity | Always preserved | Fragile | Always preserved |
| JSON round-trip | Clean | Requires parsing | Clean |
| Session storage | Clean field | Encoded in text | Clean field |
| "No signature" vs "empty" | Distinguishable | Ambiguous | Distinguishable |

## Main Constraints

- Must not break existing session data (sessions with NUL-encoded signatures must still be readable)
- Must preserve backward compatibility for JSON serialization (existing sessions on disk)
- Must not change the `StreamEvent` or `AgentMessage` public API surface beyond adding the field
- Migration path for existing NUL-encoded signatures in session storage

## Dependencies

- Task 016 (Reasoning Support) — completed, introduces the NUL-separator pattern
- `internal/types/message.go` — `ContentBlock` struct to modify
- `internal/provider/anthropic.go` — round-trip code to simplify
- `internal/session/storage.go` — JSONL session loading to handle backward compat

## Subtasks

- [ ] **017.1** — Design: Define backward-compatible migration strategy for existing NUL-encoded signatures
- [ ] **017.2** — Add `Signature string` field to `ContentBlock` in `internal/types/message.go`
- [ ] **017.3** — Update `parseStreamResponse()` in Anthropic provider to store signature in the new field instead of NUL-separator
- [ ] **017.4** — Update `messageToAnthropic()` to use the new `Signature` field directly (no NUL parsing)
- [ ] **017.5** — Add backward-compat handling in session loading: detect NUL in `BlockThinking.Text`, split into Text + Signature
- [ ] **017.6** — Update all existing tests to use the new `Signature` field
- [ ] **017.7** — Add migration test: load a session with NUL-encoded signature, verify correct split
- [ ] **017.8** — Add test: NUL byte in thinking content (not as separator) is preserved correctly

## Acceptance Criteria

- [ ] `ContentBlock` has a `Signature` field with `json:"signature,omitempty"` tag
- [ ] Anthropic provider stores signature in the new field (no NUL embedding)
- [ ] `messageToAnthropic()` uses `Signature` field directly — no NUL parsing
- [ ] Existing sessions with NUL-encoded signatures load correctly (backward compat)
- [ ] All existing tests pass (no regression)
- [ ] New tests for NUL-in-content edge case pass
- [ ] `go vet`, `go build`, `go test -race` all clean

## Testing & Verification Strategy

**Unit tests:**
- New `Signature` field is serialized/deserialized correctly via JSON
- NUL byte in thinking text (not as separator) is preserved without corruption
- `messageToAnthropic()` produces correct JSON with separate `thinking` and `signature` fields
- Session loading with legacy NUL-encoded data splits correctly

**Migration test:**
- Create a synthetic JSONL session with NUL-encoded thinking signature
- Load via session storage
- Verify `Text` contains only thinking content, `Signature` contains the signature

**Edge cases:**
- Empty signature (no NUL in text) — `Signature` remains empty
- Multiple NUL bytes in text — only the first is treated as separator during migration
- NUL at start of text — empty thinking content, rest is signature
- NUL at end of text — full thinking content, empty signature
