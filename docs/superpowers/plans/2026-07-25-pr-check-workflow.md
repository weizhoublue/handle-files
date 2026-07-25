# PR Check Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the existing test workflow with a Linux amd64 PR check for pull requests targeting `main` or `master`.

**Architecture:** Modify the existing `.github/workflows/test.yml` in place so GitHub exposes one `PR Check` workflow. Its sole Ubuntu job installs the Go toolchain specified in `go.mod`, runs all Go tests, then confirms both CLI entry points cross-compile for Linux amd64.

**Tech Stack:** GitHub Actions, `actions/checkout@v4`, `actions/setup-go@v5`, Go 1.26 from `go.mod`

## Global Constraints

- Trigger only for `pull_request` events whose target branch is `main` or `master`.
- Do not run this workflow for direct pushes.
- Use the Go version declared in `go.mod`.
- Test with `go test ./... -count=1`.
- Build `./cmd/compress-vedio` and `./cmd/check-copy` with `GOOS=linux GOARCH=amd64`.
- Do not create release artifacts or run macOS builds.

---

### Task 1: Replace the Test Workflow with PR Check

**Files:**
- Modify: `.github/workflows/test.yml`
- Test: `.github/workflows/test.yml` via local Go test and cross-build commands

**Interfaces:**
- Consumes: `go.mod`, which declares the Go version for `actions/setup-go@v5`.
- Consumes: command entry points `./cmd/compress-vedio` and `./cmd/check-copy`.
- Produces: GitHub Actions workflow `PR Check`, visible for pull requests targeting `main` and `master`.

- [ ] **Step 1: Confirm the existing repository validations pass**

Run:

```bash
go test ./... -count=1
GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/compress-vedio
GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/check-copy
```

Expected: each command exits with status 0.

- [ ] **Step 2: Replace `.github/workflows/test.yml` with the PR-only workflow**

```yaml
name: PR Check

on:
  pull_request:
    branches: [main, master]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Unit tests
        run: go test ./... -count=1

      - name: Linux amd64 builds
        run: |
          GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/compress-vedio
          GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/check-copy
```

- [ ] **Step 3: Validate the changed workflow and its commands**

Run:

```bash
git diff --check
go test ./... -count=1
GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/compress-vedio
GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/check-copy
```

Expected: all commands exit with status 0; the workflow contains no `push` trigger or macOS matrix.

- [ ] **Step 4: Commit the workflow change**

```bash
git add .github/workflows/test.yml
git commit -S -s -m "Add PR check workflow"
```
