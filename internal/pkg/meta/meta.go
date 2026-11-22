package meta

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileRecord describes a single file the generator wrote during the last run.
//
// Path is always project-relative and slash-separated, so the manifest stays
// valid regardless of where the project is checked out.
//
// Provenance is filled for every file the generator fetched through the source
// resolver, whichever channel asked for it — not just for contract specs. A
// file produced from a template carries no provenance and keeps exactly the
// shape it had before provenance existed.
type FileRecord struct {
	Path       string `yaml:"path"`
	Kind       string `yaml:"kind"`
	Hash       string `yaml:"hash"`
	Provenance `yaml:",inline"`
}

// Manifest is the list of files written by the last generator run. It is the
// authoritative answer to "did the generator write this file?" — orphan
// detection is the previous manifest minus the current output set, so files the
// generator never wrote are not read, diffed or deleted.
type Manifest []FileRecord

type Meta struct {
	Path    string `yaml:"-"`
	Version int    `yaml:"version"`
	// Inputs are the resolver inputs of the run that left no file in the
	// project. Listed before the manifest because there are a handful of them
	// and hundreds of files.
	Inputs   Inputs   `yaml:"inputs,omitempty"`
	Manifest Manifest `yaml:"manifest,omitempty"`
}

// CurrentVersion is the schema version Save writes. A meta.yaml carrying a
// higher one was produced by a newer generator: its manifest may hold records
// this binary cannot interpret, so readers are expected to refuse it rather
// than draw conclusions from the part they recognize.
//
// 6 split the provenance of a fetched file into separate fields (source_kind,
// source_repo, source_path, source_sha256 next to ref/commit), started
// recording it for every channel instead of `contract:` alone, and added the
// `inputs` section for resolver inputs that leave no file in the project.
const CurrentVersion = 6

// ErrFutureVersion reports a meta.yaml written by a newer generator. Its
// records may carry fields this binary does not know, and the manifest is the
// generator's only answer to "did I write this file?": acting on the part it
// recognizes would mean overwriting files on the strength of a format it cannot
// read in full.
var ErrFutureVersion = errors.New("meta.yaml is newer than this generator")

// Location of the metadata file inside a generated project. Exported because
// everything that reads the manifest — generation, the tree check — has to
// agree on it, and a second string literal is how they stop agreeing.
const (
	DirName  = ".project-config"
	FileName = "meta.yaml"
)

// Kinds of files recorded in the manifest.
const (
	// KindGenerated marks a file produced from a template. Its hash covers
	// only the generated region — everything up to and including the
	// disclaimer line — so user code below the marker does not affect it.
	KindGenerated = "generated"
	// KindCopied marks a file copied into the project as-is (API specs,
	// JSON schemas). Its hash covers the whole file.
	KindCopied = "copied"
)

// MarshalYAML renders a record as a single-line flow mapping. One record per
// line keeps meta.yaml diffs and merge conflicts line-local: meta.yaml changes
// on every run that changes output, so the format has to survive branch merges.
func (r FileRecord) MarshalYAML() (any, error) {
	kvs := [][2]string{
		{"path", r.Path},
		{"kind", r.Kind},
		{"hash", r.Hash},
	}

	// Provenance is omitted when absent, so records of files the generator
	// produced from a template keep the exact shape they had before it existed.
	kvs = append(kvs, r.Provenance.kvs()...)

	return flowMapping(kvs), nil
}

// Index maps project-relative path to its record.
func (m Manifest) Index() map[string]FileRecord {
	idx := make(map[string]FileRecord, len(m))
	for _, rec := range m {
		idx[rec.Path] = rec
	}

	return idx
}

// Normalize sorts records by path and drops duplicates, keeping the last one.
// Stable ordering keeps meta.yaml diffs minimal between runs.
func (m Manifest) Normalize() Manifest {
	if len(m) == 0 {
		return nil
	}

	idx := m.Index()

	res := make(Manifest, 0, len(idx))
	for _, rec := range idx {
		res = append(res, rec)
	}

	sort.Slice(res, func(i, j int) bool { return res[i].Path < res[j].Path })

	return res
}

// Merge returns the manifest with every record of upd applied on top: a record
// whose path is already present is replaced, a path not present is added. The
// result is normalized, as SetManifest expects.
//
// Merging rather than substituting is what recording a partial run needs: the
// files earlier runs wrote keep their records, or the next run would read them
// as files the generator never wrote.
func (m Manifest) Merge(upd Manifest) Manifest {
	if len(upd) == 0 {
		return m.Normalize()
	}

	merged := make(Manifest, 0, len(m)+len(upd))
	merged = append(merged, m...)
	merged = append(merged, upd...)

	// Normalize keeps the last record of a path, so upd wins.
	return merged.Normalize()
}

// HashContent returns the hex-encoded SHA-256 of content. It is the hash
// function behind FileRecord.Hash.
func HashContent(content []byte) string {
	sum := sha256.Sum256(content)

	return hex.EncodeToString(sum[:])
}

// RelPath converts an absolute path inside baseDir into the project-relative,
// slash-separated form used by FileRecord.Path.
func RelPath(baseDir, path string) (string, error) {
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return "", err
	}

	return filepath.ToSlash(rel), nil
}

func GetDefaultMeta(path string) Meta {
	return Meta{
		Path:    path,
		Version: 1,
	}
}

func GetMeta(baseDir, metaPath string) (Meta, error) {
	var meta Meta

	realMetaPath := metaPath

	if !strings.Contains(metaPath, "/") {
		realMetaPath = filepath.Join(baseDir, metaPath)
	}

	meta = GetDefaultMeta(realMetaPath)

	source, err := os.ReadFile(realMetaPath)
	if err != nil {
		errNotFound := &fs.PathError{}
		if errors.As(err, &errNotFound) {
			return meta, nil
		}
	}

	if err := yaml.Unmarshal(source, &meta); err != nil {
		return meta, err
	}

	// A newer schema is refused rather than partially understood: generation
	// decides what to overwrite, what to delete and what is a hand-written file
	// by reading this manifest, and it cannot make those calls on records whose
	// fields it does not know. The same rule the tree check applies.
	if meta.Version > CurrentVersion {
		return meta, fmt.Errorf("%w: %s is version %d, this generator knows version %d. "+
			"Generate with the version that produced it (GPSVERSION in the Makefile)",
			ErrFutureVersion, realMetaPath, meta.Version, CurrentVersion)
	}

	meta.Path = realMetaPath

	return meta, nil
}

// SetManifest replaces the manifest with a normalized copy of records.
func (m *Meta) SetManifest(records Manifest) {
	m.Manifest = records.Normalize()
}

// SetInputs replaces the inputs section with a normalized copy of records.
func (m *Meta) SetInputs(records Inputs) {
	m.Inputs = records.Normalize()
}

func (m *Meta) Save() error {
	m.Version = CurrentVersion

	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}

	return os.WriteFile(m.Path, data, 0644)
}
