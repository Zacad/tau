# Worklog: Task 060 Web Deep Research Skill Reliability

## 2026-05-25

### Research

- Reviewed `docs/research/web-deep-research-skill-evaluation-may-2026.md`.
- Reviewed generated scooter research artifacts under `docs/research/najlepsze-terenowe-hulajnogi-elektryczne-do-10000zl-na-maj-2026/`.
- Confirmed the main failure: `KuKirin G4 Max` was discovered and shortlisted but absent from the final report without explicit rejection.
- Confirmed related partial-processing failures for `Kamikaze K1 Max` and `XRIDER F10 Pro`.
- Reviewed `.tau/skills/web-deep-research/SKILL.md` and found it lacked a canonical ledger, angle delta contract, reconciliation step, and semantic verification.
- Reviewed PI and OpenCode skill handling patterns. Both keep skill workflow logic in prompt content, supporting a prompt/artifact-only fix for this iteration.
- Used a review subagent to challenge the plan. It recommended a minimal but enforced mechanism: ledger, angle deltas, reconciliation, and semantic verification.

### Implementation

- Updated `web-deep-research` goal from terse report generation to comprehensive sourced reporting with concise prose.
- Added mandatory `00-candidate-ledger.md`.
- Added tracked entity rules and final disposition statuses.
- Added required candidate/entity deltas to every angle file.
- Added mandatory `03-reconciliation.md` before report synthesis.
- Expanded final report requirements to preserve breadth and avoid narrowing to recommendations.
- Added semantic coverage verification requirements.

### Verification

- Passed: `go test ./internal/skills ./internal/tui/customcmd`.
- Attempted: `go test ./...`. The first run reached `internal/provider` and failed on `TestOllamaProvider_WithThinkingLevel` because the test calls local Ollama at `http://localhost:11434` and timed out after 30 seconds.
- Confirmed local Ollama was reachable with `curl -sS --max-time 5 "http://localhost:11434/api/tags"`.
- Passed on retry after model warmup: `go test ./internal/provider -run TestOllamaProvider_WithThinkingLevel -count=1`.
- Attempted full suite again with a longer timeout: `go test ./...`. This progressed past provider tests but failed in `internal/subagent` E2E tests because local model output did not exactly match expected phrases for researcher, implementor, and QA built-in type tests. These are external/local-model behavior failures unrelated to the skill prompt changes.
- Passed: `go build -o tau ./cmd/tau` rebuilt the binary in `./`.
- Manual inspection confirmed the updated skill would force the scooter failure cases through explicit dispositions:
  - `KuKirin G4 Max` was a shortlisted/promoted entity, so reconciliation now requires a final disposition.
  - `Kamikaze K1 Max` appeared in service research, so angle deltas now require it to be merged into the ledger or explicitly marked background-only.
  - `XRIDER F10 Pro` appeared in exploration, so the ledger/reconciliation rules now prevent silent evaporation.
