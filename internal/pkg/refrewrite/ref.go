package refrewrite

import (
	"os"
	"path/filepath"
	"strings"
)

// refOutcome describes the result of a single $ref evaluation.
type refOutcome struct {
	// NewValue is the rewritten ref. Valid only when Rewrote is true.
	NewValue string
	// Rewrote is true when the ref was rewritten.
	Rewrote bool
	// SkipReason is non-empty when the ref was deliberately skipped with a
	// noteworthy reason that should be surfaced to the user. Empty SkipReason
	// with Rewrote=false means a silent no-op (e.g. internal pointer).
	SkipReason string
}

// evaluateRef applies the rewrite rules from the plan to a single $ref value.
//
//	dir     — directory in which sibling files are looked up.
//	allowed — optional whitelist of sibling basenames (pass nil to disable).
func evaluateRef(raw, dir string, allowed map[string]struct{}) refOutcome {
	value := strings.TrimSpace(raw)
	if value == "" {
		return refOutcome{}
	}

	// Split into <path>#<frag>. # in the fragment must be encoded per RFC 3986,
	// so a single SplitN(_, _, 2) is enough.
	var path, frag string
	if i := strings.IndexByte(value, '#'); i >= 0 {
		path = value[:i]
		frag = value[i+1:]
	} else {
		path = value
	}

	if path == "" {
		// Internal ref: "#/components/...". Nothing to do.
		return refOutcome{}
	}

	lower := strings.ToLower(path)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return refOutcome{SkipReason: "absolute URL"}
	}

	if !strings.ContainsAny(path, "/\\") {
		// Already a flat reference like "foo.yaml" — nothing to rewrite.
		return refOutcome{}
	}

	base := filepath.Base(filepath.ToSlash(path))
	if base == "" || base == "." || base == "/" {
		return refOutcome{SkipReason: "unusable basename in $ref"}
	}

	if allowed != nil {
		if _, ok := allowed[base]; !ok {
			return refOutcome{SkipReason: "basename not in expected set: " + base}
		}
	}

	if _, err := os.Stat(filepath.Join(dir, base)); err != nil {
		if os.IsNotExist(err) {
			return refOutcome{SkipReason: "sibling not found: " + base}
		}
		return refOutcome{SkipReason: "stat sibling: " + err.Error()}
	}

	newValue := "./" + base
	if frag != "" {
		newValue += "#" + frag
	}

	// No-op when the rewrite would not change the value (e.g. "./foo.yaml#/X").
	if newValue == value {
		return refOutcome{}
	}

	return refOutcome{NewValue: newValue, Rewrote: true}
}
