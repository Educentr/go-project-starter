package meta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGetMetaMissingFile(t *testing.T) {
	m, err := GetMeta(t.TempDir(), "meta.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.Version != 1 {
		t.Errorf("Version = %d, want 1 (default for a project without meta.yaml)", m.Version)
	}

	if len(m.Manifest) != 0 {
		t.Errorf("Manifest = %v, want empty", m.Manifest)
	}
}

// TestGetMetaWithoutManifest covers the migration case: a project generated
// before the manifest existed stays readable and reports an empty manifest, so
// the generator falls back to scanning the target tree.
func TestGetMetaWithoutManifest(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "meta.yaml"), []byte("version: 4\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := GetMeta(dir, "meta.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.Version != 4 {
		t.Errorf("Version = %d, want 4", m.Version)
	}

	if len(m.Manifest) != 0 {
		t.Errorf("Manifest = %v, want empty", m.Manifest)
	}
}

func TestSaveAndGetMetaRoundTrip(t *testing.T) {
	dir := t.TempDir()

	m := GetDefaultMeta(filepath.Join(dir, "meta.yaml"))
	m.SetManifest(Manifest{
		{Path: "pkg/app/psg_app_gen.go", Kind: KindGenerated, Hash: HashContent([]byte("b"))},
		{Path: "api/rest/svc/v1/svc.swagger.yml", Kind: KindCopied, Hash: HashContent([]byte("a"))},
	})

	if err := m.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	if m.Version != curVer {
		t.Errorf("Save() must stamp the current version, got %d, want %d", m.Version, curVer)
	}

	got, err := GetMeta(dir, "meta.yaml")
	if err != nil {
		t.Fatalf("GetMeta() error: %v", err)
	}

	if got.Version != curVer {
		t.Errorf("Version = %d, want %d", got.Version, curVer)
	}

	if len(got.Manifest) != 2 {
		t.Fatalf("Manifest length = %d, want 2", len(got.Manifest))
	}

	// Sorted by path, so the copied spec comes first.
	if got.Manifest[0].Path != "api/rest/svc/v1/svc.swagger.yml" || got.Manifest[0].Kind != KindCopied {
		t.Errorf("Manifest[0] = %+v, want the copied spec first", got.Manifest[0])
	}

	if got.Manifest[1].Hash != HashContent([]byte("b")) {
		t.Errorf("Manifest[1].Hash = %q, want the stored hash", got.Manifest[1].Hash)
	}
}

// TestSaveManifestOneRecordPerLine pins the on-disk format: meta.yaml changes
// on every run that changes output, so each record must stay on its own line
// for diffs and merge conflicts to be resolvable line by line.
func TestSaveManifestOneRecordPerLine(t *testing.T) {
	dir := t.TempDir()

	m := GetDefaultMeta(filepath.Join(dir, "meta.yaml"))
	m.SetManifest(Manifest{
		{Path: "a.go", Kind: KindGenerated, Hash: "aaa"},
		{Path: "b.go", Kind: KindGenerated, Hash: "bbb"},
	})

	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "meta.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	want := "version: 5\nmanifest:\n    - {path: a.go, kind: generated, hash: aaa}\n    - {path: b.go, kind: generated, hash: bbb}\n"
	if string(data) != want {
		t.Errorf("meta.yaml =\n%s\nwant\n%s", data, want)
	}
}

func TestSaveManifestQuotesWhenNeeded(t *testing.T) {
	rec := FileRecord{Path: "dir/file: odd.go", Kind: KindGenerated, Hash: "h"}

	data, err := yaml.Marshal(Manifest{rec})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(string(data), "\n") != 1 {
		t.Errorf("record must stay on a single line, got:\n%s", data)
	}

	var back Manifest
	if err := yaml.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal %s: %v", data, err)
	}

	if len(back) != 1 || back[0] != rec {
		t.Errorf("round trip = %+v, want %+v", back, rec)
	}
}

func TestManifestNormalize(t *testing.T) {
	m := Manifest{
		{Path: "b.go", Kind: KindGenerated, Hash: "1"},
		{Path: "a.go", Kind: KindGenerated, Hash: "2"},
		{Path: "b.go", Kind: KindGenerated, Hash: "3"},
	}.Normalize()

	if len(m) != 2 {
		t.Fatalf("Normalize() length = %d, want 2 (duplicate dropped)", len(m))
	}

	if m[0].Path != "a.go" || m[1].Path != "b.go" {
		t.Errorf("Normalize() = %+v, want sorted by path", m)
	}

	if m[1].Hash != "3" {
		t.Errorf("Normalize() kept hash %q, want the last record for the path", m[1].Hash)
	}

	if Manifest(nil).Normalize() != nil {
		t.Error("Normalize() of an empty manifest must stay nil so meta.yaml omits the key")
	}
}

func TestManifestIndex(t *testing.T) {
	idx := Manifest{
		{Path: "a.go", Kind: KindGenerated, Hash: "1"},
		{Path: "b.yml", Kind: KindCopied, Hash: "2"},
	}.Index()

	if rec, ok := idx["b.yml"]; !ok || rec.Kind != KindCopied {
		t.Errorf("Index()[b.yml] = %+v, %v", rec, ok)
	}

	if _, ok := idx["missing.go"]; ok {
		t.Error("Index() must not invent records")
	}
}

func TestRelPath(t *testing.T) {
	got, err := RelPath(filepath.Join("/tmp", "project"), filepath.Join("/tmp", "project", "pkg", "app", "file.go"))
	if err != nil {
		t.Fatal(err)
	}

	if got != "pkg/app/file.go" {
		t.Errorf("RelPath() = %q, want %q", got, "pkg/app/file.go")
	}
}

func TestHashContent(t *testing.T) {
	if HashContent([]byte("a")) == HashContent([]byte("b")) {
		t.Error("different content must hash differently")
	}

	if got := len(HashContent(nil)); got != 64 {
		t.Errorf("hash length = %d, want 64 (sha256 hex)", got)
	}
}
