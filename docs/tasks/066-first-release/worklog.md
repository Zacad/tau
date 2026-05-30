# Worklog: Task 066 First Release

## 2026-05-30

- Researched Tau release readiness:
  - `cmd/tau/main.go` already exposes `--version` and supports `main.version` ldflags injection.
  - No existing GitHub Actions release automation was present.
  - Config, auth, and session directories are created lazily by Tau; archive distribution does not require an installer.
- Researched reference implementations:
  - PI uses a tag-triggered GitHub Actions workflow to build platform archives and create a GitHub Release.
  - OpenCode uses a larger release system with GitHub assets, curl installer, npm, Bun, Homebrew, AUR, Windows signing, and additional variants.
- Chose a PI-style first release for Tau and deferred Homebrew/AppImage/Flatpak.
- Used a subagent to critique the release design. Incorporated safeguards for semver tag validation, explicit `./cmd/tau` build, checksums, archive naming, release permissions, and rerun behavior.
- Added `.github/workflows/ci.yml`.
- Added `.github/workflows/release.yml`.
- Updated `README.md` installation documentation.
- Updated `docs/TRACKING.md` and `docs/DECISIONS.md`.

## Verification

- `go test ./...` was attempted before this task and failed in local Ollama-backed E2E subagent tests due nondeterministic model output:
  - `TestRun_E2E_BuiltinType_Researcher`
  - `TestRun_E2E_BuiltinType_QA`
- `go test -short ./...` passed after fixing a deterministic SDK test expectation that depended on local Ollama discovery state.
- Local version build verification for `0.1.0` passed.
