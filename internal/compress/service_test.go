package compress

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/weizhoublue/handle-files/internal/logx"
)

type runnerCall struct {
	name string
	args []string
}

type fakeRunner struct {
	path          string
	lookPathErr   error
	run           func(name string, args ...string) error
	runWithOutput func(stdout, stderr io.Writer, name string, args ...string) error
	calls         []runnerCall
}

func (r *fakeRunner) LookPath(string) (string, error) {
	if r.lookPathErr != nil {
		return "", r.lookPathErr
	}
	return r.path, nil
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	r.calls = append(r.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	if r.run != nil {
		return r.run(name, args...)
	}
	return nil
}

func (r *fakeRunner) RunWithOutput(_ context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	r.calls = append(r.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	if r.runWithOutput != nil {
		return r.runWithOutput(stdout, stderr, name, args...)
	}
	if r.run != nil {
		return r.run(name, args...)
	}
	return nil
}

func TestDiscoverMP4FilesSkipsOutputFiles(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "one", "two", "clip.MP4"), "video")
	mustWrite(t, filepath.Join(root, "one", "two", "clip_output.MP4"), "video")
	mustWrite(t, filepath.Join(root, "one", "two", "clip_OUTPUT.MP4"), "video")
	if err := os.Mkdir(filepath.Join(root, "one", "two", "directory.mp4"), 0o755); err != nil {
		t.Fatal(err)
	}

	files, err := discoverMP4Files(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(root, "one", "two", "clip.MP4"),
		filepath.Join(root, "one", "two", "clip_OUTPUT.MP4"),
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("discoverMP4Files() = %#v, want %#v", files, want)
	}
}

func TestRunChecksFFmpegBeforeCompressingAndDeletesSourceAfterSuccess(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "clip.MP4")
	output := filepath.Join(root, "clip_output.MP4")
	mustWrite(t, source, "video")
	runner := &fakeRunner{
		path: "/fake/ffmpeg",
		run: func(_ string, args ...string) error {
			if len(args) != 1 || args[0] != "-version" {
				mustWrite(t, args[len(args)-1], "compressed")
			}
			return nil
		},
	}
	var logs bytes.Buffer

	summary, err := Run(
		context.Background(),
		Options{Source: root, Remove: true, Execute: true, FFArgs: []string{"-c:v", "libx264"}},
		runner,
		testLogger(&logs),
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Summary{Total: 1, Succeeded: 1}); summary != want {
		t.Fatalf("Run() summary = %#v, want %#v", summary, want)
	}
	wantCalls := []runnerCall{
		{name: "/fake/ffmpeg", args: []string{"-version"}},
		{name: "/fake/ffmpeg", args: []string{"-i", source, "-c:v", "libx264", output}},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("runner calls = %#v, want %#v", runner.calls, wantCalls)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("output stat error = %v", err)
	}
	if got := strings.Count(logs.String(), "progress"); got != 0 {
		t.Fatalf("progress records = %d, want 0:\n%s", got, logs.String())
	}
	if !strings.Contains(logs.String(), "INFO, compressed ") ||
		!strings.Contains(logs.String(), "[ completed=1 failed=0 skipped=0 succeeded=1 total=1 ]") {
		t.Fatalf("compression progress missing counters:\n%s", logs.String())
	}
}

func TestRunDestinationMapsOutputAndRemovesSourceByDefault(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	destinationRoot := filepath.Join(root, "destination")
	input := filepath.Join(sourceRoot, "nested", "clip.mp4")
	output := filepath.Join(destinationRoot, "nested", "clip_output.mp4")
	mustWrite(t, input, "video")
	if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		path: "/fake/ffmpeg",
		runWithOutput: func(stdout, stderr io.Writer, _ string, args ...string) error {
			mustWrite(t, args[len(args)-1], "compressed")
			return nil
		},
	}

	summary, err := Run(
		context.Background(),
		Options{Source: sourceRoot, Destination: destinationRoot, Remove: true, Execute: true, FFArgs: []string{"-c:v", "libx264"}},
		runner,
		testLogger(&bytes.Buffer{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Summary{Total: 1, Succeeded: 1}); summary != want {
		t.Fatalf("Run() summary = %#v, want %#v", summary, want)
	}
	if got := runner.calls[len(runner.calls)-1].args[len(runner.calls[len(runner.calls)-1].args)-1]; got != output {
		t.Fatalf("output arg = %q, want %q", got, output)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("output stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Dir(output)); err != nil {
		t.Fatalf("output dir stat error = %v", err)
	}
	if _, err := os.Stat(input); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source stat error = %v, want not exist", err)
	}
}

func TestRunRemoveFalseRetainsSourceAfterSuccess(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	destinationRoot := filepath.Join(root, "destination")
	input := filepath.Join(sourceRoot, "nested", "clip.mp4")
	mustWrite(t, input, "video")
	if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		path: "/fake/ffmpeg",
		runWithOutput: func(stdout, stderr io.Writer, _ string, args ...string) error {
			mustWrite(t, args[len(args)-1], "compressed")
			return nil
		},
	}

	summary, err := Run(
		context.Background(),
		Options{
			Source:      sourceRoot,
			Destination: destinationRoot,
			Remove:      false,
			Execute:     true,
			FFArgs:      []string{"-c:v", "libx264"},
		},
		runner,
		testLogger(&bytes.Buffer{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Summary{Total: 1, Succeeded: 1}); summary != want {
		t.Fatalf("Run() summary = %#v, want %#v", summary, want)
	}
	if _, err := os.Stat(input); err != nil {
		t.Fatalf("source stat error = %v", err)
	}
}

func TestRunConfigLogsSettingsAndStartBeforeCompressed(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	destinationRoot := filepath.Join(root, "destination")
	input := filepath.Join(sourceRoot, "nested", "clip.mp4")
	output := filepath.Join(destinationRoot, "nested", "clip_output.mp4")
	mustWrite(t, input, "video")
	if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		path: "/fake/ffmpeg",
		runWithOutput: func(stdout, stderr io.Writer, _ string, args ...string) error {
			mustWrite(t, args[len(args)-1], "compressed")
			return nil
		},
	}
	var out bytes.Buffer

	summary, err := Run(
		context.Background(),
		Options{
			Source:      sourceRoot,
			Destination: destinationRoot,
			Remove:      false,
			Execute:     true,
			FFArgs:      []string{"-c:v", "libx264", "-preset", "slow"},
		},
		runner,
		testLoggerWithErr(&out, &bytes.Buffer{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Summary{Total: 1, Succeeded: 1}); summary != want {
		t.Fatalf("Run() summary = %#v, want %#v", summary, want)
	}
	logs := out.String()
	if !strings.Contains(logs, "INFO, run_config ") ||
		!strings.Contains(logs, "source="+sourceRoot) ||
		!strings.Contains(logs, "output_root="+destinationRoot) ||
		!strings.Contains(logs, "execute=true") ||
		!strings.Contains(logs, "remove=false") ||
		!strings.Contains(logs, "ffmpeg_args=-c:v libx264 -preset slow") {
		t.Fatalf("run_config log missing settings:\n%s", logs)
	}
	runConfigIndex := strings.Index(logs, "INFO, run_config ")
	startedIndex := strings.Index(logs, "INFO, compress_started ")
	compressedIndex := strings.Index(logs, "INFO, compressed input="+input+" ")
	if runConfigIndex == -1 || startedIndex == -1 || compressedIndex == -1 {
		t.Fatalf("missing expected log order markers:\n%s", logs)
	}
	if !(runConfigIndex < startedIndex && startedIndex < compressedIndex) {
		t.Fatalf("unexpected log order:\n%s", logs)
	}
	if !strings.Contains(logs, "output="+output) {
		t.Fatalf("compressed log missing mapped output:\n%s", logs)
	}
}

func TestRunStreamsChildOutputToLogger(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "clip.mp4")
	output := filepath.Join(root, "clip_output.mp4")
	mustWrite(t, source, "video")
	runner := &fakeRunner{
		path: "/fake/ffmpeg",
		runWithOutput: func(stdout, stderr io.Writer, _ string, args ...string) error {
			_, _ = io.WriteString(stderr, "ffmpeg live output\n")
			mustWrite(t, args[len(args)-1], "compressed")
			return nil
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	summary, err := Run(
		context.Background(),
		Options{Source: root, Execute: true, FFArgs: []string{"-c:v", "libx264"}},
		runner,
		testLoggerWithErr(&out, &errOut),
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Summary{Total: 1, Succeeded: 1}); summary != want {
		t.Fatalf("Run() summary = %#v, want %#v", summary, want)
	}
	if got := errOut.String(); got != "ffmpeg live output\n" {
		t.Fatalf("stderr = %q, want raw ffmpeg output", got)
	}
	logs := out.String()
	if !strings.Contains(logs, "INFO, compressed input="+source+" ") ||
		!strings.Contains(logs, "output="+output) ||
		!strings.Contains(logs, "original_size=5 B") ||
		!strings.Contains(logs, "output_size=10 B") {
		t.Fatalf("compressed log missing size details:\n%s", logs)
	}
}

func TestRunDestinationCleansFailedOutputAndRetainsSource(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	destinationRoot := filepath.Join(root, "destination")
	input := filepath.Join(sourceRoot, "nested", "clip.mp4")
	output := filepath.Join(destinationRoot, "nested", "clip_output.mp4")
	mustWrite(t, input, "video")
	if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		path: "/fake/ffmpeg",
		runWithOutput: func(stdout, stderr io.Writer, _ string, args ...string) error {
			mustWrite(t, args[len(args)-1], "partial")
			return errors.New("encoding failed")
		},
	}

	summary, err := Run(
		context.Background(),
		Options{Source: sourceRoot, Destination: destinationRoot, Execute: true, FFArgs: []string{"-c:v", "libx264"}},
		runner,
		testLogger(&bytes.Buffer{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Summary{Total: 1, Failed: 1}); summary != want {
		t.Fatalf("Run() summary = %#v, want %#v", summary, want)
	}
	if _, err := os.Stat(input); err != nil {
		t.Fatalf("source stat error = %v", err)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output stat error = %v, want not exist", err)
	}
}

func TestRunReportsSizeReductionForSuccessfulCompression(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		output         string
		wantPercentage string
	}{
		{name: "smaller output", input: "original", output: "out", wantPercentage: "62.50"},
		{name: "empty input", input: "", output: "", wantPercentage: "0.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "clip.mp4")
			output := filepath.Join(root, "clip_output.mp4")
			mustWrite(t, source, tt.input)
			runner := &fakeRunner{
				path: "/fake/ffmpeg",
				run: func(_ string, args ...string) error {
					if len(args) != 1 || args[0] != "-version" {
						mustWrite(t, args[len(args)-1], tt.output)
					}
					return nil
				},
			}
			var logs bytes.Buffer

			summary, err := Run(
				context.Background(),
				Options{Source: root, Remove: true, Execute: true, FFArgs: []string{"-c:v", "libx264"}},
				runner,
				testLogger(&logs),
			)
			if err != nil {
				t.Fatal(err)
			}
			if want := (Summary{Total: 1, Succeeded: 1}); summary != want {
				t.Fatalf("Run() summary = %#v, want %#v", summary, want)
			}
			wantLog := "INFO, compressed input=" + source + " original_size=" +
				formatSize(int64(len(tt.input))) + " output=" + output + " output_size=" +
				formatSize(int64(len(tt.output))) + " reduction_percent=" +
				tt.wantPercentage + " reduction_size=" +
				formatSize(int64(len(tt.input)-len(tt.output)))
			if !strings.Contains(logs.String(), wantLog) {
				t.Fatalf("size reduction log missing:\n%s", logs.String())
			}
			if strings.Contains(logs.String(), "original_bytes=") ||
				strings.Contains(logs.String(), "output_bytes=") ||
				strings.Contains(logs.String(), "reduction_bytes=") {
				t.Fatalf("legacy size fields present:\n%s", logs.String())
			}
		})
	}
}

func TestFormatSizeUsesDecimalUnits(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{999, "999 B"},
		{-999, "-999 B"},
		{1000, "1.0 KB"},
		{-1500, "-1.5 KB"},
		{10_000, "10.0 KB"},
		{1_000_000, "1.0 MB"},
		{1_200_000_000, "1.2 GB"},
		{1_000_000_000_000, "1.0 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatSize(tt.bytes); got != tt.want {
				t.Fatalf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestRunCleansFailedOutputAndRetainsSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "clip.mp4")
	output := filepath.Join(root, "clip_output.mp4")
	mustWrite(t, source, "video")
	runner := &fakeRunner{
		path: "/fake/ffmpeg",
		run: func(_ string, args ...string) error {
			if len(args) != 1 || args[0] != "-version" {
				mustWrite(t, args[len(args)-1], "partial")
				return errors.New("encoding failed")
			}
			return nil
		},
	}
	var logs bytes.Buffer

	summary, err := Run(
		context.Background(),
		Options{Source: root, Remove: true, Execute: true, FFArgs: []string{"-c:v", "libx264"}},
		runner,
		testLogger(&logs),
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Summary{Total: 1, Failed: 1}); summary != want {
		t.Fatalf("Run() summary = %#v, want %#v", summary, want)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source stat error = %v", err)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output stat error = %v, want not exist", err)
	}
	if got := strings.Count(logs.String(), "progress"); got != 0 {
		t.Fatalf("progress records = %d, want 0:\n%s", got, logs.String())
	}
	if !strings.Contains(logs.String(), "ERROR, compress_failed ") ||
		!strings.Contains(logs.String(), "[ completed=1 failed=1 skipped=0 succeeded=0 total=1 ]") {
		t.Fatalf("compression progress missing counters:\n%s", logs.String())
	}
}

func TestRunRetainsSourceWhenCommandProducesNoOutput(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "clip.mp4")
	mustWrite(t, source, "video")
	runner := &fakeRunner{path: "/fake/ffmpeg"}

	summary, err := Run(
		context.Background(),
		Options{Source: root, Execute: true, FFArgs: []string{"-c:v", "libx264"}},
		runner,
		testLogger(&bytes.Buffer{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Summary{Total: 1, Failed: 1}); summary != want {
		t.Fatalf("Run() summary = %#v, want %#v", summary, want)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source stat error = %v", err)
	}
}

func TestRunRetainsSourceWhenOutputIsNotRegularFile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "clip.mp4")
	mustWrite(t, source, "video")
	if err := os.Mkdir(filepath.Join(root, "clip_output.mp4"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{path: "/fake/ffmpeg"}

	summary, err := Run(
		context.Background(),
		Options{Source: root, Execute: true, FFArgs: []string{"-c:v", "libx264"}},
		runner,
		testLogger(&bytes.Buffer{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Summary{Total: 1, Failed: 1}); summary != want {
		t.Fatalf("Run() summary = %#v, want %#v", summary, want)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source stat error = %v", err)
	}
}

func TestRunPreviewDoesNotInvokeCompression(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	destinationRoot := filepath.Join(root, "destination")
	input := filepath.Join(sourceRoot, "nested", "clip.mp4")
	output := filepath.Join(destinationRoot, "nested", "clip_output.mp4")
	mustWrite(t, input, "video")
	if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{path: "/fake/ffmpeg"}

	summary, err := Run(
		context.Background(),
		Options{Source: sourceRoot, Destination: destinationRoot, FFArgs: []string{"-c:v", "libx264"}},
		runner,
		testLogger(&bytes.Buffer{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Summary{Total: 1, Succeeded: 1}); summary != want {
		t.Fatalf("Run() summary = %#v, want %#v", summary, want)
	}
	if want := []runnerCall{{name: "/fake/ffmpeg", args: []string{"-version"}}}; !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("runner calls = %#v, want %#v", runner.calls, want)
	}
	if _, err := os.Stat(input); err != nil {
		t.Fatalf("source stat error = %v", err)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preview output stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Dir(output)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preview output dir stat error = %v, want not exist", err)
	}
}

func TestRunExecutesEachLiveFileWithoutConfirmation(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "a.mp4")
	second := filepath.Join(root, "b.mp4")
	mustWrite(t, first, "video")
	mustWrite(t, second, "video")
	runner := &fakeRunner{
		path: "/fake/ffmpeg",
		run: func(_ string, args ...string) error {
			if len(args) != 1 || args[0] != "-version" {
				mustWrite(t, args[len(args)-1], "compressed")
			}
			return nil
		},
	}
	var logs bytes.Buffer

	summary, err := Run(
		context.Background(),
		Options{Source: root, Remove: true, Execute: true, FFArgs: []string{"-c:v", "libx264"}},
		runner,
		testLogger(&logs),
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Summary{Total: 2, Succeeded: 2}); summary != want {
		t.Fatalf("Run() summary = %#v, want %#v", summary, want)
	}
	if got := len(runner.calls); got != 3 {
		t.Fatalf("runner calls = %d, want 3", got)
	}
	if runner.calls[0].args[0] != "-version" ||
		runner.calls[1].args[0] != "-i" || runner.calls[2].args[0] != "-i" {
		t.Fatalf("runner calls = %#v, want version call followed by two encodes", runner.calls)
	}
	if _, err := os.Stat(first); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first source stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(second); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("confirmed source stat error = %v, want not exist", err)
	}
	if got := strings.Count(logs.String(), "progress"); got != 0 {
		t.Fatalf("progress records = %d, want 0:\n%s", got, logs.String())
	}
	if !strings.Contains(logs.String(), "INFO, compressed ") ||
		!strings.Contains(logs.String(), "[ completed=2 failed=0 skipped=0 succeeded=2 total=2 ]") {
		t.Fatalf("compression progress missing counters:\n%s", logs.String())
	}
}

func TestRunReturnsDependencyErrors(t *testing.T) {
	t.Run("lookup", func(t *testing.T) {
		_, err := Run(
			context.Background(),
			Options{Source: t.TempDir()},
			&fakeRunner{lookPathErr: errors.New("not found")},
			testLogger(&bytes.Buffer{}),
		)
		if err == nil || !strings.Contains(err.Error(), "ffmpeg") {
			t.Fatalf("Run() error = %v, want ffmpeg lookup error", err)
		}
	})

	t.Run("health check", func(t *testing.T) {
		runner := &fakeRunner{
			path: "/fake/ffmpeg",
			run: func(_ string, args ...string) error {
				if reflect.DeepEqual(args, []string{"-version"}) {
					return errors.New("version failed")
				}
				return nil
			},
		}
		_, err := Run(
			context.Background(),
			Options{Source: t.TempDir()},
			runner,
			testLogger(&bytes.Buffer{}),
		)
		if err == nil || !strings.Contains(err.Error(), "ffmpeg") || !strings.Contains(err.Error(), "version") {
			t.Fatalf("Run() error = %v, want ffmpeg version error", err)
		}
		if want := []runnerCall{{name: "/fake/ffmpeg", args: []string{"-version"}}}; !reflect.DeepEqual(runner.calls, want) {
			t.Fatalf("runner calls = %#v, want %#v", runner.calls, want)
		}
	})
}

func mustWrite(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testLogger(out *bytes.Buffer) logx.Logger {
	return testLoggerWithErr(out, out)
}

func testLoggerWithErr(out, err *bytes.Buffer) logx.Logger {
	return logx.Logger{
		Out: out,
		Err: err,
	}
}
