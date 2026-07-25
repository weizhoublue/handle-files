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
		info, err := dirEntry.Info()
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
		return nil
	}

	succeeded := 0
	failed := 0
	for completed, path := range candidates {
		if err := copyFile(opts.Source, opts.Destination, path, source[path]); err != nil {
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
	return nil
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

func copyFile(sourceRoot, destinationRoot, relativePath string, entry Entry) error {
	sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(relativePath))
	destinationPath := filepath.Join(destinationRoot, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer sourceFile.Close()

	destinationFile, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, entry.Mode.Perm())
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		destinationFile.Close()
		cleanupPartial(destinationPath)
		return fmt.Errorf("write destination: %w", err)
	}
	if err := destinationFile.Close(); err != nil {
		cleanupPartial(destinationPath)
		return fmt.Errorf("close destination: %w", err)
	}
	if err := os.Chmod(destinationPath, entry.Mode.Perm()); err != nil {
		cleanupPartial(destinationPath)
		return fmt.Errorf("set destination mode: %w", err)
	}
	if err := os.Chtimes(destinationPath, entry.ModTime, entry.ModTime); err != nil {
		cleanupPartial(destinationPath)
		return fmt.Errorf("set destination modification time: %w", err)
	}
	return nil
}

func cleanupPartial(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
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
	for _, conflict := range comparison.CaseConflicts {
		logger.Warn("case_conflict", logx.Field{Key: "paths", Value: strings.Join(conflict, ",")})
	}
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
