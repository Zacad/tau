# Task 050: Subagent Model Resolution Fix

**Status**: Done
**Date**: 2026-05-23

## Why

Researcher subagents consistently failed with `provider stream error: model: claude-sonnet-4-20250514`. The root cause was a bug in `internal/tools/subagent.go` where model resolution for subagents incorrectly inherited the parent's provider and API type instead of resolving the correct provider for the requested model.

## Constraints
- Must support cross-provider model selection (e.g., parent=anthropic, subagent=ollama)
- Must preserve backward compatibility (nil provReg = legacy behavior)
- Must follow the 4-step priority chain defined by user

## Subtasks

### ✅ 050.1: Analyze root cause
- [x] Trace error flow from `provider stream error` to Anthropic API response
- [x] Identify bug in `SubAgentTool.Execute()` model resolution
- [x] Identify secondary bug in agent definition model override

### ✅ 050.2: Implement fix
- [x] Add `provReg *provider.Registry` to `SubAgentTool`
- [x] Implement `resolveSubAgentModel()` with 4-step priority chain
- [x] Implement `trySubagentDefaults()` fallback
- [x] Add `subagentDefaultModels` list
- [x] Update SDK call site to pass `provReg`
- [x] Add legacy fallback for nil provReg (backward compat)

### ✅ 050.3: Tests
- [x] Test step 1: frontmatter model wins over prompt model
- [x] Test step 2: prompt model used when no frontmatter
- [x] Test step 3: parent model used when no frontmatter or prompt
- [x] Test step 4: fallback to subagent defaults when parent provider unavailable
- [x] Test cross-provider: ollama model with anthropic parent
- [x] Test legacy fallback: nil provReg preserves old behavior
- [x] All existing tests still pass

### ✅ 050.4: Build and verify
- [x] `go build ./...` succeeds
- [x] `go test ./... -short` passes (17 packages)
- [x] Binary rebuilt at `./tau`

## Acceptance Criteria
- [x] Subagent with cross-provider model (e.g., `ministral-3:14b` with anthropic parent) uses correct provider
- [x] All 7 new tests pass
- [x] All existing tests pass (no regressions)
- [x] Binary rebuilt successfully

## Worklog

### 2026-05-23
- **Research**: Traced full error flow from `provider stream error` through Anthropic provider, agent loop, subagent execution, and TUI rendering
- **Root cause**: `internal/tools/subagent.go` lines 103-110 — when `p.Model` is specified, code creates `types.Model` inheriting parent's `Provider` and `API` fields
- **Fix**: Added `provReg` field, implemented 4-step priority chain (`resolveSubAgentModel`), added default models fallback list
- **Tests**: Added 7 new test cases covering all priority steps, cross-provider scenario, and legacy fallback
- **Build**: All 17 packages pass, binary rebuilt
