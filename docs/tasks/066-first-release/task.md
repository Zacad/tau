# Task 066: First Release

## Why

Tau needs a repeatable first release process so users can install a tested binary without cloning the repository and building from source.

The immediate goal is to publish version `0.1.0` as GitHub Release archives for Linux, macOS, and Windows. The process must be small enough to trust for the first release, while leaving room for Homebrew and other distribution channels later.

## Comparison Analysis: Tau vs PI vs OpenCode

| Dimension | PI | OpenCode | Tau target |
|---|---|---|---|
| Primary release trigger | Git tag `v*` | Release workflow with versioning, draft release, publish steps | Git tag `vMAJOR.MINOR.PATCH` |
| Binary packaging | Platform archives from GitHub Actions | Platform archives plus npm, Bun, Homebrew, install script, signing | Platform archives from GitHub Actions |
| Platforms | macOS arm64/x64, Linux x64/arm64, Windows x64 | More variants including musl, baseline, Windows signing | macOS arm64/amd64, Linux amd64/arm64, Windows amd64 |
| Install channels | GitHub Release archives | curl installer, npm, Bun, Homebrew, AUR | GitHub Release archives first; Homebrew later |
| Complexity | Moderate | High | Low for `0.1.0` |

## Main Constraints

- Must not require users to have Go installed.
- Must inject the semantic version into `tau --version`.
- Must keep the release process reproducible from a tag.
- Must not introduce AppImage or Flatpak for the CLI-only first release.
- Must publish checksums for all archives.
- Must run deterministic tests in CI and release workflows.
- Must document the install path clearly in `README.md`.
- Must defer Homebrew until the GitHub Release artifact contract is stable.

## Design

Use two GitHub Actions workflows:

- `ci.yml` runs on pushes and pull requests to `main`.
- `release.yml` runs on pushed semver tags like `v0.1.0` and manual dispatch for an existing tag.

The CI workflow runs `go test -short ./...`, builds `./cmd/tau`, and smoke-tests `tau --version`.

The release workflow validates the tag, runs `go test -short ./...`, cross-builds with `CGO_ENABLED=0`, packages archives, generates `checksums.txt`, smoke-tests the Linux archive, and creates or updates the GitHub Release.

## Distribution Decision

For `0.1.0`, Tau ships plain archives:

- `tau_0.1.0_linux_amd64.tar.gz`
- `tau_0.1.0_linux_arm64.tar.gz`
- `tau_0.1.0_darwin_amd64.tar.gz`
- `tau_0.1.0_darwin_arm64.tar.gz`
- `tau_0.1.0_windows_amd64.zip`
- `checksums.txt`

Each archive contains a wrapper directory with the binary, `README.md`, and `LICENSE`.

## Subtasks

- [x] Research PI and OpenCode release distribution patterns.
- [x] Define first-release packaging scope.
- [x] Add CI workflow.
- [x] Add tag-triggered release workflow.
- [x] Update installation documentation.
- [x] Verify deterministic tests and local versioned build.
- [x] Commit release infrastructure.
- [ ] Tag `v0.1.0`.
- [ ] Push commit and tag.
- [ ] Verify GitHub Release assets after workflow completion.

## Acceptance Criteria

- [x] `go test -short ./...` passes locally.
- [x] A local versioned build prints `tau 0.1.0`.
- [x] CI workflow builds the release entrypoint `./cmd/tau`.
- [x] Release workflow validates semver tags.
- [x] Release workflow packages Linux, macOS, and Windows archives.
- [x] Release workflow generates SHA256 checksums.
- [x] README documents archive installation and deferred Homebrew support.
- [ ] Git tag `v0.1.0` exists locally.
- [ ] GitHub Release `v0.1.0` exists with all expected assets.

## Testing Strategy

- Run `go test -short ./...` for deterministic local verification.
- Run a local build with `-ldflags "-X main.version=0.1.0"`.
- Run `./tau --version` and verify the printed version.
- Let GitHub Actions verify cross-platform builds after the tag is pushed.

## Notes

`go test ./...` currently includes E2E-style tests that call local Ollama and can fail due to model nondeterminism. CI and release workflows use `go test -short ./...` to skip those tests by default.
