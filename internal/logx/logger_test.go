package logx

import (
	"bytes"
	"testing"
	"time"
)

func TestLoggerInfoUsesSortedFieldsAndUTC(t *testing.T) {
	var out, err bytes.Buffer
	logger := Logger{
		Out: &out, Err: &err,
		Now: func() time.Time {
			return time.Date(2026, 7, 25, 6, 12, 4, 0, time.FixedZone("PDT", -7*3600))
		},
	}

	logger.Info("progress", Field{Key: "total", Value: "10"}, Field{Key: "completed", Value: "3"})

	want := "time=2026-07-25T13:12:04Z level=INFO event=progress completed=3 total=10\n"
	if got := out.String(); got != want {
		t.Fatalf("Info() = %q, want %q", got, want)
	}
	if err.Len() != 0 {
		t.Fatalf("Info wrote stderr: %q", err.String())
	}
}
