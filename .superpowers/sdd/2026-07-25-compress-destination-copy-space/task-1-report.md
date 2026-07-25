# Task 1 Report

## Implementation
- Replaced compression parsing with `Source`, `Destination`, `Remove`, `Execute`, and `FFArgs`.
- Added `--source/-s`, `--dest/-d`, and `--remove` parsing and validation.
- Removed `--dir`; `--remove` defaults to `true` and accepts only `true`/`false`.
- Updated usage text and examples.
- Updated `compress.Run` call sites and tests to use `Source`.

## Files Changed
- `internal/compress/options.go`
- `internal/compress/options_test.go`
- `internal/compress/service.go`
- `internal/compress/service_test.go`

## TDD
- RED: not captured as a separate run before implementation.
- GREEN: `rtk go test ./internal/compress -run 'TestParseOptions|TestParseFFOptions' -count=1`
- GREEN: `rtk go test ./...`

## Tests Run
- `rtk gofmt -w internal/compress/options.go internal/compress/options_test.go internal/compress/service.go internal/compress/service_test.go && rtk go test ./internal/compress -run 'TestParseOptions|TestParseFFOptions' -count=1 && rtk go test ./...`
- Output:
  - `Go test: 25 passed in 1 packages`
  - `Go test: 77 passed in 6 packages`

## Self-Review
- Confirmed `--dir` is no longer registered.
- Confirmed `--remove` accepts spaced and equals forms and defaults to `true`.
- Confirmed destination validation is only applied when provided.
- Confirmed focused and full Go suites pass after gofmt.

## Concerns
- Task 2 runtime behavior for destination copy/remove is still pending.

## Fix Round 1

### Exact Changes
- Added `TestParseOptionsRejectsOldDashDAliasAsSourceDirectory` to prove `-d` no longer works as the old source-directory alias and still requires `--source`.
- Tightened the invalid destination case to assert the exact missing destination path appears in the validation error.
- Kept the existing `--dir` rejection test so both obsolete entry points are covered.

### Tests and Output
- Run: `rtk gofmt -w internal/compress/options_test.go && rtk go test ./internal/compress -run 'TestParseOptions|TestParseFFOptions' -count=1`
- Output: `Go test: 26 passed in 1 packages`
- Limitation: I did not capture a fresh pre-fix RED after the parser change was already present on this branch; the focused run above verifies the tightened assertions pass now.

### Self-Review
- The obsolete `-d` source alias is covered directly, not inferred from `--dir`.
- The destination validation check now matches the exact path, not only a generic message.
- No production code changed in this fix round.

### Fix-Round Report
- Status: completed
- Scope: test-only verification updates for option parsing regressions
