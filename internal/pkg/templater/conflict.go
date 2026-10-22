package templater

import (
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// TargetConflictError lists the paths of the current output set that exist on
// disk without a matching record in the previous run's manifest. The whole
// output set is classified before the error surfaces, so the user gets every
// path to deal with at once instead of one per run.
type TargetConflictError struct {
	// Paths are the conflicting paths, sorted.
	Paths []string
}

// ErrTargetConflict is returned when a path the generator is about to write is
// already occupied by a file the generator did not write — it is absent from
// the previous run's manifest. Overwriting it would take the user's content
// with it: entirely for types that carry no disclaimer marker (JSON, go.mod
// style files, .keep), and above the marker for everything else. Generation
// stops instead, before anything is written.
//
// Use errors.Is to recognize it; the concrete error is *TargetConflictError
// and carries every offending path.
var ErrTargetConflict = errors.New("file exists and was not written by the generator")

// NewTargetConflictError builds a conflict error for paths. The input is
// copied and sorted, so the message is stable regardless of map iteration
// order upstream. Returns nil when paths is empty, which makes it safe to call
// with a collected slice.
func NewTargetConflictError(paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	sorted := make([]string, len(paths))
	copy(sorted, paths)
	sort.Strings(sorted)

	return &TargetConflictError{Paths: sorted}
}

// Error renders the full list of conflicting paths with the action the user
// has to take.
func (e *TargetConflictError) Error() string {
	if len(e.Paths) == 1 {
		return "conflict: " + e.Paths[0] + " exists and was not written by the generator — rename or remove it"
	}

	var b strings.Builder

	b.WriteString("conflict: ")
	b.WriteString(strconv.Itoa(len(e.Paths)))
	b.WriteString(" files exist and were not written by the generator — rename or remove them:")

	for _, path := range e.Paths {
		b.WriteString("\n  - ")
		b.WriteString(path)
	}

	return b.String()
}

// Unwrap exposes the sentinel so callers and tests can match the condition
// with errors.Is(err, ErrTargetConflict) without knowing the concrete type.
func (e *TargetConflictError) Unwrap() error {
	return ErrTargetConflict
}
