package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/meta"
)

// writeFile writes content at rel inside dir, creating parents, and returns the
// absolute path.
func writeFile(t *testing.T, dir, rel, content string) string {
	t.Helper()

	path := filepath.Join(dir, filepath.FromSlash(rel))

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

// checkpointGenerator returns a generator whose meta.yaml lives in dir and
// whose manifest is the one a previous run left behind.
func checkpointGenerator(t *testing.T, dir string, prev meta.Manifest) *Generator {
	t.Helper()

	return &Generator{
		TargetDir: dir,
		Meta:      meta.Meta{Path: filepath.Join(dir, "meta.yaml"), Manifest: prev},
	}
}

// savedManifest reads back the manifest the generator wrote to disk.
func savedManifest(t *testing.T, dir string) meta.Manifest {
	t.Helper()

	m, err := meta.GetMeta(dir, filepath.Join(dir, "meta.yaml"))
	if err != nil {
		t.Fatalf("read back meta.yaml: %v", err)
	}

	return m.Manifest
}

// TestCheckpointBreaksTheDriftLoop is the regression behind the checkpoint: a
// run that copied specs and then failed used to leave them on disk with
// meta.yaml still describing the run before it, so the next run reported those
// copies as hand edits of copied files — and aborted before it could record
// them, which no further run could undo.
func TestCheckpointBreaksTheDriftLoop(t *testing.T) {
	const (
		rel    = "api/rest/example/v1/example.swagger.yml"
		before = "openapi: 3.0.0\npaths: {}\n"
		after  = "openapi: 3.0.0\npaths:\n  /ping: {}\n"
	)

	dir := t.TempDir()

	// What the last successful run recorded, and what the failed run copied.
	prev := meta.Manifest{{Path: rel, Kind: meta.KindCopied, Hash: meta.HashContent([]byte(before))}}
	copied := writeFile(t, dir, rel, after)

	g := checkpointGenerator(t, dir, prev)
	g.checkpoint(dir, prev, runWrites{copied: []string{copied}})

	targets := []meta.CopiedFile{{Path: rel, Source: "./example.swagger.yml"}}

	if err := savedManifest(t, dir).CheckCopiedDrift(dir, targets); err != nil {
		t.Fatalf("the copy of the failed run is still reported as a hand edit: %v", err)
	}
}

// The checkpoint describes this run, but it must not drop what earlier runs
// recorded: a copied file this run never touched keeps its hash, so a real hand
// edit of it is still caught.
func TestCheckpointKeepsRecordsOfEarlierRuns(t *testing.T) {
	const (
		copiedRel = "api/rest/example/v1/example.swagger.yml"
		editedRel = "api/schema/models/user.schema.json"
		genRel    = "internal/app/constant/psg_constant_gen.go"
	)

	dir := t.TempDir()

	prev := meta.Manifest{
		{Path: copiedRel, Kind: meta.KindCopied, Hash: meta.HashContent([]byte("spec\n"))},
		{Path: editedRel, Kind: meta.KindCopied, Hash: meta.HashContent([]byte("schema\n"))},
	}

	copied := writeFile(t, dir, copiedRel, "spec v2\n")
	writeFile(t, dir, editedRel, "schema, edited by hand\n")
	writeFile(t, dir, genRel, "package constant\n")

	g := checkpointGenerator(t, dir, prev)
	g.checkpoint(dir, prev, runWrites{
		generated: meta.Manifest{{Path: genRel, Kind: meta.KindGenerated, Hash: "abc123"}},
		copied:    []string{copied},
	})

	saved := savedManifest(t, dir).Index()

	if len(saved) != 3 {
		t.Fatalf("manifest = %+v, want all three paths", saved)
	}

	if got := saved[copiedRel].Hash; got != meta.HashContent([]byte("spec v2\n")) {
		t.Errorf("copied record hash = %q, want the content this run put on disk", got)
	}

	if got := saved[editedRel].Hash; got != prev.Index()[editedRel].Hash {
		t.Errorf("untouched record hash = %q, want the one the previous run recorded", got)
	}

	if got := saved[genRel]; got.Kind != meta.KindGenerated || got.Hash != "abc123" {
		t.Errorf("generated record = %+v, want the record the run produced", got)
	}
}

// A record whose file never made it to disk is not evidence of anything: the
// run planned the file and failed before creating it.
func TestCheckpointSkipsFilesNeverWritten(t *testing.T) {
	dir := t.TempDir()

	g := checkpointGenerator(t, dir, nil)
	g.checkpoint(dir, nil, runWrites{
		generated: meta.Manifest{{Path: "cmd/api/psg_main_gen.go", Kind: meta.KindGenerated, Hash: "abc123"}},
	})

	if _, err := os.Stat(filepath.Join(dir, "meta.yaml")); !os.IsNotExist(err) {
		t.Fatalf("meta.yaml should not be written when nothing reached the disk, stat err = %v", err)
	}
}

// Once the manifest of a successful run is on disk, a later failure must leave
// it alone: the full picture beats the partial one.
func TestCheckpointSkipsWhenManifestAlreadySaved(t *testing.T) {
	const genRel = "internal/app/constant/psg_constant_gen.go"

	dir := t.TempDir()
	writeFile(t, dir, genRel, "package constant\n")

	g := checkpointGenerator(t, dir, nil)
	g.checkpoint(dir, nil, runWrites{
		generated: meta.Manifest{{Path: genRel, Kind: meta.KindGenerated, Hash: "abc123"}},
		saved:     true,
	})

	if _, err := os.Stat(filepath.Join(dir, "meta.yaml")); !os.IsNotExist(err) {
		t.Fatalf("a saved manifest must not be rewritten by the checkpoint, stat err = %v", err)
	}
}

// Contract records carry provenance, which cannot be rebuilt from the path, so
// the checkpoint has to keep them as copyContracts produced them.
func TestCheckpointKeepsContractProvenance(t *testing.T) {
	const rel = "api/contract/limits.yaml"

	dir := t.TempDir()
	writeFile(t, dir, rel, "limits: {}\n")

	record := meta.FileRecord{
		Path:   rel,
		Kind:   meta.KindCopied,
		Hash:   meta.HashContent([]byte("limits: {}\n")),
		Source: "git+ssh://git@host/org/contracts.git@v1#limits.yaml",
		Ref:    "v1",
		Commit: "0123456789abcdef",
	}

	g := checkpointGenerator(t, dir, nil)
	g.checkpoint(dir, nil, runWrites{contracts: meta.Manifest{record}})

	if got := savedManifest(t, dir).Index()[rel]; got != record {
		t.Errorf("contract record = %+v, want %+v", got, record)
	}
}

// The copy a run failed on may not be on disk at all; what it copied before
// that still has to be recorded, or the next run reports those files as hand
// edits.
func TestCheckpointSkipsCopyThatNeverLanded(t *testing.T) {
	const (
		copiedRel = "api/rest/example/v1/example.swagger.yml"
		missedRel = "api/schema/models/user.schema.json"
	)

	dir := t.TempDir()
	copied := writeFile(t, dir, copiedRel, "spec\n")

	g := checkpointGenerator(t, dir, nil)
	g.checkpoint(dir, nil, runWrites{
		copied: []string{copied, filepath.Join(dir, filepath.FromSlash(missedRel))},
	})

	saved := savedManifest(t, dir).Index()

	if _, recorded := saved[copiedRel]; !recorded {
		t.Errorf("manifest = %+v, want the file that was copied recorded", saved)
	}

	if _, recorded := saved[missedRel]; recorded {
		t.Errorf("manifest = %+v, want no record for the copy that never landed", saved)
	}
}
