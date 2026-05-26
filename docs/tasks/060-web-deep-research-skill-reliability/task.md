# Task 060: Web Deep Research Skill Reliability

## Why

The `web-deep-research` skill completed the expected high-level workflow, but evaluation showed that it can lose important candidates between exploration, angle-specific research, and final synthesis. The clearest failure was `KuKirin G4 Max`: it appeared in exploration, was promoted to the Polish shortlist, and then disappeared from the final report without an explicit rejection.

This makes the skill unreliable for deep research because it produces coherent narratives without guaranteeing coverage, traceability, or explicit disposition of discovered entities.

## Comparison With PI and OpenCode

PI and OpenCode treat skills as lightweight prompt workflows loaded through progressive disclosure. Neither reference codebase has a built-in deep-research reliability pipeline that enforces entity persistence across subagents.

Relevant patterns from the references:

- PI emphasizes provenance through structured `sourceInfo`, which supports the need for explicit source and artifact tracking.
- OpenCode's skill loader exposes skill descriptions and locations, keeping workflow logic inside the skill content rather than runtime code.
- Existing Tau research on deep-research agents identifies structured outputs, deduplication, source tracking, and provenance sidecars as common reliability mechanisms.

The pragmatic Tau approach is to improve the skill prompt and artifacts first, not add runtime orchestration code.

## Main Constraints

- Preserve Agent Skills compatibility: `SKILL.md` frontmatter plus markdown body.
- Do not require new runtime code for this iteration.
- Keep the skill useful for broad research topics, not only product comparisons.
- Final output must be comprehensive and sourced with concise prose, not narrowed to recommendations.
- Maintain subagent support and parallel angle research.
- Avoid silently dropping weak-source or low-confidence candidates.

## Design

Add explicit research artifacts and gates to the skill:

- `00-candidate-ledger.md` as the canonical tracked entity ledger.
- Candidate/entity status and final disposition rules.
- Required candidate/entity deltas at the end of every angle file.
- `03-reconciliation.md` before final synthesis.
- Final report structure that preserves breadth and includes recommendations only as one section.
- Semantic verification in addition to URL verification.

## Subtasks

### 1. Update Skill Workflow

Rewrite `.tau/skills/web-deep-research/SKILL.md` to require the ledger, deltas, reconciliation, comprehensive report shape, and semantic verification.

Acceptance criteria:

- Skill description says the output is a comprehensive sourced report with concise prose.
- Skill requires `00-candidate-ledger.md`.
- Skill requires `03-reconciliation.md`.
- Skill defines tracked entity handling and final dispositions.
- Skill prevents recommendations from replacing full research accounting.

### 2. Update Documentation

Document the task, worklog, tracking status, and the decision to use ledger/reconciliation gates.

Acceptance criteria:

- Task folder exists under `docs/tasks/060-web-deep-research-skill-reliability/`.
- `docs/TRACKING.md` includes task 060.
- `docs/DECISIONS.md` records the prompt-level reliability decision.
- Worklog records research, implementation, and verification.

### 3. Verify

Run parser/discovery tests and rebuild the binary for manual testing.

Acceptance criteria:

- Relevant Go tests pass.
- Binary rebuild succeeds in `./`.
- Manual inspection confirms the scooter failure case would require final dispositions for `KuKirin G4 Max`, `Kamikaze K1 Max`, and `XRIDER F10 Pro`.

## Acceptance Criteria

- Every tracked candidate/entity must have exactly one final disposition.
- Every final recommendation must exist in the ledger.
- Every angle file must include candidate/entity deltas.
- Reconciliation must happen before synthesis.
- The final report must be comprehensive, not only a narrowed recommendation list.
- Weakly documented but relevant candidates must be marked `unresolved`, `conditional`, or `low-confidence`, not omitted.
- Verification must check semantic coverage, not only URL accessibility.

## Testing Strategy

- Run Go tests for the skills package.
- Run custom command template tests because `/web-research` uses `$SKILL:web-deep-research`.
- Run full `go test ./...` if feasible.
- Rebuild `./tau` for manual testing.
- Manually review the updated skill against the May 2026 scooter evaluation.
