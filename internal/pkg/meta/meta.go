package meta

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
// Source/Ref/Commit record where a fetched file came from. They are written for
// contract specs (see the `contract` section of project.yaml), whose whole
// point is that the project declares what it takes and the generator records
// what it actually took. Ref and Commit only exist for git sources; an https or
// local source is identified by Source plus Hash alone.
type FileRecord struct {
	Path string `yaml:"path"`
	Kind string `yaml:"kind"`
	Hash string `yaml:"hash"`
	// Source is the spec source the file was fetched from, as written in
	// project.yaml with any token redacted.
	Source string `yaml:"source,omitempty"`
	// Ref is the git ref the file was fetched at — the effective one, so a ref
	// that came from --sourceRef is recorded rather than the config default.
	Ref string `yaml:"ref,omitempty"`
	// Commit is the SHA the git source resolved to at fetch time.
	Commit string `yaml:"commit,omitempty"`
}

// Manifest is the list of files written by the last generator run. It is the
// authoritative answer to "did the generator write this file?" — orphan
// detection is the previous manifest minus the current output set, so files the
// generator never wrote are not read, diffed or deleted.
type Manifest []FileRecord

type Meta struct {
	Path     string   `yaml:"-"`
	Version  int      `yaml:"version"`
	Manifest Manifest `yaml:"manifest,omitempty"`
}

const curVer = 5

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
	node := &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}

	kvs := [][2]string{
		{"path", r.Path},
		{"kind", r.Kind},
		{"hash", r.Hash},
	}

	// Provenance is omitted when absent, so records of files the generator
	// produced itself keep the exact shape they had before contracts existed.
	for _, kv := range [][2]string{
		{"source", r.Source},
		{"ref", r.Ref},
		{"commit", r.Commit},
	} {
		if kv[1] != "" {
			kvs = append(kvs, kv)
		}
	}

	for _, kv := range kvs {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: kv[0]},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: kv[1]},
		)
	}

	return node, nil
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

	meta.Path = realMetaPath

	return meta, nil
}

// SetManifest replaces the manifest with a normalized copy of records.
func (m *Meta) SetManifest(records Manifest) {
	m.Manifest = records.Normalize()
}

func (m *Meta) Save() error {
	m.Version = curVer

	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}

	return os.WriteFile(m.Path, data, 0644)
}
