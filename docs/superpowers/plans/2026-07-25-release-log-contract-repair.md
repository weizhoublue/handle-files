# Release Log Contract Repair Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `main` releasable by aligning the compression configuration-log test and user documentation with the current human-readable log contract.

**Architecture:** `compress.Run` already owns the intended configuration event and field values, so it remains unchanged. Its unit test becomes the executable contract for the event text, sorted field names, and ordering before compression. The command documentation describes the same startup record for users.

**Tech Stack:** Go 1.26, standard-library testing, Make, GitHub Actions macOS cross-compilation.

## Global Constraints

- Preserve `run config: `, `source_dir`, `output_dir`, `execute_copy`, `remove_original`, and `ffmpeg_args`.
- Preserve the current `logx.Logger` field sorting and output streams.
- Do not change the release workflow or packaging commands.
- Build macOS arm64 and amd64 outputs outside the repository when validating.

---

### Task 1: Align compression configuration-log contract

**Files:**
- Modify: `internal/compress/service_test.go:207-274`
- Modify: `docs/readme.md:14-25`
- Test: `internal/compress/service_test.go:207-274`

**Interfaces:**
- Consumes: `Run(ctx context.Context, opts Options, runner CommandRunner, logger logx.Logger) (Summary, error)`.
- Produces: a regression contract for `INFO, run config: ` and documentation for its startup fields.

- [ ] **Step 1: Run the existing regression test to capture the failure**

Run:

```bash
go test ./internal/compress -run '^TestRunConfigLogsSettingsAndStartBeforeCompressed$' -count=1
```

Expected: FAIL because the test requires the removed `run_config`, `source`,
`output_root`, `execute`, and `remove` names.

- [ ] **Step 2: Update the configuration-log assertions**

Replace the legacy assertions in `TestRunConfigLogsSettingsAndStartBeforeCompressed`:

```go
if !strings.Contains(logs, "INFO, run_config ") ||
    !strings.Contains(logs, "source="+sourceRoot) ||
    !strings.Contains(logs, "output_root="+destinationRoot) ||
    !strings.Contains(logs, "execute=true") ||
    !strings.Contains(logs, "remove=false") ||
    !strings.Contains(logs, "ffmpeg_args=-c:v libx264 -preset slow") {
```

with:

```go
if !strings.Contains(logs, "INFO, run config:  ") ||
    !strings.Contains(logs, "source_dir="+sourceRoot) ||
    !strings.Contains(logs, "output_dir="+destinationRoot) ||
    !strings.Contains(logs, "execute_copy=true") ||
    !strings.Contains(logs, "remove_original=false") ||
    !strings.Contains(logs, "ffmpeg_args=-c:v libx264 -preset slow") {
```

Also change `runConfigIndex` to search for `"INFO, run config: "`.

- [ ] **Step 3: Run the focused regression test**

Run:

```bash
go test ./internal/compress -run '^TestRunConfigLogsSettingsAndStartBeforeCompressed$' -count=1
```

Expected: PASS, confirming the test recognizes the current configuration event and fields.

- [ ] **Step 4: Document the startup configuration record**

Add this bullet immediately after the `compress-vedio` introductory bullets in
`docs/readme.md`:

```markdown
- 启动时会输出 `run config:`，包含 `source_dir`、`output_dir`、`execute_copy`、`remove_original` 和 `ffmpeg_args`。
```

- [ ] **Step 5: Format and run the complete validation**

Run:

```bash
gofmt -w internal/compress/service_test.go
go test ./... -count=1
GOOS=darwin GOARCH=arm64 go build -o /tmp/handle-files-compress-darwin-arm64 ./cmd/compress-vedio
GOOS=darwin GOARCH=arm64 go build -o /tmp/handle-files-check-copy-darwin-arm64 ./cmd/check-copy
GOOS=darwin GOARCH=amd64 go build -o /tmp/handle-files-compress-darwin-amd64 ./cmd/compress-vedio
GOOS=darwin GOARCH=amd64 go build -o /tmp/handle-files-check-copy-darwin-amd64 ./cmd/check-copy
file /tmp/handle-files-compress-darwin-arm64 /tmp/handle-files-check-copy-darwin-arm64 /tmp/handle-files-compress-darwin-amd64 /tmp/handle-files-check-copy-darwin-amd64
rm -f /tmp/handle-files-compress-darwin-arm64 /tmp/handle-files-check-copy-darwin-arm64 /tmp/handle-files-compress-darwin-amd64 /tmp/handle-files-check-copy-darwin-amd64
```

Expected: all tests pass; the two arm64 files are Mach-O arm64 executables and
the two amd64 files are Mach-O x86_64 executables.

- [ ] **Step 6: Commit the repair**

```bash
git add internal/compress/service_test.go docs/readme.md
git commit -S -s -m "Fix release log contract"
```
