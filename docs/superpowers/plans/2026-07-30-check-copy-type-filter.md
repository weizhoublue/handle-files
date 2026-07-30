# Check-copy Type Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add repeatable `-t/--type` filtering so `check-copy` previews, compares, summarizes, and copies only selected file extensions while preserving all-file behavior when the option is omitted.

**Architecture:** Parse repeated type values into a normalized, deduplicated `Options.Types` slice. Build one read-only extension filter in `Run`, apply it while scanning both source and destination, then leave comparison and copy logic unchanged because they receive only the selected file universe.

**Tech Stack:** Go 1.26, standard-library `flag`, `filepath`, `strings`, and `unicode`; existing Go unit and integration test suites.

## Global Constraints

- `--type` and `-t` are repeatable aliases and may be mixed.
- Accept `jpg`, `.jpg`, and `JPG` as the same type; preserve first-seen order after deduplication.
- Match only the final extension, so `gz` matches `archive.tar.gz`; reject `tar.gz`.
- Reject empty, dot-only, multi-dot, whitespace-containing, `/`-containing, and `\`-containing normalized values.
- With a non-empty filter, `.gitignore` has no extension and does not match `gitignore`.
- With no type filter, process every regular file exactly as before, including extensionless files and dotfiles.
- Filter source and destination during scanning so all comparison output, summaries, conflict handling, previews, and copies use only selected types.
- No new dependencies, log events, warning behavior, or changes to `compress-vedio` and release workflows.
- Format changed Go files with `gofmt`.
- Every implementation commit must use `git commit -s -S` with a one-line English message and no AI attribution.

---

## File Structure

- `internal/checkcopy/options.go`: collect repeated CLI values, normalize and validate extensions, store them in `Options.Types`, and document command usage.
- `internal/checkcopy/options_test.go`: focused tests for repeatable flags, normalization, deduplication, defaults, and invalid values.
- `internal/checkcopy/service.go`: preserve public unfiltered `Scan`, add a filtered scan path, and wire one extension filter into both scans in `Run`.
- `internal/checkcopy/service_test.go`: retain existing behavior tests and add scan, summary, conflict, and copy coverage for selected types.
- `integration/cli_test.go`: verify the built command accepts repeated aliases, copies only selected extensions, and advertises the option in help.
- `docs/readme.md`: document the new option, matching rules, default behavior, and examples.

### Task 1: Parse and normalize repeated type options

**Files:**
- Create: `internal/checkcopy/options_test.go`
- Modify: `internal/checkcopy/options.go:13-71`
- Modify: `internal/checkcopy/service_test.go:152-167`

**Interfaces:**
- Consumes: existing `ParseOptions(args []string) (Options, error)` and `normalizeOptions(opts Options) (Options, error)`.
- Produces: `Options.Types []string`, `stringListFlag`, and `normalizeTypes(values []string) ([]string, error)`.

- [ ] **Step 1: Write failing option tests**

Create `internal/checkcopy/options_test.go`:

```go
package checkcopy

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseOptionsNormalizesRepeatedTypes(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()

	got, err := ParseOptions([]string{
		"--source", source,
		"--destination", destination,
		"--type", " JPG ",
		"-t", ".mp4",
		"--type", "jpg",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := Options{
		Source:      source,
		Destination: destination,
		Types:       []string{"jpg", "mp4"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseOptions() = %#v, want %#v", got, want)
	}
}

func TestParseOptionsDefaultsToAllTypes(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()

	got, err := ParseOptions([]string{
		"--source", source,
		"--destination", destination,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Types) != 0 {
		t.Fatalf("ParseOptions() types = %#v, want no filter", got.Types)
	}
}

func TestParseOptionsRejectsInvalidTypes(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	for _, value := range []string{"", ".", "tar.gz", "jpg/png", `jpg\png`, "jp g"} {
		t.Run(value, func(t *testing.T) {
			_, err := ParseOptions([]string{
				"--source", source,
				"--destination", destination,
				"--type", value,
			})
			if err == nil || !strings.Contains(err.Error(), "invalid --type value") {
				t.Fatalf("ParseOptions(--type %q) error = %v", value, err)
			}
		})
	}
}
```

- [ ] **Step 2: Run the focused tests and confirm failure**

Run:

```bash
go test ./internal/checkcopy -run 'TestParseOptions(NormalizesRepeatedTypes|DefaultsToAllTypes|RejectsInvalidTypes)$'
```

Expected: FAIL because `Options.Types` and repeatable `--type/-t` parsing do not exist.

- [ ] **Step 3: Add the repeated flag collector and normalized option field**

In `internal/checkcopy/options.go`, add `unicode` to the imports and extend `Options`:

```go
type Options struct {
	Source      string
	Destination string
	Copy        bool
	Types       []string
}

type stringListFlag []string

func (values *stringListFlag) String() string {
	return strings.Join(*values, ",")
}

func (values *stringListFlag) Set(value string) error {
	*values = append(*values, value)
	return nil
}
```

Collect both aliases in `ParseOptions`:

```go
var (
	source      string
	destination string
	copyFiles   bool
	types       stringListFlag
	help        bool
)

flags.Var(&types, "type", "file extension to include (repeatable)")
flags.Var(&types, "t", "file extension to include (repeatable)")
```

Pass the raw values into normalization:

```go
return normalizeOptions(Options{
	Source:      source,
	Destination: destination,
	Copy:        copyFiles,
	Types:       []string(types),
})
```

- [ ] **Step 4: Implement type normalization and validation**

Add this helper to `internal/checkcopy/options.go`:

```go
func normalizeTypes(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		extension := strings.TrimPrefix(strings.TrimSpace(value), ".")
		if extension == "" ||
			strings.Contains(extension, ".") ||
			strings.ContainsAny(extension, `/\`) ||
			strings.IndexFunc(extension, unicode.IsSpace) >= 0 {
			return nil, fmt.Errorf(
				"invalid --type value %q: want one extension without dots, whitespace, or path separators",
				value,
			)
		}

		extension = strings.ToLower(extension)
		if _, ok := seen[extension]; ok {
			continue
		}
		seen[extension] = struct{}{}
		normalized = append(normalized, extension)
	}
	return normalized, nil
}
```

Replace `normalizeOptions` so type validation runs before directory resolution and the normalized slice is retained:

```go
func normalizeOptions(opts Options) (Options, error) {
	normalizedTypes, err := normalizeTypes(opts.Types)
	if err != nil {
		return Options{}, err
	}
	resolvedSource, err := resolveDirectory("source", opts.Source)
	if err != nil {
		return Options{}, err
	}
	resolvedDestination, err := resolveDirectory("destination", opts.Destination)
	if err != nil {
		return Options{}, err
	}
	if resolvedSource == resolvedDestination {
		return Options{}, errors.New("source and destination directories must be different")
	}
	return Options{
		Source:      resolvedSource,
		Destination: resolvedDestination,
		Copy:        opts.Copy,
		Types:       normalizedTypes,
	}, nil
}
```

- [ ] **Step 5: Repair the existing equality assertion**

Adding a slice makes `Options` non-comparable. Replace the direct comparison in `TestParseOptionsAcceptsShortNamedFlagsAndResolvesSymlinks`:

```go
want := Options{Source: source, Destination: destination, Copy: true}
if !reflect.DeepEqual(got, want) {
	t.Fatalf("ParseOptions() = %#v, want %#v", got, want)
}
```

- [ ] **Step 6: Format and run all checkcopy tests**

Run:

```bash
gofmt -w internal/checkcopy/options.go internal/checkcopy/options_test.go internal/checkcopy/service_test.go
go test ./internal/checkcopy
```

Expected: PASS.

- [ ] **Step 7: Commit the option parsing deliverable**

```bash
git add internal/checkcopy/options.go internal/checkcopy/options_test.go internal/checkcopy/service_test.go
git commit -s -S -m "Add check-copy type option parsing"
```

### Task 2: Filter both directory scans by selected extensions

**Files:**
- Modify: `internal/checkcopy/service.go:49-91`
- Modify: `internal/checkcopy/service.go:122-138`
- Modify: `internal/checkcopy/service_test.go:48-118`
- Modify: `internal/checkcopy/service_test.go:179-259`

**Interfaces:**
- Consumes: normalized `Options.Types []string` from Task 1.
- Produces: `extensionFilter`, `newExtensionFilter(types []string) extensionFilter`, `extensionFilter.matches(name string) bool`, and `scan(root string, filter extensionFilter, logger logx.Logger) (map[string]Entry, error)`.
- Preserves: `Scan(root string, logger logx.Logger) (map[string]Entry, error)` as the unfiltered public wrapper used by existing tests and callers.

- [ ] **Step 1: Write a failing scan-filter test**

Add to `internal/checkcopy/service_test.go` after `TestScanReturnsRegularFilesWithSlashRelativePaths`:

```go
func TestScanFiltersByFinalExtension(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "photo.JPG"), "photo", 0o600)
	writeFile(t, filepath.Join(root, "archive.tar.gz"), "archive", 0o600)
	writeFile(t, filepath.Join(root, "video.mp4"), "video", 0o600)
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored", 0o600)

	got, err := scan(
		root,
		newExtensionFilter([]string{"jpg", "gz", "gitignore"}),
		testLogger(&bytes.Buffer{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"photo.JPG", "archive.tar.gz"} {
		if _, ok := got[path]; !ok {
			t.Fatalf("scan() files = %#v, want %q", got, path)
		}
	}
	for _, path := range []string{"video.mp4", ".gitignore"} {
		if _, ok := got[path]; ok {
			t.Fatalf("scan() files = %#v, did not want %q", got, path)
		}
	}
	if len(got) != 2 {
		t.Fatalf("scan() files = %#v, want two files", got)
	}
}

func TestScanWithoutFilterIncludesExtensionlessAndDotfiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "LICENSE"), "license", 0o600)
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored", 0o600)

	got, err := Scan(root, testLogger(&bytes.Buffer{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"LICENSE", ".gitignore"} {
		if _, ok := got[path]; !ok {
			t.Fatalf("Scan() files = %#v, want %q", got, path)
		}
	}
	if len(got) != 2 {
		t.Fatalf("Scan() files = %#v, want two files", got)
	}
}
```

- [ ] **Step 2: Write failing end-to-end service tests**

Add these tests to `internal/checkcopy/service_test.go`:

```go
func TestRunTypeFilterScopesCopyAndSummaries(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	writeFile(t, filepath.Join(source, "photo.JPG"), "photo", 0o600)
	writeFile(t, filepath.Join(source, "video.mp4"), "video", 0o600)
	writeFile(t, filepath.Join(source, "ignored.txt"), "ignored", 0o600)
	writeFile(t, filepath.Join(destination, "extra.jpg"), "extra", 0o600)
	writeFile(t, filepath.Join(destination, "extra.txt"), "extra", 0o600)
	var logs bytes.Buffer

	err := Run(Options{
		Source:      source,
		Destination: destination,
		Copy:        true,
		Types:       []string{".JPG", "mp4"},
	}, testLogger(&logs))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"photo.JPG", "video.mp4"} {
		if _, err := os.Stat(filepath.Join(destination, path)); err != nil {
			t.Fatalf("selected file %q was not copied: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(destination, "ignored.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unselected file was copied: %v", err)
	}
	if !strings.Contains(logs.String(), "INFO, scan_summary destination_files=1 source_files=2") {
		t.Fatalf("scan summary = %s", logs.String())
	}
	if !strings.Contains(logs.String(), "INFO, difference_summary consistent=0 copied=2 destination_larger=0 extra=1 failed=0 missing=2 source_larger=0") {
		t.Fatalf("difference summary = %s", logs.String())
	}
}

func TestRunTypeFilterScopesReportOnly(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	writeFile(t, filepath.Join(source, "photo.jpg"), "photo", 0o600)
	writeFile(t, filepath.Join(source, "ignored.txt"), "ignored", 0o600)
	writeFile(t, filepath.Join(destination, "extra.jpg"), "extra", 0o600)
	writeFile(t, filepath.Join(destination, "extra.txt"), "extra", 0o600)
	var logs bytes.Buffer

	if err := Run(Options{
		Source:      source,
		Destination: destination,
		Types:       []string{"jpg"},
	}, testLogger(&logs)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "INFO, copy_skipped total=1") {
		t.Fatalf("copy preview = %s", logs.String())
	}
	if !strings.Contains(logs.String(), "INFO, scan_summary destination_files=1 source_files=1") {
		t.Fatalf("scan summary = %s", logs.String())
	}
	if !strings.Contains(logs.String(), "INFO, difference_summary consistent=0 destination_larger=0 extra=1 missing=1 source_larger=0") {
		t.Fatalf("difference summary = %s", logs.String())
	}
	if strings.Contains(logs.String(), "ignored.txt") || strings.Contains(logs.String(), "extra.txt") {
		t.Fatalf("unselected file was reported: %s", logs.String())
	}
}

func TestRunTypeFilterNoMatchesSucceeds(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	writeFile(t, filepath.Join(source, "ignored.txt"), "ignored", 0o600)
	writeFile(t, filepath.Join(destination, "extra.txt"), "extra", 0o600)
	var logs bytes.Buffer

	if err := Run(Options{
		Source:      source,
		Destination: destination,
		Types:       []string{"jpg"},
	}, testLogger(&logs)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "INFO, scan_summary destination_files=0 source_files=0") {
		t.Fatalf("scan summary = %s", logs.String())
	}
	if !strings.Contains(logs.String(), "INFO, difference_summary consistent=0 destination_larger=0 extra=0 missing=0 source_larger=0") {
		t.Fatalf("difference summary = %s", logs.String())
	}
	if strings.Contains(logs.String(), "copy_skipped") {
		t.Fatalf("empty selection reported copy candidates: %s", logs.String())
	}
}

func TestRunTypeFilterExcludesUnselectedCaseConflicts(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	writeFile(t, filepath.Join(source, "A.jpg"), "first", 0o600)
	writeFile(t, filepath.Join(source, "a.JPG"), "second", 0o600)
	writeFile(t, filepath.Join(source, "B.txt"), "third", 0o600)
	writeFile(t, filepath.Join(source, "b.TXT"), "fourth", 0o600)
	var logs bytes.Buffer

	if err := Run(Options{
		Source:      source,
		Destination: destination,
		Types:       []string{"jpg"},
	}, testLogger(&logs)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "WARN, case_conflicts_reported files=2 groups=1 paths=A.jpg,a.JPG") {
		t.Fatalf("case-conflict warning = %s", logs.String())
	}
	if strings.Contains(logs.String(), "B.txt") || strings.Contains(logs.String(), "b.TXT") {
		t.Fatalf("unselected conflict was reported: %s", logs.String())
	}
}
```

- [ ] **Step 3: Run the focused service tests and confirm failure**

Run:

```bash
go test ./internal/checkcopy -run 'Test(ScanFiltersByFinalExtension|ScanWithoutFilterIncludesExtensionlessAndDotfiles|RunTypeFilterScopesCopyAndSummaries|RunTypeFilterScopesReportOnly|RunTypeFilterNoMatchesSucceeds|RunTypeFilterExcludesUnselectedCaseConflicts)$'
```

Expected: FAIL because the filtered scan helpers do not exist and `Run` still calls unfiltered `Scan`.

- [ ] **Step 4: Add the extension filter**

In `internal/checkcopy/service.go`, add:

```go
type extensionFilter map[string]struct{}

func newExtensionFilter(types []string) extensionFilter {
	if len(types) == 0 {
		return nil
	}
	filter := make(extensionFilter, len(types))
	for _, extension := range types {
		filter[extension] = struct{}{}
	}
	return filter
}

func (filter extensionFilter) matches(name string) bool {
	if len(filter) == 0 {
		return true
	}
	extension := filepath.Ext(name)
	if extension == "" || extension == name {
		return false
	}
	_, ok := filter[strings.ToLower(strings.TrimPrefix(extension, "."))]
	return ok
}
```

The `extension == name` condition excludes names such as `.gitignore` while allowing names such as `.config.json`.

- [ ] **Step 5: Preserve public Scan and add the filtered scan path**

Change the current `Scan` body into a wrapper plus private implementation:

```go
func Scan(root string, logger logx.Logger) (map[string]Entry, error) {
	return scan(root, nil, logger)
}

func scan(root string, filter extensionFilter, logger logx.Logger) (map[string]Entry, error) {
	logger = usableLogger(logger)
	entries := make(map[string]Entry)
	err := filepath.WalkDir(root, func(path string, dirEntry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path != root {
				logger.Warn("scan_failed",
					logx.Field{Key: "error", Value: walkErr.Error()},
					logx.Field{Key: "path", Value: path},
				)
				if dirEntry != nil && dirEntry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			return walkErr
		}
		info, err := scanEntryInfo(dirEntry)
		if err != nil {
			logger.Warn("scan_info_failed",
				logx.Field{Key: "error", Value: err.Error()},
				logx.Field{Key: "path", Value: path},
			)
			return nil
		}
		if !info.Mode().IsRegular() || !filter.matches(dirEntry.Name()) {
			return nil
		}
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("make path %q relative to %q: %w", path, root, err)
		}
		entries[filepath.ToSlash(relativePath)] = Entry{
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan directory %q: %w", root, err)
	}
	return entries, nil
}
```

The only behavioral change from the current `Scan` body is the `!filter.matches(dirEntry.Name())` condition.

- [ ] **Step 6: Wire the same filter into source and destination scans**

In `Run`, replace the two `Scan` calls:

```go
filter := newExtensionFilter(opts.Types)
source, err := scan(opts.Source, filter, logger)
if err != nil {
	return err
}
destination, err := scan(opts.Destination, filter, logger)
if err != nil {
	return err
}
```

Do not filter `Comparison` or copy candidates again. Their existing behavior is correct once both maps are filtered.

- [ ] **Step 7: Format and run all checkcopy tests**

Run:

```bash
gofmt -w internal/checkcopy/service.go internal/checkcopy/service_test.go
go test ./internal/checkcopy
```

Expected: PASS, including existing no-filter scan, copy safety, cleanup, no-space, summary, and conflict tests.

- [ ] **Step 8: Commit the scan-filtering deliverable**

```bash
git add internal/checkcopy/service.go internal/checkcopy/service_test.go
git commit -s -S -m "Filter check-copy scans by file type"
```

### Task 3: Document and verify the CLI contract

**Files:**
- Modify: `internal/checkcopy/options.go:96-111`
- Modify: `integration/cli_test.go:107-138`
- Modify: `docs/readme.md:67-100`

**Interfaces:**
- Consumes: repeatable CLI parsing from Task 1 and scan filtering from Task 2.
- Produces: updated `Usage()` text, user documentation, and binary-level regression coverage.

- [ ] **Step 1: Add failing help and binary behavior coverage**

Add this integration test after `TestCheckCopyCopyCreatesMissingTarget`:

```go
func TestCheckCopyCopyFiltersRepeatedTypes(t *testing.T) {
	binary := buildBinary(t, "./cmd/check-copy")
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	mkdirAll(t, source)
	mkdirAll(t, destination)
	writeFile(t, filepath.Join(source, "nested", "photo.JPG"), "photo")
	writeFile(t, filepath.Join(source, "nested", "clip.mp4"), "video")
	writeFile(t, filepath.Join(source, "nested", "notes.txt"), "notes")

	output, err := runCommand(binary, []string{
		"-s", source,
		"-d", destination,
		"-t", "jpg",
		"--type", ".MP4",
		"-c",
	}, "", nil)
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, output)
	}
	for _, path := range []string{"photo.JPG", "clip.mp4"} {
		contents, err := os.ReadFile(filepath.Join(destination, "nested", path))
		if err != nil {
			t.Fatalf("read selected file %q: %v", path, err)
		}
		if len(contents) == 0 {
			t.Fatalf("selected file %q is empty", path)
		}
	}
	if _, err := os.Stat(filepath.Join(destination, "nested", "notes.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unselected file was copied: %v", err)
	}
}
```

Extend `TestCheckCopyHelpPrintsUsage`:

```go
help := string(output)
if !strings.Contains(help, "Usage: check-copy") {
	t.Fatalf("missing usage: %s", output)
}
if !strings.Contains(help, "--type, -t") || !strings.Contains(help, "repeatable") {
	t.Fatalf("missing repeatable type option: %s", output)
}
```

- [ ] **Step 2: Run the focused integration tests and confirm failure**

Run:

```bash
go test ./integration -run 'TestCheckCopy(CopyFiltersRepeatedTypes|HelpPrintsUsage)$'
```

Expected: FAIL because help output does not yet document `--type, -t` as repeatable. The copy-filter test should pass after Tasks 1 and 2.

- [ ] **Step 3: Update command usage**

Change `Usage()` in `internal/checkcopy/options.go` to include:

```text
Usage: check-copy --source/-s <directory> --destination/-d <directory> [--type/-t <extension>]... [--copy/-c]

Options:
  --source, -s       source directory
  --destination, -d  destination directory
  --type, -t         file extension to include, repeatable; default: all types
  --copy, -c         copy missing and smaller destination files
  --help, -h         show help

例子
	# 只预览 JPG 文件；jpg、.jpg 和 JPG 等价
	check-copy -s /Volumes/red/1 -d /Volumes/black/1 -t jpg

	# 只拷贝 JPG 和 MP4 文件
	check-copy -s /Volumes/red/1 -d /Volumes/black/1 -t jpg -t mp4 -c

	# 不指定 -t 时处理所有文件类型
	check-copy -s /Volumes/red/1 -d /Volumes/black/1 -c
```

- [ ] **Step 4: Update user documentation**

In `docs/readme.md`, change the synopsis to:

```text
check-copy --source/-s <directory> --destination/-d <directory> [--type/-t <extension>]... [--copy/-c]
```

Add this option row:

```markdown
| `--type`, `-t` | 只处理指定的最后扩展名，可重复；忽略大小写，可写 `jpg` 或 `.jpg`。省略时处理全部常规文件。 |
```

Replace the single example with:

```bash
# 只预览 JPG 文件
check-copy -s /Volumes/red/1 -d /Volumes/black/1 -t jpg

# 只复制 JPG 和 MP4 文件
check-copy -s /Volumes/red/1 -d /Volumes/black/1 -t jpg -t mp4 -c

# 复制所有文件类型
check-copy -s /Volumes/red/1 -d /Volumes/black/1 -c
```

Add one sentence before the synopsis:

```markdown
指定 `--type/-t` 后，扫描、差异报告、统计和复制都只包含选中的类型；匹配最后一个扩展名，因此 `-t gz` 可匹配 `archive.tar.gz`，点文件 `.gitignore` 不视为 `gitignore` 类型。
```

- [ ] **Step 5: Format and run focused tests**

Run:

```bash
gofmt -w internal/checkcopy/options.go integration/cli_test.go
go test ./internal/checkcopy
go test ./integration -run 'TestCheckCopy'
```

Expected: PASS.

- [ ] **Step 6: Run repository-wide validation**

Run:

```bash
go test ./...
git diff --check
```

Expected: all tests pass and `git diff --check` produces no output.

- [ ] **Step 7: Commit the CLI documentation and integration coverage**

```bash
git add internal/checkcopy/options.go integration/cli_test.go docs/readme.md
git commit -s -S -m "Document check-copy type filtering"
```
