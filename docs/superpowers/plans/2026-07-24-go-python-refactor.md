# Go Python Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver `compress-vedio` and `check-copy` as independently built, macOS-ready Go binaries that preserve the Python tools' behavior with validated named options, structured console logs, progress, and complete tests.

**Architecture:** Standard-library Go module with two thin `cmd` entry points and independent `internal/compress` and `internal/checkcopy` packages. A shared `internal/logx` package emits deterministic structured logs; compression receives an injectable command runner so its filesystem behavior is unit-testable without ffmpeg. End-to-end tests invoke binaries built for the host OS against temporary fixtures and a fake ffmpeg executable.

**Tech Stack:** Go 1.26 standard library, `flag`, `os/exec`, `path/filepath`, `io/fs`, `testing`, GNU Make, GitHub Actions macOS runners.

## Global Constraints

- Keep `compress_mp4.py` and `sync_check.py` as behavior references; do not delete or modify them.
- Primary binaries are named exactly `compress-vedio` and `check-copy`.
- Use only Go standard-library dependencies.
- Support only named long and short options; reject positional arguments.
- `compress-vedio` defaults to a non-mutating preview; `--yes/-y` is valid only with `--execute/-x`.
- Default ffmpeg options are exactly `-c:v libx264 -crf 26 -preset slow -c:a aac -b:a 192k`.
- Parse `--ff-option/-f` without starting a shell; reject unclosed quotes, dangling escapes, and empty parsed values.
- `check-copy` defaults to report-only and writes only with `--copy/-c`.
- Logs are RFC 3339 UTC key-value records; program info/progress uses stdout and warnings/errors use stderr. Do not create log files or reformat ffmpeg output.
- Emit a progress record after every eligible compression result and every copy attempt.
- Preserve source permissions and modification times when copying.
- Target macOS `darwin/arm64` and `darwin/amd64`; keep all artifacts under ignored `dist/`.
- Run `gofmt` on every changed Go file. Every commit uses `git commit -S -s` and includes the required Copilot trailer.

---

## File Structure

| File | Responsibility |
| --- | --- |
| `go.mod` | Defines module `github.com/weizhoublue/handle-files` and Go version. |
| `.gitignore` | Ignores `dist/` build output. |
| `Makefile` | Builds both commands for both macOS architectures. |
| `internal/logx/logger.go` | Writes timestamped, levelled, sorted key-value log records to supplied streams. |
| `internal/logx/logger_test.go` | Verifies log format and stdout/stderr routing. |
| `internal/compress/options.go` | Parses and validates compression CLI values. |
| `internal/compress/ffargs.go` | Converts a shell-like option string into an argument slice without executing a shell. |
| `internal/compress/options_test.go` | Covers compression flag and ff-option validation. |
| `internal/compress/service.go` | Discovers MP4 files, verifies/runs ffmpeg, manages confirmation, cleanup, summary, and progress. |
| `internal/compress/service_test.go` | Covers compression behavior with temporary files and a fake command runner. |
| `internal/checkcopy/options.go` | Parses and validates source, destination, and copy CLI values. |
| `internal/checkcopy/service.go` | Scans, compares, reports, copies, and reports progress. |
| `internal/checkcopy/service_test.go` | Covers comparison, conflicts, metadata-preserving copy, and failures. |
| `cmd/compress-vedio/main.go` | Binds process arguments and standard streams to `compress.Run`. |
| `cmd/check-copy/main.go` | Binds process arguments and standard streams to `checkcopy.Run`. |
| `integration/cli_test.go` | Builds host binaries and exercises real CLI behavior with temporary fixtures and fake ffmpeg. |
| `.github/workflows/test.yml` | Runs tests on native Intel and Apple Silicon macOS runners and builds all release binaries. |
| `README.md` | Documents Go prerequisites, macOS builds, binary installation, named options, dependency checks, and examples. |

## Task 1: Establish Go module, logging, and macOS build target

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `Makefile`
- Create: `internal/logx/logger.go`
- Create: `internal/logx/logger_test.go`

**Interfaces:**
- Produces `logx.Logger` for both domain packages:

```go
type Logger struct {
    Out io.Writer
    Err io.Writer
    Now func() time.Time
}

type Field struct {
    Key   string
    Value string
}

func (l Logger) Info(event string, fields ...Field)
func (l Logger) Warn(event string, fields ...Field)
func (l Logger) Error(event string, fields ...Field)
```

- Produces `make build-macos`, writing:

```text
dist/macos-arm64/compress-vedio
dist/macos-arm64/check-copy
dist/macos-amd64/compress-vedio
dist/macos-amd64/check-copy
```

- Consumes no earlier task outputs.

- [ ] **Step 1: Write failing logger tests**

```go
func TestLoggerInfoUsesSortedFieldsAndUTC(t *testing.T) {
    var out, err bytes.Buffer
    logger := Logger{
        Out: &out, Err: &err,
        Now: func() time.Time { return time.Date(2026, 7, 25, 6, 12, 4, 0, time.FixedZone("PDT", -7*3600)) },
    }

    logger.Info("progress", Field{Key: "total", Value: "10"}, Field{Key: "completed", Value: "3"})

    want := "time=2026-07-25T13:12:04Z level=INFO event=progress completed=3 total=10\n"
    if got := out.String(); got != want {
        t.Fatalf("Info() = %q, want %q", got, want)
    }
    if err.Len() != 0 {
        t.Fatalf("Info wrote stderr: %q", err.String())
    }
}
```

- [ ] **Step 2: Run the logger test to verify it fails**

Run: `go test ./internal/logx -run TestLoggerInfoUsesSortedFieldsAndUTC -count=1`

Expected: FAIL because package `internal/logx` does not exist.

- [ ] **Step 3: Add module, ignored output, build target, and logger**

Create `go.mod`:

```go
module github.com/weizhoublue/handle-files

go 1.26
```

Add `dist/` to `.gitignore`. Create `Makefile` with four explicit cross-build commands:

```make
build-macos:
	mkdir -p dist/macos-arm64 dist/macos-amd64
	GOOS=darwin GOARCH=arm64 go build -o dist/macos-arm64/compress-vedio ./cmd/compress-vedio
	GOOS=darwin GOARCH=arm64 go build -o dist/macos-arm64/check-copy ./cmd/check-copy
	GOOS=darwin GOARCH=amd64 go build -o dist/macos-amd64/compress-vedio ./cmd/compress-vedio
	GOOS=darwin GOARCH=amd64 go build -o dist/macos-amd64/check-copy ./cmd/check-copy
```

Implement `Logger` by sorting fields by `Key`, formatting `Now().UTC()` with `time.RFC3339`, and writing `INFO` to `Out` and `WARN`/`ERROR` to `Err`.

- [ ] **Step 4: Run formatter and logger tests**

Run: `gofmt -w internal/logx/*.go && go test ./internal/logx -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the foundation**

```bash
git add go.mod .gitignore Makefile internal/logx
git commit -S -s -m $'Add Go logging foundation\n\nCo-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>'
```

## Task 2: Add compression option parsing and safe ffmpeg option lexing

**Files:**
- Create: `internal/compress/options.go`
- Create: `internal/compress/ffargs.go`
- Create: `internal/compress/options_test.go`

**Interfaces:**
- Consumes: `logx` only indirectly in later tasks.
- Produces:

```go
const DefaultFFOptions = "-c:v libx264 -crf 26 -preset slow -c:a aac -b:a 192k"

type Options struct {
    Directory string
    Execute   bool
    Yes       bool
    FFArgs    []string
}

func ParseOptions(args []string) (Options, error)
func ParseFFOptions(value string) ([]string, error)
func Usage() string
```

- [ ] **Step 1: Write failing parser tests**

```go
func TestParseFFOptionsHonorsQuotesAndEscapes(t *testing.T) {
    got, err := ParseFFOptions(`-metadata title="A clip" -vf scale=1280\:720`)
    if err != nil {
        t.Fatal(err)
    }
    want := []string{"-metadata", "title=A clip", "-vf", "scale=1280:720"}
    if !slices.Equal(got, want) {
        t.Fatalf("ParseFFOptions() = %#v, want %#v", got, want)
    }
}

func TestParseOptionsRejectsInvalidCombinations(t *testing.T) {
    _, err := ParseOptions([]string{"--dir", t.TempDir(), "--yes"})
    if err == nil || !strings.Contains(err.Error(), "--yes requires --execute") {
        t.Fatalf("ParseOptions() error = %v", err)
    }
}
```

Also table-test empty directory, missing directory, a file supplied as directory, positional arguments, malformed quoted ff-option values, and a successful `-d`, `-x`, `-y`, `-f` parse.

- [ ] **Step 2: Run compression parser tests to verify they fail**

Run: `go test ./internal/compress -run 'TestParse(FFOptions|Options)' -count=1`

Expected: FAIL because package `internal/compress` does not exist.

- [ ] **Step 3: Implement options and lexer**

Use a `flag.NewFlagSet` configured with `flag.ContinueOnError`, discard its default output, and bind each long/short alias to the same variable:

```go
fs.StringVar(&directory, "dir", "", "directory to scan")
fs.StringVar(&directory, "d", "", "directory to scan")
fs.BoolVar(&execute, "execute", false, "compress files")
fs.BoolVar(&execute, "x", false, "compress files")
```

Define `--help/-h` explicitly, reject remaining `fs.Args()`, then validate the absolute directory with `os.Stat`. Parse `DefaultFFOptions` when `-f` is omitted. Implement a rune loop for `ParseFFOptions` with states `unquoted`, `singleQuoted`, and `doubleQuoted`; whitespace separates unquoted arguments, a backslash appends the next rune, and terminal quote/escape states return descriptive errors. Do not import a shell parser or call a shell.

- [ ] **Step 4: Run formatter and all compression parser tests**

Run: `gofmt -w internal/compress/*.go && go test ./internal/compress -run 'TestParse(FFOptions|Options)' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit compression option parsing**

```bash
git add internal/compress
git commit -S -s -m $'Add compression option parsing\n\nCo-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>'
```

## Task 3: Implement compression service, dependency checks, cleanup, and progress

**Files:**
- Create: `internal/compress/service.go`
- Create: `internal/compress/service_test.go`

**Interfaces:**
- Consumes: `compress.Options`, `logx.Logger`.
- Produces:

```go
type CommandRunner interface {
    LookPath(name string) (string, error)
    Run(ctx context.Context, name string, args ...string) error
}

func NewCommandRunner() CommandRunner

type Summary struct {
    Total     int
    Succeeded int
    Skipped   int
    Failed    int
}

func Run(ctx context.Context, opts Options, runner CommandRunner, input io.Reader, logger logx.Logger) (Summary, error)
```

- [ ] **Step 1: Write failing service tests**

Adapt existing `test_compress_mp4.py` to Go with a recursive uppercase-extension fixture:

```go
func TestDiscoverMP4FilesSkipsOutputFiles(t *testing.T) {
    root := t.TempDir()
    mustWrite(t, filepath.Join(root, "one", "two", "clip.MP4"), "video")
    mustWrite(t, filepath.Join(root, "one", "two", "clip_output.MP4"), "video")

    files, err := discoverMP4Files(root)
    if err != nil {
        t.Fatal(err)
    }
    want := []string{filepath.Join(root, "one", "two", "clip.MP4")}
    if !reflect.DeepEqual(files, want) {
        t.Fatalf("discoverMP4Files() = %#v, want %#v", files, want)
    }
}
```

Add fake-runner tests that assert `ffmpeg -version` is run before work, successful execution deletes source and retains output, a failing execution deletes output but retains source, and each attempted live file produces one `event=progress` record with the final summary counters.

- [ ] **Step 2: Run service tests to verify they fail**

Run: `go test ./internal/compress -run 'Test(DiscoverMP4Files|Run)' -count=1`

Expected: FAIL because `discoverMP4Files` and `Run` do not exist.

- [ ] **Step 3: Implement compression execution**

Implement `CommandRunner` with an unexported `execRunner` backed by `exec.LookPath` and `exec.CommandContext(...).Run()`. `Run` must:

1. call `LookPath("ffmpeg")` and `Run(ctx, ffmpegPath, "-version")`;
2. discover regular `.mp4` files with case-insensitive extension, sort paths, and emit a skip log for exact `_output` stems;
3. preview without invoking ffmpeg when `opts.Execute` is false;
4. read one confirmation line for each live file unless `opts.Yes`;
5. run `ffmpegPath`, `-i`, input path, `opts.FFArgs...`, output path;
6. delete the source only after success; on command failure, remove the output if it exists and preserve the source;
7. emit an `event=progress` info record after every live result with `completed`, `total`, `succeeded`, `skipped`, and `failed`.

Use `filepath.Ext` and `strings.EqualFold`; preserve input extension when forming `<stem>_output<extension>`.

- [ ] **Step 4: Run formatter and compression package tests**

Run: `gofmt -w internal/compress/*.go && go test ./internal/compress -count=1`

Expected: PASS.

- [ ] **Step 5: Commit compression service**

```bash
git add internal/compress
git commit -S -s -m $'Implement video compression service\n\nCo-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>'
```

## Task 4: Implement directory comparison, copying, and progress

**Files:**
- Create: `internal/checkcopy/options.go`
- Create: `internal/checkcopy/service.go`
- Create: `internal/checkcopy/service_test.go`

**Interfaces:**
- Consumes: `logx.Logger`.
- Produces:

```go
type Options struct {
    Source      string
    Destination string
    Copy        bool
}

type Entry struct {
    Size    int64
    Mode    fs.FileMode
    ModTime time.Time
}

type Comparison struct {
    Missing        []string
    Extra          []string
    SourceLarger   []string
    DestLarger     []string
    CaseConflicts  [][]string
}

func ParseOptions(args []string) (Options, error)
func Scan(root string, logger logx.Logger) (map[string]Entry, error)
func Compare(source, destination map[string]Entry) Comparison
func Run(opts Options, logger logx.Logger) error
func Usage() string
```

- [ ] **Step 1: Write failing check-copy tests**

```go
func TestCompareClassifiesEveryDifference(t *testing.T) {
    source := map[string]Entry{
        "missing.txt": {Size: 1},
        "src-large":   {Size: 3},
        "dst-large":   {Size: 1},
        "same":        {Size: 2},
    }
    destination := map[string]Entry{
        "extra.txt": {Size: 1},
        "src-large": {Size: 1},
        "dst-large": {Size: 3},
        "same":      {Size: 2},
    }

    got := Compare(source, destination)
    if !reflect.DeepEqual(got.Missing, []string{"missing.txt"}) ||
        !reflect.DeepEqual(got.Extra, []string{"extra.txt"}) ||
        !reflect.DeepEqual(got.SourceLarger, []string{"src-large"}) ||
        !reflect.DeepEqual(got.DestLarger, []string{"dst-large"}) {
        t.Fatalf("Compare() = %#v", got)
    }
}
```

Add tests for option rejection of positional, empty, file, and equal resolved directories; case-folding conflict groups; report-only mode not writing; copy mode creating nested parents; copied mode and modification time; failed copy cleanup; and one progress record per non-conflicting copy candidate. In copy mode, verify every source path in a case-conflict group is skipped, non-conflicting work continues, and one structured warning after all processing reports skipped conflict group and file counts.

- [ ] **Step 2: Run check-copy tests to verify they fail**

Run: `go test ./internal/checkcopy -count=1`

Expected: FAIL because package `internal/checkcopy` does not exist.

- [ ] **Step 3: Implement comparison and copying**

Parse `--source/-s`, `--destination/-d`, and `--copy/-c` with the same `flag.ContinueOnError` rules as Task 2. Validate each path via `filepath.Abs`, `filepath.EvalSymlinks`, and `os.Stat`.

Implement scanning with `filepath.WalkDir`, `DirEntry.Info`, `filepath.Rel`, and `filepath.ToSlash`. Warn and continue for per-file information failures. Sort every result slice. Group case conflicts by `strings.ToLower(relativePath)`. In copy mode, skip every source path in a case-conflict group, continue with non-conflicting `Missing` and `SourceLarger` entries using `os.MkdirAll`, `os.Open`, `os.OpenFile`, `io.Copy`, `os.Chmod`, and `os.Chtimes`, then emit one structured warning after all processing with skipped conflict group and file counts. Remove a destination partial file after a write or metadata failure. Emit `event=progress completed=<n> total=<n> succeeded=<n> failed=<n>` after each non-conflicting copy attempt.

- [ ] **Step 4: Run formatter and check-copy tests**

Run: `gofmt -w internal/checkcopy/*.go && go test ./internal/checkcopy -count=1`

Expected: PASS.

- [ ] **Step 5: Commit check-copy service**

```bash
git add internal/checkcopy
git commit -S -s -m $'Implement directory check and copy\n\nCo-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>'
```

## Task 5: Add command entry points and end-to-end tests

**Files:**
- Create: `cmd/compress-vedio/main.go`
- Create: `cmd/check-copy/main.go`
- Create: `integration/cli_test.go`

**Interfaces:**
- Consumes: `compress.ParseOptions`, `compress.Run`, `checkcopy.ParseOptions`, `checkcopy.Run`, `logx.Logger`.
- Produces process exit code `0` for completed batches and nonzero for invalid CLI values or missing/unhealthy ffmpeg.

- [ ] **Step 1: Write failing compiled-CLI tests**

```go
func TestCompressVedioExecuteUsesFakeFFmpeg(t *testing.T) {
    binary := buildBinary(t, "./cmd/compress-vedio")
    root := t.TempDir()
    input := filepath.Join(root, "clip.mp4")
    os.WriteFile(input, []byte("input"), 0o644)
    fakeBin := writeFakeFFmpeg(t)

    cmd := exec.Command(binary, "--dir", root, "--execute", "--yes")
    cmd.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
    output, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("command failed: %v\n%s", err, output)
    }
    if _, err := os.Stat(input); !errors.Is(err, os.ErrNotExist) {
        t.Fatalf("source still exists: %v", err)
    }
    if !strings.Contains(string(output), "event=progress completed=1 total=1") {
        t.Fatalf("missing progress: %s", output)
    }
}
```

Add a report-only `check-copy` invocation that leaves the destination unchanged, a `--copy` invocation that creates the missing target, and invalid-flag and missing-ffmpeg invocations that return nonzero.

- [ ] **Step 2: Run integration tests to verify they fail**

Run: `go test ./integration -count=1`

Expected: FAIL because command entry points and integration package do not exist.

- [ ] **Step 3: Implement CLI entry points and fixtures**

Each `main` must create a `logx.Logger` using `os.Stdout`, `os.Stderr`, and `time.Now`; call the relevant `ParseOptions(os.Args[1:])`; print the package `Usage()` text and return zero for `flag.ErrHelp`; log other parse errors with `event=validation_failed` and exit nonzero on returned fatal errors. `compress-vedio` passes `compress.NewCommandRunner()` and `os.Stdin` to `compress.Run`.

In `integration/cli_test.go`, implement `buildBinary` with `go build -o <temp path> <package>` and implement `writeFakeFFmpeg` as an executable shell script. The script exits zero for `-version`; for a normal invocation, writes its final argument as the output file and exits zero. Keep fake scripts and generated binaries only in test temporary directories.

- [ ] **Step 4: Run all Go tests**

Run: `gofmt -w cmd/compress-vedio/main.go cmd/check-copy/main.go integration/cli_test.go && go test ./... -count=1`

Expected: PASS.

- [ ] **Step 5: Commit command-line integration**

```bash
git add cmd integration
git commit -S -s -m $'Add Go command line binaries\n\nCo-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>'
```

## Task 6: Add macOS CI and update usage documentation

**Files:**
- Create: `.github/workflows/test.yml`
- Modify: `README.md`

**Interfaces:**
- Consumes: `make build-macos`, `go test ./...`, binary option interfaces.
- Produces macOS CI verification for native Intel and Apple Silicon, plus user-facing build and run instructions.

- [ ] **Step 1: Write workflow and README acceptance checks**

Record the expected workflow matrix in a testable YAML review checklist:

```yaml
matrix:
  include:
    - runner: macos-13
      goarch: amd64
    - runner: macos-14
      goarch: arm64
```

The README must contain exact examples:

```text
make build-macos
dist/macos-arm64/compress-vedio --dir /Volumes/Data/Videos
dist/macos-arm64/compress-vedio --dir /Volumes/Data/Videos --execute --yes
dist/macos-arm64/check-copy --source /Volumes/red/1 --destination /Volumes/black/1 --copy
```

- [ ] **Step 2: Verify the acceptance checks fail against current repository content**

Run: `test -f .github/workflows/test.yml; grep -F 'make build-macos' README.md`

Expected: the workflow check fails and README does not contain the Go build command.

- [ ] **Step 3: Add workflow and documentation**

Create `.github/workflows/test.yml` with `pull_request` and `push` triggers. Use the native macOS runner matrix above, set `GOOS=darwin` and matrix `GOARCH`, install the configured Go version with `actions/setup-go`, run `go test ./... -count=1`, then run `make build-macos` and assert all four expected files with `test -x`.

Revise README so Go binaries are first-class, Python is marked as behavior reference, and each option is documented. Include macOS ffmpeg installation guidance (`brew install ffmpeg`), explain that startup validates both lookup and `ffmpeg -version`, explain preview versus `--execute`, `--yes`, and `--ff-option`, and state that logs and per-file progress are emitted to the console.

- [ ] **Step 4: Run final local verification**

Run: `gofmt -w $(git ls-files '*.go') && go test ./... -count=1 && make build-macos && test -x dist/macos-arm64/compress-vedio && test -x dist/macos-arm64/check-copy && test -x dist/macos-amd64/compress-vedio && test -x dist/macos-amd64/check-copy`

Expected: all unit tests, end-to-end tests, and four cross-compiles pass.

- [ ] **Step 5: Remove generated artifacts and commit documentation and CI**

Run: `rm -rf dist`

```bash
git add .github/workflows/test.yml README.md
git commit -S -s -m $'Add macOS builds and documentation\n\nCo-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>'
```

## Plan Self-Review

- **Spec coverage:** Task 1 creates macOS dual-architecture builds and ignored artifacts. Tasks 2-5 implement every named option, validation rule, dependency check, structured logging, progress behavior, and preservation rule. Task 5 supplies compiled-binary end-to-end coverage, and Task 6 adds native macOS CI and README guidance.
- **Placeholder scan:** No incomplete requirements or deferred implementation markers remain.
- **Type consistency:** `compress.Options`, `compress.Run`, `checkcopy.Options`, `checkcopy.Run`, and `logx.Logger` are declared before later tasks consume them. No third-party package is named.
