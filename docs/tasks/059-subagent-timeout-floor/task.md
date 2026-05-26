# Task 059: Subagent Timeout Floor

## Why
Subagents still failed with `subagent: execution timed out after 2m0s: context deadline exceeded`. The OpenAI replay fix addressed the follow-up API error, but the delegated work itself could still be cancelled too early.

## Comparison Analysis

### PI
- PI delegates long-running work through the agent runtime and generally relies on caller/runtime cancellation rather than short model-selected deadlines.
- The relevant lesson is that delegated agent work needs enough time for multiple model/tool iterations.

### OpenCode
- OpenCode task/subagent flows are expected to run autonomously for non-trivial work and are not encouraged to use very short per-task timeouts.

### Tau
- Tau documented a 5 minute subagent default, but the subagent tool could fall back to the 2 minute minimum when config defaults were unavailable.
- The tool schema also gave examples like `2m`, encouraging the model to select too-short deadlines.
- `subagent_timeout` was documented as a string (`"5m"`), but `time.Duration` did not accept that JSON form directly.

## Constraints
- Keep subagent execution bounded.
- Preserve explicit longer user timeouts.
- Keep documented config format working.
- Avoid changing subagent concurrency or agent loop behavior.

## Design
- Raise the subagent minimum timeout from 2 minutes to 5 minutes.
- If no timeout is provided and no config default is available, use `subagent.DefaultTimeout` instead of falling through to the minimum path.
- Update the subagent tool schema to recommend `5m`, `10m`, and `20m`, and to tell the model to omit timeout unless the user explicitly requests one.
- Add config JSON unmarshalling for human-readable `subagent_timeout` strings such as `"5m"`.
- If SDK config loading fails, use `config.DefaultConfig()` instead of an empty config struct.

## Testing Strategy
- Unit test omitted timeout with missing config default.
- Unit test too-short explicit timeout is raised to 5 minutes.
- Unit test documented `subagent_timeout` string parsing.
- Run targeted subagent timeout and config tests.

## Acceptance Criteria
- [x] Omitted subagent timeout uses 5 minutes even when config default is missing.
- [x] Explicit subagent timeout below 5 minutes is raised to 5 minutes.
- [x] `subagent_timeout: "10m"` parses successfully.
- [x] Invalid duration strings return a config error.
- [x] Documentation and tracking updated.

## Subtasks
- [x] Diagnose why timeout remained 2 minutes.
- [x] Fix subagent timeout default/floor handling.
- [x] Fix config duration string parsing.
- [x] Add/update tests.
- [x] Run targeted verification.
