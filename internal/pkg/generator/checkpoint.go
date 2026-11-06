package generator

import (
	"log"
	"os"
	"path/filepath"

	"github.com/Educentr/go-project-starter/internal/pkg/meta"
)

// runWrites collects what the current run has already put into the project
// tree, so that a run which fails halfway can still record it.
//
// The manifest is the generator's only answer to "did I write this file?", and
// it used to be written once, at the end of a run that succeeded in full. A run
// that copied specs and then failed therefore left the copies on disk while
// meta.yaml still described the run before it — and the next run read those
// copies as hand edits of copied files and aborted on them, before it could
// write a manifest saying otherwise. That loop no further run could leave.
type runWrites struct {
	// generated holds the records of the template output written so far, in
	// the form a successful run records them.
	generated meta.Manifest
	// copied holds the absolute paths of the files copied verbatim (specs,
	// schemas). Their hash is taken from disk at checkpoint time: ref
	// rewriting edits them in place after the copy, so the bytes that landed
	// are not necessarily the bytes that stay.
	copied []string
	// contracts holds the records of the copied contract specs, provenance
	// included — that part cannot be rebuilt from the path alone.
	contracts meta.Manifest
	// saved marks that the manifest of this run has already been written in
	// full, after which there is nothing left to checkpoint.
	saved bool
}

// checkpoint records the files the run wrote before it failed, so that the next
// run recognizes them as the generator's own output rather than hand edits. It
// is deferred unconditionally and does nothing for a run that got as far as
// writing its own manifest.
//
// The records are merged over the previous manifest instead of replacing it:
// the output of this run is partial by definition, and dropping the records of
// the files earlier runs wrote would turn them into conflicts. Only files that
// are actually on disk are recorded — a write that failed leaves a planned
// record with no file behind it.
//
// A run with no previous manifest is not checkpointed at all: there a partial
// manifest is worse than none. The manifest is also the switch between the
// exact path and the pre-manifest one (templater.GetUserCodeFromFiles), so
// writing one turns the project into "tracked" — and every file earlier runs
// put there that this one did not reach (the specs under api/, the generated
// files past the point of failure) becomes a file the generator never wrote,
// i.e. a conflict. Leaving meta.yaml as it was keeps the project on the path it
// was already on, and the first run that completes records the whole picture.
// Nothing is lost by it: without records every check the checkpoint feeds —
// copied drift, contract drift, conflicts — is a no-op, so such a project
// cannot be in the loop the checkpoint exists for.
//
// Failures are logged rather than returned: the run is already failing on
// something the operator has to act on, and that error is the one worth
// showing.
func (g *Generator) checkpoint(targetPath string, prev meta.Manifest, w runWrites) {
	if w.saved || len(prev) == 0 {
		return
	}

	written := make(meta.Manifest, 0, len(w.generated)+len(w.contracts)+len(w.copied))
	written = append(written, w.generated...)
	written = append(written, w.contracts...)

	for _, path := range w.copied {
		record, err := copiedFileRecord(targetPath, path)
		if err != nil {
			// The copy this run failed on may not be on disk at all, and
			// whatever else it copied still has to be recorded.
			log.Printf("warning: could not record the copied file %s of the failed run: %v", path, err)

			continue
		}

		written = append(written, record)
	}

	written = recordsOnDisk(targetPath, written)
	if len(written) == 0 {
		return
	}

	g.Meta.SetManifest(prev.Merge(written))

	if err := g.Meta.Save(); err != nil {
		log.Printf("warning: could not record the files written by the failed run in %s: %v", g.Meta.Path, err)
	}
}

// recordsOnDisk keeps the records whose file exists as a regular file.
func recordsOnDisk(targetPath string, records meta.Manifest) meta.Manifest {
	kept := make(meta.Manifest, 0, len(records))

	for _, rec := range records {
		st, err := os.Lstat(filepath.Join(targetPath, filepath.FromSlash(rec.Path)))
		if err != nil || !st.Mode().IsRegular() {
			continue
		}

		kept = append(kept, rec)
	}

	return kept
}
