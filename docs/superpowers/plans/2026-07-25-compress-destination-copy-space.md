# Compression destination and disk-space handling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add source/destination/removal controls and live FFmpeg output to `compress-vedio`, and abort `check-copy` after a no-space copy failure while naming every uncopied file.

**Architecture:** `compress.Options` owns CLI normalization, while `compress.Run` derives output paths and manages encoding lifecycle. Extend the existing command-runner seam to stream production FFmpeg stdout/stderr without weakening unit tests. `checkcopy.Run` remains the sole copy-loop owner and recognizes wrapped `syscall.ENOSPC` errors before attempting later candidates.

**Tech Stack:** Go 1.26 standard library (`flag`, `os/exec`, `errors`, `syscall`), Go `testing`, repository integration tests, FFmpeg.

## Global Constraints

- `compress-vedio --dir` and its old `-d` alias must be rejected; use `--source/-s` and optional `--dest/-d`.
- `--source` and a supplied `--dest` must name existing directories.
- `--remove` accepts only `true` or `false`, defaults to `true`, and must accept both `--remove false` and `--remove=false`.
- No destination writes occur in preview mode. With no destination, write `<stem>_output<extension>` beside input; with one, preserve input's relative source-tree path under destination.
- Source deletion only follows valid output creation and only when `Remove` is true.
- Real FFmpeg stdout and stderr must be forwarded unmodified during encoding; preserve readable structured logs before and after it.
- An error matching `syscall.ENOSPC` stops all remaining `check-copy --copy` candidates and reports the failed plus unstarted relative paths.
- Preserve existing structured logger output style, output-file cleanup on encoding failure, case-conflict skips, and copy metadata behavior.

---

### Task 1: Parse and validate compression source, destination, and removal options

**Files:**
- Modify: `internal/compress/options.go:15-103`
- Modify: `internal/compress/options_test.go:1-171`

**Interfaces:**
- Consumes: `DefaultFFOptions` and `ParseFFOptions(string) ([]string, error)`.
- Produces: `Options{Source string, Destination string, Remove bool, Execute bool, FFArgs []string}` for `compress.Run`.

- [ ] **Step 1: Write failing parser tests for the renamed options and removal values**

Add table-driven cases that create temporary source and destination directories, then assert the exact normalized `Options` fields:

```go
func TestParseOptionsAcceptsSourceDestinationAndRemove(t *testing.T) {
    source, destination := t.TempDir(), t.TempDir()
    got, err := ParseOptions([]string{
        "-s", source, "-d", destination, "--remove", "false", "-x",
    })
    if err != nil {
        t.Fatal(err)
    }
    if got.Source != source || got.Destination != destination ||
        got.Remove || !got.Execute {
        t.Fatalf("ParseOptions() = %#v", got)
    }
}
```

Add cases proving:

```go
ParseOptions([]string{"--source", source})              // Remove == true
ParseOptions([]string{"--source", source, "--remove=false"})
ParseOptions([]string{"--dir", source})                 // undefined flag error
ParseOptions([]string{"--source", source, "--dest", missing})
ParseOptions([]string{"--source", source, "--remove", "invalid"})
```

Assert the destination case names the destination directory and the invalid removal case identifies `--remove`.

- [ ] **Step 2: Run the option tests and verify they fail**

Run:

```bash
go test ./internal/compress -run 'TestParseOptions' -count=1
```

Expected: FAIL because `--source`, `--dest`, and `--remove` do not yet populate the new `Options` fields, and old `--dir` is still accepted.

- [ ] **Step 3: Implement the renamed option contract**

Replace `Options.Directory` with:

```go
type Options struct {
    Source      string
    Destination string
    Remove      bool
    Execute     bool
    FFArgs      []string
}
```

Bind `--source/-s`, `--dest/-d`, and `--remove` with `flag.StringVar`. Initialize the remove value to `"true"`, accept only literal `"true"` and `"false"` after parsing, and return a validation error for any other value. Normalize the required source and optional destination through an existing-directory helper; leave `Destination` empty when omitted. Remove both `--dir` registrations so the flag package rejects them. Update `Usage()` to show:

```text
compress-vedio --source/-s <directory> [--dest/-d <directory>] [--remove <true|false>] [--execute/-x] [--ff-option/-f "<ffmpeg options>"]
```

Include one preview example with `-s` and one execution example with `-s`, `-d`, and `--remove false`.

- [ ] **Step 4: Run the focused option tests and verify they pass**

Run:

```bash
gofmt -w internal/compress/options.go internal/compress/options_test.go
go test ./internal/compress -run 'TestParseOptions|TestParseFFOptions' -count=1
```

Expected: PASS, including default removal, explicit spaced and equals removal values, destination validation, and rejected `--dir`.

- [ ] **Step 5: Commit the parser change**

```bash
git add internal/compress/options.go internal/compress/options_test.go
git commit -S -s -m "Add compression source and destination options"
```

### Task 2: Stream FFmpeg and map compression output paths

**Files:**
- Modify: `internal/compress/service.go:18-220`
- Modify: `internal/compress/service_test.go:1-430`

**Interfaces:**
- Consumes: `Options.Source`, `Options.Destination`, `Options.Remove`, and `logx.Logger`.
- Produces: `Run(context.Context, Options, CommandRunner, logx.Logger) (Summary, error)` with mapped output paths and live process output.
- Extends: `CommandRunner` with `RunWithOutput(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error`.

- [ ] **Step 1: Write failing service tests for paths, removal, logs, and streamed child output**

Extend `fakeRunner` with a `runWithOutput` callback and make the fake `RunWithOutput` append its call to `calls`. Add focused tests that:

1. Place `nested/clip.mp4` under a temporary source and use a temporary destination. Have fake FFmpeg create its last argument. Assert the argument is `destination/nested/clip_output.mp4`, the destination subdirectory was made, and the source was removed by default.
2. Set `Remove: false`; assert the source remains after a successful mapped output.
3. Capture logger output and assert `run_config` includes source, effective output root, `execute=true`, `remove=false`, and serialized FFmpeg arguments. Assert `compress_started` precedes `compressed`.
4. In `runWithOutput`, write `ffmpeg live output\n` to supplied stderr. Assert it reaches the logger error buffer and the `compressed` record still has input, output, `original_bytes`, and `output_bytes`.
5. Make `runWithOutput` create a partial mapped output and return an error. Assert the source remains and the mapped output was removed.

The streaming test callback should follow this shape:

```go
runWithOutput: func(stdout, stderr io.Writer, _ string, args ...string) error {
    _, _ = io.WriteString(stderr, "ffmpeg live output\n")
    mustWrite(t, args[len(args)-1], "compressed")
    return nil
},
```

- [ ] **Step 2: Run the new service tests and verify they fail**

Run:

```bash
go test ./internal/compress -run 'TestRun.*(Destination|Remove|Config|Streams)' -count=1
```

Expected: FAIL because no streaming runner method, effective destination computation, or conditional removal exists.

- [ ] **Step 3: Implement output streaming and deterministic output mapping**

Extend `CommandRunner` and `execRunner`:

```go
type CommandRunner interface {
    LookPath(name string) (string, error)
    Run(ctx context.Context, name string, args ...string) error
    RunWithOutput(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error
}

func (execRunner) RunWithOutput(
    ctx context.Context, stdout, stderr io.Writer, name string, args ...string,
) error {
    command := exec.CommandContext(ctx, name, args...)
    command.Stdout = stdout
    command.Stderr = stderr
    return command.Run()
}
```

At the top of `Run`, write `run_config` using `opts.Source`, the effective output root (`opts.Destination` when set, otherwise `opts.Source`), `opts.Execute`, `opts.Remove`, and `strings.Join(opts.FFArgs, " ")`. Scan `opts.Source`.

Replace `outputPath(input)` with an option-aware helper that returns `(string, error)`: for an empty destination, use input's directory; otherwise call `filepath.Rel(opts.Source, input)`, replace its basename with `<stem>_output<extension>`, and join it beneath `opts.Destination`. Before live FFmpeg execution, call `os.MkdirAll(filepath.Dir(output), 0o755)`, log `compress_started`, and call `RunWithOutput(ctx, logger.Out, logger.Err, ffmpegPath, args...)`.

Use the same helper for preview logs. Keep output validation and size calculations. Guard the existing `os.Remove(path)` behind `if opts.Remove`; do not count a successful retained source as skipped or failed. Use the computed mapped output for all cleanup and errors.

- [ ] **Step 4: Run all compression unit tests and verify they pass**

Run:

```bash
gofmt -w internal/compress/service.go internal/compress/service_test.go
go test ./internal/compress -count=1
```

Expected: PASS. Existing same-directory behavior remains valid after replacing fixtures such as `Options{Directory: root, Execute: true, FFArgs: []string{"-c:v", "libx264"}}` with `Options{Source: root, Execute: true, FFArgs: []string{"-c:v", "libx264"}}`.

- [ ] **Step 5: Commit the compression service change**

```bash
git add internal/compress/service.go internal/compress/service_test.go
git commit -S -s -m "Stream compression output and map destinations"
```

### Task 3: Verify the public compression command and document it

**Files:**
- Modify: `integration/cli_test.go:12-107`
- Modify: `README.md:17-51`

**Interfaces:**
- Consumes: `compress-vedio` built from `cmd/compress-vedio` and a `PATH`-injected fake `ffmpeg`.
- Produces: documented public CLI behavior using `--source/-s`, `--dest/-d`, and `--remove`.

- [ ] **Step 1: Write failing integration tests for public argument behavior and raw FFmpeg output**

Replace all compression `--dir` invocations with `--source`. Add a test with a source `nested/clip.mp4`, an existing destination, and `--remove false`; assert source remains and output exists only at `destination/nested/clip_output.mp4`.

Change `writeFakeFFmpeg` so non-version calls emit a recognizable line to stderr:

```sh
printf '%s\n' 'fake ffmpeg: encoding' >&2
```

Assert command output contains both `INFO, compress_started ` and `fake ffmpeg: encoding`. Add an invocation using `--dir` and assert it returns a structured `ERROR, validation_failed ` record. Update the missing-FFmpeg test to use `--source`.

- [ ] **Step 2: Run the compression integration tests and verify they fail**

Run:

```bash
go test ./integration -run 'TestCompressVedio' -count=1
```

Expected: FAIL until the binary accepts new flags, maps destination subdirectories, preserves a source with `--remove false`, and forwards fake FFmpeg stderr.

- [ ] **Step 3: Update README usage and behavior description**

Replace the compression command synopsis, option table, and examples with the public contract:

```text
compress-vedio --source/-s <directory> [--dest/-d <directory>] [--remove <true|false>] [--execute/-x] [--ff-option/-f "<ffmpeg options>"]
```

Document that no destination retains same-directory output, an explicit destination preserves source-relative subdirectories, destination must exist, source removal defaults to true, and FFmpeg output is shown directly during encoding. Remove all claims that `--dir/-d` selects the source.

- [ ] **Step 4: Run integration tests and formatting**

Run:

```bash
gofmt -w integration/cli_test.go
go test ./integration -count=1
```

Expected: PASS for both binaries, including readable logs and public compression behavior.

- [ ] **Step 5: Commit public CLI tests and documentation**

```bash
git add integration/cli_test.go README.md
git commit -S -s -m "Document compression destination controls"
```

### Task 4: Abort copying when destination storage is exhausted

**Files:**
- Modify: `internal/checkcopy/service.go:1-190`
- Modify: `internal/checkcopy/service_test.go:1-560`

**Interfaces:**
- Consumes: `copyFile(sourceRoot string, destinationRoot *os.Root, relativePath string, entry Entry) error`, sorted non-conflicting copy candidates, and errors wrapping `syscall.ENOSPC`.
- Produces: a stopped copy loop, `copy_aborted_no_space` summary, and one `copy_not_completed` warning per failed or unstarted relative path.

- [ ] **Step 1: Write a failing no-space test**

Add `syscall` to test imports. Create source files `a-fails.txt` and `b-unstarted.txt` so sorted candidates are deterministic. Replace `copyStream` only for `a-fails.txt`:

```go
original := copyStream
copyStream = func(destination io.Writer, source io.Reader) (int64, error) {
    if sourceFile, ok := source.(*os.File); ok &&
        sourceFile.Name() == failedSource {
        return 0, syscall.ENOSPC
    }
    return io.Copy(destination, source)
}
t.Cleanup(func() { copyStream = original })
```

Call `Run` in copy mode, then assert:

```go
_, err := os.Stat(filepath.Join(destination, "b-unstarted.txt"))
if !errors.Is(err, os.ErrNotExist) { t.Fatalf("later copy was attempted: %v", err) }
if !strings.Contains(logs.String(), "WARN, copy_aborted_no_space") {
    t.Fatalf("missing no-space summary:\n%s", logs.String())
}
if !strings.Contains(logs.String(), "WARN, copy_not_completed path=a-fails.txt") {
    t.Fatalf("failed path missing from summary:\n%s", logs.String())
}
if !strings.Contains(logs.String(), "WARN, copy_not_completed path=b-unstarted.txt") {
    t.Fatalf("unstarted path missing from summary:\n%s", logs.String())
}
```

Also assert the standard difference summary reports `failed=1` and no successful copy occurred.

- [ ] **Step 2: Run the no-space test and verify it fails**

Run:

```bash
go test ./internal/checkcopy -run TestRunStopsCopyAfterNoSpace -count=1
```

Expected: FAIL because the existing loop logs the first failure and continues to copy `b-unstarted.txt`.

- [ ] **Step 3: Stop the loop on wrapped `ENOSPC` and log its remaining work**

Import `syscall` in `internal/checkcopy/service.go`. In the failure branch after `copyFile`, keep the existing `copy_failed` progress record. Then check:

```go
if errors.Is(err, syscall.ENOSPC) {
    remaining := candidates[completed:]
    logger.Warn("copy_aborted_no_space",
        logx.Field{Key: "error", Value: err.Error()},
        logx.Field{Key: "failed_path", Value: path},
        logx.Field{Key: "remaining", Value: strconv.Itoa(len(remaining))},
    )
    for _, remainingPath := range remaining {
        logger.Warn("copy_not_completed",
            logx.Field{Key: "path", Value: remainingPath},
        )
    }
    break
}
```

Do not return early: preserve `logScanSummary`, `logDifferenceSummary`, and `logCaseConflicts` after the loop. Ordinary copy failures must continue to later candidates unchanged.

- [ ] **Step 4: Run all check-copy unit tests and verify they pass**

Run:

```bash
gofmt -w internal/checkcopy/service.go internal/checkcopy/service_test.go
go test ./internal/checkcopy -count=1
```

Expected: PASS, including partial-output cleanup, continued ordinary failures, and immediate no-space termination with a complete remaining-file list.

- [ ] **Step 5: Run repository verification and commit the copy behavior**

Run:

```bash
go test ./... -count=1
git add internal/checkcopy/service.go internal/checkcopy/service_test.go
git commit -S -s -m "Stop copies after disk space exhaustion"
```

Expected: all Go unit and integration tests pass before the signed, sign-off commit.
