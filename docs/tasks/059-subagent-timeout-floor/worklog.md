# Worklog: Task 059

## 2026-05-25
- Diagnosed that the subagent tool could produce a 2 minute deadline when no default timeout was available or when the model supplied a short timeout.
- Raised the enforced minimum to 5 minutes to match the documented default.
- Made omitted timeout fall back to `subagent.DefaultTimeout` when config does not supply a default.
- Updated the tool schema to avoid suggesting `2m` timeouts.
- Added config unmarshalling for documented duration strings such as `"5m"`.
- Changed SDK config-load failure fallback to use `config.DefaultConfig()` instead of an empty config.
- Verification:
  - `go test ./internal/tools -run 'TestSubAgentTool_Execute_Timeout|TestSubAgentTool_Execute_DefaultTimeout'` passed.
  - `go test ./internal/config -run 'TestLoadConfig_SubagentTimeout|TestLoadConfig_DefaultWhenMissing|TestLoadConfig_ValidJSON'` passed.
