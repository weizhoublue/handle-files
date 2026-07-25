# Release log contract repair design

## Goal

Restore a releasable `main` branch by making the compression configuration-log
test and user documentation match the current human-readable logging contract.

## Logging contract

At the start of `compress-vedio`, the compression service emits:

```text
INFO, run config:  execute_copy=<bool> ffmpeg_args=<args> output_dir=<path> remove_original=<bool> source_dir=<path>
```

The logger continues to sort fields alphabetically. The configuration record
precedes `compress_started`, which precedes `compressed`.

## Changes

- Keep the current compression-service event and fields unchanged.
- Update the configuration-log test to assert the current event, field names,
  values, and ordering.
- Add a concise `compress-vedio` documentation note explaining the startup
  configuration fields.
- Do not modify the release workflow or packaging commands.

## Validation

Run the focused compression test and full Go test suite. Cross-compile both
commands for macOS arm64 and amd64, then verify the resulting files are Mach-O
executables.
