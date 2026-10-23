// Package treecheck verifies that the generated area of a project tree still
// matches the manifest the last generator run wrote into
// .project-config/meta.yaml.
//
// It is the read-only half of the manifest. Generation already compares the
// tree against it, but it does so while overwriting the tree: a hand edit above
// a disclaimer marker is reported in the same breath as it is destroyed, and a
// copied spec edited in place aborts a run the operator wanted for other
// reasons. The check answers the same question before anything is written,
// which is what makes it usable as a CI gate.
//
// The check reads exactly two things: meta.yaml and the files it names. It
// never loads project.yaml and never resolves a spec source, so its verdict
// does not depend on a contract repository being reachable, nor on what has
// been committed there since the last generation. That is also the difference
// from "regenerate and diff": this answers "was the generated area edited by
// hand", not "is the tree up to date with the contracts".
//
// What it deliberately does not cover: files outside the manifest. A stray file
// in the tree is not a change of the generated area — the generator does not
// write it, read it or delete it either (see meta.Manifest).
package treecheck

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Educentr/go-project-starter/internal/pkg/meta"
	"github.com/Educentr/go-project-starter/internal/pkg/templater"
)

// Reason classifies how a file stopped matching its manifest record.
type Reason string

// Finding is one file that no longer matches its manifest record.
//
// Path is project-relative and slash-separated, as recorded in the manifest.
// Expected is the hash from the manifest; Actual is the hash of the same area
// on disk, empty when there was nothing to hash (the file is gone, or its
// marker is).
type Finding struct {
	Path     string
	Kind     string
	Reason   Reason
	Expected string
	Actual   string
}

// Result is the verdict of one check run.
type Result struct {
	// MetaPath is the project-relative path of the manifest that was read.
	MetaPath string
	// Checked counts records compared against the tree.
	Checked int
	// Skipped counts records with nothing to compare: no recorded hash, or a
	// path that is user-owned by the time of the check.
	Skipped int
	// Findings are the mismatches, in manifest order (sorted by path).
	Findings []Finding
}

const (
	// ReasonModified — the file is there, but the hash of the area the
	// generator owns differs from the recorded one.
	ReasonModified Reason = "modified"
	// ReasonMissing — the manifest records the file and it is not on disk. A
	// deleted generated file is an edit of the generated area like any other:
	// the tree no longer holds what the generator produced.
	ReasonMissing Reason = "missing"
	// ReasonMarkerLost — a generated file no longer carries the disclaimer
	// marker, so its generated region cannot be told from hand-written code at
	// all. Reported with its own wording instead of the bare "split disclaimer"
	// failure the generator used to produce.
	ReasonMarkerLost Reason = "marker-lost"
	// ReasonNotRegular — the path is occupied by a symlink, socket, device or
	// fifo. Whatever it points at, the generated content is no longer there.
	ReasonNotRegular Reason = "not-regular"
)

// Hints shown under a report, one per reason actually present. Copied files
// reuse meta.CopiedFileDriftHint: the rule is the same one generation enforces,
// and two wordings for it would read as two rules.
const (
	hintGenerated = "the area above the disclaimer marker is generator output, not an edit point: " +
		"move hand-written code below the marker (it survives regeneration) or change the generator template. " +
		"To drop the local edits, restore the files from version control."
	hintMissing = "a file the generator wrote is missing: restore it with `make regenerate` " +
		"or from version control."
	hintNotRegular = "a generated path is occupied by a symlink or another non-regular entry: " +
		"remove it and regenerate."

	layoutStatFailed = "stat %s: %w"
)

// Sentinels for the conditions that stop the check from running at all. They
// are failures of the check, not verdicts about the tree, and are kept
// distinguishable so a caller can tell "the gate did not run" from "the gate
// says no".
var (
	// ErrMetaNotFound — no meta.yaml in the tree, so there is nothing to
	// compare against.
	ErrMetaNotFound = errors.New("no manifest to check the tree against")
	// ErrNoManifest — meta.yaml exists but carries no manifest: the project was
	// generated before manifests existed.
	ErrNoManifest = errors.New("manifest is empty")
	// ErrFutureMeta — meta.yaml was written by a newer generator, whose records
	// this binary may not understand.
	ErrFutureMeta = errors.New("manifest is newer than this generator")
	// ErrUnknownKind — a manifest record carries a kind this binary has no
	// hashing rule for. Reachable only through a hand-edited meta.yaml, since a
	// newer schema is already refused by ErrFutureMeta.
	ErrUnknownKind = errors.New("unknown record kind in the manifest")
)

// OK reports whether the generated area of the tree matches the manifest.
func (r Result) OK() bool { return len(r.Findings) == 0 }

// Run checks the tree rooted at targetDir against its manifest. An empty
// targetDir means the current directory.
//
// The returned error covers only the cases where the check could not be
// performed (see the sentinels above) — a tree that does not match is a
// Result with findings, not an error. Nothing is written: neither the project
// files nor meta.yaml itself.
func Run(targetDir string) (Result, error) {
	relMeta := filepath.ToSlash(filepath.Join(meta.DirName, meta.FileName))
	metaDir := filepath.Join(targetDir, meta.DirName)
	metaPath := filepath.Join(metaDir, meta.FileName)

	// Checked before reading: meta.GetMeta answers a missing file with a
	// default, manifest-less Meta, which here would be indistinguishable from a
	// project predating manifests — a different diagnosis and a different fix.
	if _, err := os.Stat(metaPath); err != nil {
		if os.IsNotExist(err) {
			return Result{}, fmt.Errorf("%w: %s does not exist. "+
				"It is written by every generator run and is expected to be committed with the project",
				ErrMetaNotFound, relMeta)
		}

		return Result{}, fmt.Errorf(layoutStatFailed, relMeta, err)
	}

	m, err := meta.GetMeta(metaDir, meta.FileName)
	if err != nil {
		return Result{}, fmt.Errorf("parse %s: %w", relMeta, err)
	}

	if m.Version > meta.CurrentVersion {
		return Result{}, fmt.Errorf("%w: %s is version %d, this generator knows version %d. "+
			"Check the tree with the generator that produced it (GPSVERSION in the Makefile)",
			ErrFutureMeta, relMeta, m.Version, meta.CurrentVersion)
	}

	if len(m.Manifest) == 0 {
		return Result{}, fmt.Errorf("%w: %s (version %d) lists no files, so the project was generated "+
			"before the manifest existed. Run `make regenerate` once to write one, then check again",
			ErrNoManifest, relMeta, m.Version)
	}

	res := Result{MetaPath: relMeta}

	for _, rec := range m.Manifest {
		// An empty hash says the file was written but its content was not
		// hashed; a user-owned path (go.mod, static/**) is handed over after the
		// first write and is not part of the generated area — the generator does
		// not track one in the manifest, so a record for it can only come from a
		// hand-edited file. Neither is something to compare.
		if rec.Hash == "" || templater.IsUserOwned(rec.Path) {
			res.Skipped++

			continue
		}

		res.Checked++

		reason, actual, err := checkRecord(targetDir, rec)
		if err != nil {
			return Result{}, err
		}

		if reason == "" {
			continue
		}

		res.Findings = append(res.Findings, Finding{
			Path:     rec.Path,
			Kind:     rec.Kind,
			Reason:   reason,
			Expected: rec.Hash,
			Actual:   actual,
		})
	}

	return res, nil
}

// checkRecord compares one manifest record against the file on disk. An empty
// reason means the file still matches; actual is the hash found in the tree,
// and is empty whenever there was nothing to hash.
func checkRecord(targetDir string, rec meta.FileRecord) (reason Reason, actual string, err error) {
	if rec.Kind != meta.KindGenerated && rec.Kind != meta.KindCopied {
		return "", "", fmt.Errorf("%w: %s has kind %q", ErrUnknownKind, rec.Path, rec.Kind)
	}

	path := filepath.Join(targetDir, filepath.FromSlash(rec.Path))

	// Lstat, not Stat: the question is what sits on the path, not what it
	// points at. A generated path replaced by a symlink to an identical file
	// still means the generated content is not there.
	st, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ReasonMissing, "", nil
		}

		return "", "", fmt.Errorf(layoutStatFailed, rec.Path, err)
	}

	if !st.Mode().IsRegular() {
		return ReasonNotRegular, "", nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read %s: %w", rec.Path, err)
	}

	// The same split as the generator records: the whole file for a copied one,
	// the region up to and including the disclaimer line for a generated one.
	hash := meta.HashContent(content)

	if rec.Kind == meta.KindGenerated {
		if hash, err = templater.GeneratedRegionHash(rec.Path, content); err != nil {
			if errors.Is(err, templater.ErrDisclaimerNotFound) {
				return ReasonMarkerLost, "", nil
			}

			return "", "", fmt.Errorf("hash %s: %w", rec.Path, err)
		}
	}

	if hash == rec.Hash {
		return "", "", nil
	}

	return ReasonModified, hash, nil
}

// Report renders the verdict for a human and for a CI log: one block per
// finding with the path, the kind, what happened and both hashes, followed by
// what to do about each kind of mismatch.
func (r Result) Report() string {
	// How many records were not compared is part of the verdict: a green line
	// claiming N files, silently standing for fewer, is how a gate stops
	// meaning anything.
	skipped := ""
	if r.Skipped > 0 {
		skipped = fmt.Sprintf(" (%d records had nothing to compare)", r.Skipped)
	}

	if r.OK() {
		return fmt.Sprintf("check: the generated area of %d files matches %s%s", r.Checked, r.MetaPath, skipped)
	}

	var b strings.Builder

	b.WriteString("check: " + strconv.Itoa(len(r.Findings)) + " of " + strconv.Itoa(r.Checked) +
		" files no longer match " + r.MetaPath + skipped + ":")

	for _, f := range r.Findings {
		b.WriteString("\n  " + f.Path + " (" + f.Kind + "): " + f.description())
		b.WriteString("\n    manifest: " + f.Expected)

		if f.Actual != "" {
			b.WriteString("\n    on disk:  " + f.Actual)
		}
	}

	for _, hint := range r.hints() {
		b.WriteString("\n" + hint)
	}

	return b.String()
}

// description is the human-readable form of a finding's reason, worded for the
// kind of file it happened to: "edited" means something different for a file
// whose user code lives below a marker and for a spec copied in whole.
func (f Finding) description() string {
	switch f.Reason {
	case ReasonModified:
		if f.Kind == meta.KindGenerated {
			return "the generated region (above the disclaimer marker) was edited"
		}

		return "the copied file was edited"
	case ReasonMissing:
		return "missing from the tree"
	case ReasonMarkerLost:
		return "the disclaimer marker was removed, the generated region cannot be located"
	case ReasonNotRegular:
		return "the path is occupied by a non-regular entry (symlink, socket, device)"
	default:
		return string(f.Reason)
	}
}

// hints returns the advice lines for the reasons present in the findings, in a
// fixed order and without repeats — a report of forty edited files should not
// repeat the same paragraph forty times.
func (r Result) hints() []string {
	var generated, copied, missing, notRegular bool

	for _, f := range r.Findings {
		switch {
		case f.Reason == ReasonMissing:
			missing = true
		case f.Reason == ReasonNotRegular:
			notRegular = true
		case f.Kind == meta.KindCopied:
			copied = true
		default:
			generated = true
		}
	}

	candidates := []struct {
		present bool
		text    string
	}{
		{generated, hintGenerated},
		{copied, meta.CopiedFileDriftHint},
		{missing, hintMissing},
		{notRegular, hintNotRegular},
	}

	hints := make([]string, 0, len(candidates))

	for _, h := range candidates {
		if h.present {
			hints = append(hints, h.text)
		}
	}

	return hints
}
