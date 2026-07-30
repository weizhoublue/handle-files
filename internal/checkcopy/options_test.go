package checkcopy

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseOptionsNormalizesRepeatedTypes(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()

	got, err := ParseOptions([]string{
		"--source", source,
		"--destination", destination,
		"--type", " JPG ",
		"-t", ".mp4",
		"--type", "jpg",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	wantDestination, err := filepath.EvalSymlinks(destination)
	if err != nil {
		t.Fatal(err)
	}
	want := Options{
		Source:      wantSource,
		Destination: wantDestination,
		Types:       []string{"jpg", "mp4"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseOptions() = %#v, want %#v", got, want)
	}
}

func TestParseOptionsDefaultsToAllTypes(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()

	got, err := ParseOptions([]string{
		"--source", source,
		"--destination", destination,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	wantDestination, err := filepath.EvalSymlinks(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Types) != 0 {
		t.Fatalf("ParseOptions() types = %#v, want no filter", got.Types)
	}
	if got.Source != wantSource || got.Destination != wantDestination {
		t.Fatalf("ParseOptions() = %#v, want resolved directories %#v %#v", got, wantSource, wantDestination)
	}
}

func TestParseOptionsRejectsInvalidTypes(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	for _, value := range []string{"", ".", "tar.gz", "jpg/png", `jpg\png`, "jp g"} {
		t.Run(value, func(t *testing.T) {
			_, err := ParseOptions([]string{
				"--source", source,
				"--destination", destination,
				"--type", value,
			})
			if err == nil || !strings.Contains(err.Error(), "invalid --type value") {
				t.Fatalf("ParseOptions(--type %q) error = %v", value, err)
			}
		})
	}
}
