package compress

import (
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
	RunWithOutput(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error
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

func (execRunner) RunWithOutput(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

type Summary struct {
	Total     int
	Succeeded int
	Skipped   int
	Failed    int
}

func Run(ctx context.Context, opts Options, runner CommandRunner, logger logx.Logger) (Summary, error) {
	logger = usableLogger(logger)
	if runner == nil {
		return Summary{}, errors.New("ffmpeg command runner is required")
	}

	outputRoot := opts.Source
	if opts.Destination != "" {
		outputRoot = opts.Destination
	}
	logger.Info("run_config",
		logx.Field{Key: "source", Value: opts.Source},
		logx.Field{Key: "output_root", Value: outputRoot},
		logx.Field{Key: "execute", Value: strconv.FormatBool(opts.Execute)},
		logx.Field{Key: "remove", Value: strconv.FormatBool(opts.Remove)},
		logx.Field{Key: "ffmpeg_args", Value: strings.Join(opts.FFArgs, " ")},
	)

	ffmpegPath, err := runner.LookPath("ffmpeg")
	if err != nil {
		return Summary{}, fmt.Errorf("ffmpeg dependency lookup failed: %w", err)
	}
	if err := runner.Run(ctx, ffmpegPath, "-version"); err != nil {
		return Summary{}, fmt.Errorf("ffmpeg dependency health check (-version) failed: %w", err)
	}

	files, err := walkMP4Files(opts.Source, func(path string) {
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
			output, err := outputPath(opts, path)
			if err != nil {
				return summary, fmt.Errorf("compute output path for %q: %w", path, err)
			}
			logger.Info("preview",
				logx.Field{Key: "input", Value: path},
				logx.Field{Key: "output", Value: output},
			)
			summary.Succeeded++
		}
		logSummary(logger, summary)
		return summary, nil
	}

	for _, path := range files {
		output, err := outputPath(opts, path)
		if err != nil {
			return summary, fmt.Errorf("compute output path for %q: %w", path, err)
		}

		args := make([]string, 0, len(opts.FFArgs)+3)
		args = append(args, "-i", path)
		args = append(args, opts.FFArgs...)
		args = append(args, output)
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			summary.Failed++
			logger.ErrorProgress("output_dir_create_failed", []logx.Field{
				{Key: "error", Value: err.Error()},
				{Key: "path", Value: filepath.Dir(output)},
			}, progressFields(summary))
			continue
		}
		logger.Info("compress_started",
			logx.Field{Key: "input", Value: path},
			logx.Field{Key: "output", Value: output},
		)
		if err := runner.RunWithOutput(ctx, logger.Out, logger.Err, ffmpegPath, args...); err != nil {
			summary.Failed++
			fields := []logx.Field{
				{Key: "error", Value: err.Error()},
				{Key: "path", Value: path},
			}
			if cleanupErr := os.Remove(output); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
				fields = append(fields, logx.Field{Key: "cleanup_error", Value: cleanupErr.Error()})
			}
			logger.ErrorProgress("compress_failed", fields, progressFields(summary))
			continue
		}
		info, err := os.Stat(output)
		if err != nil || !info.Mode().IsRegular() {
			if err == nil {
				err = errors.New("output is not a regular file")
			}
			summary.Failed++
			logger.ErrorProgress("output_missing", []logx.Field{
				{Key: "error", Value: err.Error()},
				{Key: "path", Value: output},
			}, progressFields(summary))
			continue
		}
		sourceInfo, err := os.Stat(path)
		if err != nil {
			summary.Failed++
			logger.ErrorProgress("source_size_failed", []logx.Field{
				{Key: "error", Value: err.Error()},
				{Key: "path", Value: path},
			}, progressFields(summary))
			continue
		}
		reductionBytes := sourceInfo.Size() - info.Size()
		reductionPercent := 0.0
		if sourceInfo.Size() != 0 {
			reductionPercent = float64(reductionBytes) / float64(sourceInfo.Size()) * 100
		}
		if opts.Remove {
			if err := os.Remove(path); err != nil {
				summary.Failed++
				logger.ErrorProgress("source_delete_failed", []logx.Field{
					{Key: "error", Value: err.Error()},
					{Key: "path", Value: path},
				}, progressFields(summary))
				continue
			}
		}

		summary.Succeeded++
		logger.InfoProgress("compressed", []logx.Field{
			{Key: "input", Value: path},
			{Key: "output", Value: output},
			{Key: "original_bytes", Value: strconv.FormatInt(sourceInfo.Size(), 10)},
			{Key: "output_bytes", Value: strconv.FormatInt(info.Size(), 10)},
			{Key: "reduction_bytes", Value: strconv.FormatInt(reductionBytes, 10)},
			{Key: "reduction_percent", Value: fmt.Sprintf("%.2f", reductionPercent)},
		}, progressFields(summary))
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

func outputPath(opts Options, input string) (string, error) {
	extension := filepath.Ext(input)
	outputName := strings.TrimSuffix(filepath.Base(input), extension) + "_output" + extension
	if opts.Destination == "" {
		return filepath.Join(filepath.Dir(input), outputName), nil
	}

	relativePath, err := filepath.Rel(opts.Source, input)
	if err != nil {
		return "", fmt.Errorf("relative path: %w", err)
	}
	return filepath.Join(opts.Destination, filepath.Dir(relativePath), outputName), nil
}

func progressFields(summary Summary) []logx.Field {
	return []logx.Field{
		{Key: "completed", Value: strconv.Itoa(summary.Succeeded + summary.Skipped + summary.Failed)},
		{Key: "total", Value: strconv.Itoa(summary.Total)},
		{Key: "succeeded", Value: strconv.Itoa(summary.Succeeded)},
		{Key: "skipped", Value: strconv.Itoa(summary.Skipped)},
		{Key: "failed", Value: strconv.Itoa(summary.Failed)},
	}
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
