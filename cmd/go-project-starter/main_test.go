package main

import (
	"errors"
	"testing"

	"github.com/spf13/pflag"

	"github.com/Educentr/go-project-starter/internal/pkg/specsource"
)

// sourceRefFlagSet re-creates the flag wiring runGenerator sets up, on a fresh
// flag set so subtests don't leak state into each other.
func sourceRefFlagSet(t *testing.T, flagValues ...string) *pflag.FlagSet {
	t.Helper()

	fs := pflag.NewFlagSet(t.Name(), pflag.ContinueOnError)
	fs.StringArray(flagSourceRef, nil, usageSourceRef)

	for _, v := range flagValues {
		if err := fs.Set(flagSourceRef, v); err != nil {
			t.Fatalf("set flag: %v", err)
		}
	}

	return fs
}

// getenvFunc returns an env lookup that only knows GPS_SOURCE_REF.
func getenvFunc(value string) func(string) string {
	return func(name string) string {
		if name == envSourceRef {
			return value
		}

		return ""
	}
}

// What reaches the generator is asserted through SourceRefs.String(): it
// renders exactly the overrides the set holds, so it shows both which entries
// were read and which source (flag or env) they came from. Per-repository
// matching itself is covered in internal/pkg/specsource.
func TestResolveSourceRefs(t *testing.T) {
	tests := []struct {
		name  string
		flags []string
		env   string
		want  string
	}{
		{
			name: "neither set means no override",
			want: "",
		},
		{
			name:  "blanket flag",
			flags: []string{"release/a"},
			want:  "every other repository -> release/a",
		},
		{
			name:  "repeated flag, one repository each",
			flags: []string{"org/specs:release/a", "org/proto:release/b"},
			want:  "org/specs -> release/a, org/proto -> release/b",
		},
		{
			name:  "one flag value may carry a list",
			flags: []string{"org/specs:release/a,org/proto:release/b"},
			want:  "org/specs -> release/a, org/proto -> release/b",
		},
		{
			name:  "scoped and blanket together",
			flags: []string{"org/specs:release/a", "main"},
			want:  "org/specs -> release/a, every other repository -> main",
		},
		{
			name: "env only",
			env:  "release/b",
			want: "every other repository -> release/b",
		},
		{
			name: "env may carry a list",
			env:  "org/specs:release/a org/proto:release/b",
			want: "org/specs -> release/a, org/proto -> release/b",
		},
		{
			// All-or-nothing on purpose: a per-repository set is only
			// meaningful as a whole (see resolveSourceRefs).
			name:  "any flag ignores the env entirely",
			flags: []string{"org/specs:release/a"},
			env:   "release/from-env",
			want:  "org/specs -> release/a",
		},
		{
			name: "whitespace around an env value is ignored",
			env:  "  release/b  ",
			want: "every other repository -> release/b",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveSourceRefs(sourceRefFlagSet(t, tc.flags...), getenvFunc(tc.env))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.String() != tc.want {
				t.Errorf("got %q, want %q", got.String(), tc.want)
			}

			if got.IsEmpty() != (tc.want == "") {
				t.Errorf("IsEmpty: got %v for %q", got.IsEmpty(), got.String())
			}
		})
	}
}

func TestResolveSourceRefs_RejectsUnusableRef(t *testing.T) {
	_, err := resolveSourceRefs(sourceRefFlagSet(t, "--upload-pack=evil"), getenvFunc(""))
	if !errors.Is(err, specsource.ErrInvalidRef) {
		t.Fatalf("got %v, want ErrInvalidRef", err)
	}
}

// Two blanket refs in one invocation have no defined meaning — refusing is the
// only answer that cannot build a tree from two states of the contracts.
func TestResolveSourceRefs_RejectsContradictingEntries(t *testing.T) {
	_, err := resolveSourceRefs(sourceRefFlagSet(t, "release/a", "release/b"), getenvFunc(""))
	if !errors.Is(err, specsource.ErrInvalidSourceRef) {
		t.Fatalf("got %v, want ErrInvalidSourceRef", err)
	}
}
