package compress

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const DefaultFFOptions = "-c:v libx264 -crf 26 -preset slow -c:a aac -b:a 192k"

type Options struct {
	Directory string
	Execute   bool
	Yes       bool
	FFArgs    []string
}

func ParseOptions(args []string) (Options, error) {
	var (
		directory string
		execute   bool
		yes       bool
		ffOption  string
		help      bool
	)

	fs := flag.NewFlagSet("compress-vedio", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&directory, "dir", "", "directory to scan")
	fs.StringVar(&directory, "d", "", "directory to scan")
	fs.BoolVar(&execute, "execute", false, "compress files")
	fs.BoolVar(&execute, "x", false, "compress files")
	fs.BoolVar(&yes, "yes", false, "confirm compression")
	fs.BoolVar(&yes, "y", false, "confirm compression")
	fs.StringVar(&ffOption, "ff-option", "", "ffmpeg options")
	fs.StringVar(&ffOption, "f", "", "ffmpeg options")
	fs.BoolVar(&help, "help", false, "show help")
	fs.BoolVar(&help, "h", false, "show help")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	if help {
		return Options{}, flag.ErrHelp
	}
	if len(fs.Args()) > 0 {
		return Options{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	if yes && !execute {
		return Options{}, errors.New("--yes requires --execute")
	}
	if directory == "" {
		return Options{}, errors.New("--dir is required")
	}

	absoluteDirectory, err := filepath.Abs(directory)
	if err != nil {
		return Options{}, fmt.Errorf("resolve directory %q: %w", directory, err)
	}
	info, err := os.Stat(absoluteDirectory)
	if err != nil {
		return Options{}, fmt.Errorf("stat directory %q: %w", absoluteDirectory, err)
	}
	if !info.IsDir() {
		return Options{}, fmt.Errorf("path %q is not a directory", absoluteDirectory)
	}

	if ffOption == "" {
		ffOption = DefaultFFOptions
	}
	ffArgs, err := ParseFFOptions(ffOption)
	if err != nil {
		return Options{}, fmt.Errorf("invalid --ff-option: %w", err)
	}

	return Options{
		Directory: absoluteDirectory,
		Execute:   execute,
		Yes:       yes,
		FFArgs:    ffArgs,
	}, nil
}

func Usage() string {
	return `Usage: compress-vedio --dir/-d <directory> [--execute/-x] [--yes/-y] [--ff-option/-f "<ffmpeg options>"]

Options:
  --dir, -d       directory to scan
  --execute, -x   compress files
  --yes, -y       confirm compression
  --ff-option, -f ffmpeg options
  --help, -h      show help
`
}
