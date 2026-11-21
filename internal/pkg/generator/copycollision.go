package generator

import (
	"slices"
	"strings"

	"github.com/pkg/errors"

	"github.com/Educentr/go-project-starter/internal/pkg/meta"
)

// copyDestCollision is one file inside the project that more than one `path:`
// entry copies to.
type copyDestCollision struct {
	path    string
	sources []string
}

// copyDestCollisionError reports every colliding copy destination of the run at
// once: a config with one clashing basename usually has more.
type copyDestCollisionError struct {
	collisions []copyDestCollision
}

// copyDestCollisionHint is the operator-facing part of the message: why the
// destinations clash and what the ways out are.
const copyDestCollisionHint = "a copied file keeps the basename of its source, so two sources with the same " +
	"basename cannot share one target directory — one would overwrite the other. Rename the file at one of the " +
	"sources, or move it to a transport/jsonschema block of its own (each block copies into its own directory)."

// errCopyDestCollision is the sentinel behind every copy-destination collision
// report. Its text is also the first line of the message.
var errCopyDestCollision = errors.New("more than one `path:` entry copies to the same file inside the project")

// Unwrap exposes errCopyDestCollision so callers can match the error by
// sentinel without depending on the concrete type.
func (e *copyDestCollisionError) Unwrap() error { return errCopyDestCollision }

func (e *copyDestCollisionError) Error() string {
	var b strings.Builder

	b.WriteString(errCopyDestCollision.Error() + ":")

	for _, collision := range e.collisions {
		b.WriteString("\n  " + collision.path)

		for _, source := range collision.sources {
			b.WriteString("\n    - " + source)
		}
	}

	b.WriteString("\n" + copyDestCollisionHint)

	return b.String()
}

// checkCopyDestCollisions fails the run when two different sources are copied
// to the same file inside the project. The copy pass writes planned
// destinations one by one, so without this check the last source silently wins
// and the generator goes on to build against a spec nobody asked for (or
// against a file whose $ref targets are now missing).
//
// Only *different* sources collide. The same source reaching one destination
// several times is normal and expected: a transport is snapshotted into every
// application that serves it. Sources are compared by their description
// (SpecSource.Describe): the `path:` value as written, minus the token value.
//
// Callers must run this before anything is written, so that a non-nil error
// leaves the project tree exactly as it was.
func checkCopyDestCollisions(planned []meta.CopiedFile) error {
	sources := map[string][]string{}

	for _, target := range planned {
		if slices.Contains(sources[target.Path], target.Source) {
			continue
		}

		sources[target.Path] = append(sources[target.Path], target.Source)
	}

	collisions := []copyDestCollision{}

	for path, srcs := range sources {
		if len(srcs) < 2 {
			continue
		}

		// Stable order: the map walk above is random, and both the message a
		// human reads and the one tests diff must not depend on it.
		slices.Sort(srcs)

		collisions = append(collisions, copyDestCollision{path: path, sources: srcs})
	}

	if len(collisions) == 0 {
		return nil
	}

	slices.SortFunc(collisions, func(a, b copyDestCollision) int { return strings.Compare(a.path, b.path) })

	return &copyDestCollisionError{collisions: collisions}
}
