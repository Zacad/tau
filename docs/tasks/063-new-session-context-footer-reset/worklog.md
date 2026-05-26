# Worklog: Task 063 - Reset Footer Context Usage On New Session

## 2026-05-25
- Created task documentation for stale footer context usage after `/new`
- Researched reference behavior in local PI and OpenCode via reviewer subagent
- Confirmed Tau root cause: `cmdNew` reset messages/turns/usage but did not refresh cached `contextWindow`, `contextTokens`, and `contextKnown`
- Implemented minimal fix by refreshing cached context state in `internal/tui/command.go` after `Session.NewSession()` succeeds
- Added regression test that verifies stale footer context usage is cleared after `/new`
- Ran targeted tests: `go test ./internal/tui ./internal/sdk` ✅
- Ran full suite: `go test ./...` ⚠️ unrelated pre-existing failure in `internal/subagent` (`TestRun_E2E_BuiltinType_Researcher` expected output to contain "go", got "bashbash")
- Rebuilt binary: `go build -o ./tau ./cmd/tau` ✅
- Pending: update tracking after task completion confirmation
