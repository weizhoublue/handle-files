# Release workflow design

## Goal

Publish versioned GitHub Releases for tagged Go binaries and retain a matching release branch.

## Trigger and permissions

The workflow is named `Release`. It runs when a tag matching `v*` is pushed and grants `contents: write` so it can create the GitHub Release and push a release branch.

## Build and release flow

One Ubuntu job:

1. Checks out the tagged source.
2. Installs the Go version declared in `go.mod`.
3. Runs `go test ./... -count=1`.
4. Runs `make build-macos`, the repository's canonical macOS build command.
5. Verifies all four executables exist and are executable:
   - `dist/macos-arm64/compress-vedio`
   - `dist/macos-arm64/check-copy`
   - `dist/macos-amd64/compress-vedio`
   - `dist/macos-amd64/check-copy`
6. Copies them to `dist/release/` with unique asset names:
   - `compress-vedio-macos-arm64`
   - `check-copy-macos-arm64`
   - `compress-vedio-macos-amd64`
   - `check-copy-macos-amd64`
7. Creates a GitHub Release with generated release notes and uploads the four uniquely named assets.
8. Derives the tag from `GITHUB_REF`, creates `release/<tag>` at the tagged commit, and pushes it to `origin`.

## Failure behavior

Tests, build failures, missing artifacts, release creation failures, and branch push failures fail the job. The workflow does not overwrite an existing remote release branch.

## Scope

The workflow adds only `.github/workflows/release.yml`. Existing build commands and documentation remain unchanged because `make build-macos` already produces the intended release artifacts.
