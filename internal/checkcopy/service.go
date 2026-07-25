package checkcopy

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/weizhoublue/handle-files/internal/logx"
)

var (
	scanEntryInfo = func(dirEntry fs.DirEntry) (fs.FileInfo, error) {
		return dirEntry.Info()
	}
	copyStream          = io.Copy
	openDestinationFile = func(root *os.Root, name string, flag int, perm fs.FileMode) (*os.File, error) {
		return root.OpenFile(name, flag, perm)
	}
	changeMode = func(root *os.Root, name string, mode fs.FileMode) error {
		return root.Chmod(name, mode)
	}
	changeTimes = func(root *os.Root, name string, accessTime, modificationTime time.Time) error {
		return root.Chtimes(name, accessTime, modificationTime)
	}
)

type Entry struct {
	Size    int64
	Mode    fs.FileMode
	ModTime time.Time
}

type Comparison struct {
	Missing       []string
	Extra         []string
	SourceLarger  []string
	DestLarger    []string
	CaseConflicts [][]string
}

func Scan(root string, logger logx.Logger) (map[string]Entry, error) {
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
		if !info.Mode().IsRegular() {
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

func Compare(source, destination map[string]Entry) Comparison {
	comparison := Comparison{}
	for path, sourceEntry := range source {
		destinationEntry, ok := destination[path]
		if !ok {
			comparison.Missing = append(comparison.Missing, path)
			continue
		}
		switch {
		case sourceEntry.Size > destinationEntry.Size:
			comparison.SourceLarger = append(comparison.SourceLarger, path)
		case sourceEntry.Size < destinationEntry.Size:
			comparison.DestLarger = append(comparison.DestLarger, path)
		}
	}
	for path := range destination {
		if _, ok := source[path]; !ok {
			comparison.Extra = append(comparison.Extra, path)
		}
	}
	sort.Strings(comparison.Missing)
	sort.Strings(comparison.Extra)
	sort.Strings(comparison.SourceLarger)
	sort.Strings(comparison.DestLarger)
	comparison.CaseConflicts = caseConflicts(source)
	return comparison
}

func Run(opts Options, logger logx.Logger) error {
	logger = usableLogger(logger)
	normalizedOptions, err := normalizeOptions(opts)
	if err != nil {
		return err
	}
	opts = normalizedOptions

	source, err := Scan(opts.Source, logger)
	if err != nil {
		return err
	}
	destination, err := Scan(opts.Destination, logger)
	if err != nil {
		return err
	}
	comparison := Compare(source, destination)
	logComparison(logger, comparison)

	candidates := append(append([]string{}, comparison.Missing...), comparison.SourceLarger...)
	sort.Strings(candidates)
	if !opts.Copy {
		if len(candidates) > 0 {
			logger.Info("copy_skipped", logx.Field{Key: "total", Value: strconv.Itoa(len(candidates))})
		}
		logScanSummary(logger, source, destination)
		logDifferenceSummary(logger, source, comparison, 0, 0, false)
		logCaseConflicts(logger, comparison.CaseConflicts, "case_conflicts_reported")
		return nil
	}

	destinationRoot, err := os.OpenRoot(opts.Destination)
	if err != nil {
		return fmt.Errorf("open destination root: %w", err)
	}
	defer destinationRoot.Close()

	conflictedPaths := make(map[string]struct{})
	for _, group := range comparison.CaseConflicts {
		for _, path := range group {
			conflictedPaths[path] = struct{}{}
		}
	}
	candidates = withoutConflictedPaths(candidates, conflictedPaths)

	succeeded := 0
	failed := 0
	for completed, path := range candidates {
		if err := copyFile(opts.Source, destinationRoot, path, source[path]); err != nil {
			failed++
			logger.Warn("copy_failed",
				logx.Field{Key: "error", Value: err.Error()},
				logx.Field{Key: "path", Value: path},
			)
		} else {
			succeeded++
			logger.Info("copied", logx.Field{Key: "path", Value: path})
		}
		logger.Info("progress",
			logx.Field{Key: "completed", Value: strconv.Itoa(completed + 1)},
			logx.Field{Key: "total", Value: strconv.Itoa(len(candidates))},
			logx.Field{Key: "succeeded", Value: strconv.Itoa(succeeded)},
			logx.Field{Key: "failed", Value: strconv.Itoa(failed)},
		)
	}
	logScanSummary(logger, source, destination)
	logDifferenceSummary(logger, source, comparison, succeeded, failed, true)
	logCaseConflicts(logger, comparison.CaseConflicts, "case_conflicts_skipped")
	return nil
}

func withoutConflictedPaths(paths []string, conflicted map[string]struct{}) []string {
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, ok := conflicted[path]; !ok {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

func conflictedPathsInOrder(groups [][]string) []string {
	paths := make([]string, 0)
	for _, group := range groups {
		paths = append(paths, group...)
	}
	return paths
}

func caseConflicts(source map[string]Entry) [][]string {
	groups := make(map[string][]string)
	for path := range source {
		key := strings.ToLower(path)
		groups[key] = append(groups[key], path)
	}

	keys := make([]string, 0, len(groups))
	for key, paths := range groups {
		if len(paths) < 2 {
			continue
		}
		sort.Strings(paths)
		groups[key] = paths
		keys = append(keys, key)
	}
	sort.Strings(keys)

	conflicts := make([][]string, 0, len(keys))
	for _, key := range keys {
		conflicts = append(conflicts, groups[key])
	}
	return conflicts
}

func copyFile(sourceRoot string, destinationRoot *os.Root, relativePath string, entry Entry) error {
	sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(relativePath))
	if err := verifyRegularFile(sourcePath); err != nil {
		return fmt.Errorf("verify source: %w", err)
	}
	destinationName := filepath.Clean(filepath.FromSlash(relativePath))
	if parent := filepath.Dir(destinationName); parent != "." {
		if err := destinationRoot.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("create destination directory: %w", err)
		}
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer sourceFile.Close()

	destinationFile, err := openDestinationFile(destinationRoot, destinationName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, entry.Mode.Perm())
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	if _, err := copyStream(destinationFile, sourceFile); err != nil {
		destinationFile.Close()
		return copyFailure(destinationRoot, destinationName, "write destination", err)
	}
	if err := destinationFile.Close(); err != nil {
		return copyFailure(destinationRoot, destinationName, "close destination", err)
	}
	if err := changeMode(destinationRoot, destinationName, entry.Mode.Perm()); err != nil {
		return copyFailure(destinationRoot, destinationName, "set destination mode", err)
	}
	if err := changeTimes(destinationRoot, destinationName, entry.ModTime, entry.ModTime); err != nil {
		return copyFailure(destinationRoot, destinationName, "set destination modification time", err)
	}
	return nil
}

func verifyRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("source path is not a regular file")
	}
	return nil
}

func copyFailure(destinationRoot *os.Root, relativePath, operation string, operationErr error) error {
	if cleanupErr := cleanupPartial(destinationRoot, relativePath); cleanupErr != nil {
		return fmt.Errorf("%s: %w (cleanup partial destination: %v)", operation, operationErr, cleanupErr)
	}
	return fmt.Errorf("%s: %w", operation, operationErr)
}

func cleanupPartial(destinationRoot *os.Root, relativePath string) error {
	if err := destinationRoot.Remove(relativePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func logComparison(logger logx.Logger, comparison Comparison) {
	for _, path := range comparison.Missing {
		logger.Info("missing", logx.Field{Key: "path", Value: path})
	}
	for _, path := range comparison.Extra {
		logger.Info("extra", logx.Field{Key: "path", Value: path})
	}
	for _, path := range comparison.SourceLarger {
		logger.Info("source_larger", logx.Field{Key: "path", Value: path})
	}
	for _, path := range comparison.DestLarger {
		logger.Info("destination_larger", logx.Field{Key: "path", Value: path})
	}
}

func logScanSummary(logger logx.Logger, source, destination map[string]Entry) {
	logger.Info("scan_summary",
		logx.Field{Key: "source_files", Value: strconv.Itoa(len(source))},
		logx.Field{Key: "destination_files", Value: strconv.Itoa(len(destination))},
	)
}

func logDifferenceSummary(logger logx.Logger, source map[string]Entry, comparison Comparison, copied, failed int, copiedFiles bool) {
	consistent := len(source) - len(comparison.Missing) - len(comparison.SourceLarger) - len(comparison.DestLarger)
	fields := []logx.Field{
		{Key: "missing", Value: strconv.Itoa(len(comparison.Missing))},
		{Key: "extra", Value: strconv.Itoa(len(comparison.Extra))},
		{Key: "source_larger", Value: strconv.Itoa(len(comparison.SourceLarger))},
		{Key: "destination_larger", Value: strconv.Itoa(len(comparison.DestLarger))},
		{Key: "consistent", Value: strconv.Itoa(consistent)},
	}
	if copiedFiles {
		fields = append(fields,
			logx.Field{Key: "copied", Value: strconv.Itoa(copied)},
			logx.Field{Key: "failed", Value: strconv.Itoa(failed)},
		)
	}
	logger.Info("difference_summary", fields...)
}

func logCaseConflicts(logger logx.Logger, conflicts [][]string, event string) {
	if len(conflicts) == 0 {
		return
	}
	paths := conflictedPathsInOrder(conflicts)
	logger.Warn(event,
		logx.Field{Key: "files", Value: strconv.Itoa(len(paths))},
		logx.Field{Key: "groups", Value: strconv.Itoa(len(conflicts))},
		logx.Field{Key: "paths", Value: strings.Join(paths, ",")},
	)
}

func usableLogger(logger logx.Logger) logx.Logger {
	if logger.Out == nil {
		logger.Out = io.Discard
	}
	if logger.Err == nil {
		logger.Err = io.Discard
	}
	return logger
}
