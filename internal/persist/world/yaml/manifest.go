// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// ManifestFile is zones.yaml's name within the world directory. §3: the
// world is the one part of this tree that keeps an explicit index, because
// a directory listing cannot distinguish "disabled" from "not here".
const ManifestFile = "zones.yaml"

// ManifestSchema is zones.yaml's schema tag.
const ManifestSchema = "dl/zones@1"

// ManifestEntry is one zone.yaml list entry. A bare integer decodes as
// {Vnum: n, Enabled: true}; the expanded form spells out Enabled and Note.
type ManifestEntry struct {
	Vnum    int32  `yaml:"vnum"`
	Enabled bool   `yaml:"enabled"`
	Note    string `yaml:"note,omitempty"`
}

// UnmarshalYAML accepts both zones.yaml forms: a bare vnum, or a mapping.
func (e *ManifestEntry) UnmarshalYAML(b []byte) error {
	var n int32
	if err := yaml.Unmarshal(b, &n); err == nil {
		*e = ManifestEntry{Vnum: n, Enabled: true}
		return nil
	}
	type alias ManifestEntry
	var a alias
	a.Enabled = true // absent "enabled:" defaults to true, matching a bare vnum
	if err := yaml.UnmarshalWithOptions(b, &a, yaml.Strict()); err != nil {
		return err
	}
	*e = ManifestEntry(a)
	return nil
}

// MarshalYAML writes the compact bare-vnum form for a plain enabled entry
// with no note, and the expanded mapping otherwise — §3's example shows
// both forms coexisting for exactly this reason.
func (e ManifestEntry) MarshalYAML() ([]byte, error) {
	if e.Enabled && e.Note == "" {
		return yaml.Marshal(e.Vnum)
	}
	type alias ManifestEntry
	return yaml.MarshalWithOptions(alias(e), yaml.Indent(2))
}

type manifestDoc struct {
	Schema string          `yaml:"schema"`
	Zones  []ManifestEntry `yaml:"zones"`
}

// readManifest loads zones.yaml from dir.
func readManifest(dir string) (manifestDoc, error) {
	data, err := os.ReadFile(filepath.Join(dir, ManifestFile)) //nolint:gosec // operator-supplied directory
	if err != nil {
		return manifestDoc{}, err
	}
	var doc manifestDoc
	if err := yaml.UnmarshalWithOptions(data, &doc, yaml.Strict()); err != nil {
		return manifestDoc{}, fmt.Errorf("%s: %w", ManifestFile, err)
	}
	return doc, nil
}

// WriteManifest writes zones.yaml listing entries, for tools (dlctl world
// import) that build a whole yaml directory from scratch rather than
// modifying one that already has a manifest.
func WriteManifest(dir string, entries []ManifestEntry) error {
	return writeManifest(dir, manifestDoc{Schema: ManifestSchema, Zones: entries})
}

// writeManifest writes zones.yaml back, sorted by vnum — §10.3's
// determinism requirement applied to the one file in this tree that is not
// itself a zone.
func writeManifest(dir string, doc manifestDoc) error {
	sorted := make([]ManifestEntry, len(doc.Zones))
	copy(sorted, doc.Zones)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Vnum < sorted[j].Vnum })
	doc.Zones = sorted
	out, err := yaml.MarshalWithOptions(doc, yaml.Indent(2))
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, ManifestFile), out)
}

// zoneFiles scans dir for candidate zone files — every *.yaml file except
// the manifest and sets.yaml — and returns them keyed by the vnum found
// inside, per §3's "the vnum inside the file is authoritative". A file
// that fails to parse, or has no `zone.vnum`, is reported rather than
// silently skipped.
func zoneFiles(dir string) (byVnum map[int32]string, findings []string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	byVnum = make(map[int32]string)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		if name == ManifestFile || name == "sets.yaml" {
			continue
		}
		path := filepath.Join(dir, name)
		data, rerr := os.ReadFile(path) //nolint:gosec // operator-supplied directory
		if rerr != nil {
			findings = append(findings, fmt.Sprintf("%s: %v", name, rerr))
			continue
		}
		var peek struct {
			Schema string `yaml:"schema"`
			Zone   struct {
				Vnum int32 `yaml:"vnum"`
			} `yaml:"zone"`
		}
		if uerr := yaml.Unmarshal(data, &peek); uerr != nil {
			findings = append(findings, fmt.Sprintf("%s: %v", name, uerr))
			continue
		}
		if peek.Schema != ZoneSchema {
			findings = append(findings, fmt.Sprintf("%s: schema %q, want %q", name, peek.Schema, ZoneSchema))
			continue
		}
		if existing, dup := byVnum[peek.Zone.Vnum]; dup {
			findings = append(findings, fmt.Sprintf("%s: zone %d already loaded from %s", name, peek.Zone.Vnum, existing))
			continue
		}
		byVnum[peek.Zone.Vnum] = name
	}
	return byVnum, findings, nil
}

// zoneFileName is the canonical filename dlctl fmt --type=world writes a zone to:
// "<vnum>-<slugified-name>.yaml". §3: this is a convenience for humans, not
// something the loader resolves by.
func zoneFileName(vnum int32, name string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d-", vnum)
	slug := strings.ToLower(strings.TrimSpace(name))
	var lastDash bool
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-") + ".yaml"
}
