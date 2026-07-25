package checkcopy

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/weizhoublue/handle-files/internal/logx"
)

func TestCompareClassifiesEveryDifference(t *testing.T) {
	source := map[string]Entry{
		"missing.txt": {Size: 1},
		"src-large":   {Size: 3},
		"dst-large":   {Size: 1},
		"same":        {Size: 2},
	}
	destination := map[string]Entry{
		"extra.txt": {Size: 1},
		"src-large": {Size: 1},
		"dst-large": {Size: 3},
		"same":      {Size: 2},
	}

	got := Compare(source, destination)
	if !reflect.DeepEqual(got.Missing, []string{"missing.txt"}) ||
		!reflect.DeepEqual(got.Extra, []string{"extra.txt"}) ||
		!reflect.DeepEqual(got.SourceLarger, []string{"src-large"}) ||
		!reflect.DeepEqual(got.DestLarger, []string{"dst-large"}) {
		t.Fatalf("Compare() = %#v", got)
	}
}

func TestCompareFindsSortedCaseConflicts(t *testing.T) {
	got := Compare(map[string]Entry{
		"Dir/File.txt": {},
		"dir/file.TXT": {},
		"one":          {},
		"ONE":          {},
	}, nil)
	want := [][]string{
		{"Dir/File.txt", "dir/file.TXT"},
		{"ONE", "one"},
	}
	if !reflect.DeepEqual(got.CaseConflicts, want) {
		t.Fatalf("Compare() case conflicts = %#v, want %#v", got.CaseConflicts, want)
	}
}

func TestScanReturnsRegularFilesWithSlashRelativePaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "nested", "file.txt"), "contents", 0o640)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	writeFile(t, outside, "outside", 0o600)
	if err := os.Symlink(outside, filepath.Join(root, "nested", "symlink.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Scan(root, testLogger(&bytes.Buffer{}))
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := got["nested/file.txt"]
	if !ok {
		t.Fatalf("Scan() = %#v, want nested/file.txt", got)
	}
	if entry.Size != int64(len("contents")) || entry.Mode.Perm() != 0o640 {
		t.Fatalf("Scan() entry = %#v", entry)
	}
	if len(got) != 1 {
		t.Fatalf("Scan() files = %#v, want one file", got)
	}
}

func TestScanWarnsAndContinuesAfterEntryInformationFailure(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "bad.txt"), "bad", 0o600)
	writeFile(t, filepath.Join(root, "good.txt"), "good", 0o600)
	var logs bytes.Buffer

	original := scanEntryInfo
	scanEntryInfo = func(dirEntry fs.DirEntry) (fs.FileInfo, error) {
		if dirEntry.Name() == "bad.txt" {
			return nil, errors.New("injected entry information failure")
		}
		return dirEntry.Info()
	}
	t.Cleanup(func() {
		scanEntryInfo = original
	})

	got, err := Scan(root, testLogger(&logs))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["bad.txt"]; ok {
		t.Fatalf("Scan() included failed entry: %#v", got)
	}
	if _, ok := got["good.txt"]; !ok {
		t.Fatalf("Scan() did not continue after entry failure: %#v", got)
	}
	if !strings.Contains(logs.String(), "WARN, scan_info_failed") {
		t.Fatalf("Scan() logs = %q, want scan_info_failed warning", logs.String())
	}
}

func TestParseOptionsValidatesNamedDirectoryArguments(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	file := filepath.Join(t.TempDir(), "file")
	writeFile(t, file, "file", 0o600)
	alias := filepath.Join(t.TempDir(), "source-alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing source", args: []string{"--destination", destination}, want: "--source is required"},
		{name: "empty destination", args: []string{"--source", source, "--destination", ""}, want: "--destination is required"},
		{name: "source file", args: []string{"--source", file, "--destination", destination}, want: "not a directory"},
		{name: "positional", args: []string{"--source", source, "--destination", destination, "extra"}, want: "unexpected positional arguments"},
		{name: "equal resolved paths", args: []string{"--source", alias, "--destination", source}, want: "must be different"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseOptions(tt.args)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.want)) {
				t.Fatalf("ParseOptions() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestParseOptionsAcceptsShortNamedFlagsAndResolvesSymlinks(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	alias := filepath.Join(t.TempDir(), "source-alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}

	got, err := ParseOptions([]string{"-s", alias, "-d", destination, "-c"})
	if err != nil {
		t.Fatal(err)
	}
	if got != (Options{Source: source, Destination: destination, Copy: true}) {
		t.Fatalf("ParseOptions() = %#v", got)
	}
}

func TestParseOptionsHelp(t *testing.T) {
	_, err := ParseOptions([]string{"--help"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("ParseOptions(--help) error = %v, want flag.ErrHelp", err)
	}
	if Usage() == "" {
		t.Fatal("Usage() returned empty string")
	}
}

func TestRunReportOnlyDoesNotWriteCopyCandidates(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	writeFile(t, filepath.Join(source, "missing", "file.txt"), "source", 0o640)
	writeFile(t, filepath.Join(source, "larger.txt"), "source", 0o640)
	writeFile(t, filepath.Join(destination, "larger.txt"), "dst", 0o600)
	var logs bytes.Buffer

	if err := Run(Options{Source: source, Destination: destination}, testLogger(&logs)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "missing", "file.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("report-only copy candidate stat error = %v, want not exist", err)
	}
	contents, err := os.ReadFile(filepath.Join(destination, "larger.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "dst" {
		t.Fatalf("report-only destination contents = %q, want %q", contents, "dst")
	}
	if strings.Contains(logs.String(), "progress") {
		t.Fatalf("report-only progress logs = %q, want no copy progress", logs.String())
	}
}

func TestRunReportOnlyReportsCaseConflictsAfterComparison(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	writeFile(t, filepath.Join(source, "A.txt"), "first", 0o600)
	writeFile(t, filepath.Join(source, "a.txt"), "second", 0o600)
	var logs bytes.Buffer

	if err := Run(Options{Source: source, Destination: destination}, testLogger(&logs)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "A.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("report-only case conflict wrote destination: %v", err)
	}
	if got := strings.Count(logs.String(), "WARN, case_conflicts_reported"); got != 1 {
		t.Fatalf("case-conflict warnings = %d, want one:\n%s", got, logs.String())
	}
	if !strings.Contains(logs.String(), "WARN, case_conflicts_reported files=2 groups=1 paths=A.txt,a.txt") {
		t.Fatalf("case-conflict warning fields = %s", logs.String())
	}
	if !strings.HasSuffix(logs.String(), "WARN, case_conflicts_reported files=2 groups=1 paths=A.txt,a.txt\n") {
		t.Fatalf("case-conflict warning was not final:\n%s", logs.String())
	}
}

func TestRunEmitsScanAndDifferenceSummaries(t *testing.T) {
	for _, copyMode := range []bool{false, true} {
		t.Run(map[bool]string{false: "report-only", true: "copy"}[copyMode], func(t *testing.T) {
			source := t.TempDir()
			destination := t.TempDir()
			writeFile(t, filepath.Join(source, "missing.txt"), "missing", 0o600)
			writeFile(t, filepath.Join(source, "source-larger.txt"), "source is larger", 0o600)
			writeFile(t, filepath.Join(source, "destination-larger.txt"), "dst", 0o600)
			writeFile(t, filepath.Join(source, "same.txt"), "same", 0o600)
			writeFile(t, filepath.Join(destination, "source-larger.txt"), "dst", 0o600)
			writeFile(t, filepath.Join(destination, "destination-larger.txt"), "destination is larger", 0o600)
			writeFile(t, filepath.Join(destination, "same.txt"), "same", 0o600)
			writeFile(t, filepath.Join(destination, "extra.txt"), "extra", 0o600)
			var logs bytes.Buffer

			if err := Run(Options{Source: source, Destination: destination, Copy: copyMode}, testLogger(&logs)); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(logs.String(), "INFO, scan_summary destination_files=4 source_files=4") {
				t.Fatalf("scan summary = %s", logs.String())
			}
			differenceSummary := "INFO, difference_summary consistent=1 destination_larger=1 extra=1 missing=1 source_larger=1"
			if copyMode {
				differenceSummary = "INFO, difference_summary consistent=1 copied=2 destination_larger=1 extra=1 failed=0 missing=1 source_larger=1"
			}
			if !strings.Contains(logs.String(), differenceSummary) {
				t.Fatalf("difference summary = %s", logs.String())
			}
		})
	}
}

func TestRunValidatesNormalizedDirectories(t *testing.T) {
	source := t.TempDir()
	alias := filepath.Join(t.TempDir(), "source-alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}

	err := Run(Options{Source: source, Destination: alias}, testLogger(&bytes.Buffer{}))
	if err == nil || !strings.Contains(err.Error(), "must be different") {
		t.Fatalf("Run() error = %v, want normalized-directory validation error", err)
	}
}

func TestRunCopiesCandidatesWithParentsAndMetadata(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	modTime := time.Date(2024, 8, 12, 14, 30, 0, 123456789, time.UTC)
	missing := filepath.Join(source, "nested", "missing.txt")
	larger := filepath.Join(source, "larger.txt")
	writeFile(t, missing, "missing", 0o640)
	writeFile(t, larger, "source is larger", 0o600)
	if err := os.Chtimes(missing, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(larger, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(destination, "larger.txt"), "dst", 0o666)

	if err := Run(Options{Source: source, Destination: destination, Copy: true}, testLogger(&bytes.Buffer{})); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		path string
		want string
		mode fs.FileMode
	}{
		{path: filepath.Join(destination, "nested", "missing.txt"), want: "missing", mode: 0o640},
		{path: filepath.Join(destination, "larger.txt"), want: "source is larger", mode: 0o600},
	} {
		contents, err := os.ReadFile(tt.path)
		if err != nil {
			t.Fatal(err)
		}
		if string(contents) != tt.want {
			t.Fatalf("%s contents = %q, want %q", tt.path, contents, tt.want)
		}
		info, err := os.Stat(tt.path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != tt.mode.Perm() {
			t.Fatalf("%s mode = %o, want %o", tt.path, info.Mode().Perm(), tt.mode.Perm())
		}
		if !info.ModTime().Equal(modTime) {
			t.Fatalf("%s mod time = %s, want %s", tt.path, info.ModTime(), modTime)
		}
	}
}

func TestRunDoesNotWriteOrCleanThroughDestinationSymlink(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(destination, "linked")
	blockedSource := filepath.Join(source, "linked", "blocked.txt")
	writeFile(t, blockedSource, "blocked", 0o600)
	writeFile(t, filepath.Join(source, "continued.txt"), "continued", 0o600)
	writeFile(t, filepath.Join(outside, "blocked.txt"), "outside", 0o600)
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	original := copyStream
	copyStream = func(destination io.Writer, source io.Reader) (int64, error) {
		if sourceFile, ok := source.(*os.File); ok && sourceFile.Name() == blockedSource {
			return 0, errors.New("injected write failure")
		}
		return io.Copy(destination, source)
	}
	t.Cleanup(func() {
		copyStream = original
	})
	var logs bytes.Buffer

	if err := Run(Options{Source: source, Destination: destination, Copy: true}, testLogger(&logs)); err != nil {
		t.Fatal(err)
	}
	if contents, err := os.ReadFile(filepath.Join(outside, "blocked.txt")); err != nil || string(contents) != "outside" {
		t.Fatalf("destination symlink changed outside file: contents=%q err=%v", contents, err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("destination symlink changed: info=%v err=%v", info, err)
	}
	if contents, err := os.ReadFile(filepath.Join(destination, "continued.txt")); err != nil || string(contents) != "continued" {
		t.Fatalf("safe candidate was not copied: contents=%q err=%v", contents, err)
	}
	requireCopyFailureRecord(t, logs.String())
}

func TestRunKeepsDestinationOperationsConfinedWhenParentChanges(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(source, "swapped", "file.txt"), "source", 0o600)
	writeFile(t, filepath.Join(source, "continued.txt"), "continued", 0o600)
	writeFile(t, filepath.Join(destination, "swapped", "placeholder"), "placeholder", 0o600)
	writeFile(t, filepath.Join(outside, "file.txt"), "outside", 0o600)
	var logs bytes.Buffer

	original := openDestinationFile
	openDestinationFile = func(root *os.Root, name string, flag int, perm fs.FileMode) (*os.File, error) {
		if name == filepath.Join("swapped", "file.txt") {
			if err := os.Rename(filepath.Join(destination, "swapped"), filepath.Join(destination, "moved")); err != nil {
				return nil, err
			}
			if err := os.Symlink(outside, filepath.Join(destination, "swapped")); err != nil {
				return nil, err
			}
		}
		return root.OpenFile(name, flag, perm)
	}
	t.Cleanup(func() {
		openDestinationFile = original
	})

	if err := Run(Options{Source: source, Destination: destination, Copy: true}, testLogger(&logs)); err != nil {
		t.Fatal(err)
	}
	if contents, err := os.ReadFile(filepath.Join(outside, "file.txt")); err != nil || string(contents) != "outside" {
		t.Fatalf("parent swap changed outside file: contents=%q err=%v", contents, err)
	}
	if contents, err := os.ReadFile(filepath.Join(destination, "continued.txt")); err != nil || string(contents) != "continued" {
		t.Fatalf("safe candidate was not copied: contents=%q err=%v", contents, err)
	}
	requireCopyFailureRecord(t, logs.String())
}

func TestRunSkipsCaseConflictGroupsDuringCopy(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	writeFile(t, filepath.Join(source, "Dir", "File.txt"), "first", 0o600)
	writeFile(t, filepath.Join(source, "dir", "file.TXT"), "second", 0o600)
	writeFile(t, filepath.Join(source, "A.txt"), "third", 0o600)
	writeFile(t, filepath.Join(source, "a.txt"), "fourth", 0o600)
	writeFile(t, filepath.Join(source, "A.TXT"), "fifth", 0o600)
	writeFile(t, filepath.Join(source, "other.txt"), "copied", 0o600)
	writeFile(t, filepath.Join(destination, "dir", "file.txt"), "destination", 0o600)
	var logs bytes.Buffer

	if err := Run(Options{Source: source, Destination: destination, Copy: true}, testLogger(&logs)); err != nil {
		t.Fatal(err)
	}
	if contents, err := os.ReadFile(filepath.Join(destination, "dir", "file.txt")); err != nil || string(contents) != "destination" {
		t.Fatalf("case-conflict candidate overwrote destination: contents=%q err=%v", contents, err)
	}
	if _, err := os.Stat(filepath.Join(destination, "Dir", "File.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("case-conflict candidate was copied: %v", err)
	}
	if contents, err := os.ReadFile(filepath.Join(destination, "other.txt")); err != nil || string(contents) != "copied" {
		t.Fatalf("non-conflicting candidate was not copied: contents=%q err=%v", contents, err)
	}
	if got := strings.Count(logs.String(), "WARN, case_conflicts_skipped"); got != 1 {
		t.Fatalf("case-conflict warnings = %d, want one:\n%s", got, logs.String())
	}
	if !strings.Contains(logs.String(), "WARN, case_conflicts_skipped files=5 groups=2") {
		t.Fatalf("case-conflict warning counts = %s", logs.String())
	}
	if strings.Index(logs.String(), "WARN, case_conflicts_skipped") < strings.LastIndex(logs.String(), "INFO, copied ") {
		t.Fatalf("case-conflict warning was emitted before processing completed:\n%s", logs.String())
	}
}

func TestRunCleansPartialDestinationAfterInjectedFailures(t *testing.T) {
	tests := []struct {
		name   string
		inject func(t *testing.T, failedPath string)
	}{
		{
			name: "write",
			inject: func(t *testing.T, failedPath string) {
				original := copyStream
				copyStream = func(destination io.Writer, source io.Reader) (int64, error) {
					if sourceFile, ok := source.(*os.File); ok && sourceFile.Name() == failedPath {
						return 0, errors.New("injected write failure")
					}
					return io.Copy(destination, source)
				}
				t.Cleanup(func() {
					copyStream = original
				})
			},
		},
		{
			name: "chmod",
			inject: func(t *testing.T, failedPath string) {
				original := changeMode
				changeMode = func(root *os.Root, path string, mode fs.FileMode) error {
					if path == failedPath {
						return errors.New("injected chmod failure")
					}
					return root.Chmod(path, mode)
				}
				t.Cleanup(func() {
					changeMode = original
				})
			},
		},
		{
			name: "chtimes",
			inject: func(t *testing.T, failedPath string) {
				original := changeTimes
				changeTimes = func(root *os.Root, path string, accessTime, modificationTime time.Time) error {
					if path == failedPath {
						return errors.New("injected chtimes failure")
					}
					return root.Chtimes(path, accessTime, modificationTime)
				}
				t.Cleanup(func() {
					changeTimes = original
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := t.TempDir()
			destination := t.TempDir()
			failedSource := filepath.Join(source, "failed.txt")
			failedDestination := filepath.Join(destination, "failed.txt")
			writeFile(t, failedSource, "failed", 0o600)
			writeFile(t, filepath.Join(source, "continued.txt"), "continued", 0o600)
			tt.inject(t, failedPathFor(tt.name, failedSource, failedDestination))
			var logs bytes.Buffer

			if err := Run(Options{Source: source, Destination: destination, Copy: true}, testLogger(&logs)); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(failedDestination); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("partial destination stat error = %v, want not exist", err)
			}
			if contents, err := os.ReadFile(filepath.Join(destination, "continued.txt")); err != nil || string(contents) != "continued" {
				t.Fatalf("continued candidate was not copied: contents=%q err=%v", contents, err)
			}
			requireCopyFailureRecord(t, logs.String())
		})
	}
}

func TestRunStopsCopyAfterNoSpace(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	failedSource := filepath.Join(source, "a-fails.txt")
	writeFile(t, failedSource, "failed", 0o600)
	writeFile(t, filepath.Join(source, "b-unstarted.txt"), "unstarted", 0o600)
	var logs bytes.Buffer

	original := copyStream
	copyStream = func(destination io.Writer, source io.Reader) (int64, error) {
		if sourceFile, ok := source.(*os.File); ok && sourceFile.Name() == failedSource {
			return 0, fmt.Errorf("injected no space: %w", syscall.ENOSPC)
		}
		return io.Copy(destination, source)
	}
	t.Cleanup(func() {
		copyStream = original
	})

	if err := Run(Options{Source: source, Destination: destination, Copy: true}, testLogger(&logs)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "b-unstarted.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("later copy was attempted: %v", err)
	}
	if !strings.Contains(logs.String(), "WARN, copy_aborted_no_space") {
		t.Fatalf("missing no-space summary:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "WARN, copy_not_completed path=a-fails.txt") {
		t.Fatalf("failed path missing from summary:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "WARN, copy_not_completed path=b-unstarted.txt") {
		t.Fatalf("unstarted path missing from summary:\n%s", logs.String())
	}
	if strings.Index(logs.String(), "WARN, copy_not_completed path=a-fails.txt") >
		strings.Index(logs.String(), "WARN, copy_not_completed path=b-unstarted.txt") {
		t.Fatalf("copy_not_completed records out of order:\n%s", logs.String())
	}
	if strings.Contains(logs.String(), "INFO, copied ") {
		t.Fatalf("unexpected successful copy:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "INFO, difference_summary consistent=0 copied=0 destination_larger=0 extra=0 failed=1 missing=2 source_larger=0") {
		t.Fatalf("difference summary missing failed count:\n%s", logs.String())
	}
}

func failedPathFor(failure, sourcePath, destinationPath string) string {
	if failure == "write" {
		return sourcePath
	}
	return filepath.Base(destinationPath)
}

func TestRunEmitsCopyOutcomeProgressForEveryCopyCandidate(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	writeFile(t, filepath.Join(source, "missing.txt"), "missing", 0o600)
	writeFile(t, filepath.Join(source, "source-larger.txt"), "source is larger", 0o600)
	writeFile(t, filepath.Join(destination, "source-larger.txt"), "dst", 0o600)
	writeFile(t, filepath.Join(source, "destination-larger.txt"), "dst", 0o600)
	writeFile(t, filepath.Join(destination, "destination-larger.txt"), "destination is larger", 0o600)
	var logs bytes.Buffer

	if err := Run(Options{Source: source, Destination: destination, Copy: true}, testLogger(&logs)); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(logs.String(), "INFO, copied "); got != 2 {
		t.Fatalf("copied records = %d, want 2:\n%s", got, logs.String())
	}
	if !strings.Contains(logs.String(), "INFO, copied path=source-larger.txt [ completed=2 failed=0 succeeded=2 total=2 ]") {
		t.Fatalf("final copied record missing progress:\n%s", logs.String())
	}
	if strings.Contains(logs.String(), "progress") {
		t.Fatalf("standalone progress record emitted:\n%s", logs.String())
	}
}

func writeFile(t *testing.T, path, contents string, mode fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}

func testLogger(out *bytes.Buffer) logx.Logger {
	return logx.Logger{
		Out: out,
		Err: out,
	}
}

func requireCopyFailureRecord(t *testing.T, logs string) {
	t.Helper()
	const progress = "[ completed=2 failed=1 succeeded=1 total=2 ]"
	count := 0
	for _, line := range strings.Split(logs, "\n") {
		if strings.HasPrefix(line, "WARN, copy_failed ") {
			count++
			if !strings.Contains(line, progress) {
				t.Fatalf("copy failure record missing progress: %q\n%s", line, logs)
			}
		}
	}
	if count != 1 {
		t.Fatalf("copy failure records = %d, want 1:\n%s", count, logs)
	}
}
