# Signal Interruption Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop `compress-vedio` and `check-copy` safely on `SIGINT` or `SIGTERM`, delete only active partial outputs, and return the signal's conventional exit status.

**Architecture:** Each service independently installs a short-lived signal listener that derives a cancellable execution context and returns an exported typed interruption error containing the first signal. Compression reuses its existing failed-output cleanup and stops immediately after cancellation. Check-copy receives a context throughout scanning, retrying, and copying; its copy loop becomes chunked and context-aware so the existing partial-destination cleanup executes before the service returns.

**Tech Stack:** Go 1.26; standard-library `context`, `errors`, `os`, `os/signal`, `syscall`, `sync`, `time`, `io`, and existing Go unit tests.

## Global Constraints

- Handle only `SIGINT` and `SIGTERM`; map them to exit status 130 and 143 respectively.
- Do not attempt cleanup for `SIGKILL`.
- Each service owns its signal subscription; do not add a shared signal package.
- Return an identifiable interruption error even if cleanup reports an error.
- Preserve source files, previously completed compressed outputs, and previously completed copied files.
- Stop `check-copy` during scanning, retry waiting, and active copying; do not start a later candidate after interruption.
- Keep non-signal error behavior and cleanup behavior unchanged.
- Do not add dependencies or change CLI flags.
- Format changed Go files with `gofmt`.
- Every implementation commit uses `git commit -s -S` and a concise one-line English summary.

---

## File Structure

- `internal/compress/service.go`: own compression signal subscription, expose an interruption error, and stop the compression loop after an interrupted FFmpeg invocation has cleaned its current output.
- `internal/compress/service_test.go`: inject a signal into the service listener and prove active-output cleanup, source preservation, and early termination.
- `internal/checkcopy/service.go`: own copy-service signal subscription, accept and propagate `context.Context`, make scans/retries/copying cancellation-aware, and retain partial-copy cleanup.
- `internal/checkcopy/service_test.go`: update `Run` callers for the context parameter and test cancellation during scans, retry delays, and copy chunks.
- `cmd/compress-vedio/main.go`: map a compression interruption error to its conventional process exit status.
- `cmd/compress-vedio/main_test.go`: test compression command exit-status mapping without starting a process.
- `cmd/check-copy/main.go`: pass `context.Background()` to the changed check-copy service and map its interruption error to process status.
- `cmd/check-copy/main_test.go`: test check-copy command exit-status mapping without starting a process.

### Task 1: Add interruption-aware compression

**Files:**
- Modify: `internal/compress/service.go:3-193`
- Modify: `internal/compress/service_test.go:17-53, existing Run tests`

**Interfaces:**
- Consumes: `Run(ctx context.Context, opts Options, runner CommandRunner, logger logx.Logger) (Summary, error)` and the existing `CommandRunner.RunWithOutput` context argument.
- Produces: `type InterruptedError struct { Signal os.Signal; Err error }`, `func (e *InterruptedError) Unwrap() error`, `func (e *InterruptedError) ExitCode() int`, and the unchanged public `compress.Run` signature.

- [ ] **Step 1: Add a failing interrupted-compression test**

Extend `fakeRunner` so its callbacks receive the context:

```go
type fakeRunner struct {
    path          string
    lookPathErr   error
    run           func(name string, args ...string) error
    runWithOutput func(stdout, stderr io.Writer, name string, args ...string) error
    runWithOutputContext func(context.Context, io.Writer, io.Writer, string, ...string) error
    calls         []runnerCall
}

func (r *fakeRunner) RunWithOutput(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
    r.calls = append(r.calls, runnerCall{name: name, args: append([]string(nil), args...)})
    if r.runWithOutputContext != nil {
        return r.runWithOutputContext(ctx, stdout, stderr, name, args...)
    }
    if r.runWithOutput != nil {
        return r.runWithOutput(stdout, stderr, name, args...)
    }
    if r.run != nil {
        return r.run(name, args...)
    }
    return nil
}
```

Add a test that replaces the package-private signal notifier with a function that waits until the fake FFmpeg callback has created an output file and then sends `os.Interrupt`. The callback must wait for `<-ctx.Done()` and return `ctx.Err()`. Assert all of the following:

```go
if !errors.As(err, &interrupted) || interrupted.Signal != os.Interrupt {
    t.Fatalf("Run() error = %v, want SIGINT interruption", err)
}
if got := interrupted.ExitCode(); got != 130 {
    t.Fatalf("ExitCode() = %d, want 130", got)
}
if _, statErr := os.Stat(output); !errors.Is(statErr, os.ErrNotExist) {
    t.Fatalf("partial output stat error = %v, want not exist", statErr)
}
if _, statErr := os.Stat(source); statErr != nil {
    t.Fatalf("source stat error = %v", statErr)
}
if got := len(startedInputs); got != 1 {
    t.Fatalf("started input count = %d, want 1", got)
}
```

Have `runWithOutputContext` append `args[1]` to `startedInputs` before it
creates the partial output and waits for cancellation. Configure two source
videos so this assertion proves the second FFmpeg command never starts.

Add a table-driven test for `InterruptedError.ExitCode()` with `os.Interrupt`, `syscall.SIGTERM`, and an unsupported signal. Expect 130, 143, and 1.

- [ ] **Step 2: Run the new compression tests and confirm failure**

Run:

```bash
go test ./internal/compress -run 'TestRunInterrupt|TestInterruptedErrorExitCode' -count=1
```

Expected: FAIL because the service does not subscribe to signals, does not expose `InterruptedError`, and continues after the failed FFmpeg call.

- [ ] **Step 3: Add a service-local signal listener and interruption error**

In `internal/compress/service.go`, add `os/signal`, `sync`, and `syscall` imports. Keep the listener private to this package:

```go
var (
    notifyInterrupt = signal.Notify
    stopInterrupt   = signal.Stop
)

type InterruptedError struct {
    Signal os.Signal
    Err    error
}

func (e *InterruptedError) Error() string {
    return fmt.Sprintf("interrupted by %s: %v", e.Signal, e.Err)
}

func (e *InterruptedError) Unwrap() error { return e.Err }

func (e *InterruptedError) ExitCode() int {
    switch e.Signal {
    case os.Interrupt:
        return 130
    case syscall.SIGTERM:
        return 143
    default:
        return 1
    }
}
```

Add a package-private listener helper returning an execution context and a
finalizer. It must:

1. Subscribe a buffered channel to `os.Interrupt` and `syscall.SIGTERM`.
2. Record only the first signal and cancel the derived context.
3. Call `stopInterrupt` immediately after receiving that signal, so a second
   signal uses default Go behavior.
4. On finalization, stop the subscription, wait for its goroutine to exit, and
   return `nil` or `&InterruptedError{Signal: received, Err: runErr}`.

Use a `done` channel, a `finished` channel, and `sync.Once` to make
finalization race-free. Add `<-parent.Done()` as another listener select case;
caller cancellation must not manufacture an `InterruptedError`.

- [ ] **Step 4: Split the existing run body and stop after cleanup**

Keep the exported entrypoint as the signal-owning wrapper:

```go
func Run(ctx context.Context, opts Options, runner CommandRunner, logger logx.Logger) (Summary, error) {
    runCtx, finish := interruptContext(ctx)
    summary, err := run(runCtx, opts, runner, logger)
    if interrupted := finish(err); interrupted != nil {
        return summary, interrupted
    }
    return summary, err
}
```

Move the current body into private `run`. In the `RunWithOutput` error branch,
retain the current `os.Remove(output)` cleanup and `compress_failed` log. After
that log, return `summary, ctx.Err()` when the context is canceled instead of
executing `continue`. Also check `ctx.Err()` before each input and after a
successful `RunWithOutput` call, so a cancellation never begins another input
or marks a partially interrupted invocation successful.

- [ ] **Step 5: Format and run all compression tests**

Run:

```bash
gofmt -w internal/compress/service.go internal/compress/service_test.go
go test ./internal/compress -count=1
```

Expected: PASS, including existing failed-output cleanup tests and the new
interruption tests.

- [ ] **Step 6: Commit the compression service change**

```bash
git add internal/compress/service.go internal/compress/service_test.go
git commit -s -S -m "feat: clean compression output on interrupt"
```

### Task 2: Make check-copy cancellation-aware and clean partial copies

**Files:**
- Modify: `internal/checkcopy/service.go:1-538`
- Modify: `internal/checkcopy/service_test.go:existing Run callers and copy tests`

**Interfaces:**
- Consumes: `checkcopy.Options`, existing `copyFailure(destinationRoot, relativePath, operationErr) error`, and `logx.Logger`.
- Produces: `Run(ctx context.Context, opts Options, logger logx.Logger) error`; `InterruptedError` with `Unwrap` and `ExitCode`; `scan(ctx context.Context, root string, filter extensionFilter, logger logx.Logger) (map[string]Entry, error)`; `copyWithRetries(ctx context.Context, ...) error`; and `copyFile(ctx context.Context, ...) error`.

- [ ] **Step 1: Change existing check-copy tests to pass a context**

Mechanically update every existing `Run` call in
`internal/checkcopy/service_test.go` to pass `context.Background()` first:

```go
err := Run(context.Background(), Options{
    Source: source,
    Destination: destination,
    Copy: true,
}, testLogger(&logs))
```

Add `context` to the test imports. Keep `Scan(root, logger)` as its existing
background-context convenience API, so its existing focused tests keep their
signature.

- [ ] **Step 2: Add failing cancellation tests**

Add these tests in `internal/checkcopy/service_test.go`:

1. `TestRunStopsWhenInterruptedDuringScan`: inject `os.Interrupt` before a
   scan walk can complete; expect an `*InterruptedError`, exit code 130, and
   no destination file.
2. `TestRunStopsRetryDelayWhenInterrupted`: force first copy attempt to fail,
   make the retry wait block on a test channel, inject `syscall.SIGTERM`, and
   assert no second attempt and exit code 143.
3. `TestCopyWithContextStopsAtChunkBoundary`: use a reader that calls
   `cancel()` after its first chunk and a `bytes.Buffer` destination; expect
   `context.Canceled` before a second source read.
4. `TestRunInterruptCleansPartialDestination`: make the context-aware copy
   function write `"partial"` to its destination and return `ctx.Err()` after
   cancellation; assert `copyFailure` removed that destination, a later
   candidate was not opened, and the error is an `*InterruptedError`.

Use the same package-private `notifyInterrupt` injection pattern as Task 1,
but define it independently in `checkcopy`; do not import compression's
helper.

- [ ] **Step 3: Run the focused check-copy tests and confirm failure**

Run:

```bash
go test ./internal/checkcopy -run 'TestRunStopsWhenInterruptedDuringScan|TestRunStopsRetryDelayWhenInterrupted|TestCopyWithContextStopsAtChunkBoundary|TestRunInterruptCleansPartialDestination' -count=1
```

Expected: FAIL because `Run` has no context argument, scan and retry waiting do
not observe cancellation, and file copying uses uninterruptible `io.Copy`.

- [ ] **Step 4: Add check-copy's service-local listener and context plumbing**

Add a dedicated listener directly in `internal/checkcopy/service.go`; do not
import compression code. Add the same imports used by its own implementation:
`os/signal`, `sync`, and `syscall`. Define these check-copy-local symbols:

```go
var (
    notifyInterrupt = signal.Notify
    stopInterrupt   = signal.Stop
)

type InterruptedError struct {
    Signal os.Signal
    Err    error
}

func (e *InterruptedError) Error() string {
    return fmt.Sprintf("interrupted by %s: %v", e.Signal, e.Err)
}

func (e *InterruptedError) Unwrap() error { return e.Err }

func (e *InterruptedError) ExitCode() int {
    switch e.Signal {
    case os.Interrupt:
        return 130
    case syscall.SIGTERM:
        return 143
    default:
        return 1
    }
}
```

Implement private `interruptContext(parent context.Context)` with a buffered
signal channel, `done` and `finished` channels, and `sync.Once`. Its goroutine
selects among the first `os.Interrupt`/`syscall.SIGTERM`, `<-parent.Done()`, and
`<-done`. On a received signal it stores that signal, calls `stopInterrupt`,
and cancels the derived context. Its finalizer stops the subscription, closes
`done` once, waits for `finished`, cancels the derived context, and wraps the
run error in `InterruptedError` only when a signal was stored.

Change the public service entrypoint and preserve the listener boundary:

```go
func Run(ctx context.Context, opts Options, logger logx.Logger) error {
    runCtx, finish := interruptContext(ctx)
    err := run(runCtx, opts, logger)
    if interrupted := finish(err); interrupted != nil {
        return interrupted
    }
    return err
}
```

Move the current body into private `run`. Change `scan`, `copyWithRetries`, and
`copyFile` to receive `ctx context.Context`. Return `ctx.Err()`:

- before starting either scan;
- at the beginning of every `filepath.WalkDir` callback;
- before every copy candidate;
- before each retry attempt;
- immediately when a retry delay's `select` receives `<-ctx.Done()`.

Use `time.NewTimer(time.Second)` for retry waiting and stop/drain it correctly
when the context fires. Do not call the old unconditional
`sleepBeforeCopyRetry` in the cancellation-aware path.

When `copyWithRetries` or `copyFile` returns `ctx.Err()`, return immediately:
do not log `copy_retrying`, do not increment a normal failed-copy summary, and
do not attempt another candidate. The already-existing `copyFailure` call must
remain on the error path after a destination file has opened.

- [ ] **Step 5: Replace uninterruptible file-copy optimization with chunked copying**

Replace the `io.Copy`-based `copyStream` path and the `ReadFrom` optimization
types that bypass a wrapper's `Read` method. Add a private copy helper:

```go
const copyBufferSize = 32 * 1024

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
    buffer := make([]byte, copyBufferSize)
    var total int64
    for {
        if err := ctx.Err(); err != nil {
            return total, err
        }
        read, readErr := src.Read(buffer)
        if read > 0 {
            written, writeErr := dst.Write(buffer[:read])
            total += int64(written)
            if writeErr != nil {
                return total, writeErr
            }
            if written != read {
                return total, io.ErrShortWrite
            }
        }
        if readErr != nil {
            if errors.Is(readErr, io.EOF) {
                return total, nil
            }
            return total, readErr
        }
    }
}
```

Call it through a package-private function variable with the same signature so
the partial-destination test can deterministically write before returning
`ctx.Err()`:

```go
var copyStream = copyWithContext
```

Keep `newCopySourceReader` and `newCopyDestinationWriter` only if they still
provide the existing read/write operation error wrapping. Remove their
file-specific fields and `ReadFrom` implementations once `copyWithContext`
makes them unreachable. Replace the prior optimization-preservation test with
the chunk-boundary cancellation test from Step 2.

- [ ] **Step 6: Format and run all check-copy tests**

Run:

```bash
gofmt -w internal/checkcopy/service.go internal/checkcopy/service_test.go
go test ./internal/checkcopy -count=1
```

Expected: PASS. Existing non-signal copy failure tests still verify removal of
partial files, while new tests verify cancellation at scans, retry waits, and
copy chunk boundaries.

- [ ] **Step 7: Commit the check-copy service change**

```bash
git add internal/checkcopy/service.go internal/checkcopy/service_test.go
git commit -s -S -m "feat: stop copy safely on interrupt"
```

### Task 3: Map service interruption errors to command exit statuses

**Files:**
- Modify: `cmd/compress-vedio/main.go:3-34`
- Create: `cmd/compress-vedio/main_test.go`
- Modify: `cmd/check-copy/main.go:3-32`
- Create: `cmd/check-copy/main_test.go`

**Interfaces:**
- Consumes: `*compress.InterruptedError` and `*checkcopy.InterruptedError`, each exposing `ExitCode() int`.
- Produces: package-private `exitCode(err error) int` helpers in both command packages; `check-copy` calls `checkcopy.Run(context.Background(), options, logger)`.

- [ ] **Step 1: Add failing exit-code tests for both command packages**

Create `cmd/compress-vedio/main_test.go`:

```go
package main

import (
    "errors"
    "os"
    "syscall"
    "testing"

    "github.com/weizhoublue/handle-files/internal/compress"
)

func TestExitCodeMapsCompressionInterruptions(t *testing.T) {
    tests := []struct {
        name string
        err  error
        want int
    }{
        {"SIGINT", &compress.InterruptedError{Signal: os.Interrupt}, 130},
        {"SIGTERM", &compress.InterruptedError{Signal: syscall.SIGTERM}, 143},
        {"other", errors.New("failed"), 1},
    }
    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            if got := exitCode(test.err); got != test.want {
                t.Fatalf("exitCode(%v) = %d, want %d", test.err, got, test.want)
            }
        })
    }
}
```

Create `cmd/check-copy/main_test.go`:

```go
package main

import (
    "errors"
    "os"
    "syscall"
    "testing"

    "github.com/weizhoublue/handle-files/internal/checkcopy"
)

func TestExitCodeMapsCheckCopyInterruptions(t *testing.T) {
    tests := []struct {
        name string
        err  error
        want int
    }{
        {"SIGINT", &checkcopy.InterruptedError{Signal: os.Interrupt}, 130},
        {"SIGTERM", &checkcopy.InterruptedError{Signal: syscall.SIGTERM}, 143},
        {"other", errors.New("failed"), 1},
    }
    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            if got := exitCode(test.err); got != test.want {
                t.Fatalf("exitCode(%v) = %d, want %d", test.err, got, test.want)
            }
        })
    }
}
```

- [ ] **Step 2: Run command tests and confirm failure**

Run:

```bash
go test ./cmd/compress-vedio ./cmd/check-copy -count=1
```

Expected: FAIL because neither command defines `exitCode`, and check-copy has
not yet been wired to its context-taking `Run`.

- [ ] **Step 3: Add error classification and use it in both commands**

In each `main.go`, add a private mapper matching that service's type:

```go
func exitCode(err error) int {
    var interrupted *compress.InterruptedError
    if errors.As(err, &interrupted) {
        return interrupted.ExitCode()
    }
    return 1
}
```

Use `checkcopy.InterruptedError` in check-copy's counterpart. In both `run`
functions, retain the existing `run_failed` log but return `exitCode(err)`
instead of literal `1`. Add `context` to `cmd/check-copy/main.go` and call:

```go
if err := checkcopy.Run(context.Background(), options, logger); err != nil {
    logger.Error("run_failed", logx.Field{Key: "error", Value: err.Error()})
    return exitCode(err)
}
```

Do not alter help or validation behavior; those still return 0 and 1.

- [ ] **Step 4: Format and run affected, integration, and full test suites**

Run:

```bash
gofmt -w cmd/compress-vedio/main.go cmd/compress-vedio/main_test.go cmd/check-copy/main.go cmd/check-copy/main_test.go
go test ./internal/compress ./internal/checkcopy ./cmd/compress-vedio ./cmd/check-copy -count=1
go test ./integration -count=1
go test ./... -count=1
```

Expected: PASS. The first command tests prove 130/143 mapping, focused service
tests prove cleanup, integration tests retain existing CLI behavior, and the
full suite guards all callers updated for `checkcopy.Run`'s new signature.

- [ ] **Step 5: Commit command wiring and tests**

```bash
git add cmd/compress-vedio/main.go cmd/compress-vedio/main_test.go cmd/check-copy/main.go cmd/check-copy/main_test.go
git commit -s -S -m "feat: return signal exit statuses"
```

## Plan Self-Review

**Spec coverage:** Task 1 covers compression cancellation, active-output
cleanup, source preservation, and no later input. Task 2 covers check-copy
signal ownership, scan cancellation, retry cancellation, chunked copy
cancellation, partial-destination cleanup, and no later candidate. Task 3
covers conventional exit statuses for both commands. The global constraints
cover signal scope, `SIGKILL`, no new dependencies, and non-signal behavior.

**Placeholder scan:** No tasks defer implementation or testing; all interfaces,
files, failure commands, expected results, and commit commands are specified.

**Type consistency:** Both services expose their own
`*InterruptedError{Signal os.Signal, Err error}` and `ExitCode() int`. Only
`checkcopy.Run` changes to `Run(context.Context, Options, logx.Logger) error`;
Task 2 updates its unit-test callers and Task 3 updates its production caller.
