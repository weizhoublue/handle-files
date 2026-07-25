package compress

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
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

func TestParseFFOptionsHandlesQuotedAndEscapedEdgeCombinations(t *testing.T) {
	got, err := ParseFFOptions(`prefix"two words"'!' path='C:\clips\input file.mp4'`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"prefixtwo words!", "path=C:clipsinput file.mp4"}
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

func TestParseOptionsAcceptsSourceDestinationAndRemove(t *testing.T) {
	source, destination := t.TempDir(), t.TempDir()
	got, err := ParseOptions([]string{
		"-s", source, "-d", destination, "--remove", "false", "-x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != source || got.Destination != destination ||
		got.Remove || !got.Execute {
		t.Fatalf("ParseOptions() = %#v", got)
	}
}

func TestParseOptionsDefaultsRemoveToTrue(t *testing.T) {
	source := t.TempDir()
	got, err := ParseOptions([]string{"--source", source})
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != source || !got.Remove {
		t.Fatalf("ParseOptions() = %#v", got)
	}
}

func TestParseOptionsAcceptsRemoveEqualsForm(t *testing.T) {
	source := t.TempDir()
	got, err := ParseOptions([]string{"--source", source, "--remove=false"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Remove {
		t.Fatalf("ParseOptions() = %#v", got)
	}
}

func TestParseOptionsAcceptsRemoveShortAlias(t *testing.T) {
	source := t.TempDir()
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"--source", source, "-r", "false"}, false},
		{[]string{"--source", source, "-r=false"}, false},
		{[]string{"--source", source, "-r=true"}, true},
	}

	for _, tt := range tests {
		got, err := ParseOptions(tt.args)
		if err != nil {
			t.Fatal(err)
		}
		if got.Remove != tt.want {
			t.Fatalf("ParseOptions(%#v).Remove = %t, want %t", tt.args, got.Remove, tt.want)
		}
	}
	if !strings.Contains(Usage(), "--remove/-r") {
		t.Fatalf("Usage() = %q, want --remove/-r", Usage())
	}
}

func TestParseOptionsRejectsUndefinedDirFlag(t *testing.T) {
	_, err := ParseOptions([]string{"--dir", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("ParseOptions(--dir) error = %v", err)
	}
}

func TestParseOptionsRejectsOldDashDAliasAsSourceDirectory(t *testing.T) {
	_, err := ParseOptions([]string{"-d", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "--source is required") {
		t.Fatalf("ParseOptions(-d) error = %v", err)
	}
}

func TestParseOptionsRejectsInvalidDestinationAndRemove(t *testing.T) {
	source := t.TempDir()
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing destination",
			args: []string{"--source", source, "--dest", filepath.Join(source, "missing")},
			want: filepath.Join(source, "missing"),
		},
		{
			name: "invalid remove",
			args: []string{"--source", source, "--remove", "invalid"},
			want: "--remove",
		},
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
		{name: "empty source", args: []string{"--source", ""}, want: "--source is required"},
		{name: "missing source", args: []string{"--source", t.TempDir() + "/missing"}, want: "source directory"},
		{name: "file supplied as source", args: []string{"--source", file}, want: "not a directory"},
		{name: "positional argument", args: []string{"--source", t.TempDir(), "extra"}, want: "unexpected positional arguments"},
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
	source := t.TempDir()
	got, err := ParseOptions([]string{
		"-s", source,
		"-x",
		"-f", `-c:v libx264 -metadata title="A clip"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := Options{
		Source:  source,
		Remove:  true,
		Execute: true,
		FFArgs:  []string{"-c:v", "libx264", "-metadata", "title=A clip"},
	}
	if got.Source != want.Source || got.Remove != want.Remove || got.Execute != want.Execute ||
		!slices.Equal(got.FFArgs, want.FFArgs) {
		t.Fatalf("ParseOptions() = %#v, want %#v", got, want)
	}
}

func TestParseOptionsAcceptsLongFlagsAndQuotedEscapedFFOptions(t *testing.T) {
	source := t.TempDir()
	got, err := ParseOptions([]string{
		"--source", source,
		"--execute",
		"--ff-option", `-metadata title="A clip" -vf scale=1280\:720`,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-metadata", "title=A clip", "-vf", "scale=1280:720"}
	if !slices.Equal(got.FFArgs, want) {
		t.Fatalf("ParseOptions() FFArgs = %#v, want %#v", got.FFArgs, want)
	}
}

func TestParseOptionsRejectsExplicitEmptyFFOption(t *testing.T) {
	_, err := ParseOptions([]string{"--source", t.TempDir(), "--ff-option", ""})
	if err == nil || !strings.Contains(err.Error(), "invalid --ff-option") ||
		!strings.Contains(err.Error(), "empty ffmpeg option") {
		t.Fatalf("ParseOptions() error = %v, want explicit empty ff-option rejection", err)
	}
}

func TestParseOptionsUsesDefaultFFOptions(t *testing.T) {
	got, err := ParseOptions([]string{"--source", t.TempDir()})
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
