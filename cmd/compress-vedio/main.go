package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/weizhoublue/handle-files/internal/compress"
	"github.com/weizhoublue/handle-files/internal/logx"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := logx.Logger{Out: os.Stdout, Err: os.Stderr, Now: time.Now}
	options, err := compress.ParseOptions(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(os.Stdout, compress.Usage())
		return 0
	}
	if err != nil {
		logger.Error("validation_failed", logx.Field{Key: "error", Value: err.Error()})
		return 1
	}
	if _, err := compress.Run(context.Background(), options, compress.NewCommandRunner(), os.Stdin, logger); err != nil {
		logger.Error("run_failed", logx.Field{Key: "error", Value: err.Error()})
		return 1
	}
	return 0
}
