package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/weizhoublue/handle-files/internal/checkcopy"
	"github.com/weizhoublue/handle-files/internal/logx"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := logx.Logger{Out: os.Stdout, Err: os.Stderr}
	options, err := checkcopy.ParseOptions(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(os.Stdout, checkcopy.Usage())
		return 0
	}
	if err != nil {
		logger.Error("validation_failed", logx.Field{Key: "error", Value: err.Error()})
		return 1
	}
	if err := checkcopy.Run(context.Background(), options, logger); err != nil {
		logger.Error("run_failed", logx.Field{Key: "error", Value: err.Error()})
		return exitCode(err)
	}
	return 0
}

func exitCode(err error) int {
	var interrupted *checkcopy.InterruptedError
	if errors.As(err, &interrupted) {
		return interrupted.ExitCode()
	}
	return 1
}
