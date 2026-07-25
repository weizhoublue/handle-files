# PR Check workflow design

## Goal

Validate pull requests targeting `main` or `master` with Go tests and Linux amd64 builds before review or merge.

## Trigger

The existing `Test` workflow becomes `PR Check` and runs only for the `pull_request` event when its base branch is `main` or `master`. It no longer runs on direct pushes.

## Job flow

One `ubuntu-latest` job:

1. Checks out the pull request source with `actions/checkout@v4`.
2. Installs the Go version declared by `go.mod` with `actions/setup-go@v5`.
3. Runs `go test ./... -count=1`.
4. Builds both repository command entry points for Linux amd64:
   - `./cmd/compress-vedio`
   - `./cmd/check-copy`

Each build discards its output with `/dev/null`; build failures fail the job.

## Scope

Change only `.github/workflows/test.yml`. The workflow does not run macOS builds, create release artifacts, or run for pushes.
