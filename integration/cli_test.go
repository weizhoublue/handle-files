package integration

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompressVedioExecuteUsesFakeFFmpeg(t *testing.T) {
	binary := buildBinary(t, "./cmd/compress-vedio")
	root := t.TempDir()
	input := filepath.Join(root, "clip.mp4")
	writeFile(t, input, "input")
	fakeBin := writeFakeFFmpeg(t)

	output, err := runCommand(binary, []string{"--dir", root, "--execute"}, fakeBin, nil)
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(input); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "clip_output.mp4")); err != nil {
		t.Fatalf("compressed output missing: %v", err)
	}
	assertReadableOutput(t, output, "INFO, compressed ")
	if !strings.Contains(string(output), "completed=1") ||
		!strings.Contains(string(output), "total=1") {
		t.Fatalf("missing progress: %s", output)
	}
}

func TestCompressVedioExecuteCompressesAllFilesWithoutStdin(t *testing.T) {
	binary := buildBinary(t, "./cmd/compress-vedio")
	root := t.TempDir()
	firstInput := filepath.Join(root, "first.mp4")
	secondInput := filepath.Join(root, "second.mp4")
	writeFile(t, firstInput, "first")
	writeFile(t, secondInput, "second")

	output, err := runCommand(binary, []string{"--dir", root, "--execute"}, writeFakeFFmpeg(t), nil)
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, output)
	}
	for _, input := range []string{firstInput, secondInput} {
		if _, err := os.Stat(input); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("source still exists: %s: %v", input, err)
		}
	}
	for _, outputPath := range []string{
		filepath.Join(root, "first_output.mp4"),
		filepath.Join(root, "second_output.mp4"),
	} {
		if _, err := os.Stat(outputPath); err != nil {
			t.Fatalf("compressed output missing: %s: %v", outputPath, err)
		}
	}
}

func TestCompressVedioHelpPrintsUsage(t *testing.T) {
	output, err := runCommand(buildBinary(t, "./cmd/compress-vedio"), []string{"--help"}, "", nil)
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "Usage: compress-vedio") {
		t.Fatalf("missing usage: %s", output)
	}
}

func TestCheckCopyReportOnlyLeavesDestinationUnchanged(t *testing.T) {
	binary := buildBinary(t, "./cmd/check-copy")
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	mkdirAll(t, source)
	mkdirAll(t, destination)
	writeFile(t, filepath.Join(source, "clip.mp4"), "input")

	output, err := runCommand(binary, []string{"--source", source, "--destination", destination}, "", nil)
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(destination, "clip.mp4")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("report-only command wrote destination: %v", err)
	}
	assertReadableOutput(t, output, "INFO, copy_skipped ")
}

func TestCheckCopyCopyCreatesMissingTarget(t *testing.T) {
	binary := buildBinary(t, "./cmd/check-copy")
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	mkdirAll(t, source)
	mkdirAll(t, destination)
	writeFile(t, filepath.Join(source, "nested", "clip.mp4"), "input")

	output, err := runCommand(binary, []string{"--source", source, "--destination", destination, "--copy"}, "", nil)
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, output)
	}
	copied, err := os.ReadFile(filepath.Join(destination, "nested", "clip.mp4"))
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(copied) != "input" {
		t.Fatalf("copied content = %q, want %q", copied, "input")
	}
	assertReadableOutput(t, output, "INFO, copied ")
}

func TestCheckCopyHelpPrintsUsage(t *testing.T) {
	output, err := runCommand(buildBinary(t, "./cmd/check-copy"), []string{"--help"}, "", nil)
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "Usage: check-copy") {
		t.Fatalf("missing usage: %s", output)
	}
}

func TestInvalidFlagsFailWithStructuredValidationError(t *testing.T) {
	for _, command := range []string{"./cmd/compress-vedio", "./cmd/check-copy"} {
		t.Run(command, func(t *testing.T) {
			output, err := runCommand(buildBinary(t, command), []string{"--unknown"}, "", nil)
			if err == nil {
				t.Fatalf("invalid flag succeeded: %s", output)
			}
			assertReadableOutput(t, output, "ERROR, validation_failed ")
		})
	}
}

func TestCompressVedioMissingFFmpegFailsWithStructuredError(t *testing.T) {
	root := t.TempDir()
	output, err := runCommand(buildBinary(t, "./cmd/compress-vedio"), []string{"--dir", root}, t.TempDir(), nil)
	if err == nil {
		t.Fatalf("missing ffmpeg succeeded: %s", output)
	}
	assertReadableOutput(t, output, "ERROR, run_failed ")
}

func assertReadableOutput(t *testing.T, output []byte, expected string) {
	t.Helper()
	text := string(output)
	if !strings.Contains(text, expected) {
		t.Fatalf("missing %q: %s", expected, output)
	}
	for _, forbidden := range []string{"time=", "level=", "event="} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("output contains legacy field %q: %s", forbidden, output)
		}
	}
}

func buildBinary(t *testing.T, packagePath string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), filepath.Base(packagePath))
	command := exec.Command("go", "build", "-o", binary, packagePath)
	repositoryRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	command.Dir = repositoryRoot
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", packagePath, err, output)
	}
	return binary
}

func writeFakeFFmpeg(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, "ffmpeg")
	script := "#!/bin/sh\nif [ \"$1\" = \"-version\" ]; then\n  exit 0\nfi\nfor output do :; done\n: > \"$output\"\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	return directory
}

func runCommand(binary string, args []string, fakeBin string, input io.Reader) ([]byte, error) {
	command := exec.Command(binary, args...)
	if fakeBin != "" {
		command.Env = append(os.Environ(), "PATH="+fakeBin)
	}
	if input != nil {
		command.Stdin = input
	}
	return command.CombinedOutput()
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("make directory %q: %v", path, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %q: %v", path, err)
	}
}
