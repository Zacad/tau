# Task 002: Requirements Definition

## Why

Before designing architecture, we must understand what the tau should actually do. Task 001 gave us deep insight into PI's internals, but those are implementation details — not product requirements. This session defines what the tau must deliver from the user's perspective, grounded in real needs rather than assumptions.

Requirements will directly feed into Task 004 (Architecture) and Task 005 (Task Breakdown).

## Comparison Analysis: What We're Defining vs What PI Offers

| Dimension | PI Approach | Our Approach |
|-----------|-------------|--------------|
| Scope | Full-featured coding agent with extensibility | Minimal but ready-to-use tau |
| Customization | Extensions, skills, themes, packages | Built-in features, no extension layer initially |
| Target User | Developers who want to customize everything | Developers who want it to work out of the box |
| Philosophy | "Build what you need via extensions" | "Ship what most people need by default" |

## Main Constraints

- Requirements only — no architecture or implementation decisions in this task
- Must be derived from user conversation, not assumed
- Must be specific enough to drive architecture decisions
- Must identify what "minimal but ready to use" actually means
- Must identify which PI features are essential vs nice-to-have vs unnecessary

## Subtasks

- [x] **002.1** — Define target user and primary use cases
- [x] **002.2** — Define core features: what must work out of the box
- [x] **002.3** — Define sub-agent requirements: how they should work, what scenarios they serve
- [x] **002.4** — Define skills requirements: what skills mean for this project, how they're used
- [x] **002.5** — Define provider requirements: which providers, auth methods, model selection
- [x] **002.6** — Define session management requirements: persistence, branching, naming, resume
- [x] **002.7** — Define TUI requirements: what the interface must provide (DEFERRED)
- [x] **002.8** — Define non-functional requirements: performance, binary size, dependencies
- [x] **002.9** — Define out-of-scope: what we explicitly don't build
- [x] **002.10** — Consolidate into REQUIREMENTS.md

## Acceptance Criteria

- [x] All subtasks completed through interactive conversation with user
- [x] REQUIREMENTS.md contains complete, specific, testable requirements
- [x] Each requirement has a clear "must have" vs "should have" vs "could have" priority
- [x] Sub-agent and skills requirements are detailed enough to inform architecture
- [x] Out-of-scope items are explicitly documented
- [x] Requirements are concise enough to fit on a few pages

**Note**: TUI requirements (002.7) deferred to deeper conversation during architecture/implementation phase.

## Subtask Acceptance Criteria

### 002.1 — Target User & Use Cases
- [ ] Primary user persona defined
- [ ] Top 3-5 use cases documented with scenarios

### 002.2 — Core Features
- [ ] Each core feature described with "what it does" and "why it matters"
- [ ] Feature priority assigned (must/should/could)
- [ ] Gaps from PI exploration addressed (what PI does vs what we need)

### 002.3 — Sub-Agent Requirements
- [ ] Sub-agent scenarios documented (when would user spawn a sub-agent?)
- [ ] Sub-agent capabilities defined (what can sub-agents do?)
- [ ] Sub-agent isolation requirements defined (filesystem, context, tools)
- [ ] Sub-agent communication model defined (how do parent and child interact?)

### 002.4 — Skills Requirements
- [ ] Skill definition format agreed upon
- [ ] Skill loading mechanism defined (auto vs manual)
- [ ] Skill-tool relationship defined (can skills add tools?)
- [ ] Skill directory structure defined

### 002.5 — Provider Requirements
- [ ] Required providers listed (from initial assumptions: OpenAI, Anthropic, Gemini, OpenCode Zen, OpenCode Go, OpenRouter, local models)
- [ ] Auth method requirements (API keys? OAuth? Both?)
- [ ] Model selection UX defined
- [ ] Custom provider/local model support requirements defined

### 002.6 — Session Management Requirements
- [ ] Session persistence requirements (when, where, format)
- [ ] Session lifecycle (create, resume, fork, delete)
- [ ] Session naming and discovery
- [ ] Whether tree/branching is needed initially

### 002.7 — TUI Requirements
- [ ] Minimum interface elements defined
- [ ] Keyboard interaction model
- [ ] Streaming output requirements
- [ ] Whether file reference (@), path completion, multi-line input are needed

### 002.8 — Non-Functional Requirements
- [ ] Target binary size (if any)
- [ ] Target startup time (if any)
- [ ] Memory constraints (if any)
- [ ] Dependency policy (stdlib only? specific external deps allowed?)

### 002.9 — Out of Scope
- [ ] Features explicitly excluded documented
- [ ] Rationale for exclusions recorded

### 002.10 — Consolidation
- [ ] All requirements written to docs/REQUIREMENTS.md
- [ ] Requirements organized by category
- [ ] Priority labels applied consistently
- [ ] No implementation details leaked into requirements

## Worklog

See `worklog.md` for detailed work documentation.
