package buildinfo

import (
	"strings"
	"testing"
)

func TestGetFillsDefaults(t *testing.T) {
	// Built without -ldflags, as `go test` is, the values must still be
	// truthful rather than empty.
	i := Get()
	if i.Version == "" {
		t.Error("Version is empty, want a fallback")
	}
	if i.Commit == "" {
		t.Error("Commit is empty, want a fallback")
	}
	if !strings.HasPrefix(i.GoVersion, "go") {
		t.Errorf("GoVersion = %q, want a go... version", i.GoVersion)
	}
}

func TestShortCommit(t *testing.T) {
	for in, want := range map[string]string{
		"06f8653dde012de68f653a9aa268892806801189": "06f8653",
		"unknown": "unknown",
		"":        "",
	} {
		if got := (Info{Commit: in}).ShortCommit(); got != want {
			t.Errorf("Info{Commit: %q}.ShortCommit() = %q, want %q", in, got, want)
		}
	}
}

func TestStringIncludesDirtyMarker(t *testing.T) {
	i := Info{Version: "v1.2.3", Commit: "abcdef0123", GoVersion: "go1.25.3", Dirty: true}
	got := i.String()
	for _, want := range []string{"v1.2.3", "abcdef0", "-dirty", "go1.25.3"} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, want it to contain %q", got, want)
		}
	}

	clean := Info{Version: "v1.2.3", Commit: "abcdef0123", GoVersion: "go1.25.3"}
	if strings.Contains(clean.String(), "dirty") {
		t.Errorf("String() = %q, want no dirty marker on a clean build", clean.String())
	}
}
