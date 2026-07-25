# Go binary refactor design

## Goal

Replace the two Python command-line tools with independently built Go binaries while preserving their file-processing behavior. Keep the Python scripts as behavior references, but make the Go binaries the documented primary entry points.

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

### check-copy

```text
check-copy --source/-s <directory> --destination/-d <directory> [--copy/-c]
```

| Option | Meaning |
| --- | --- |
| `--source`, `-s` | Required source directory. |
| `--destination`, `-d` | Required destination directory. |
| `--copy`, `-c` | Copy source-only files and files where the source is larger. Without it, report only. |

## Behavior

### compress-vedio

1. Validate the input directory and confirm `ffmpeg` is available.
2. Recursively find regular files with a case-insensitive `.mp4` extension, sorting paths stably.
3. Skip and report files whose stem ends exactly in `_output`.
4. For every remaining file, form an output path by appending `_output` before the original extension.
5. In preview mode, report all planned work without invoking ffmpeg or modifying files.
6. In execute mode, prompt for each file unless `--yes` is set. Only an affirmative `y` proceeds.
7. Invoke ffmpeg with the input path, parsed `--ff-option` values, and output path.
8. On successful ffmpeg completion, report size reduction and delete the original file.
9. On ffmpeg failure, report it, delete the output path if present, and continue processing later files.

Per-file failures are counted and shown in the summary but do not stop the batch, matching the Python script's behavior.

### check-copy

1. Validate that source and destination exist, are directories, and resolve to different locations.
2. Recursively map each regular file as a slash-normalized relative path to its byte size. Failed stat operations produce warnings and scanning continues.
3. Detect source paths that collide after case folding and report each conflict group.
4. Compare maps to identify source-only files, destination-only files, and same-path size mismatches.
5. Split mismatches into source-larger and destination-larger groups, then print all groups in sorted-path order.
6. With `--copy`, copy source-only and source-larger files only. Create needed parent directories and preserve source permissions and modification time.
7. On a copy failure, warn, remove any partial destination file, count the failure, and continue.
8. Print the same scan and difference summary categories as the Python script.

Destination-only files and destination-larger files are always reported but never deleted or overwritten.

## Errors and exit behavior

Unknown options, missing required options, malformed `--ff-option` input, invalid directories, identical source/destination paths, and unavailable ffmpeg cause an error message and nonzero exit. Recoverable file-level failures remain warnings or summary counts so a batch can complete.

## Tests and validation

Unit tests will use temporary directories and injected process execution:

- MP4 detection is recursive, case-insensitive, sorted, and skips `_output` stems.
- ffmpeg option parsing accepts quoted and escaped values and rejects malformed input.
- Compression command construction and success/failure cleanup work without a real ffmpeg binary.
- Directory comparisons classify missing, extra, source-larger, and destination-larger files correctly.
- Case-conflict detection reports colliding paths.
- Copying creates parent directories and preserves permissions and modification time.

Run `gofmt` on changed Go files and validate with `go test ./...`.
