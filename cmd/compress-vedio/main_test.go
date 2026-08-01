package main

import (
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/weizhoublue/handle-files/internal/compress"
)

func TestExitCodeMapsCompressionInterruptions(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "SIGINT", err: &compress.InterruptedError{Signal: os.Interrupt}, want: 130},
		{name: "SIGTERM", err: &compress.InterruptedError{Signal: syscall.SIGTERM}, want: 143},
		{name: "other", err: errors.New("failed"), want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := exitCode(test.err); got != test.want {
				t.Fatalf("exitCode(%v) = %d, want %d", test.err, got, test.want)
			}
		})
	}
}
