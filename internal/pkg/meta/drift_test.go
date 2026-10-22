package meta

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	driftSpecRel    = "api/rest/example/v1/example.swagger.yml"
	driftSpecSource = "./example.swagger.yml"
	driftSpecBody   = "openapi: 3.0.0\npaths: {}\n"
)

// writeCopied plants a copied file on disk and returns the record a run that
// copied that exact content would have left in the manifest.
func writeCopied(t *testing.T, dir, rel, onDisk, recorded string) FileRecord {
	t.Helper()

	path := filepath.Join(dir, filepath.FromSlash(rel))

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(path, []byte(onDisk), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	return FileRecord{Path: rel, Kind: KindCopied, Hash: HashContent([]byte(recorded))}
}

func specTarget() []CopiedFile {
	return []CopiedFile{{Path: driftSpecRel, Source: driftSpecSource}}
}

func TestCheckCopiedDriftReportsEditedSpec(t *testing.T) {
	dir := t.TempDir()
	rec := writeCopied(t, dir, driftSpecRel, driftSpecBody+"# hand edit\n", driftSpecBody)

	err := Manifest{rec}.CheckCopiedDrift(dir, specTarget())
	if err == nil {
		t.Fatal("expected an error for an edited copied spec")
	}

	if !errors.Is(err, ErrCopiedDrift) {
		t.Errorf("error must match ErrCopiedDrift, got %v", err)
	}

	// The message has to be actionable on its own: which file in the project,
	// which source to fix instead, and what to do about the local edit.
	for _, want := range []string{driftSpecRel, driftSpecSource, CopiedFileDriftHint} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message must mention %q, got:\n%s", want, err)
		}
	}

	var driftErr *CopiedDriftError
	if !errors.As(err, &driftErr) {
		t.Fatalf("expected *CopiedDriftError, got %T", err)
	}

	if len(driftErr.Files) != 1 || driftErr.Files[0].Path != driftSpecRel {
		t.Errorf("unexpected drifted set: %+v", driftErr.Files)
	}
}

func TestCheckCopiedDriftAcceptsUntouchedSpec(t *testing.T) {
	dir := t.TempDir()
	rec := writeCopied(t, dir, driftSpecRel, driftSpecBody, driftSpecBody)

	if err := (Manifest{rec}).CheckCopiedDrift(dir, specTarget()); err != nil {
		t.Errorf("untouched spec must not be drift, got %v", err)
	}
}

// A project generated before the manifest existed has nothing to compare
// against, so its first run under the new generator must not fail.
func TestCheckCopiedDriftWithoutManifest(t *testing.T) {
	dir := t.TempDir()
	writeCopied(t, dir, driftSpecRel, driftSpecBody+"# hand edit\n", driftSpecBody)

	if err := Manifest(nil).CheckCopiedDrift(dir, specTarget()); err != nil {
		t.Errorf("empty manifest must not fail, got %v", err)
	}
}

// Same for a channel added to project.yaml since the last run: the file is
// copied for the first time, there is no record for it yet.
func TestCheckCopiedDriftWithoutRecordForFile(t *testing.T) {
	dir := t.TempDir()
	writeCopied(t, dir, driftSpecRel, driftSpecBody+"# hand edit\n", driftSpecBody)

	other := Manifest{{Path: "api/schema/models/user.schema.json", Kind: KindCopied, Hash: "deadbeef"}}

	if err := other.CheckCopiedDrift(dir, specTarget()); err != nil {
		t.Errorf("a target with no record must not fail, got %v", err)
	}
}

// Drift above a disclaimer marker stays a warning (templater reports it); this
// check must not turn it into an error by also looking at generated records.
func TestCheckCopiedDriftIgnoresGeneratedRecords(t *testing.T) {
	dir := t.TempDir()

	rec := writeCopied(t, dir, driftSpecRel, driftSpecBody+"# hand edit\n", driftSpecBody)
	rec.Kind = KindGenerated

	if err := (Manifest{rec}).CheckCopiedDrift(dir, specTarget()); err != nil {
		t.Errorf("generated records are not this check's business, got %v", err)
	}
}

// A deleted spec is not drift — the copy recreates it.
func TestCheckCopiedDriftMissingFile(t *testing.T) {
	dir := t.TempDir()

	rec := FileRecord{Path: driftSpecRel, Kind: KindCopied, Hash: HashContent([]byte(driftSpecBody))}

	if err := (Manifest{rec}).CheckCopiedDrift(dir, specTarget()); err != nil {
		t.Errorf("missing file must not be drift, got %v", err)
	}
}

func TestCheckCopiedDriftReportsEveryFileSorted(t *testing.T) {
	dir := t.TempDir()

	const schemaRel = "api/schema/models/user.schema.json"

	specRec := writeCopied(t, dir, driftSpecRel, driftSpecBody+"# edit\n", driftSpecBody)
	schemaRec := writeCopied(t, dir, schemaRel, "{\"edited\": true}\n", "{}\n")

	err := Manifest{schemaRec, specRec}.CheckCopiedDrift(dir, []CopiedFile{
		{Path: schemaRel, Source: "./user.schema.json"},
		{Path: driftSpecRel, Source: driftSpecSource},
	})

	var driftErr *CopiedDriftError
	if !errors.As(err, &driftErr) {
		t.Fatalf("expected *CopiedDriftError, got %v", err)
	}

	got := []string{driftErr.Files[0].Path, driftErr.Files[1].Path}
	want := []string{driftSpecRel, schemaRel}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("drifted files must be sorted by path, got %v want %v", got, want)

			break
		}
	}
}
