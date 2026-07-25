# Compression destination and disk-space handling design

## Goal

Extend `compress-vedio` with explicit source, destination, and source-removal controls. Make its FFmpeg activity visible immediately in the terminal. Stop `check-copy` safely when a destination runs out of disk space and report every file left uncopied.

## Compression CLI and paths

`compress-vedio` will use:

```text
compress-vedio --source/-s <directory> [--dest/-d <directory>] [--remove <true|false>] [--execute/-x] [--ff-option/-f "<ffmpeg options>"]
```

`--dir` and its old `-d` alias will be removed and rejected. `--source/-s` is required. `--dest/-d` is optional, but when supplied it must name an existing directory. `--remove` accepts `true` or `false` and defaults to `true`; `--remove=false` remains accepted by the flag parser.

When no destination is supplied, output keeps the existing location and naming: the source file's directory receives `<stem>_output<extension>`. When a destination is supplied, each output is placed below that root using the input's path relative to the source root, while retaining the `_output` suffix. Parent directories beneath the destination root are created as needed.

## Compression execution and logging

At startup, the command emits a `run_config` INFO record that includes the source, effective output root, execute mode, remove mode, and parsed FFmpeg options. Before each live encoding, it emits `compress_started` with complete input and output paths.

The production command runner will expose an output-streaming execution method. Its FFmpeg child process will send stdout and stderr directly to the terminal, so native FFmpeg messages are visible while encoding. Test runners retain a controllable implementation of the same interface.

After successful output validation, the `compressed` record includes the absolute input and output paths, both exact byte sizes, and reduction bytes and percentage. Source deletion happens only after a valid output exists and only when `--remove=true`. Failed FFmpeg calls retain the source and remove an incomplete output at its effective output path.

## Copy exhaustion handling

During `check-copy --copy`, a copy error that wraps `ENOSPC` ends the copy loop immediately. The command does not attempt later candidates. It emits an exhaustion warning and then its normal summaries, including a per-file uncopied record for the failed candidate and every candidate that was not started. Other copy errors continue to be reported per file as before.

## Validation

Unit and integration tests will cover:

1. Parsing `--source/-s`, `--dest/-d`, and both values of `--remove`, with `--dir` rejected.
2. Same-directory output without a destination and relative-subdirectory output with one.
3. Conditional source deletion, startup configuration, size records, and streamed FFmpeg output.
4. `check-copy` stopping after an injected `ENOSPC`, with no later copy attempt and all remaining relative paths reported.

README usage and examples will match the revised compression CLI and behavior.

## Scope

Changes are confined to the Go compression options, command runner, compression service and tests, check-copy copy loop and tests, integration tests, and README. The legacy Python utilities are out of scope.
