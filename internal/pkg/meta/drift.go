package meta

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ErrCopiedDrift is the sentinel behind every copied-file drift report. Match
// it with errors.Is to tell a drifted tree apart from any other generator
// failure.
var ErrCopiedDrift = errors.New("file copied by the generator was edited inside the project")

// CopiedFileDriftHint is the operator-facing part of the drift message: what to
// do about it. Kept separate from the error so other channels that copy files
// into the project (and hit the same rule) reuse one wording instead of
// inventing their own.
const CopiedFileDriftHint = "files copied by the generator are not an edit point: change them at the source " +
	"(the contract repository the path points to) and re-run the generator. " +
	"To drop the local edits, restore the files from version control."

// CopiedFile pairs a file the generator copies into the project as-is with the
// source it comes from.
//
// Path is project-relative and slash-separated — the FileRecord.Path form, so
// it can be looked up in the manifest directly. Source is the source as the
// operator wrote it in project.yaml (a local path or a token-redacted URI), not
// the per-run staging path the resolver materializes it into: a staging path
// says nothing about where the contract actually lives.
type CopiedFile struct {
	Path   string
	Source string
}

// CopiedDriftError reports copied files whose content on disk no longer matches
// the hash recorded for them in the manifest.
type CopiedDriftError struct {
	Files []CopiedFile
}

// Unwrap exposes ErrCopiedDrift so callers can match the error by sentinel
// without depending on the concrete type.
func (e *CopiedDriftError) Unwrap() error { return ErrCopiedDrift }

func (e *CopiedDriftError) Error() string {
	var b strings.Builder

	// Reads as a sentence after the caller's "failed to generate: " prefix.
	if len(e.Files) == 1 {
		b.WriteString("a file copied by the generator was edited inside the project tree:")
	} else {
		b.WriteString(strconv.Itoa(len(e.Files)) +
			" files copied by the generator were edited inside the project tree:")
	}

	for _, f := range e.Files {
		b.WriteString("\n  " + f.Path + " (source: " + f.Source + ")")
	}

	b.WriteString("\n" + CopiedFileDriftHint)

	return b.String()
}

// CheckCopiedDrift verifies that every file in targets still carries the
// content the generator copied there, and returns a *CopiedDriftError naming
// the ones that do not.
//
// Only targets the manifest records as KindCopied are compared. A target with
// no record has nothing to compare against — a project generated before the
// manifest existed, or a channel added to project.yaml since the last run — and
// is not reported. A target missing from disk is not drift either: the copy
// recreates it.
//
// The comparison is against the recorded hash, never against the source: the
// contract may legitimately have moved on in its own repository, and that is
// not an edit inside the project.
//
// Callers are expected to run this before writing anything, so that a non-nil
// error leaves the project tree exactly as it was.
func (m Manifest) CheckCopiedDrift(targetPath string, targets []CopiedFile) error {
	if len(m) == 0 || len(targets) == 0 {
		return nil
	}

	idx := m.Index()
	drifted := make([]CopiedFile, 0, len(targets))

	for _, target := range targets {
		rec, ok := idx[target.Path]
		if !ok || rec.Kind != KindCopied || rec.Hash == "" {
			continue
		}

		path := filepath.Join(targetPath, filepath.FromSlash(target.Path))

		content, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}

			return fmt.Errorf("error read copied file %s: %w", path, err)
		}

		if HashContent(content) == rec.Hash {
			continue
		}

		drifted = append(drifted, target)
	}

	if len(drifted) == 0 {
		return nil
	}

	// Stable order: the message is read by a human and diffed by tests.
	sort.Slice(drifted, func(i, j int) bool { return drifted[i].Path < drifted[j].Path })

	return &CopiedDriftError{Files: drifted}
}
