# Go binary refactor design

## Goal

Replace the two Python command-line tools with independently built Go binaries while preserving their file-processing behavior. Keep the Python scripts as behavior references, but make the Go binaries the documented primary entry points.

The release targets macOS and produces separate Apple Silicon (`darwin/arm64`) and Intel (`darwin/amd64`) binaries. Build artifacts are not committed.

The binaries are:

- `compress-vedio`, replacing `compress_mp4.py`
- `check-copy`, replacing `sync_check.py`

## Repository structure

Create one Go module with separate command entry points:

```text
cmd/
  compress-vedio/
    main.go
  check-copy/
    main.go
internal/
  compress/
    ...
  checkcopy/
    ...
```

`internal/compress` owns MP4 discovery, ffmpeg validation and execution, per-file confirmation, output cleanup, and source deletion. `internal/checkcopy` owns recursive scanning, comparisons, case-conflict detection, reporting, and copying. The packages remain independent because their domain behavior does not overlap.

The project provides a `make build-macos` target that runs `go build` for both commands and both target architectures. It writes the four ignored executables to `dist/macos-arm64/` and `dist/macos-amd64/`.

## Command-line interfaces

Both programs use Go's standard `flag` package and support `--help` and `-h`.

### compress-vedio

```text
compress-vedio --dir/-d <directory> [--execute/-x] [--yes/-y] [--ff-option/-f "<ffmpeg options>"]
```

| Option | Meaning |
| --- | --- |
| `--dir`, `-d` | Required root directory to scan recursively. |
| `--execute`, `-x` | Run compression. Without it, the program performs a non-mutating preview. |
| `--yes`, `-y` | In execute mode, process every file without a per-file confirmation prompt. |
| `--ff-option`, `-f` | ffmpeg encoder options as one quoted string. Default: `-c:v libx264 -crf 26 -preset slow -c:a aac -b:a 192k`. |

`--ff-option` is parsed into an argument array using a small shell-like lexer that supports quoted values and backslash escaping. It never invokes a shell. Unclosed quotes or dangling escapes are command-line errors.

All inputs are validated before scanning: `--dir` is required, non-empty, and resolves to an existing directory; no positional arguments are accepted; `--yes` requires `--execute`; and `--ff-option` must parse to at least one argument.

### check-copy

```text
check-copy --source/-s <directory> --destination/-d <directory> [--copy/-c]
```

| Option | Meaning |
| --- | --- |
| `--source`, `-s` | Required source directory. |
| `--destination`, `-d` | Required destination directory. |
| `--copy`, `-c` | Copy source-only files and files where the source is larger. Without it, report only. |

All inputs are validated before scanning: both paths are required, non-empty, existing directories; no positional arguments are accepted; and the resolved source and destination must differ.

## Behavior

### compress-vedio

1. Validate all command-line values, the input directory, and the `--yes`/`--execute` combination.
2. Locate `ffmpeg` with the executable lookup API and execute `ffmpeg -version`. Absence, launch failure, or nonzero status is a fatal dependency error that names the required dependency and the failed check.
3. Recursively find regular files with a case-insensitive `.mp4` extension, sorting paths stably.
4. Skip and report files whose stem ends exactly in `_output`.
5. For every remaining file, form an output path by appending `_output` before the original extension.
6. In preview mode, report all planned work without invoking ffmpeg or modifying files.
7. In execute mode, prompt for each file unless `--yes` is set. Only an affirmative `y` proceeds.
8. Invoke ffmpeg with the input path, parsed `--ff-option` values, and output path.
9. On successful ffmpeg completion, report size reduction and delete the original file.
10. After each eligible file is confirmed, skipped, completed, or fails, emit its overall progress: completed count, total count, and success, skip, and failure counts.
11. On ffmpeg failure, report it, delete the output path if present, and continue processing later files.

Per-file failures are counted and shown in the summary but do not stop the batch, matching the Python script's behavior.

### check-copy

1. Validate all command-line values, including source and destination existence, directory type, and distinct resolved paths.
2. Recursively map each regular file as a slash-normalized relative path to its byte size. Failed stat operations produce warnings and scanning continues.
3. Detect source paths that collide after case folding and report each conflict group.
4. Compare maps to identify source-only files, destination-only files, and same-path size mismatches.
5. Split mismatches into source-larger and destination-larger groups, then print all groups in sorted-path order.
6. With `--copy`, copy source-only and source-larger files only. For every source path in a case-conflict group, skip copying it; non-conflicting candidates continue. After all processing, emit one structured warning with the skipped conflict group and file counts. Create needed parent directories and preserve source permissions and modification time for copied files.
7. After every non-conflicting copy attempt, emit its overall progress: completed count, total copy candidates, and success and failure counts.
8. On a copy failure, warn, remove any partial destination file, count the failure, and continue.
9. Print the same scan and difference summary categories as the Python script.

Destination-only files and destination-larger files are always reported but never deleted or overwritten.

## Errors and exit behavior

Unknown options, positional arguments, missing or invalid required values, an invalid `--yes` combination, malformed or empty `--ff-option` input, invalid directories, identical source/destination paths, and unavailable or unhealthy ffmpeg cause an error message and nonzero exit. Recoverable file-level failures remain warnings or summary counts so a batch can complete.

Both commands emit structured, human-readable console logs by default. Every program-owned operational line includes an RFC 3339 timestamp, severity, event name, and relevant key-value fields, such as `time=2026-07-25T06:12:04Z level=INFO event=progress completed=3 total=10`. Informational and progress records use standard output; validation failures and warnings use standard error. Native ffmpeg output is not reformatted. No log files are created.

## Tests and validation

Unit tests will use temporary directories and injected process execution:

- MP4 detection is recursive, case-insensitive, sorted, and skips `_output` stems.
- ffmpeg option parsing accepts quoted and escaped values and rejects malformed input.
- Compression command construction and success/failure cleanup work without a real ffmpeg binary.
- Directory comparisons classify missing, extra, source-larger, and destination-larger files correctly.
- In copy mode, every path in a case-conflict group is skipped while non-conflicting copies continue, followed by one structured warning with skipped group and file counts.
- Copying creates parent directories and preserves permissions and modification time.
- Flag parsing rejects positional arguments, empty required values, invalid flag combinations, and invalid path values.
- ffmpeg dependency checks distinguish executable absence, launch failure, and nonzero version checks.
- Progress and structured console log records appear once for every attempted compression or copy.

End-to-end tests build each command and invoke it against temporary source and destination fixtures. Compression tests place a controlled fake `ffmpeg` on `PATH` so they verify dependency checks, option forwarding, output handling, source deletion, logs, and progress without encoding video. Copy tests execute report and copy modes through the compiled `check-copy` binary.

The CI verification loop uses a macOS runner matrix with native Apple Silicon and native Intel runners. Each runner runs unit and end-to-end tests for its native architecture, while `make build-macos` compiles all four release binaries. Run `gofmt` on changed Go files and validate with `go test ./...`.
