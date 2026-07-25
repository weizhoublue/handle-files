package compress

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/weizhoublue/handle-files/internal/logx"
)

type runnerCall struct {
	name string
	args []string
}

type fakeRunner struct {
	path        string
	lookPathErr error
	run         func(name string, args ...string) error
	calls       []runnerCall
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
		Options{Directory: root, Execute: true, Yes: true, FFArgs: []string{"-c:v", "libx264"}},
		runner,
		strings.NewReader(""),
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
	if got := strings.Count(logs.String(), "event=progress"); got != 1 {
		t.Fatalf("progress records = %d, want 1:\n%s", got, logs.String())
	}
	if !strings.Contains(logs.String(), "completed=1 failed=0 skipped=0 succeeded=1 total=1") {
		t.Fatalf("final progress missing counters:\n%s", logs.String())
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
		Options{Directory: root, Execute: true, Yes: true, FFArgs: []string{"-c:v", "libx264"}},
		runner,
		strings.NewReader(""),
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
	if got := strings.Count(logs.String(), "event=progress"); got != 1 {
		t.Fatalf("progress records = %d, want 1:\n%s", got, logs.String())
	}
	if !strings.Contains(logs.String(), "completed=1 failed=1 skipped=0 succeeded=0 total=1") {
		t.Fatalf("final progress missing counters:\n%s", logs.String())
	}
}

func TestRunRetainsSourceWhenCommandProducesNoOutput(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "clip.mp4")
	mustWrite(t, source, "video")
	runner := &fakeRunner{path: "/fake/ffmpeg"}

	summary, err := Run(
		context.Background(),
		Options{Directory: root, Execute: true, Yes: true, FFArgs: []string{"-c:v", "libx264"}},
		runner,
		strings.NewReader(""),
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
		Options{Directory: root, Execute: true, Yes: true, FFArgs: []string{"-c:v", "libx264"}},
		runner,
		strings.NewReader(""),
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
	source := filepath.Join(root, "clip.mp4")
	mustWrite(t, source, "video")
	runner := &fakeRunner{path: "/fake/ffmpeg"}

	summary, err := Run(
		context.Background(),
		Options{Directory: root, FFArgs: []string{"-c:v", "libx264"}},
		runner,
		strings.NewReader(""),
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
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source stat error = %v", err)
	}
}

func TestRunRequiresConfirmationForEachLiveFile(t *testing.T) {
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
		Options{Directory: root, Execute: true, FFArgs: []string{"-c:v", "libx264"}},
		runner,
		strings.NewReader("n\ny\n"),
		testLogger(&logs),
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Summary{Total: 2, Succeeded: 1, Skipped: 1}); summary != want {
		t.Fatalf("Run() summary = %#v, want %#v", summary, want)
	}
	if got := len(runner.calls); got != 2 {
		t.Fatalf("runner calls = %d, want 2", got)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatalf("skipped source stat error = %v", err)
	}
	if _, err := os.Stat(second); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("confirmed source stat error = %v, want not exist", err)
	}
	if got := strings.Count(logs.String(), "event=progress"); got != 2 {
		t.Fatalf("progress records = %d, want 2:\n%s", got, logs.String())
	}
}

func TestRunReturnsDependencyErrors(t *testing.T) {
	t.Run("lookup", func(t *testing.T) {
		_, err := Run(
			context.Background(),
			Options{Directory: t.TempDir()},
			&fakeRunner{lookPathErr: errors.New("not found")},
			strings.NewReader(""),
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
			Options{Directory: t.TempDir()},
			runner,
			strings.NewReader(""),
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
	return logx.Logger{
		Out: out,
		Err: out,
		Now: func() time.Time {
			return time.Date(2026, 7, 25, 6, 12, 4, 0, time.UTC)
		},
	}
}
