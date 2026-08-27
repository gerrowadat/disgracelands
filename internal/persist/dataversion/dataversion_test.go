// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package dataversion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ref is a stand-in for "whatever release this build is" in the tests
// below. It is deliberately not Current(): under `go test` there is no
// -ldflags stamp, so Current() has no version at all (see
// TestCurrentHasNoVersionUnderGoTest), and every comparison here needs two
// concrete versions to compare.
var ref = Version{Major: 3, Minor: 4, Patch: 5}

func TestParseRoundTripsString(t *testing.T) {
	v := Version{Major: 1, Minor: 2, Patch: 3}
	got, err := Parse(v.String())
	if err != nil {
		t.Fatalf("Parse(%q): %v", v.String(), err)
	}
	if got != v {
		t.Errorf("Parse(%q) = %+v, want %+v", v.String(), got, v)
	}
}

func TestParseRejectsMalformedInput(t *testing.T) {
	for _, s := range []string{"", "1", "1.2", "1.2.3.4", "a.b.c", "1.2.-3", "1..3"} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) succeeded, want an error", s)
		}
	}
}

// TestParseBuildReducesDescribeOutputToItsRelease covers what the Makefile
// actually stamps in: `git describe --tags --always --dirty`, whose output
// shape depends on where HEAD sits relative to the nearest tag and whether
// the tree is clean. All of these name one release.
func TestParseBuildReducesDescribeOutputToItsRelease(t *testing.T) {
	want := Version{Major: 0, Minor: 1, Patch: 2}
	for _, s := range []string{
		"v0.1.2",                  // built from the tag itself
		"0.1.2",                   // an unprefixed tag; the repo has one
		"v0.1.2-4-gabc1234",       // four commits past it
		"v0.1.2-4-gabc1234-dirty", // ... with uncommitted changes
		"v0.1.2-dirty",            // on the tag, with uncommitted changes
		"v0.1.2+somebuildmeta",    // semver build metadata, if it ever appears
	} {
		got, ok := parseBuild(s)
		if !ok {
			t.Errorf("parseBuild(%q) found no version, want %s", s, want)
			continue
		}
		if got != want {
			t.Errorf("parseBuild(%q) = %s, want %s", s, got, want)
		}
	}
}

func TestParseBuildRejectsWhatIsNotARelease(t *testing.T) {
	// "devel" is buildinfo's own fallback with no -ldflags; a bare hash is
	// what `git describe --always` produces in a repo with no tags at all.
	for _, s := range []string{"devel", "", "abc1234", "unknown", "v1.2"} {
		if got, ok := parseBuild(s); ok {
			t.Errorf("parseBuild(%q) = %s, want no version", s, got)
		}
	}
}

// TestCurrentHasNoVersionUnderGoTest pins the hole the design accepts
// rather than leaving it to be discovered: a test binary is linked without
// the Makefile's -ldflags, so it has no release to name and stamps and
// enforces nothing. If this ever starts failing, something began injecting
// a version into test builds, and CheckBuild's unreleased path — and every
// test below that relies on it — needs revisiting.
func TestCurrentHasNoVersionUnderGoTest(t *testing.T) {
	if v, ok := Current(); ok {
		t.Errorf("Current() = %s under `go test`, want no version", v)
	}
}

func TestCheckNoStampFileIsSilentlyFine(t *testing.T) {
	warning, err := Check(t.TempDir(), ref)
	if err != nil || warning != "" {
		t.Errorf("Check(no stamp) = (%q, %v), want (\"\", nil)", warning, err)
	}
}

func TestCheckSameVersionIsSilentlyFine(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, ref); err != nil {
		t.Fatalf("Write: %v", err)
	}
	warning, err := Check(dir, ref)
	if err != nil || warning != "" {
		t.Errorf("Check(same version) = (%q, %v), want (\"\", nil)", warning, err)
	}
}

// TestCheckDifferingPatchIsSilentlyFine, both directions: a patch bump is
// defined as a change no reader can observe, so it is not a finding.
func TestCheckDifferingPatchIsSilentlyFine(t *testing.T) {
	for _, patch := range []int{ref.Patch - 1, ref.Patch + 1} {
		dir := t.TempDir()
		if err := Write(dir, Version{Major: ref.Major, Minor: ref.Minor, Patch: patch}); err != nil {
			t.Fatalf("Write: %v", err)
		}
		warning, err := Check(dir, ref)
		if err != nil || warning != "" {
			t.Errorf("Check(patch %d against %s) = (%q, %v), want (\"\", nil)", patch, ref, warning, err)
		}
	}
}

// TestCheckDifferingMinorWarnsEitherWay is the rule that changed: it is
// the *difference* that warns, not the direction. Data written by an older
// minor may be missing what this build expects to find; data written by a
// newer one may hold what this build will not read. Both are worth saying
// and neither is worth refusing over.
func TestCheckDifferingMinorWarnsEitherWay(t *testing.T) {
	for _, minor := range []int{ref.Minor - 1, ref.Minor + 1} {
		dir := t.TempDir()
		other := Version{Major: ref.Major, Minor: minor, Patch: 0}
		if err := Write(dir, other); err != nil {
			t.Fatalf("Write: %v", err)
		}
		warning, err := Check(dir, ref)
		if err != nil {
			t.Fatalf("Check(minor %d) returned an error, want a warning and nil: %v", minor, err)
		}
		if warning == "" {
			t.Errorf("Check(minor %d) returned no warning, want one describing the mismatch", minor)
			continue
		}
		if !strings.Contains(warning, other.String()) || !strings.Contains(warning, ref.String()) {
			t.Errorf("Check(minor %d) warning %q does not name both versions", minor, warning)
		}
	}
}

// TestCheckDifferingMajorRefusesEitherWay is the other half of the change.
// Refusing data from a *newer* major was always the rule; refusing data
// from an older one is new, and is what "only loads its exact major" means
// — a major bump says an other-versioned reader gets the files wrong, and
// getting them wrong is not a direction.
func TestCheckDifferingMajorRefusesEitherWay(t *testing.T) {
	for _, major := range []int{ref.Major - 1, ref.Major + 1} {
		dir := t.TempDir()
		other := Version{Major: major, Minor: 0, Patch: 0}
		if err := Write(dir, other); err != nil {
			t.Fatalf("Write: %v", err)
		}
		warning, err := Check(dir, ref)
		if err == nil {
			t.Errorf("Check(major %d) returned no error, want a refusal to boot", major)
			continue
		}
		if warning != "" {
			t.Errorf("Check(major %d) also returned a warning %q, want none — it should refuse instead", major, warning)
		}
		if !strings.Contains(err.Error(), other.String()) || !strings.Contains(err.Error(), ref.String()) {
			t.Errorf("Check(major %d) error %q does not name both versions", major, err.Error())
		}
	}
}

func TestCheckMalformedVersionIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("schema: dl/dataversion@1\nformat: yaml\nversion: garbage\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	if _, err := Check(dir, ref); err == nil {
		t.Error("Check(malformed version) succeeded, want an error")
	}
}

func TestCheckUnknownFieldIsAnError(t *testing.T) {
	// Strict decoding, the same posture data-format.md §10.2 takes for
	// every other yaml document in this tree.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("schema: dl/dataversion@1\nformat: yaml\nversion: 1.0.0\nbogus: true\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	if _, err := Check(dir, ref); err == nil {
		t.Error("Check(unknown field) succeeded, want an error")
	}
}

func TestReadReportsWhetherThereIsAStampAtAll(t *testing.T) {
	dir := t.TempDir()
	if _, ok, err := Read(dir); ok || err != nil {
		t.Errorf("Read(unstamped) = (_, %v, %v), want (_, false, nil)", ok, err)
	}
	if err := Write(dir, ref); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, ok, err := Read(dir)
	if err != nil || !ok {
		t.Fatalf("Read(stamped) = (_, %v, %v), want (_, true, nil)", ok, err)
	}
	if got != ref {
		t.Errorf("Read(stamped) = %s, want %s", got, ref)
	}
}

// TestCheckBuildUnreleasedEnforcesNothing: under `go test` there is no
// release version, so even a stamp this build could never satisfy — a
// different major — must not turn into a refusal. It says so instead.
func TestCheckBuildUnreleasedEnforcesNothing(t *testing.T) {
	dir := t.TempDir()
	other := Version{Major: 99, Minor: 0, Patch: 0}
	if err := Write(dir, other); err != nil {
		t.Fatalf("Write: %v", err)
	}
	warning, err := CheckBuild(dir)
	if err != nil {
		t.Fatalf("CheckBuild(unreleased build) returned an error, want a warning: %v", err)
	}
	if !strings.Contains(warning, other.String()) {
		t.Errorf("CheckBuild warning %q does not name the data's version", warning)
	}
}

// ... and says nothing at all when there is no stamp either, which is the
// case every `go test` and `go run` in this tree actually hits.
func TestCheckBuildUnreleasedAndUnstampedIsSilent(t *testing.T) {
	warning, err := CheckBuild(t.TempDir())
	if err != nil || warning != "" {
		t.Errorf("CheckBuild(unstamped) = (%q, %v), want (\"\", nil)", warning, err)
	}
}

func TestWriteIsAtomic(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, ref); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName+".tmp")); !os.IsNotExist(err) {
		t.Errorf("a .tmp file survived Write: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatalf("reading the stamp: %v", err)
	}
	if !strings.Contains(string(b), ref.String()) {
		t.Errorf("stamp %q does not contain %q", b, ref.String())
	}
}
