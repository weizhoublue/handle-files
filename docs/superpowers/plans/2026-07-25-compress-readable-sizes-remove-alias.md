# Compression readable sizes and remove alias Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Format successful compression sizes in suitable decimal units and
add `-r` as the `--remove` alias.

**Architecture:** Keep formatting private to `internal/compress` and replace
the three successful-record byte fields with size fields. Bind `-r` to the
existing remove variable and document the alias.

**Tech Stack:** Go standard library, Go tests, Make.

## Global Constraints

- Use decimal `B`, `KB`, `MB`, `GB`, and `TB` units with a 1000 step.
- Use integer bytes below 1000 and one decimal place otherwise.
- Use `original_size`, `output_size`, and `reduction_size`; preserve paths,
  reduction percentage, FFmpeg behavior, and deletion timing.
- Preserve the `--remove=true` default; support `-r false` and `-r=false`.
- Do not modify the default branch; commits must be GPG-signed and signed off.

---

### Task 1: Format and log readable compression sizes

**Files:**
- Modify: `internal/compress/service.go`
- Modify: `internal/compress/service_test.go`

**Interfaces:**
- Produces: private `formatSize(bytes int64) string`.

- [ ] **Step 1: Add failing formatter tests**

  Add `TestFormatSizeUsesDecimalUnits` with these cases:

  ```go
  {0, "0 B"}, {999, "999 B"}, {1000, "1.0 KB"},
  {10_000, "10.0 KB"}, {1_000_000, "1.0 MB"},
  {1_200_000_000, "1.2 GB"}, {1_000_000_000_000, "1.0 TB"},
  ```

- [ ] **Step 2: Verify the formatter test fails**

  ```bash
  go test ./internal/compress -run '^TestFormatSizeUsesDecimalUnits$' -count=1
  ```

- [ ] **Step 3: Add the formatter**

  ```go
  func formatSize(bytes int64) string {
      const base = 1000
      units := []string{"B", "KB", "MB", "GB", "TB"}
      if bytes < base {
          return fmt.Sprintf("%d B", bytes)
      }
      size, unit := float64(bytes), 0
      for size >= base && unit < len(units)-1 {
          size /= base
          unit++
      }
      return fmt.Sprintf("%.1f %s", size, units[unit])
  }
  ```

- [ ] **Step 4: Migrate successful-record size fields**

  Replace exact byte fields with:

  ```go
  {Key: "original_size", Value: formatSize(sourceInfo.Size())},
  {Key: "output_size", Value: formatSize(info.Size())},
  {Key: "reduction_size", Value: formatSize(reductionBytes)},
  ```

  Update the success-log test to assert those fields and reject
  `original_bytes`, `output_bytes`, and `reduction_bytes`.

- [ ] **Step 5: Format, test, and commit**

  ```bash
  gofmt -w internal/compress/service.go internal/compress/service_test.go
  go test ./internal/compress -count=1
  git add internal/compress/service.go internal/compress/service_test.go
  git commit -S -s -m "feat: format compression sizes" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
  ```

### Task 2: Add remove alias and document it

**Files:**
- Modify: `internal/compress/options.go`
- Modify: `internal/compress/options_test.go`
- Modify: `docs/readme.md`

**Interfaces:**
- Produces: `-r` parsing equivalent to `--remove`.

- [ ] **Step 1: Add failing alias and usage tests**

  Verify `-r false` yields `Remove == false`, `-r=true` yields
  `Remove == true`, and `Usage()` contains `--remove/-r`.

- [ ] **Step 2: Verify the alias test fails**

  ```bash
  go test ./internal/compress -run '^TestParseOptionsAcceptsRemoveShortAlias$' -count=1
  ```

- [ ] **Step 3: Bind the alias**

  ```go
  fs.StringVar(&remove, "r", remove, "remove source after successful compression")
  ```

- [ ] **Step 4: Update command documentation**

  Change the usage and README synopsis to `[--remove/-r <true|false>]`;
  use `--remove, -r` in descriptions and `` `--remove`, `-r` `` in the
  README table. Keep the long-form `--remove false` example.

- [ ] **Step 5: Format, validate, build, and commit**

  ```bash
  gofmt -w internal/compress/options.go internal/compress/options_test.go
  go test ./internal/compress -count=1
  go test ./... -count=1
  make build-macos
  git add internal/compress/options.go internal/compress/options_test.go docs/readme.md dist/macos-arm64/compress-vedio dist/macos-amd64/compress-vedio
  git commit -S -s -m "feat: add remove short alias" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
  ```
