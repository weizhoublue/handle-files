package checkcopy

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type Options struct {
	Source      string
	Destination string
	Copy        bool
	Types       []string
}

type stringListFlag []string

func (values *stringListFlag) String() string {
	return strings.Join(*values, ",")
}

func (values *stringListFlag) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func ParseOptions(args []string) (Options, error) {
	var (
		source      string
		destination string
		copyFiles   bool
		types       stringListFlag
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
	flags.Var(&types, "type", "file extension to include (repeatable)")
	flags.Var(&types, "t", "file extension to include (repeatable)")
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

	return normalizeOptions(Options{
		Source:      source,
		Destination: destination,
		Copy:        copyFiles,
		Types:       []string(types),
	})
}

func normalizeTypes(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		extension := strings.TrimPrefix(strings.TrimSpace(value), ".")
		if extension == "" ||
			strings.Contains(extension, ".") ||
			strings.ContainsAny(extension, `/\`) ||
			strings.IndexFunc(extension, unicode.IsSpace) >= 0 {
			return nil, fmt.Errorf(
				"invalid --type value %q: want one extension without dots, whitespace, or path separators",
				value,
			)
		}

		extension = strings.ToLower(extension)
		if _, ok := seen[extension]; ok {
			continue
		}
		seen[extension] = struct{}{}
		normalized = append(normalized, extension)
	}
	return normalized, nil
}

func normalizeOptions(opts Options) (Options, error) {
	normalizedTypes, err := normalizeTypes(opts.Types)
	if err != nil {
		return Options{}, err
	}
	resolvedSource, err := resolveDirectory("source", opts.Source)
	if err != nil {
		return Options{}, err
	}
	resolvedDestination, err := resolveDirectory("destination", opts.Destination)
	if err != nil {
		return Options{}, err
	}
	if resolvedSource == resolvedDestination {
		return Options{}, errors.New("source and destination directories must be different")
	}
	return Options{
		Source:      resolvedSource,
		Destination: resolvedDestination,
		Copy:        opts.Copy,
		Types:       normalizedTypes,
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
	return `Usage: check-copy --source/-s <directory> --destination/-d <directory> [--type/-t <extension>]... [--copy/-c]

Options:
  --source, -s       source directory
  --destination, -d  destination directory
  --type, -t         file extension to include, repeatable; default: all types
  --copy, -c         copy missing and smaller destination files; each failed copy gets at most 5 total attempts, including the first, with a 1-second interval; returns a nonzero exit status if any file still fails
  --help, -h         show help

例子
	# 预览要拷贝哪些文件，但不会实施拷贝
	check-copy -s /Volumes/red/1 -d /Volumes/black/1

	# 只预览 JPG 文件；jpg、.jpg 和 JPG 等价
	check-copy -s /Volumes/red/1 -d /Volumes/black/1 -t jpg

	# 只拷贝 JPG 和 MP4 文件
	check-copy -s /Volumes/red/1 -d /Volumes/black/1 -t jpg -t mp4 -c

	# 不指定 -t 时处理所有文件类型
	check-copy -s /Volumes/red/1 -d /Volumes/black/1 -c
`
}
