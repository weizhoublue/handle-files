package checkcopy

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Options struct {
	Source      string
	Destination string
	Copy        bool
}

func ParseOptions(args []string) (Options, error) {
	var (
		source      string
		destination string
		copyFiles   bool
		help        bool
	)

	flags := flag.NewFlagSet("check-copy", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&source, "source", "", "source directory")
	flags.StringVar(&source, "s", "", "source directory")
	flags.StringVar(&destination, "destination", "", "destination directory")
	flags.StringVar(&destination, "d", "", "destination directory")
	flags.BoolVar(&copyFiles, "copy", false, "copy missing and smaller destination files")
	flags.BoolVar(&copyFiles, "c", false, "copy missing and smaller destination files")
	flags.BoolVar(&help, "help", false, "show help")
	flags.BoolVar(&help, "h", false, "show help")

	if err := flags.Parse(args); err != nil {
		return Options{}, err
	}
	if help {
		return Options{}, flag.ErrHelp
	}
	if len(flags.Args()) > 0 {
		return Options{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}

	resolvedSource, err := resolveDirectory("source", source)
	if err != nil {
		return Options{}, err
	}
	resolvedDestination, err := resolveDirectory("destination", destination)
	if err != nil {
		return Options{}, err
	}
	if resolvedSource == resolvedDestination {
		return Options{}, errors.New("source and destination directories must be different")
	}

	return Options{
		Source:      resolvedSource,
		Destination: resolvedDestination,
		Copy:        copyFiles,
	}, nil
}

func resolveDirectory(name, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("--%s is required", name)
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s directory %q: %w", name, path, err)
	}
	resolvedPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return "", fmt.Errorf("resolve %s directory %q: %w", name, absolutePath, err)
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("stat %s directory %q: %w", name, resolvedPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s path %q is not a directory", name, resolvedPath)
	}
	return resolvedPath, nil
}

func Usage() string {
	return `Usage: check-copy --source/-s <directory> --destination/-d <directory> [--copy/-c]

Options:
  --source, -s       source directory
  --destination, -d  destination directory
  --copy, -c         copy missing and smaller destination files
  --help, -h         show help
`
}
