package checkcopy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/weizhoublue/handle-files/internal/logx"
)

const copyMaxAttempts = 5
const copyBufferSize = 32 * 1024

type retryTimer interface {
	Channel() <-chan time.Time
	Stop() bool
}

type standardRetryTimer struct {
	*time.Timer
}

func (t *standardRetryTimer) Channel() <-chan time.Time {
	return t.C
}

var (
	scanEntryInfo = func(dirEntry fs.DirEntry) (fs.FileInfo, error) {
		return dirEntry.Info()
	}
	copyStream          = copyWithContext
	newRetryTimer       = func(delay time.Duration) retryTimer { return &standardRetryTimer{Timer: time.NewTimer(delay)} }
	waitBeforeCopyRetry = func(ctx context.Context) error {
		timer := newRetryTimer(time.Second)
		defer func() {
			if !timer.Stop() {
				select {
				case <-timer.Channel():
				default:
				}
			}
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.Channel():
			return nil
		}
	}
	openDestinationFile = func(root *os.Root, name string, flag int, perm fs.FileMode) (*os.File, error) {
		return root.OpenFile(name, flag, perm)
	}
	changeMode = func(root *os.Root, name string, mode fs.FileMode) error {
		return root.Chmod(name, mode)
	}
	changeTimes = func(root *os.Root, name string, accessTime, modificationTime time.Time) error {
		return root.Chtimes(name, accessTime, modificationTime)
	}
	notifyInterrupt = signal.Notify
	stopInterrupt   = signal.Stop
)

type copyOperationError struct {
	operation string
	err       error
}

func (e *copyOperationError) Error() string {
	return fmt.Sprintf("%s: %v", e.operation, e.err)
}

func (e *copyOperationError) Unwrap() error {
	return e.err
}

type copySourceReader struct {
	reader io.Reader
}

func newCopySourceReader(reader io.Reader) *copySourceReader {
	return &copySourceReader{reader: reader}
}

func (r *copySourceReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, &copyOperationError{operation: "read source", err: err}
	}
	return n, err
}

func (r *copySourceReader) Name() string {
	if named, ok := r.reader.(interface{ Name() string }); ok {
		return named.Name()
	}
	return ""
}

type copyDestinationWriter struct {
	writer io.Writer
}

func newCopyDestinationWriter(writer io.Writer) *copyDestinationWriter {
	return &copyDestinationWriter{writer: writer}
}

func (w *copyDestinationWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if err != nil {
		return n, &copyOperationError{operation: "write destination", err: err}
	}
	return n, nil
}

type InterruptedError struct {
	Signal os.Signal
	Err    error
}

func (e *InterruptedError) Error() string {
	return fmt.Sprintf("interrupted by %s: %v", e.Signal, e.Err)
}

func (e *InterruptedError) Unwrap() error {
	return e.Err
}

func (e *InterruptedError) ExitCode() int {
	switch e.Signal {
	case os.Interrupt:
		return 130
	case syscall.SIGTERM:
		return 143
	default:
		return 1
	}
}

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

type extensionFilter map[string]struct{}

func newExtensionFilter(types []string) extensionFilter {
	if len(types) == 0 {
		return nil
	}
	filter := make(extensionFilter, len(types))
	for _, extension := range types {
		filter[strings.ToLower(extension)] = struct{}{}
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

func Scan(root string, logger logx.Logger) (map[string]Entry, error) {
	return scan(context.Background(), root, nil, logger)
}

func scan(ctx context.Context, root string, filter extensionFilter, logger logx.Logger) (map[string]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	logger = usableLogger(logger)
	entries := make(map[string]Entry)
	err := filepath.WalkDir(root, func(path string, dirEntry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
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
		if errors.Is(err, ctx.Err()) {
			return nil, err
		}
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

func Run(ctx context.Context, opts Options, logger logx.Logger) error {
	runCtx, finish := interruptContext(ctx)
	err := run(runCtx, opts, logger)
	if interrupted := finish(err); interrupted != nil {
		return interrupted
	}
	return err
}

func interruptContext(parent context.Context) (context.Context, func(error) error) {
	ctx, cancel := context.WithCancel(parent)
	signals := make(chan os.Signal, 1)
	notifyInterrupt(signals, os.Interrupt, syscall.SIGTERM)

	done := make(chan struct{})
	finished := make(chan struct{})
	var once sync.Once
	var interrupted os.Signal

	go func() {
		defer close(finished)

		select {
		case received := <-signals:
			interrupted = received
			stopInterrupt(signals)
			cancel()
		case <-parent.Done():
		case <-done:
		}
	}()

	return ctx, func(runErr error) error {
		once.Do(func() {
			stopInterrupt(signals)
			close(done)
			<-finished
			cancel()
		})
		if interrupted == nil {
			return nil
		}
		return &InterruptedError{Signal: interrupted, Err: runErr}
	}
}

func run(ctx context.Context, opts Options, logger logx.Logger) error {
	logger = usableLogger(logger)
	normalizedOptions, err := normalizeOptions(opts)
	if err != nil {
		return err
	}
	opts = normalizedOptions

	filter := newExtensionFilter(opts.Types)
	source, err := scan(ctx, opts.Source, filter, logger)
	if err != nil {
		return err
	}
	destination, err := scan(ctx, opts.Destination, filter, logger)
	if err != nil {
		return err
	}
	comparison := Compare(source, destination)

	fmt.Println("")
	logComparison(logger, comparison)
	fmt.Println("")

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
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := copyWithRetries(ctx, opts.Source, destinationRoot, path, source[path], logger); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
				return err
			}
			failed++
			logger.WarnProgress("copy_failed",
				[]logx.Field{
					{Key: "error", Value: err.Error()},
					{Key: "path", Value: path},
				},
				copyProgressFields(completed+1, len(candidates), succeeded, failed),
			)
		} else {
			succeeded++
			logger.InfoProgress(
				"copied",
				[]logx.Field{{Key: "path", Value: path}},
				copyProgressFields(completed+1, len(candidates), succeeded, failed),
			)
		}
	}

	fmt.Println("")
	logScanSummary(logger, source, destination)
	logDifferenceSummary(logger, source, comparison, succeeded, failed, true)
	logCaseConflicts(logger, comparison.CaseConflicts, "case_conflicts_skipped")
	if failed > 0 {
		return fmt.Errorf("%d files failed to copy", failed)
	}
	return nil
}

func copyProgressFields(completed, total, succeeded, failed int) []logx.Field {
	return []logx.Field{
		{Key: "completed", Value: strconv.Itoa(completed)},
		{Key: "total", Value: strconv.Itoa(total)},
		{Key: "succeeded", Value: strconv.Itoa(succeeded)},
		{Key: "failed", Value: strconv.Itoa(failed)},
	}
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

func copyWithRetries(ctx context.Context, sourceRoot string, destinationRoot *os.Root, relativePath string, entry Entry, logger logx.Logger) error {
	for attempt := 1; attempt <= copyMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := copyFile(ctx, sourceRoot, destinationRoot, relativePath, entry)
		if err == nil {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
			return err
		}
		if attempt == copyMaxAttempts {
			return err
		}
		logger.Warn("copy_retrying",
			logx.Field{Key: "attempt", Value: strconv.Itoa(attempt)},
			logx.Field{Key: "error", Value: err.Error()},
			logx.Field{Key: "path", Value: relativePath},
			logx.Field{Key: "total_attempts", Value: strconv.Itoa(copyMaxAttempts)},
		)
		if err := waitBeforeCopyRetry(ctx); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(ctx context.Context, sourceRoot string, destinationRoot *os.Root, relativePath string, entry Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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
	if _, err := copyStream(ctx, newCopyDestinationWriter(destinationFile), newCopySourceReader(sourceFile)); err != nil {
		destinationFile.Close()
		return copyFailure(destinationRoot, destinationName, normalizeCopyStreamError(err))
	}
	if err := destinationFile.Close(); err != nil {
		return copyFailure(destinationRoot, destinationName, fmt.Errorf("close destination: %w", err))
	}
	if err := changeMode(destinationRoot, destinationName, entry.Mode.Perm()); err != nil {
		return copyFailure(destinationRoot, destinationName, fmt.Errorf("set destination mode: %w", err))
	}
	if err := changeTimes(destinationRoot, destinationName, entry.ModTime, entry.ModTime); err != nil {
		return copyFailure(destinationRoot, destinationName, fmt.Errorf("set destination modification time: %w", err))
	}
	return nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buffer := make([]byte, copyBufferSize)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		read, readErr := src.Read(buffer)
		if read > 0 {
			written, writeErr := dst.Write(buffer[:read])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != read {
				return total, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return total, nil
			}
			return total, readErr
		}
	}
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

func copyFailure(destinationRoot *os.Root, relativePath string, operationErr error) error {
	if cleanupErr := cleanupPartial(destinationRoot, relativePath); cleanupErr != nil {
		return fmt.Errorf("%w (cleanup partial destination: %v)", operationErr, cleanupErr)
	}
	return operationErr
}

func cleanupPartial(destinationRoot *os.Root, relativePath string) error {
	if err := destinationRoot.Remove(relativePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func normalizeCopyStreamError(err error) error {
	if err == nil {
		return nil
	}
	var operationErr *copyOperationError
	if errors.As(err, &operationErr) {
		return err
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		switch pathErr.Op {
		case "read":
			return &copyOperationError{operation: "read source", err: err}
		case "write":
			return &copyOperationError{operation: "write destination", err: err}
		}
	}
	if errors.Is(err, io.ErrShortWrite) {
		return &copyOperationError{operation: "write destination", err: err}
	}
	return err
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
