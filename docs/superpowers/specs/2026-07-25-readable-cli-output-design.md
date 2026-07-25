# Readable CLI output design

## Goal

Make both Go binaries' logs concise for terminal users and make `compress-vedio --execute` run without per-file confirmation.

## Log format

`internal/logx.Logger` will produce human-readable records without timestamps or structured key names for all levels:

```text
INFO, missing path=1/test1
WARN, cleanup_failed error=<error> path=<path>
ERROR, run_failed error=<error>
```

The level is followed by a comma and the event name. Existing event fields remain sorted as `key=value` pairs. INFO continues to write to standard output; WARN and ERROR continue to write to standard error.

The logger will no longer need its clock dependency, so both command entry points will remove their `time` imports and clock injection.

## Compression execution

`compress-vedio --execute/-x` will process all discovered input files directly. The command will remove:

- `--yes/-y` flag parsing and usage documentation.
- `Options.Yes`.
- Confirmation prompt logging and stdin scanning.

Preview mode remains unchanged. Dependency checks, failed-output cleanup, source-file retention on failure, and source-file deletion after a successful compression remain unchanged.

## Per-item progress

Both commands will remove their standalone `progress` records. Each actual copy or compression outcome will instead include its counters in a bracketed suffix:

```text
INFO, copied path=1/test2 [ completed=2 failed=0 succeeded=2 total=2 ]
```

Failure outcomes include the same counters. Scan, comparison, preview, and final summary records remain separate because they do not represent completion of a copy or compression attempt.

## Documentation and tests

Update README and command usage to remove `--yes/-y` and state that execution proceeds directly. Update unit and integration tests to assert:

1. INFO, WARN, and ERROR records use the readable format without timestamps or `level=`/`event=`.
2. Execution succeeds without `--yes/-y` or stdin confirmation.
3. The removed confirmation flag is rejected.
4. Each copy or compression result includes counters and no standalone progress record is emitted.

## Scope

Changes are limited to the shared logger, command setup, compression options/execution behavior, related tests, and CLI documentation.
