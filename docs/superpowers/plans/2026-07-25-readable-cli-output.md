# Readable CLI Output Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make both Go command-line tools emit concise human-readable logs, combine processing results with progress counters, and make `compress-vedio --execute` non-interactive.

**Architecture:** `internal/logx` owns line rendering, field ordering, and an optional bracketed progress group. `checkcopy` and `compress` calculate their existing counters immediately before logging each copy or compression outcome; they no longer create separate progress records. The command entry points no longer provide clocks or stdin to these internals.

**Tech Stack:** Go 1.26, standard library `flag`, `os/exec`, `testing`, existing fake ffmpeg integration harness.

## Global Constraints

- All logger levels use `<LEVEL>, <event> [key=value ...]` without timestamps, `level=`, or `event=`.
- INFO writes to stdout; WARN and ERROR write to stderr.
- Field names remain sorted lexicographically.
- A copy or compression attempt produces its outcome record with a bracketed, sorted progress group; no standalone `progress` record is emitted.
- `compress-vedio --execute/-x` runs directly, without `--yes/-y`, confirmation logs, or stdin reads.
- Preserve preview behavior, ffmpeg dependency checks, partial-output cleanup, source retention on failures, and source deletion after successful compression.
- Use `git commit -s -S -m "<concise English summary>"` for every plan commit.

---

## File Structure

| File | Responsibility |
| --- | --- |
| `internal/logx/logger.go` | Render readable level/event lines and optional bracketed progress fields. |
| `internal/logx/logger_test.go` | Lock down normal and progress-rendered log records. |
| `cmd/check-copy/main.go` | Create the timestamp-free logger. |
| `cmd/compress-vedio/main.go` | Create the timestamp-free logger and call the non-interactive compression API. |
| `internal/checkcopy/service.go` | Attach counters to each copy result and remove separate progress records. |
| `internal/checkcopy/service_test.go` | Assert copy success and failure records each include their counters. |
| `internal/compress/options.go` | Remove confirmation option parsing and help text. |
| `internal/compress/options_test.go` | Assert direct execution option parsing and rejection of removed flags. |
| `internal/compress/service.go` | Remove stdin confirmation and attach counters to each compression result. |
| `internal/compress/service_test.go` | Assert direct execution and one progress-bearing result record per attempted file. |
| `integration/cli_test.go` | Verify compiled binaries use non-interactive execution and readable validation/output records. |
| `README.md` | Remove `--yes/-y` documentation and describe direct execution. |

### Task 1: Render readable logger records

**Files:**
- Modify: `internal/logx/logger.go`
- Modify: `internal/logx/logger_test.go`
- Modify: `cmd/check-copy/main.go`
- Modify: `cmd/compress-vedio/main.go`

**Interfaces:**
- Produces: `Logger.InfoProgress(event string, fields []Field, progress []Field)`, `Logger.WarnProgress(event string, fields []Field, progress []Field)`, and `Logger.ErrorProgress(event string, fields []Field, progress []Field)`.
- Consumes: Existing `logx.Field` values from both services.

- [ ] **Step 1: Write failing logger-format tests**

Replace the timestamp-based INFO expectation and add exact WARN/ERROR and progress assertions:

```go
logger.Info("missing", Field{Key: "path", Value: "1/test1"})
if got := out.String(); got != "INFO, missing path=1/test1\n" {
    t.Fatalf("Info() = %q", got)
}

logger.ErrorProgress(
    "copy_failed",
    []Field{{Key: "path", Value: "1/test2"}, {Key: "error", Value: "write failed"}},
    []Field{{Key: "total", Value: "2"}, {Key: "failed", Value: "1"}, {Key: "completed", Value: "2"}, {Key: "succeeded", Value: "1"}},
)
if got := err.String(); got != "ERROR, copy_failed error=write failed path=1/test2 [ completed=2 failed=1 succeeded=1 total=2 ]\n" {
    t.Fatalf("ErrorProgress() = %q", got)
}
```

- [ ] **Step 2: Run logger tests to verify failure**

Run:

```bash
go test ./internal/logx -count=1
```

Expected: FAIL because current output starts with `time=`, and progress-aware logger methods do not exist.

- [ ] **Step 3: Implement readable record rendering**

In `logger.go`:

```go
type Logger struct {
    Out io.Writer
    Err io.Writer
}

func (l Logger) InfoProgress(event string, fields []Field, progress []Field) {
    l.log(l.Out, "INFO", event, fields, progress)
}

func (l Logger) WarnProgress(event string, fields []Field, progress []Field) {
    l.log(l.Err, "WARN", event, fields, progress)
}

func (l Logger) ErrorProgress(event string, fields []Field, progress []Field) {
    l.log(l.Err, "ERROR", event, fields, progress)
}
```

Keep `Info`, `Warn`, and `Error` as convenience methods that call the same renderer with no progress group. Sort normal fields and progress fields independently, write `"<LEVEL>, <event>"`, then normal fields, then `" [ "` plus sorted progress fields plus `" ]"` only when progress is non-empty. Remove `time` and `Now`.

In both command `main.go` files, remove the `time` import and initialize the logger with:

```go
logger := logx.Logger{Out: os.Stdout, Err: os.Stderr}
```

- [ ] **Step 4: Run logger and command compilation tests**

Run:

```bash
go test ./internal/logx ./cmd/check-copy ./cmd/compress-vedio -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit logger rendering**

```bash
git add internal/logx/logger.go internal/logx/logger_test.go cmd/check-copy/main.go cmd/compress-vedio/main.go
git commit -s -S -m "Format CLI logs for terminal users"
```

### Task 2: Remove compression confirmation and merge compression progress

**Files:**
- Modify: `internal/compress/options.go`
- Modify: `internal/compress/options_test.go`
- Modify: `internal/compress/service.go`
- Modify: `internal/compress/service_test.go`
- Modify: `cmd/compress-vedio/main.go`

**Interfaces:**
- Consumes: `logx.Logger.InfoProgress`, `WarnProgress`, and `ErrorProgress` from Task 1.
- Produces: `Run(ctx context.Context, opts Options, runner CommandRunner, logger logx.Logger) (Summary, error)` with no `io.Reader` parameter.
- Produces: `Options` with `Directory string`, `Execute bool`, and `FFArgs []string`; it has no `Yes` field.

- [ ] **Step 1: Write failing option and execution tests**

Replace confirmation tests with direct-execution behavior:

```go
got, err := ParseOptions([]string{"-d", dir, "-x", "-f", "-c:v libx264"})
if err != nil {
    t.Fatal(err)
}
if got.Execute != true {
    t.Fatalf("Execute = %v, want true", got.Execute)
}

_, err = ParseOptions([]string{"--dir", dir, "--yes"})
if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
    t.Fatalf("ParseOptions(--yes) error = %v", err)
}
```

Replace `TestRunRequiresConfirmationForEachLiveFile` with a test using two source files and no stdin argument. Assert `Summary{Total: 2, Succeeded: 2}`, both source files are removed, two ffmpeg encoding calls follow the version call, and logs include:

```go
"INFO, compressed "
"[ completed=2 failed=0 skipped=0 succeeded=2 total=2 ]"
```

Assert `strings.Count(logs.String(), "progress") == 0`.

- [ ] **Step 2: Run compression tests to verify failure**

Run:

```bash
go test ./internal/compress -count=1
```

Expected: FAIL because `--yes` is still accepted, `Run` requires a reader, and compression awaits confirmation.

- [ ] **Step 3: Remove confirmation configuration and stdin dependency**

In `options.go`, delete `Yes`, flag registrations for `--yes/-y`, the `--yes requires --execute` validation, and their usage text. Change usage to:

```text
Usage: compress-vedio --dir/-d <directory> [--execute/-x] [--ff-option/-f "<ffmpeg options>"]
```

In `service.go`, remove `bufio` and the reader argument from `Run`. Delete confirmation scanner setup, `confirm` records, `not_confirmed` skips, and scanner error handling. In `cmd/compress-vedio/main.go`, call:

```go
compress.Run(context.Background(), options, compress.NewCommandRunner(), logger)
```

- [ ] **Step 4: Attach counters to every compression outcome**

Add a local helper:

```go
func progressFields(summary Summary) []logx.Field {
    return []logx.Field{
        {Key: "completed", Value: strconv.Itoa(summary.Succeeded + summary.Skipped + summary.Failed)},
        {Key: "total", Value: strconv.Itoa(summary.Total)},
        {Key: "succeeded", Value: strconv.Itoa(summary.Succeeded)},
        {Key: "skipped", Value: strconv.Itoa(summary.Skipped)},
        {Key: "failed", Value: strconv.Itoa(summary.Failed)},
    }
}
```

After incrementing `Succeeded` or `Failed`, use the matching progress-aware logger method for `compressed`, `compress_failed`, `output_missing`, `source_size_failed`, and `source_delete_failed`; remove `logProgress`. For an encoding failure, attempt cleanup before logging the one failure result and include `cleanup_error=<error>` in that record when cleanup fails, rather than emitting a separate `cleanup_failed` record. Do not emit `compressed` until source deletion succeeds, so every attempted file has one outcome record.

- [ ] **Step 5: Run compression tests to verify pass**

Run:

```bash
go test ./internal/compress -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit non-interactive compression**

```bash
git add internal/compress/options.go internal/compress/options_test.go internal/compress/service.go internal/compress/service_test.go cmd/compress-vedio/main.go
git commit -s -S -m "Run video compression without confirmation"
```

### Task 3: Merge copy progress with copy outcomes

**Files:**
- Modify: `internal/checkcopy/service.go`
- Modify: `internal/checkcopy/service_test.go`

**Interfaces:**
- Consumes: `logx.Logger.InfoProgress` and `logx.Logger.WarnProgress` from Task 1.
- Produces: Copy outcome records containing `completed`, `failed`, `succeeded`, and `total` in a bracketed suffix.

- [ ] **Step 1: Write failing copy-progress tests**

Replace `TestRunEmitsProgressForEveryCopyCandidate` with a test that checks exactly two `copied` result lines, no `progress` record, and final counters on the second result:

```go
if got := strings.Count(logs.String(), "INFO, copied "); got != 2 {
    t.Fatalf("copied records = %d, want 2:\n%s", got, logs.String())
}
if !strings.Contains(logs.String(), "INFO, copied path=source-larger.txt [ completed=2 failed=0 succeeded=2 total=2 ]") {
    t.Fatalf("final copied record missing progress:\n%s", logs.String())
}
if strings.Contains(logs.String(), "progress") {
    t.Fatalf("standalone progress record emitted:\n%s", logs.String())
}
```

Update injected-copy-failure tests to expect:

```go
"WARN, copy_failed "
"[ completed=2 failed=1 succeeded=1 total=2 ]"
```

Update existing `event=` expectations in this package to readable records, including scan summaries, difference summaries, and case-conflict warnings.

- [ ] **Step 2: Run check-copy tests to verify failure**

Run:

```bash
go test ./internal/checkcopy -count=1
```

Expected: FAIL because successful and failed copy events do not contain bracketed counters and standalone progress records still exist.

- [ ] **Step 3: Implement outcome-attached copy progress**

Add a local helper:

```go
func copyProgressFields(completed, total, succeeded, failed int) []logx.Field {
    return []logx.Field{
        {Key: "completed", Value: strconv.Itoa(completed)},
        {Key: "total", Value: strconv.Itoa(total)},
        {Key: "succeeded", Value: strconv.Itoa(succeeded)},
        {Key: "failed", Value: strconv.Itoa(failed)},
    }
}
```

Inside the copy loop, increment the relevant counter first. Call `WarnProgress("copy_failed", ...)` or `InfoProgress("copied", ...)` with `copyProgressFields(completed+1, len(candidates), succeeded, failed)`. Remove the standalone `logger.Info("progress", ...)` call. Keep comparison, scan, difference summary, and conflict warnings unchanged apart from their Task 1 readable rendering.

- [ ] **Step 4: Run check-copy tests to verify pass**

Run:

```bash
go test ./internal/checkcopy -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit copy progress records**

```bash
git add internal/checkcopy/service.go internal/checkcopy/service_test.go
git commit -s -S -m "Merge copy results with progress"
```

### Task 4: Update CLI integration coverage and documentation

**Files:**
- Modify: `integration/cli_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: Non-interactive `compress-vedio --execute` from Task 2 and readable log rendering from Task 1.
- Produces: User-facing help and examples without `--yes/-y`.

- [ ] **Step 1: Write failing binary-level tests**

Change direct compression invocation to omit `--yes`:

```go
output, err := runCommand(binary, []string{"--dir", root, "--execute"}, fakeBin, nil)
```

Replace stdin-confirmation integration coverage with an assertion that both files are compressed when stdin is nil. Add readable-output checks:

```go
if !strings.Contains(string(output), "INFO, copied ") || strings.Contains(string(output), "time=") {
    t.Fatalf("unexpected copy output: %s", output)
}
```

Update invalid-flag coverage to require:

```go
"ERROR, validation_failed "
```

- [ ] **Step 2: Run integration tests to verify failure**

Run:

```bash
go test ./integration -count=1
```

Expected: FAIL because tests still pass `--yes` and expect `event=`-prefixed output.

- [ ] **Step 3: Update README and integration tests**

In `README.md`, remove `--yes/-y` from the synopsis and options table. Replace confirmation wording with “`--execute/-x` directly compresses all discovered files.” Update the execution example to:

```bash
compress-vedio --dir /Volumes/Data/Videos --execute
```

Update integration assertions and test names to reflect direct execution and the `INFO, ...`/`ERROR, ...` formats.

- [ ] **Step 4: Run full verification**

Run:

```bash
go test ./... -count=1
make build-macos
```

Expected: PASS; all four macOS binaries build.

- [ ] **Step 5: Commit documentation and integration coverage**

```bash
git add integration/cli_test.go README.md
git commit -s -S -m "Document direct video compression"
```

