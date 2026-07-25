package compress

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/weizhoublue/handle-files/internal/logx"
)

type CommandRunner interface {
	LookPath(name string) (string, error)
	Run(ctx context.Context, name string, args ...string) error
}

type execRunner struct{}

func NewCommandRunner() CommandRunner {
	return execRunner{}
}

func (execRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func (execRunner) Run(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

type Summary struct {
	Total     int
	Succeeded int
	Skipped   int
	Failed    int
}

func Run(ctx context.Context, opts Options, runner CommandRunner, input io.Reader, logger logx.Logger) (Summary, error) {
	logger = usableLogger(logger)
	if runner == nil {
		return Summary{}, errors.New("ffmpeg command runner is required")
	}

	ffmpegPath, err := runner.LookPath("ffmpeg")
	if err != nil {
		return Summary{}, fmt.Errorf("ffmpeg dependency lookup failed: %w", err)
	}
	if err := runner.Run(ctx, ffmpegPath, "-version"); err != nil {
		return Summary{}, fmt.Errorf("ffmpeg dependency health check (-version) failed: %w", err)
	}

	files, err := walkMP4Files(opts.Directory, func(path string) {
		logger.Info("skip",
			logx.Field{Key: "path", Value: path},
			logx.Field{Key: "reason", Value: "already_processed"},
		)
	})
	if err != nil {
		return Summary{}, fmt.Errorf("discover MP4 files: %w", err)
	}

	summary := Summary{Total: len(files)}
	if !opts.Execute {
		for _, path := range files {
			logger.Info("preview",
				logx.Field{Key: "input", Value: path},
				logx.Field{Key: "output", Value: outputPath(path)},
			)
			summary.Succeeded++
		}
		logSummary(logger, summary)
		return summary, nil
	}

	if input == nil {
		input = strings.NewReader("")
	}
	confirmations := bufio.NewScanner(input)
	for _, path := range files {
		output := outputPath(path)
		if !opts.Yes {
			logger.Info("confirm",
				logx.Field{Key: "input", Value: path},
				logx.Field{Key: "output", Value: output},
			)
			if !confirmations.Scan() || !strings.EqualFold(strings.TrimSpace(confirmations.Text()), "y") {
				summary.Skipped++
				logger.Info("skip",
					logx.Field{Key: "path", Value: path},
					logx.Field{Key: "reason", Value: "not_confirmed"},
				)
				logProgress(logger, summary)
				continue
			}
		}

		args := make([]string, 0, len(opts.FFArgs)+3)
		args = append(args, "-i", path)
		args = append(args, opts.FFArgs...)
		args = append(args, output)
		if err := runner.Run(ctx, ffmpegPath, args...); err != nil {
			summary.Failed++
			logger.Error("compress_failed",
				logx.Field{Key: "error", Value: err.Error()},
				logx.Field{Key: "path", Value: path},
			)
			if cleanupErr := os.Remove(output); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
				logger.Warn("cleanup_failed",
					logx.Field{Key: "error", Value: cleanupErr.Error()},
					logx.Field{Key: "path", Value: output},
				)
			}
			logProgress(logger, summary)
			continue
		}
		if _, err := os.Stat(output); err != nil {
			summary.Failed++
			logger.Error("output_missing",
				logx.Field{Key: "error", Value: err.Error()},
				logx.Field{Key: "path", Value: output},
			)
			logProgress(logger, summary)
			continue
		}
		if err := os.Remove(path); err != nil {
			summary.Failed++
			logger.Error("source_delete_failed",
				logx.Field{Key: "error", Value: err.Error()},
				logx.Field{Key: "path", Value: path},
			)
			logProgress(logger, summary)
			continue
		}

		summary.Succeeded++
		logger.Info("compressed",
			logx.Field{Key: "input", Value: path},
			logx.Field{Key: "output", Value: output},
		)
		logProgress(logger, summary)
	}
	if err := confirmations.Err(); err != nil {
		return summary, fmt.Errorf("read compression confirmation: %w", err)
	}
	logSummary(logger, summary)
	return summary, nil
}

func discoverMP4Files(root string) ([]string, error) {
	return walkMP4Files(root, nil)
}

func walkMP4Files(root string, skipped func(string)) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		extension := filepath.Ext(entry.Name())
		if !strings.EqualFold(extension, ".mp4") {
			return nil
		}
		stem := strings.TrimSuffix(entry.Name(), extension)
		if strings.HasSuffix(stem, "_output") {
			if skipped != nil {
				skipped(path)
			}
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func outputPath(input string) string {
	extension := filepath.Ext(input)
	stem := strings.TrimSuffix(input, extension)
	return stem + "_output" + extension
}

func logProgress(logger logx.Logger, summary Summary) {
	logger.Info("progress",
		logx.Field{Key: "completed", Value: strconv.Itoa(summary.Succeeded + summary.Skipped + summary.Failed)},
		logx.Field{Key: "total", Value: strconv.Itoa(summary.Total)},
		logx.Field{Key: "succeeded", Value: strconv.Itoa(summary.Succeeded)},
		logx.Field{Key: "skipped", Value: strconv.Itoa(summary.Skipped)},
		logx.Field{Key: "failed", Value: strconv.Itoa(summary.Failed)},
	)
}

func logSummary(logger logx.Logger, summary Summary) {
	logger.Info("summary",
		logx.Field{Key: "total", Value: strconv.Itoa(summary.Total)},
		logx.Field{Key: "succeeded", Value: strconv.Itoa(summary.Succeeded)},
		logx.Field{Key: "skipped", Value: strconv.Itoa(summary.Skipped)},
		logx.Field{Key: "failed", Value: strconv.Itoa(summary.Failed)},
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
