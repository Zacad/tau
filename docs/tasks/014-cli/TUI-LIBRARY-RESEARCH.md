# Go TUI/Readline Library Research — Comprehensive Analysis

## The Core Problem

We need a Go library (or combination) that provides:

1. **Input handling** — line editing, history, completion, multi-line
2. **Screen layout** — message area above input, optional footer
3. **Streaming output** — display LLM output in real-time while input stays active
4. **Message queuing** — user types while agent works, messages delivered at right time
5. **PI-like UX** — scrollable message area, input at bottom, status footer

---

## Option E: Bubbletea v2 + Bubbles (Chat Pattern)

### What It Is
The most popular Go TUI framework (42k+ stars), actively maintained, recently released v2. Has a **chat example** using exactly the viewport + textarea pattern we need.

### Key Components for Our Use Case

| Component | What It Does | Status |
|-----------|-------------|--------|
| `bubbletea` v2 | TUI framework (Elm architecture) | ✅ v2.0.6, April 2026 |
| `bubbles/viewport` | Scrollable text area | ✅ Active |
| `bubbles/textarea` | Multi-line input | ✅ Active |
| `bubbles/textinput` | Single-line input | ✅ Active |
| `bubbles/spinner` | Loading indicator | ✅ Active |
| `bubbles/list` | Selectable list (model picker) | ✅ Active |
| `lipgloss` v2 | Styling/theming | ✅ Active |
| `glamour` | Markdown rendering | ✅ Active |

### The Chat Example (from bubbletea repo)

```go
type model struct {
    viewport    viewport.Model     // scrollable message area
    messages    []string           // message history
    textarea    textarea.Model     // input at bottom
    senderStyle lipgloss.Style
}

func (m model) View() tea.View {
    viewportView := m.viewport.View()
    v := tea.NewView(viewportView + "\n" + m.textarea.View())
    v.Cursor = m.textarea.Cursor()
    v.AltScreen = true  // fullscreen mode
    return v
}
```

This is exactly our layout: viewport on top, textarea on bottom, cursor coordination.

### Render Modes

| Mode | How | Pros | Cons |
|------|-----|------|------|
| **Alt Screen** (`v.AltScreen = true`) | Takes over full terminal | Full control, can update any zone, clean exit | Breaks terminal scrollback |
| **Inline** (`v.AltScreen = false`) | Prints to stdout normally | Terminal scrollback works | Can't update above prompt, each render replaces everything |
| **Mixed** | Start inline, switch to alt screen for specific interactions | Best of both | Complex state management |

### How Streaming Works with Bubbletea

```go
// SDK events arrive as tea.Cmd responses
func listenForSDK(sdkChan chan types.AgentEvent) tea.Cmd {
    return func() tea.Msg {
        event := <-sdkChan
        return agentEventMsg{event}
    }
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case agentEventMsg:
        switch msg.Event.Type {
        case types.AgentEventTextDelta:
            // Append to current message, update viewport
            m.appendToCurrentMessage(msg.Event.Data.(string))
            m.viewport.SetContent(m.renderMessages())
            m.viewport.GotoBottom()
        case types.AgentEventToolExecStart:
            // Show tool call in messages
            m.addToolCallMessage(msg.Event)
        }
        return m, waitForNextEvent(m.sdkChan)
    case tea.KeyPressMsg:
        // Route keys to textarea
        var cmd tea.Cmd
        m.textarea, cmd = m.textarea.Update(msg)
        return m, cmd
    }
    return m, nil
}
```

### Architecture Diagram

```
┌─ Bubbletea Program ──────────────────────────────┐
│                                                   │
│  Model:                                           │
│  ├── viewport.Model  (message history)            │
│  ├── textarea.Model  (input editor)               │
│  ├── spinner.Model   (working indicator)          │
│  ├── messages []     (conversation transcript)    │
│  └── sdkSession      (SDK session reference)      │
│                                                   │
│  Init():                                          │
│  └── Start SDK session, return waitForEvent cmd   │
│                                                   │
│  Update(msg):                                     │
│  ├── KeyPressMsg → route to textarea              │
│  ├── agentEventMsg → update viewport content      │
│  ├── WindowSizeMsg → resize viewport              │
│  └── submitMsg → send to SDK, clear textarea      │
│                                                   │
│  View():                                          │
│  ├── header (model, session, cwd)                 │
│  ├── viewport (messages, tool calls, thinking)    │
│  ├── spinner (if agent is working)                │
│  ├── footer (usage, cost)                         │
│  └── textarea (input prompt)                      │
│                                                   │
└───────────────────────────────────────────────────┘
```

### Code Estimate

| Area | Lines | Notes |
|------|-------|-------|
| `main.go` (flag parsing, program setup) | ~120 | Standard CLI + bubbletea init |
| `interactive.go` (model, Update, View) | ~400 | Core TUI logic |
| `events.go` (SDK event → tea.Msg mapping) | ~150 | Event translation layer |
| `commands.go` (slash commands) | ~150 | Parsed from textarea input |
| `render.go` (message formatting, lipgloss) | ~200 | Styled output |
| `print.go` (print mode) | ~60 | Non-TUI, simple |
| `json.go` (JSON mode) | ~80 | Non-TUI, simple |
| `sessions.go` (session flags) | ~100 | Session handling |
| **Total** | **~1260** | Plus tests |

### Pros
- ✅ **Most popular Go TUI framework** — 42k stars, active community
- ✅ **Chat example exists** — exactly our pattern (viewport + textarea)
- ✅ **Declarative v2 API** — clean, predictable rendering
- ✅ **All components exist** — viewport, textarea, spinner, list, table
- ✅ **Markdown rendering** — via glamour (from same ecosystem)
- ✅ **Resize handling** — automatic via WindowSizeMsg
- ✅ **Message queuing** — natural via Elm architecture (textarea stays active)
- ✅ **Styling** — lipgloss provides PI-like theming
- ✅ **Production-tested** — used by glow, gum, soft-serve, many others

### Cons
- ⚠️ **Elm architecture learning curve** — functional state machine pattern
- ⚠️ **Alt screen breaks scrollback** — users can't use terminal scrollbar (PI does this too)
- ⚠️ **Heavier dependency tree** — pulls in charmbracelet ecosystem
- ⚠️ **IME support** — not as good as PI's custom solution
- ⚠️ **Cursor coordination** — needs manual work (as seen in chat example)
- ⚠️ **v2 is relatively new** — breaking changes from v1, smaller ecosystem than v1

### Real-World Usage
- **glow** (24k stars) — markdown renderer, uses bubbletea
- **gum** (23k stars) — shell script UI, uses bubbletea
- **soft-serve** (6.8k stars) — git server TUI, uses bubbletea
- **marchat** — terminal chat app, uses bubbletea (exactly our pattern)

---

## Option F: `tview` (Rivo)

### What It Is
Terminal UI library with rich widgets (13.8k stars). Widget-based, imperative API. Used by K9s (Kubernetes CLI).

### Architecture
```
Application → Pages → FlexBox → [TextView (messages), TextArea (input)]
```

### Pros
- ✅ Mature, stable (since 2018)
- ✅ Rich widget set (table, tree, form, list, image)
- ✅ Flexbox/grid layout system
- ✅ Used by major projects (K9s)
- ✅ Good documentation

### Cons
- ❌ **No textarea widget** — only single-line input field
- ❌ **No built-in markdown rendering**
- ❌ **No viewport/scrollable text area** with streaming support
- ❌ **Widget-based, not compositional** — harder to create custom layouts
- ❌ **Less active** — fewer updates than bubbletea
- ❌ **Different paradigm** — imperative widget tree vs declarative

### Verdict
Not suitable. Missing critical components (textarea, viewport, streaming).

---

## Option G: `gocui` / `awesome-gocui`

### What It Is
Minimalist console UI library (10.5k stars original, 381 stars fork). Grid-based layout.

### Pros
- ✅ Simple, minimalist
- ✅ Grid-based layout
- ✅ Low-level control

### Cons
- ❌ **Very low-level** — you build everything from scratch
- ❌ **No input widget** — no line editing, no history
- ❌ **No markdown rendering**
- ❌ **No streaming text support**
- ❌ **Fork is more active than original** — stability concerns

### Verdict
Too low-level. Would need to build input handling from scratch (same as Option D).

---

## Option H: `termui`

### What It Is
Terminal dashboard library (13.5k stars). Widget-based with charts, graphs, gauges.

### Cons
- ❌ **Dashboard-focused** — not designed for chat/input
- ❌ **No input widget**
- ❌ **Not actively maintained**
- ❌ **No streaming text support**

### Verdict
Wrong tool for the job.

---

## Option I: `charmbracelet/huh` (Forms)

### What It Is
Interactive prompts/forms library (6.8k stars). Built on bubbletea.

### What It Provides
- Text input, confirm, select, multi-select
- Form grouping, validation
- Beautiful styling

### Why It's Not the Answer
- ❌ Designed for **forms**, not **chat**
- ❌ No message history/viewport
- ❌ No streaming output
- ❌ Could be used for the `/model` selector, but not the main UI

### Verdict
Good complement, not a replacement.

---

## Option J: `charmbracelet/fang` (CLI Starter Kit)

### What It Is
CLI starter kit (1.9k stars). Combines bubbletea, cobra, viper for CLI apps.

### What It Provides
- Cobra command structure
- Bubbletea TUI integration
- Config management

### Verdict
Could be useful for CLI scaffolding, but adds complexity. Our CLI is simple enough to not need it.

---

## Option K: `reeflective/readline` (Modern Readline)

### What It Is
Modern shell library with `.inputrc` support (138 stars). More feature-rich than chzyer/readline.

### Features
- Full line editing with history
- `.inputrc` compatibility
- Multi-line input
- Completion system
- Modal editing support

### Pros
- ✅ Modern, actively maintained
- ✅ More features than chzyer/readline
- ✅ `.inputrc` support (familiar to bash users)

### Cons
- ❌ **Still readline** — same fundamental limitation (can't print above prompt)
- ❌ **Smaller community** — only 138 stars
- ❌ **No screen management** — same zone problem as Option A
- ❌ **Message queuing impossible** — same ceiling

### Verdict
Better readline, but same architectural ceiling as Option A.

---

## Option L: `ergochat/readline` (IRC Project's Readline)

### What It Is
Pure Go readline from the Ergo IRC project (52 stars).

### Verdict
Too niche. Similar limitations to other readline libraries.

---

## Option M: `mattn/go-rl` (Simple Readline)

### What It Is
Simple, cross-platform readline (22 stars).

### Verdict
Too minimal. Not suitable for our needs.

---

## Comprehensive Comparison Matrix

| Criterion | A: chzyer/readline | D: Custom Input | **E: Bubbletea v2** | F: tview |
|-----------|-------------------|-----------------|---------------------|----------|
| **Stars** | 2,285 | N/A | **42,060** | 13,822 |
| **Last Update** | 2025-06 | N/A | **2026-04** | 2026-03 |
| **Input handling** | ✅ Full | ⚠️ Build from scratch | ✅ textarea/textinput | ❌ No textarea |
| **Message history** | ❌ Terminal scrollback only | ⚠️ Build from scratch | ✅ viewport | ❌ No viewport |
| **Streaming output** | ⚠️ While suspended | ✅ Full control | ✅ Via Update loop | ❌ Not designed for it |
| **Message queuing** | ❌ Impossible | ✅ Built-in | ✅ Natural | ❌ No input widget |
| **Persistent footer** | ❌ Not possible | ✅ Built-in | ✅ In View() | ⚠️ Possible but complex |
| **Terminal scrollback** | ✅ Works | ⚠️ Must manage | ❌ Alt screen breaks it | ❌ Alt screen |
| **Markdown rendering** | ❌ Manual | ⚠️ Manual | ✅ glamour | ❌ Manual |
| **Resize handling** | ❌ Manual | ⚠️ Manual | ✅ Automatic | ✅ Automatic |
| **Styling** | ❌ Manual ANSI | ⚠️ Manual | ✅ lipgloss | ✅ Built-in |
| **Spinner** | ❌ Manual | ⚠️ Manual | ✅ bubbles/spinner | ❌ Manual |
| **Model selector** | ❌ Manual | ⚠️ Manual | ✅ bubbles/list | ✅ Built-in |
| **Learning curve** | Low | High | **Medium** | Medium |
| **Code estimate** | ~600 lines | ~3,000 lines | **~1,300 lines** | ~2,000 lines |
| **Extensibility** | Low | Unlimited | **High** | Medium |
| **PI-like UX** | ~40% | ~95% | **~85%** | ~60% |
| **Production usage** | High | N/A | **Very High** | High |
| **Maintenance** | Library | Us | Library | Library |

---

## Why Bubbletea v2 Is the Right Choice

### 1. The Chat Example Is Exactly Our Pattern

The official bubbletea repo includes a **chat example** that uses:
- `viewport` for scrollable message history
- `textarea` for input at the bottom
- Cursor coordination between viewport and textarea
- Message accumulation

This is our exact use case. We don't need to invent the pattern.

### 2. The Elm Architecture Maps to Our SDK

Our SDK is event-driven (`Subscribe(func(AgentEvent))`). The Elm architecture is event-driven (`Update(Msg) → Model`). The mapping is natural:

```
SDK AgentEvent → tea.Msg → Update() → Model change → View() → Screen
```

### 3. Message Queuing Is Natural

With bubbletea, the textarea stays active while the agent works. Key presses go to the textarea. When the user presses Enter, the message is stored. When the agent finishes, the stored message is sent. No coordination headaches.

### 4. The Ecosystem Covers Everything We Need

| Need | Bubbletea Solution |
|------|-------------------|
| Message display | `viewport` |
| Input | `textarea` / `textinput` |
| Working indicator | `spinner` |
| Model selection | `list` |
| Styling | `lipgloss` |
| Markdown | `glamour` |
| Forms/prompts | `huh` |
| Animations | `harmonica` |

### 5. Production-Proven

Used by glow (24k stars), gum (23k stars), soft-serve (6.8k stars), and many others. It's battle-tested.

### 6. v2 Solves Real Problems

- **Declarative views** — no more scattered program options, everything in `View()`
- **Better key handling** — `tea.KeyPressMsg` interface, cleaner than v1
- **Improved rendering** — cell-based renderer, color downsampling
- **Clipboard support** — native clipboard integration
- **Cursor control** — explicit cursor positioning

---

## Addressing the Alt Screen Concern

**The main tradeoff:** bubbletea's chat example uses `AltScreen = true`, which means the terminal's scrollbar won't show message history.

**But PI does the same thing.** PI's TUI also takes over the full terminal. Users scroll within PI's viewport, not the terminal's scrollbar.

**This is the right UX for a chat-like application.** Terminal scrollback is designed for command output, not interactive chat. In a chat app, you want:
- Searchable history (within the app)
- Collapsible tool output
- Thinking block toggle
- These require app-level control, not terminal scrollback

**For users who want scrollback:** print mode (`-p`) and JSON mode (`--mode json`) output to stdout normally. Users can pipe to `less`, `tee`, etc.

---

## Recommended Architecture with Bubbletea v2

```
cmd/tau/
├── main.go              # CLI entry, flag parsing, mode routing
├── interactive.go       # Bubbletea model (Init, Update, View)
├── events.go            # SDK AgentEvent → tea.Msg mapping
├── render.go            # Message formatting with lipgloss
├── commands.go          # Slash command parsing and execution
├── print.go             # Print mode (non-TUI)
├── json.go              # JSON mode (non-TUI)
└── sessions.go          # Session flag handling

internal/tui/
├── model.go             # Bubbletea model definition
├── update.go            # Event handling logic
├── view.go              # Screen layout (header, viewport, footer, input)
└── components/
    ├── message.go       # Message rendering (user, assistant, tool, thinking)
    ├── footer.go        # Status bar (model, session, usage, cost)
    └── spinner.go       # Working indicator
```

---

## Conclusion

**Bubbletea v2 is the best choice** for our use case because:

1. **It solves our exact problem** — the chat example is our pattern
2. **The ecosystem covers everything** — viewport, textarea, spinner, list, styling
3. **It's production-proven** — used by major Go CLI tools
4. **The Elm architecture maps naturally** to our event-driven SDK
5. **Message queuing is natural** — textarea stays active during streaming
6. **It's actively maintained** — v2.0.6 released April 2026
7. **The code estimate is reasonable** — ~1,300 lines vs ~3,000 for custom

The main tradeoff (alt screen breaking scrollback) is acceptable because:
- PI does the same thing
- It's the right UX for chat-like applications
- Print/JSON modes provide scrollback alternatives
