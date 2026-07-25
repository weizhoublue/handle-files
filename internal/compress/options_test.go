package compress

import (
	"errors"
	"flag"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestParseFFOptionsHonorsQuotesAndEscapes(t *testing.T) {
	got, err := ParseFFOptions(`-metadata title="A clip" -vf scale=1280\:720`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-metadata", "title=A clip", "-vf", "scale=1280:720"}
	if !slices.Equal(got, want) {
		t.Fatalf("ParseFFOptions() = %#v, want %#v", got, want)
	}
}

func TestParseFFOptionsRejectsMalformedValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "unclosed single quote", value: `-metadata 'title`, want: "unclosed single quote"},
		{name: "unclosed double quote", value: `-metadata "title`, want: "unclosed double quote"},
		{name: "dangling escape", value: `-metadata title\`, want: "dangling escape"},
		{name: "empty value", value: `""`, want: "empty ffmpeg option"},
		{name: "only whitespace", value: " \t\n", want: "empty ffmpeg option"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFFOptions(tt.value)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParseFFOptions() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestParseOptionsRejectsInvalidCombinations(t *testing.T) {
	_, err := ParseOptions([]string{"--dir", t.TempDir(), "--yes"})
	if err == nil || !strings.Contains(err.Error(), "--yes requires --execute") {
		t.Fatalf("ParseOptions() error = %v", err)
	}
}

func TestParseOptionsRejectsInvalidDirectoriesAndArguments(t *testing.T) {
	file := t.TempDir() + "/file.mp4"
	if err := os.WriteFile(file, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "empty directory", args: []string{"--dir", ""}, want: "--dir is required"},
		{name: "missing directory", args: []string{"--dir", t.TempDir() + "/missing"}, want: "directory"},
		{name: "file supplied as directory", args: []string{"--dir", file}, want: "not a directory"},
		{name: "positional argument", args: []string{"--dir", t.TempDir(), "extra"}, want: "unexpected positional arguments"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseOptions(tt.args)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.want)) {
				t.Fatalf("ParseOptions() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestParseOptionsAcceptsShortFlagsAndExplicitValues(t *testing.T) {
	dir := t.TempDir()
	got, err := ParseOptions([]string{
		"-d", dir,
		"-x",
		"-y",
		"-f", `-c:v libx264 -metadata title="A clip"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := Options{
		Directory: dir,
		Execute:   true,
		Yes:       true,
		FFArgs:    []string{"-c:v", "libx264", "-metadata", "title=A clip"},
	}
	if got.Directory != want.Directory || got.Execute != want.Execute || got.Yes != want.Yes ||
		!slices.Equal(got.FFArgs, want.FFArgs) {
		t.Fatalf("ParseOptions() = %#v, want %#v", got, want)
	}
}

func TestParseOptionsUsesDefaultFFOptions(t *testing.T) {
	got, err := ParseOptions([]string{"--dir", t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	want, err := ParseFFOptions(DefaultFFOptions)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.FFArgs, want) {
		t.Fatalf("ParseOptions() FFArgs = %#v, want %#v", got.FFArgs, want)
	}
}

func TestParseOptionsHelp(t *testing.T) {
	_, err := ParseOptions([]string{"--help"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("ParseOptions(--help) error = %v, want flag.ErrHelp", err)
	}
	if Usage() == "" {
		t.Fatal("Usage() returned empty string")
	}
}
