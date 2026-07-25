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
	Source      string
	Destination string
	Remove      bool
	Execute     bool
	FFArgs      []string
}

func ParseOptions(args []string) (Options, error) {
	var (
		source      string
		destination string
		remove      string = "true"
		execute     bool
		ffOption    string
		help        bool
	)

	fs := flag.NewFlagSet("compress-vedio", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&source, "source", "", "source directory")
	fs.StringVar(&source, "s", "", "source directory")
	fs.StringVar(&destination, "dest", "", "destination directory")
	fs.StringVar(&destination, "d", "", "destination directory")
	fs.StringVar(&remove, "remove", remove, "remove source after successful compression")
	fs.StringVar(&remove, "r", remove, "remove source after successful compression")
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
	if source == "" {
		return Options{}, errors.New("--source is required")
	}
	if remove != "true" && remove != "false" {
		return Options{}, fmt.Errorf("invalid --remove value %q: want true or false", remove)
	}

	absoluteSource, err := filepath.Abs(source)
	if err != nil {
		return Options{}, fmt.Errorf("resolve source directory %q: %w", source, err)
	}
	sourceInfo, err := os.Stat(absoluteSource)
	if err != nil {
		return Options{}, fmt.Errorf("stat source directory %q: %w", absoluteSource, err)
	}
	if !sourceInfo.IsDir() {
		return Options{}, fmt.Errorf("path %q is not a directory", absoluteSource)
	}

	var absoluteDestination string
	if destination != "" {
		absoluteDestination, err = filepath.Abs(destination)
		if err != nil {
			return Options{}, fmt.Errorf("resolve destination directory %q: %w", destination, err)
		}
		destinationInfo, err := os.Stat(absoluteDestination)
		if err != nil {
			return Options{}, fmt.Errorf("stat destination directory %q: %w", absoluteDestination, err)
		}
		if !destinationInfo.IsDir() {
			return Options{}, fmt.Errorf("path %q is not a directory", absoluteDestination)
		}
	}

	if !ffOptionSet {
		ffOption = DefaultFFOptions
	}
	ffArgs, err := ParseFFOptions(ffOption)
	if err != nil {
		return Options{}, fmt.Errorf("invalid --ff-option: %w", err)
	}

	return Options{
		Source:      absoluteSource,
		Destination: absoluteDestination,
		Remove:      remove == "true",
		Execute:     execute,
		FFArgs:      ffArgs,
	}, nil
}

func Usage() string {
	return `Usage: compress-vedio --source/-s <directory> [--dest/-d <directory>] [--remove/-r <true|false>] [--execute/-x] [--ff-option/-f "<ffmpeg options>"]

Options:
  --source, -s      source directory
  --dest, -d        destination directory
  --remove, -r      remove source after successful compression, 默认值 true
  --execute, -x     compress files
  --ff-option, -f   ffmpeg options, 默认值 "-c:v libx264 -crf 26 -preset slow -c:a aac -b:a 192k"
  --help, -h        show help

例子
	# 仅仅预览（要压缩哪些文件），不会真的执行
	compress-vedio -s /Volumes/Data/Videos

	# 实施压缩, 并且压缩后的文件 位于 原目录, 且删除原来的老文件
	compress-vedio -s /Volumes/Data/Videos -d /Volumes/Data/Archive -x 

	# 实施压缩, 压缩后的文件 位于 新目录，且不删除原来的老文件
	compress-vedio -s /Volumes/Data/Videos -d /Volumes/Data/Archive -r false -x 
`
}
