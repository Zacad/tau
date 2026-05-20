# Task 023: Tab Completion, Testing & Polish — Implementation Plan

## Overview
This task adds tab completion for slash commands and skills, mock provider injection for E2E testing, centralized error handling, signal handling, help/version flags, and comprehensive tests.

---

## Phase 1: Tab Completion (023.1, 023.2)

### New File: `internal/tui/completion.go`
```go
package tui

import "strings"

// slashCommands is the list of all available slash commands.
var slashCommands = []string{
    "/quit", "/exit", "/help", "/name", "/session",
    "/model", "/compact", "/clear", "/skills", "/skill:",
}

// completeCommand returns the best completion for a partial command input.
func completeCommand(input string) string {
    input = strings.TrimSpace(input)
    if input == "" || !strings.HasPrefix(input, "/") {
        return ""
    }
    var matches []string
    for _, cmd := range slashCommands {
        if strings.HasPrefix(cmd, input) {
            matches = append(matches, cmd)
        }
    }
    if len(matches) == 0 {
        return ""
    }
    if len(matches) == 1 {
        return matches[0]
    }
    return longestCommonPrefix(matches)
}

// completeSkill returns a skill name completion for text after "/skill:".
func completeSkill(input string, skills []string) string {
    input = strings.TrimSpace(input)
    if input == "" {
        return ""
    }
    var matches []string
    for _, skill := range skills {
        if strings.HasPrefix(strings.ToLower(skill), strings.ToLower(input)) {
            matches = append(matches, skill)
        }
    }
    if len(matches) == 0 {
        return ""
    }
    if len(matches) == 1 {
        return matches[0]
    }
    return longestCommonPrefix(matches)
}

func longestCommonPrefix(strs []string) string {
    if len(strs) == 0 { return "" }
    if len(strs) == 1 { return strs[0] }
    prefix := strs[0]
    for _, s := range strs[1:] {
        for len(s) < len(prefix) || prefix != s[:len(prefix)] {
            prefix = prefix[:len(prefix)-1]
            if prefix == "" { return "" }
        }
    }
    return prefix
}
```

### Modify: `internal/tui/update.go`
Add `tab` case to `handleKeyPress()`:

```go
case "tab":
    if m.state == stateIdle {
        val := m.input.Value()
        if strings.HasPrefix(val, "/skill:") {
            skillPart := strings.TrimPrefix(val, "/skill:")
            completion := completeSkill(skillPart, m.session.Skills())
            if completion != "" {
                m.input.SetValue("/skill:" + completion)
                m.input.SetCursor(len("/skill:" + completion))
            }
        } else if strings.HasPrefix(val, "/") {
            completion := completeCommand(val)
            if completion != "" {
                m.input.SetValue(completion)
                m.input.SetCursor(len(completion))
            }
        }
    }
    return nil
```

### New File: `internal/tui/completion_test.go`
Test cases:
- `completeCommand("/")` → returns longest common prefix of all commands
- `completeCommand("/qui")` → returns `/quit`
- `completeCommand("/skill")` → returns `/skill:`
- `completeCommand("/bogus")` → returns `""`
- `completeSkill("test", []string{"test-skill", "test-other"})` → returns longest common prefix
- `completeSkill("unique", []string{"unique-skill"})` → returns `unique-skill`
- `completeSkill("nonexistent", []string{"foo"})` → returns `""`
- `longestCommonPrefix` edge cases

---

## Phase 2: Mock Provider Injection (023.3)

### Modify: `internal/sdk/sdk.go`
Add mock URL support to `CreateSession()`:

After the provider registry creation (step 2), check for `TAU_MOCK_URL` env var:

```go
// Check for mock provider URL (for E2E testing)
if mockURL := os.Getenv("TAU_MOCK_URL"); mockURL != "" {
    slog.Info("using mock provider", "url", mockURL)
    mockProv := provider.NewOpenAICompatProvider("", provider.OpenAICompatConfig{
        BaseURL:      mockURL,
        ProviderName: "mock",
    })
    provReg.Register(mockProv)
    model = types.Model{
        ID:       "mock-model",
        Name:     "mock-model",
        Provider: "mock",
        API:      "openai-completions",
        BaseURL:  mockURL,
    }
    prov = mockProv
    // Skip normal model resolution
    goto skipModelResolution
}
```

Add label `skipModelResolution:` before step 4 (skills discovery).

### Modify: `cmd/tau/main.go`
Add `--mock` flag to `cliConfig` and `parseFlags()`:

```go
type cliConfig struct {
    // ... existing fields ...
    MockURL string // mock provider URL for E2E testing
}

// In parseFlags():
var mockURL string
fs.StringVar(&mockURL, "mock", "", "mock provider URL for E2E testing")

// After flag parsing, if mockURL is set:
if mockURL != "" {
    os.Setenv("TAU_MOCK_URL", mockURL)
}
```

---

## Phase 3: Error Handling & Exit Codes (023.4, 023.5)

### New File: `cmd/tau/errors.go`
```go
package main

import (
    "fmt"
    "os"
    "strings"
)

const (
    exitSuccess = 0
    exitRuntime = 1
    exitUsage   = 2
)

// exitError formats and prints an error then exits with the given code.
func exitError(err error, code int) {
    msg := friendlyError(err)
    fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
    os.Exit(code)
}

// friendlyError converts internal errors to user-friendly messages.
func friendlyError(err error) string {
    if err == nil {
        return ""
    }
    msg := err.Error()
    
    // Provider errors
    if strings.Contains(msg, "resolve model") {
        return "No model available. Set a model with --model or configure a default in your config."
    }
    if strings.Contains(msg, "no provider registered") {
        return "No provider available. Set an API key or run Ollama locally."
    }
    if strings.Contains(msg, "no model specified, no default configured") {
        return "No model configured. Set TAU_DEFAULT_MODEL or run with --model."
    }
    if strings.Contains(msg, "API key is empty") {
        return "API key not configured. Set the appropriate *_API_KEY environment variable."
    }
    
    // Session errors
    if strings.Contains(msg, "no sessions found") {
        return "No previous sessions found. Start a new session without -c or -r flags."
    }
    if strings.Contains(msg, "open session") {
        return "Failed to open session file. It may be corrupted or from a different version."
    }
    
    // Config errors (already handled gracefully, but document)
    if strings.Contains(msg, "config") {
        return "Using default configuration (config file not found or invalid)."
    }
    
    // Fallback
    return msg
}
```

### Modify: `cmd/tau/interactive.go`
Replace direct `fmt.Fprintf(os.Stderr, "Error: %v\n", err)` + `os.Exit(1)` calls with `exitError(err, exitRuntime)`.

### Modify: `cmd/tau/print.go` and `cmd/tau/json.go`
Same pattern — use `exitError()` for consistent error handling.

---

## Phase 4: Signal Handling & Session Cleanup (023.6, 023.7)

### Modify: `cmd/tau/interactive.go`
Add signal handling with context:

```go
import (
    "context"
    "os/signal"
    "syscall"
)

func runInteractive(cfg cliConfig) {
    // ... existing setup ...
    
    // Create context with signal handling
    ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer cancel()
    
    // Create SDK session
    session, err := sdk.CreateSession(ctx, sdk.SessionOptions{...})
    if err != nil {
        exitError(err, exitRuntime)
    }
    
    // Ensure session is closed on exit
    defer func() {
        if err := session.Close(); err != nil {
            fmt.Fprintf(os.Stderr, "Warning: failed to close session: %v\n", err)
        }
    }()
    
    // ... create model and run ...
    
    // Handle context cancellation (signal received)
    go func() {
        <-ctx.Done()
        // Session close will happen via defer
        p.Send(tea.QuitMsg{})
    }()
    
    if _, err := p.Run(); err != nil {
        exitError(err, exitRuntime)
    }
}
```

Note: Ctrl+C handling during streaming and double-tap to exit is already implemented in the TUI (`handleKeyPress`). Signal handling here covers SIGTERM and ensures graceful shutdown.

---

## Phase 5: Help & Version (023.12, 023.13)

### Modify: `cmd/tau/main.go`
Add `--version` flag and enhance `--help`:

```go
import "runtime/debug"

var version = "dev" // set by ldflags at build time

func showVersion() {
    fmt.Printf("tau %s\n", version)
    if info, ok := debug.ReadBuildInfo(); ok {
        fmt.Printf("Go version: %s\n", info.GoVersion)
        for _, s := range info.Settings {
            if s.Key == "vcs.revision" {
                fmt.Printf("Commit: %s\n", s.Value)
                break
            }
        }
    }
    os.Exit(0)
}

func parseFlags() cliConfig {
    fs := flag.NewFlagSet("tau", flag.ContinueOnError)
    fs.Usage = func() {
        fmt.Fprintf(os.Stderr, `tau — AI assistant CLI

Usage: tau [flags]

Modes:
  Interactive (default): Full-screen TUI with chat history
  Print (-p):            Single prompt, plain text output
  JSON (--mode json):    Single prompt, JSONL event output

Flags:
  -p, --print TEXT   Print mode: send TEXT as prompt, output response, exit
  --mode MODE        Output mode: interactive (default), print, json
  --model PATTERN    Model pattern or ID (e.g., gpt-4o, ollama/llama3)
  -c, --continue     Resume most recent session
  -r                 Open session picker to resume a past session
  --no-session       Run in ephemeral mode (no session persistence)
  --mock URL         Use mock provider at URL (for E2E testing)
  --version          Show version information
  -h, --help         Show this help

Interactive Mode Commands:
  /quit, /exit    Exit the application
  /help           Show help message
  /name <name>    Rename the current session
  /session        Show session information
  /model          Change the active model
  /compact        Trigger context compaction
  /clear          Clear the viewport
  /skills         List available skills
  /skill:<name>   Load a skill's content

Keyboard Shortcuts:
  Enter         Send message
  Shift+Enter   New line
  Tab           Auto-complete commands and skill names
  Ctrl+D        Quit (when input is empty)
  Ctrl+C        Abort current response / Exit (double-tap when idle)
  Esc           Clear input

Environment Variables:
  TAU_HOME            Config directory (default: ~/.tau)
  TAU_DEFAULT_MODEL   Default model to use
  TAU_MOCK_URL        Mock provider URL for testing
  TAU_DEBUG=1         Enable debug logging
  OPENAI_API_KEY      OpenAI API key
  ANTHROPIC_API_KEY   Anthropic API key
  GOOGLE_API_KEY      Google API key

Examples:
  tau                          Start interactive chat
  tau -p "Explain quantum computing"  Single prompt
  echo "hello" | tau           Pipe stdin
  tau -c                       Resume last session
  tau --model gpt-4o           Use specific model
`)
    }
    
    var showVersion bool
    fs.BoolVar(&showVersion, "version", false, "show version")
    fs.BoolVar(&showVersion, "v", false, "show version")
    
    // ... existing flag parsing ...
    
    if showVersion {
        showVersion()
    }
}
```

---

## Phase 6: Tests & Quality Gates (023.8 - 023.11, 023.14)

### New File: `internal/tui/completion_test.go`
```go
package tui

import "testing"

func TestCompleteCommand(t *testing.T) {
    tests := []struct {
        input    string
        expected string
    }{
        {"/qui", "/quit"},
        {"/exi", "/exit"},
        {"/help", "/help"},
        {"/skill", "/skill:"},
        {"/bogus", ""},
        {"/", ""}, // multiple matches → LCP
        {"not-a-command", ""},
        {"", ""},
    }
    for _, tt := range tests {
        t.Run(tt.input, func(t *testing.T) {
            got := completeCommand(tt.input)
            if got != tt.expected {
                t.Errorf("completeCommand(%q) = %q, want %q", tt.input, got, tt.expected)
            }
        })
    }
}

func TestCompleteSkill(t *testing.T) {
    skills := []string{"test-skill", "test-other", "unique-skill"}
    tests := []struct {
        input    string
        expected string
    }{
        {"test", "test-"},      // LCP of test-skill and test-other
        {"unique", "unique-skill"},
        {"nonexistent", ""},
        {"", ""},
    }
    for _, tt := range tests {
        t.Run(tt.input, func(t *testing.T) {
            got := completeSkill(tt.input, skills)
            if got != tt.expected {
                t.Errorf("completeSkill(%q) = %q, want %q", tt.input, got, tt.expected)
            }
        })
    }
}

func TestLongestCommonPrefix(t *testing.T) {
    tests := []struct {
        input    []string
        expected string
    }{
        {[]string{"foo", "foobar", "foobaz"}, "foo"},
        {[]string{"a", "ab", "abc"}, "a"},
        {[]string{"single"}, "single"},
        {[]string{}, ""},
        {[]string{"foo", "bar"}, ""},
    }
    for _, tt := range tests {
        t.Run(strings.Join(tt.input, ","), func(t *testing.T) {
            got := longestCommonPrefix(tt.input)
            if got != tt.expected {
                t.Errorf("longestCommonPrefix(%v) = %q, want %q", tt.input, got, tt.expected)
            }
        })
    }
}
```

### New File: `cmd/tau/e2e_test.go`
```go
//go:build e2e

package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "os"
    "os/exec"
    "strings"
    "testing"
)

// mockHandler returns an HTTP handler that serves canned SSE responses.
func mockHandler(t *testing.T, events string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/event-stream")
        fmt.Fprint(w, events)
    }
}

const streamingResponse = `data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":""}]}

data: {"choices":[{"delta":{"content":" world"},"finish_reason":""}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

`

func TestE2E_PrintMode(t *testing.T) {
    server := httptest.NewServer(mockHandler(t, streamingResponse))
    defer server.Close()

    bin := buildTestBinary(t)
    cmd := exec.Command(bin, "--mock", server.URL, "-p", "hello")
    cmd.Env = append(os.Environ(), "TAU_MOCK_URL="+server.URL)
    out, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("command failed: %v\n%s", err, out)
    }
    if !strings.Contains(string(out), "Hello world") {
        t.Errorf("expected 'Hello world' in output, got: %s", out)
    }
}

func TestE2E_JSONMode(t *testing.T) {
    server := httptest.NewServer(mockHandler(t, streamingResponse))
    defer server.Close()

    bin := buildTestBinary(t)
    cmd := exec.Command(bin, "--mock", server.URL, "--mode", "json", "-p", "hello")
    cmd.Env = append(os.Environ(), "TAU_MOCK_URL="+server.URL)
    out, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("command failed: %v\n%s", err, out)
    }

    // Verify each line is valid JSON
    lines := strings.Split(strings.TrimSpace(string(out)), "\n")
    for i, line := range lines {
        var obj map[string]any
        if err := json.Unmarshal([]byte(line), &obj); err != nil {
            t.Errorf("line %d is not valid JSON: %v\n  line: %s", i, err, line)
        }
    }
}

func TestE2E_ExitCode(t *testing.T) {
    server := httptest.NewServer(mockHandler(t, streamingResponse))
    defer server.Close()

    bin := buildTestBinary(t)
    cmd := exec.Command(bin, "--mock", server.URL, "-p", "hello")
    cmd.Env = append(os.Environ(), "TAU_MOCK_URL="+server.URL)
    err := cmd.Run()
    if err != nil {
        t.Fatalf("expected exit code 0, got error: %v", err)
    }
}
```

### New File: `cmd/tau/README.md`
Comprehensive documentation covering:
- Installation
- Usage modes (interactive, print, json)
- Flags and environment variables
- Interactive mode commands and keyboard shortcuts
- Session management
- Configuration
- E2E testing with mock provider

---

## Implementation Order

1. **completion.go + completion_test.go** — Tab completion logic and tests
2. **Modify update.go** — Add tab key handling in handleKeyPress()
3. **Modify sdk.go** — Add TAU_MOCK_URL support
4. **Modify main.go** — Add --mock, --version flags, enhance --help
5. **errors.go** — Centralized error handling
6. **Modify interactive.go** — Signal handling, use exitError()
7. **Modify print.go, json.go** — Use exitError()
8. **e2e_test.go** — E2E tests with mock HTTP server
9. **README.md** — CLI documentation
10. **Quality gates** — go vet, go build -race, go mod tidy, go test -race

---

## Acceptance Criteria Verification

- [ ] Typing `/` + Tab shows command completions (inline: appends best match)
- [ ] Typing `/skill:` + Tab shows skill name completions (inline: appends best match)
- [ ] Invalid API key → user-friendly error message, exit code 1
- [ ] Missing config file → graceful fallback to defaults
- [ ] Ctrl+C during streaming → abort current turn, return to input (already works)
- [ ] Ctrl+C twice (when idle) → exit application (already works)
- [ ] SIGTERM → save session, exit gracefully
- [ ] Session file flushed and closed on exit
- [ ] `./tau --help` shows comprehensive help text
- [ ] `./tau --version` shows version information
- [ ] `go test -race ./cmd/tau/... ./internal/tui/...` — all pass
- [ ] E2E test: mock provider returns canned streaming response, verify output
- [ ] E2E test: print mode outputs expected text, exit code 0
- [ ] E2E test: JSON mode outputs valid JSONL, each line parseable independently
- [ ] `go vet ./cmd/tau/... ./internal/tui/...` — clean
- [ ] `go build -race ./cmd/tau` — clean
- [ ] `go mod tidy` — clean
