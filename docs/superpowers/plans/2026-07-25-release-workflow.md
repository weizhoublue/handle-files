# Release Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create tagged GitHub Releases containing uniquely named macOS binaries and matching release branches.

**Architecture:** A single GitHub Actions workflow responds to `v*` tag pushes. It reuses `make build-macos`, stages the four architecture-specific executables under unique release asset names, creates the GitHub Release, then pushes `release/<tag>` from the tagged commit.

**Tech Stack:** GitHub Actions, Go 1.26 from `go.mod`, GNU Make, `softprops/action-gh-release@v2`.

## Global Constraints

- Trigger only on pushed tags matching `v*`.
- Use `go-version-file: go.mod`; do not hard-code a Go version.
- Build only macOS `arm64` and `amd64` assets using existing `make build-macos`.
- Upload exactly `compress-vedio-macos-arm64`, `check-copy-macos-arm64`, `compress-vedio-macos-amd64`, and `check-copy-macos-amd64`.
- Generate GitHub Release notes automatically.
- Create and push `release/<tag>` without force-pushing or overwriting an existing remote branch.

---

## File Structure

- Create: `.github/workflows/release.yml` — tag-triggered build, validation, GitHub Release, and release-branch workflow.
- No Go sources, Makefile targets, or README content change because the existing `make build-macos` command already builds every required source executable.

### Task 1: Create tagged release workflow

**Files:**
- Create: `.github/workflows/release.yml`
- Test: `.github/workflows/release.yml` through its commands on the local checkout

**Interfaces:**
- Consumes: `go.mod` for the Go version, `Makefile` target `build-macos`, and tag ref `refs/tags/vX.Y.Z`.
- Produces: four GitHub Release assets and a remote branch named `release/vX.Y.Z`.

- [ ] **Step 1: Add the release workflow**

Create `.github/workflows/release.yml` with:

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Unit tests
        run: go test ./... -count=1

      - name: Build macOS binaries
        run: make build-macos

      - name: Stage release assets
        run: |
          mkdir -p dist/release
          cp dist/macos-arm64/compress-vedio dist/release/compress-vedio-macos-arm64
          cp dist/macos-arm64/check-copy dist/release/check-copy-macos-arm64
          cp dist/macos-amd64/compress-vedio dist/release/compress-vedio-macos-amd64
          cp dist/macos-amd64/check-copy dist/release/check-copy-macos-amd64
          test -x dist/release/compress-vedio-macos-arm64
          test -x dist/release/check-copy-macos-arm64
          test -x dist/release/compress-vedio-macos-amd64
          test -x dist/release/check-copy-macos-amd64

      - name: Create GitHub Release
        uses: softprops/action-gh-release@v2
        with:
          files: |
            dist/release/compress-vedio-macos-arm64
            dist/release/check-copy-macos-arm64
            dist/release/compress-vedio-macos-amd64
            dist/release/check-copy-macos-amd64
          generate_release_notes: true

      - name: Create release branch
        run: |
          tag_name=${GITHUB_REF#refs/tags/}
          branch=release/${tag_name}
          git switch -c "$branch"
          git push origin "$branch"
```

- [ ] **Step 2: Run unit and integration tests**

Run:

```bash
go test ./... -count=1
```

Expected: every package, including `integration`, passes.

- [ ] **Step 3: Build the exact release source binaries**

Run:

```bash
make build-macos
```

Expected: build completes and creates the macOS arm64 and amd64 source executables under `dist/macos-arm64/` and `dist/macos-amd64/`.

- [ ] **Step 4: Reproduce asset staging locally**

Run:

```bash
mkdir -p /tmp/handle-files-release-assets
cp dist/macos-arm64/compress-vedio /tmp/handle-files-release-assets/compress-vedio-macos-arm64
cp dist/macos-arm64/check-copy /tmp/handle-files-release-assets/check-copy-macos-arm64
cp dist/macos-amd64/compress-vedio /tmp/handle-files-release-assets/compress-vedio-macos-amd64
cp dist/macos-amd64/check-copy /tmp/handle-files-release-assets/check-copy-macos-amd64
test -x /tmp/handle-files-release-assets/compress-vedio-macos-arm64
test -x /tmp/handle-files-release-assets/check-copy-macos-arm64
test -x /tmp/handle-files-release-assets/compress-vedio-macos-amd64
test -x /tmp/handle-files-release-assets/check-copy-macos-amd64
rm -rf /tmp/handle-files-release-assets
```

Expected: all executable checks pass; the temporary directory is removed.

- [ ] **Step 5: Review workflow changes**

Run:

```bash
git diff --check
git diff -- .github/workflows/release.yml
```

Expected: no whitespace errors; trigger, permissions, build, unique assets, release action, and non-force release branch command match the workflow above.

- [ ] **Step 6: Commit the workflow**

Run:

```bash
git add .github/workflows/release.yml
git commit -S -s -m "Add release workflow"
```

Expected: one signed, signed-off commit containing only the release workflow.
