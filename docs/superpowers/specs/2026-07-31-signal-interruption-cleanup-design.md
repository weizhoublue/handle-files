# Signal interruption cleanup design

## Goal

When `compress-vedio` or `check-copy` receives `SIGINT` or `SIGTERM`, stop the
active operation, remove its incomplete output, and exit with the conventional
status for that signal. Do not remove source files, completed outputs, or
completed copies.

## Scope

The change applies to `compress-vedio` and `check-copy`.

- `SIGINT` exits with status 130.
- `SIGTERM` exits with status 143.
- `SIGKILL` is not catchable and is outside the cleanup guarantee.
- A signal received while `check-copy` is scanning or waiting to retry stops
  the command even though no partial output exists.

## Signal ownership

Each service owns its signal subscription rather than sharing a command-level
helper.

While executing, each service subscribes to `SIGINT` and `SIGTERM`, records the
first received signal, and cancels an internal context. It stops the
subscription before returning so later calls do not retain signal handlers. A
second signal uses Go's normal default behavior, preventing a stalled cleanup
from trapping the user.

Each service returns a typed interruption error that retains the received
signal. Command entrypoints recognize that type and return 130 for `SIGINT` or
143 for `SIGTERM`. Other failures retain exit status 1.

## `compress-vedio` flow

`compress.Run` derives its FFmpeg command context from the service-managed
signal context. When the service receives an interruption, `exec.CommandContext`
terminates the active FFmpeg process. The existing failed-command cleanup
removes the current output path.

After that cleanup, the service returns the typed interruption error immediately
instead of continuing to another input. It retains the source input and leaves
earlier completed output files unchanged.

## `check-copy` flow

`checkcopy.Run` accepts a context and uses its service-managed signal context
for all long-running steps:

1. Directory scans check cancellation during their walk.
2. The copy candidate loop checks cancellation before starting a candidate.
3. Retry waiting stops when the context is canceled rather than sleeping until
   its normal deadline.
4. File copying uses context-aware chunked I/O so cancellation is checked while
   a large file is copied.

When copying is interrupted, the current destination file follows the existing
`copyFailure` cleanup path. The service then returns the typed interruption
error without retrying that file, attempting later candidates, or emitting the
normal completion summaries.

## Error handling

Existing cleanup failures remain visible in the relevant compression or copy
failure log. The returned interruption remains identifiable even when cleanup
reports an error, so command exit status still reflects the signal.

Cancellation from a caller without a captured OS signal remains a normal
context error rather than being mapped to a signal exit status.

## Validation

Tests will cover:

1. Each command maps `SIGINT` to 130 and `SIGTERM` to 143.
2. Interrupted compression cancels FFmpeg, deletes the active output, preserves
   the source, and does not begin a later input.
3. `check-copy` stops when interrupted during scanning, retry waiting, and
   active copying.
4. Interrupted copying removes only the active partial destination and does not
   start later candidates.
5. Existing non-signal compression and copy failures retain their current
   cleanup and reporting behavior.
