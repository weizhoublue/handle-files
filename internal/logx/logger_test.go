package logx

import (
	"bytes"
	"testing"
)

func TestLoggerInfoUsesSortedFields(t *testing.T) {
	var out, err bytes.Buffer
	logger := Logger{
		Out: &out, Err: &err,
	}

	logger.Info("missing", Field{Key: "path", Value: "1/test1"})

	if got := out.String(); got != "INFO, missing path=1/test1\n" {
		t.Fatalf("Info() = %q", got)
	}
	if err.Len() != 0 {
		t.Fatalf("Info wrote stderr: %q", err.String())
	}
}

func TestLoggerErrorProgressUsesSortedFields(t *testing.T) {
	var out, err bytes.Buffer
	logger := Logger{Out: &out, Err: &err}

	logger.ErrorProgress(
		"copy_failed",
		[]Field{{Key: "path", Value: "1/test2"}, {Key: "error", Value: "write failed"}},
		[]Field{{Key: "total", Value: "2"}, {Key: "failed", Value: "1"}, {Key: "completed", Value: "2"}, {Key: "succeeded", Value: "1"}},
	)

	if got := err.String(); got != "ERROR, copy_failed error=write failed path=1/test2 [ completed=2 failed=1 succeeded=1 total=2 ]\n" {
		t.Fatalf("ErrorProgress() = %q", got)
	}
	if out.Len() != 0 {
		t.Fatalf("ErrorProgress wrote stdout: %q", out.String())
	}
}
