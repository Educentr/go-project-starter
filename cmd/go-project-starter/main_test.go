package main

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestVersionRequested(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "no arguments generates, not reports",
			args: nil,
			want: false,
		},
		{
			name: "subcommand form",
			args: []string{"version"},
			want: true,
		},
		{
			name: "flag form",
			args: []string{"--version"},
			want: true,
		},
		{
			// The flag short-circuits whatever else was asked for, the way
			// --version does in every other tool.
			name: "flag after other flags of the generator",
			args: []string{"--configDir=.project-config", "--target=.", "--version"},
			want: true,
		},
		{
			// Documented as common to all commands, so it answers even where
			// the subcommand's own flag set would have rejected it.
			name: "flag after a subcommand",
			args: []string{"migrate", "--dry-run", "--version"},
			want: true,
		},
		{
			name: "another subcommand is not a version request",
			args: []string{"setup", "ci"},
			want: false,
		},
		{
			// `version` is the generator's subcommand, not a word any other
			// subcommand hands on to this check.
			name: "version as an argument of a subcommand",
			args: []string{"setup", "version"},
			want: false,
		},
		{
			name: "past the end-of-flags terminator everything is an operand",
			args: []string{"--", "--version"},
			want: false,
		},
		{
			name: "the string is only a request on its own",
			args: []string{"--config=--version"},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := versionRequested(tc.args); got != tc.want {
				t.Errorf("versionRequested(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

// TestVersionInvocationsAgree builds the binary the way the Makefile does and
// runs both documented forms. The flag form is the one that used to fail —
// pflag rejected it as an unknown flag — so what proves it answers now is the
// real process exiting 0, not the dispatch check alone.
func TestVersionInvocationsAgree(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}

	const (
		wantVersion   = "v9.9.9-test"
		wantCommit    = "deadbee"
		wantBuildDate = "2026-01-01T00:00:00Z"
	)

	bin := filepath.Join(t.TempDir(), "go-project-starter")

	build := exec.Command("go", "build",
		"-ldflags", "-X main.version="+wantVersion+
			" -X main.commit="+wantCommit+
			" -X main.buildDate="+wantBuildDate,
		"-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	want := "go-project-starter " + wantVersion + "\ncommit: " + wantCommit + "\nbuilt: " + wantBuildDate + "\n"

	for _, args := range [][]string{{"version"}, {"--version"}, {"migrate", "--version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cmd := exec.Command(bin, args...)
			// An empty directory: asking for the version must not need a
			// project configuration, let alone generate anything.
			cmd.Dir = t.TempDir()

			var stdout, stderr bytes.Buffer

			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			if err := cmd.Run(); err != nil {
				t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
			}

			if stdout.String() != want {
				t.Errorf("got %q, want %q", stdout.String(), want)
			}
		})
	}
}
