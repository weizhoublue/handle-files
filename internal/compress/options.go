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
	FFArgs    []string
}

func ParseOptions(args []string) (Options, error) {
	var (
		directory string
		execute   bool
		ffOption  string
		help      bool
	)

	fs := flag.NewFlagSet("compress-vedio", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&directory, "dir", "", "directory to scan")
	fs.StringVar(&directory, "d", "", "directory to scan")
	fs.BoolVar(&execute, "execute", false, "compress files")
	fs.BoolVar(&execute, "x", false, "compress files")
	fs.StringVar(&ffOption, "ff-option", "", "ffmpeg options")
	fs.StringVar(&ffOption, "f", "", "ffmpeg options")
	fs.BoolVar(&help, "help", false, "show help")
	fs.BoolVar(&help, "h", false, "show help")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	ffOptionSet := false
	fs.Visit(func(flag *flag.Flag) {
		if flag.Name == "ff-option" || flag.Name == "f" {
			ffOptionSet = true
		}
	})
	if help {
		return Options{}, flag.ErrHelp
	}
	if len(fs.Args()) > 0 {
		return Options{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
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

	if !ffOptionSet {
		ffOption = DefaultFFOptions
	}
	ffArgs, err := ParseFFOptions(ffOption)
	if err != nil {
		return Options{}, fmt.Errorf("invalid --ff-option: %w", err)
	}

	return Options{
		Directory: absoluteDirectory,
		Execute:   execute,
		FFArgs:    ffArgs,
	}, nil
}

func Usage() string {
	return `Usage: compress-vedio --dir/-d <directory> [--execute/-x] [--ff-option/-f "<ffmpeg options>"]

Options:
  --dir, -d         directory to scan
  --execute, -x     compress files
  --ff-option, -f   ffmpeg options, 默认值 "-c:v libx264 -crf 26 -preset slow -c:a aac -b:a 192k"
  --help, -h        show help

例子
	# 仅仅预览（要压缩哪些文件），不会真的执行
	compress-vedio -d /Volumes/Data/Videos

	# 实施压缩
	compress-vedio -d /Volumes/Data/Videos -x 

`
}
