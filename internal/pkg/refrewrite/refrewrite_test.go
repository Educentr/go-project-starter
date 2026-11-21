package refrewrite

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// fixtureFile describes a file written to the test working directory.
type fixtureFile struct {
	name    string
	content string
}

// caseSpec describes a single rewrite scenario.
type caseSpec struct {
	name          string
	files         []fixtureFile
	target        string // file that gets passed through the rewriter (others are siblings)
	expected      string // expected content of `target` after rewrite
	expectedRefs  int    // RefsRewritten
	expectedSkip  int    // RefsSkipped
	expectModif   bool   // whether `target` should change on disk
	expectedFiles int    // Report.FilesModified (counted even in DryRun, see refrewrite.go)
	expectedWarns int    // expected number of warnings
	whitelist     []string
	dryRun        bool
}

func TestRewriteLocalRefs(t *testing.T) {
	t.Parallel()

	cases := []caseSpec{
		{
			name: "external_parent_yaml",
			files: []fixtureFile{
				{name: "common.yaml", content: "components:\n  schemas:\n    Shared:\n      type: object\n"},
				{name: "api.yaml", content: "" +
					"components:\n" +
					"  schemas:\n" +
					"    Foo:\n" +
					"      allOf:\n" +
					"        - $ref: '../common/common.yaml#/components/schemas/Shared'\n",
				},
			},
			target: "api.yaml",
			expected: "" +
				"components:\n" +
				"  schemas:\n" +
				"    Foo:\n" +
				"      allOf:\n" +
				"        - $ref: ./common.yaml#/components/schemas/Shared\n",
			expectedRefs:  1,
			expectModif:   true,
			expectedFiles: 1,
		},
		{
			name: "external_subdir_yaml",
			files: []fixtureFile{
				{name: "shared.yaml", content: "components: {schemas: {X: {type: integer}}}\n"},
				{name: "api.yaml", content: "" +
					"components:\n" +
					"  schemas:\n" +
					"    Wrap:\n" +
					"      $ref: 'sub/shared.yaml#/components/schemas/X'\n",
				},
			},
			target: "api.yaml",
			expected: "" +
				"components:\n" +
				"  schemas:\n" +
				"    Wrap:\n" +
				"      $ref: ./shared.yaml#/components/schemas/X\n",
			expectedRefs:  1,
			expectModif:   true,
			expectedFiles: 1,
		},
		{
			name: "external_dot_slash_yaml",
			files: []fixtureFile{
				{name: "shared.yaml", content: "components: {schemas: {X: {type: integer}}}\n"},
				{name: "api.yaml", content: "" +
					"components:\n" +
					"  schemas:\n" +
					"    Wrap:\n" +
					"      $ref: './shared.yaml#/components/schemas/X'\n",
				},
			},
			target: "api.yaml",
			// Already flat — no change.
			expected: "" +
				"components:\n" +
				"  schemas:\n" +
				"    Wrap:\n" +
				"      $ref: './shared.yaml#/components/schemas/X'\n",
			expectModif: false,
		},
		{
			name: "internal_only_yaml",
			files: []fixtureFile{
				{name: "api.yaml", content: "" +
					"components:\n" +
					"  schemas:\n" +
					"    Wrap:\n" +
					"      $ref: '#/components/schemas/X'\n",
				},
			},
			target: "api.yaml",
			expected: "" +
				"components:\n" +
				"  schemas:\n" +
				"    Wrap:\n" +
				"      $ref: '#/components/schemas/X'\n",
			expectModif: false,
		},
		{
			name: "external_http_yaml",
			files: []fixtureFile{
				{name: "api.yaml", content: "" +
					"components:\n" +
					"  schemas:\n" +
					"    Wrap:\n" +
					"      $ref: 'https://example.com/x.yaml#/X'\n",
				},
			},
			target: "api.yaml",
			expected: "" +
				"components:\n" +
				"  schemas:\n" +
				"    Wrap:\n" +
				"      $ref: 'https://example.com/x.yaml#/X'\n",
			expectModif:   false,
			expectedSkip:  1,
			expectedWarns: 1,
		},
		{
			name: "no_match_yaml",
			files: []fixtureFile{
				{name: "api.yaml", content: "" +
					"components:\n" +
					"  schemas:\n" +
					"    Wrap:\n" +
					"      $ref: '../missing/zzz.yaml#/X'\n",
				},
			},
			target: "api.yaml",
			expected: "" +
				"components:\n" +
				"  schemas:\n" +
				"    Wrap:\n" +
				"      $ref: '../missing/zzz.yaml#/X'\n",
			expectModif:   false,
			expectedSkip:  1,
			expectedWarns: 1,
		},
		{
			name: "comments_preserved_yaml",
			files: []fixtureFile{
				{name: "common.yaml", content: "components: {schemas: {Shared: {type: object}}}\n"},
				{name: "api.yaml", content: "" +
					"# header comment\n" +
					"components:\n" +
					"  schemas:\n" +
					"    # inline before key\n" +
					"    Foo:\n" +
					"      # before $ref\n" +
					"      $ref: '../common/common.yaml#/components/schemas/Shared' # tail\n",
				},
			},
			target: "api.yaml",
			// We only assert that comments survive; expected is loosely compared below.
			expectedRefs:  1,
			expectModif:   true,
			expectedFiles: 1,
		},
		{
			name: "multiple_refs_one_file_yaml",
			files: []fixtureFile{
				{name: "common.yaml", content: "components: {schemas: {Shared: {type: object}}}\n"},
				{name: "api.yaml", content: "" +
					"components:\n" +
					"  schemas:\n" +
					"    A:\n" +
					"      $ref: '../common/common.yaml#/components/schemas/Shared'\n" +
					"    B:\n" +
					"      $ref: '../missing/zzz.yaml#/X'\n" +
					"    C:\n" +
					"      $ref: '#/components/schemas/A'\n",
				},
			},
			target: "api.yaml",
			expected: "" +
				"components:\n" +
				"  schemas:\n" +
				"    A:\n" +
				"      $ref: ./common.yaml#/components/schemas/Shared\n" +
				"    B:\n" +
				"      $ref: '../missing/zzz.yaml#/X'\n" +
				"    C:\n" +
				"      $ref: '#/components/schemas/A'\n",
			expectedRefs:  1,
			expectedSkip:  1,
			expectModif:   true,
			expectedFiles: 1,
			expectedWarns: 1,
		},
		{
			name: "whitelist_blocks_unknown_match_yaml",
			files: []fixtureFile{
				{name: "common.yaml", content: "components: {schemas: {Shared: {type: object}}}\n"},
				{name: "api.yaml", content: "" +
					"components:\n" +
					"  schemas:\n" +
					"    Foo:\n" +
					"      $ref: '../common/common.yaml#/components/schemas/Shared'\n",
				},
			},
			target: "api.yaml",
			expected: "" +
				"components:\n" +
				"  schemas:\n" +
				"    Foo:\n" +
				"      $ref: '../common/common.yaml#/components/schemas/Shared'\n",
			whitelist:     []string{"api.yaml"}, // common.yaml deliberately omitted
			expectModif:   false,
			expectedSkip:  1,
			expectedWarns: 1,
		},
		{
			name: "json_external_parent",
			files: []fixtureFile{
				{name: "common.json", content: "{\"$defs\":{\"X\":{\"type\":\"integer\"}}}\n"},
				{name: "api.json", content: "" +
					"{\n" +
					"  \"$defs\": {\n" +
					"    \"Foo\": { \"$ref\": \"../common/common.json#/$defs/X\" }\n" +
					"  }\n" +
					"}\n",
				},
			},
			target: "api.json",
			expected: "" +
				"{\n" +
				"  \"$defs\": {\n" +
				"    \"Foo\": { \"$ref\": \"./common.json#/$defs/X\" }\n" +
				"  }\n" +
				"}\n",
			expectedRefs:  1,
			expectModif:   true,
			expectedFiles: 1,
		},
		{
			name: "json_no_match",
			files: []fixtureFile{
				{name: "api.json", content: "" +
					"{\n" +
					"  \"$defs\": {\n" +
					"    \"Foo\": { \"$ref\": \"../missing/zzz.json#/X\" }\n" +
					"  }\n" +
					"}\n",
				},
			},
			target: "api.json",
			expected: "" +
				"{\n" +
				"  \"$defs\": {\n" +
				"    \"Foo\": { \"$ref\": \"../missing/zzz.json#/X\" }\n" +
				"  }\n" +
				"}\n",
			expectModif:   false,
			expectedSkip:  1,
			expectedWarns: 1,
		},
		{
			name: "dry_run_yaml",
			files: []fixtureFile{
				{name: "common.yaml", content: "components: {schemas: {Shared: {type: object}}}\n"},
				{name: "api.yaml", content: "" +
					"components:\n" +
					"  schemas:\n" +
					"    Foo:\n" +
					"      $ref: '../common/common.yaml#/components/schemas/Shared'\n",
				},
			},
			target: "api.yaml",
			// File on disk should remain unchanged.
			expected: "" +
				"components:\n" +
				"  schemas:\n" +
				"    Foo:\n" +
				"      $ref: '../common/common.yaml#/components/schemas/Shared'\n",
			dryRun:       true,
			expectedRefs: 1,
			// DryRun still reports the file as modified, but must not touch disk.
			expectedFiles: 1,
			expectModif:   false,
		},
		{
			name: "invalid_yaml",
			files: []fixtureFile{
				{name: "api.yaml", content: ": : : invalid : ::: :\n  bad\nyaml::::\n"},
			},
			target:        "api.yaml",
			expectModif:   false,
			expectedWarns: 1,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for _, f := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, f.name), []byte(f.content), 0o644); err != nil {
					t.Fatalf("write fixture %s: %v", f.name, err)
				}
			}

			targetPath := filepath.Join(dir, tc.target)

			// Snapshot the on-disk content so "was the file written?" can be
			// answered by comparing bytes. Deliberately not mtime: its
			// granularity is a filesystem property (on overlayfs two writes
			// milliseconds apart land in the same timestamp tick), which made
			// this assertion flaky.
			beforeBytes, err := os.ReadFile(targetPath)
			if err != nil {
				t.Fatalf("read target before rewrite: %v", err)
			}

			var logged []string
			report, err := RewriteLocalRefs(dir, tc.whitelist, Options{
				DryRun: tc.dryRun,
				Logf: func(format string, args ...any) {
					logged = append(logged, strings.TrimSpace(strings.TrimRight(strings.ReplaceAll(formatLine(format, args...), "\n", " "), " ")))
				},
			})
			if err != nil {
				t.Fatalf("RewriteLocalRefs returned error: %v", err)
			}

			if report.RefsRewritten != tc.expectedRefs {
				t.Errorf("RefsRewritten = %d, want %d (warnings=%v)", report.RefsRewritten, tc.expectedRefs, report.Warnings)
			}
			if report.RefsSkipped != tc.expectedSkip {
				t.Errorf("RefsSkipped = %d, want %d (warnings=%v)", report.RefsSkipped, tc.expectedSkip, report.Warnings)
			}
			if len(report.Warnings) != tc.expectedWarns {
				t.Errorf("Warnings count = %d, want %d: %v", len(report.Warnings), tc.expectedWarns, report.Warnings)
			}

			gotBytes, err := os.ReadFile(targetPath)
			if err != nil {
				t.Fatalf("read target after rewrite: %v", err)
			}
			got := string(gotBytes)

			if tc.expected != "" {
				if tc.name == "comments_preserved_yaml" {
					// Loose check: just confirm rewrite happened and comments survived.
					if !strings.Contains(got, "$ref: ./common.yaml#/components/schemas/Shared") {
						t.Errorf("expected rewritten ref in output, got:\n%s", got)
					}
					if !strings.Contains(got, "# header comment") {
						t.Errorf("expected header comment preserved, got:\n%s", got)
					}
					if !strings.Contains(got, "# before $ref") {
						t.Errorf("expected line comment before $ref preserved, got:\n%s", got)
					}
				} else if got != tc.expected {
					t.Errorf("target content mismatch.\nGot:\n%s\nWant:\n%s", got, tc.expected)
				}
			}

			if modified := got != string(beforeBytes); modified != tc.expectModif {
				t.Errorf("target content changed on disk=%v, want %v", modified, tc.expectModif)
			}

			if report.FilesModified != tc.expectedFiles {
				t.Errorf("FilesModified = %d, want %d (warnings=%v)", report.FilesModified, tc.expectedFiles, report.Warnings)
			}

			// FilesScanned should be at least 1 for every test (we always
			// place `target` in dir).
			if report.FilesScanned == 0 {
				t.Errorf("FilesScanned = 0, want >= 1")
			}

			// Spot-check that log messages were produced when files were modified.
			if tc.expectModif {
				found := false
				for _, line := range logged {
					if strings.Contains(line, "rewrite refs:") && strings.Contains(line, tc.target) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected per-file log entry for %s, got: %v", tc.target, logged)
				}
			}
		})
	}
}

func TestEvaluateRefAlgorithm(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	must := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("ok"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	must("common.yaml")

	// A directory named like a spec: existence alone would make the ref eligible,
	// and the rewritten "./dir.yaml#/X" would resolve to nothing.
	if err := os.Mkdir(filepath.Join(dir, "dir.yaml"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// A symlink to a spec is a fine rewrite target: whoever resolves the
	// rewritten ref follows it just as os.Stat does.
	if err := os.Symlink(filepath.Join(dir, "common.yaml"), filepath.Join(dir, "link.yaml")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	allowed := map[string]struct{}{"common.yaml": {}, "dir.yaml": {}, "link.yaml": {}}

	tests := []struct {
		raw     string
		want    string
		rewrote bool
		skip    bool
	}{
		{raw: "", want: "", rewrote: false, skip: false},
		{raw: "#/components/schemas/X", want: "", rewrote: false, skip: false},
		{raw: "https://example.com/x.yaml#/X", want: "", rewrote: false, skip: true},
		{raw: "common.yaml#/X", want: "", rewrote: false, skip: false},
		{raw: "../common/common.yaml#/X", want: "./common.yaml#/X", rewrote: true, skip: false},
		{raw: "../common/common.yaml", want: "./common.yaml", rewrote: true, skip: false},
		{raw: "sub/common.yaml#/X", want: "./common.yaml#/X", rewrote: true, skip: false},
		{raw: "../missing/zzz.yaml#/X", want: "", rewrote: false, skip: true},
		{raw: "../common/dir.yaml#/X", want: "", rewrote: false, skip: true},
		{raw: "../common/link.yaml#/X", want: "./link.yaml#/X", rewrote: true, skip: false},
	}

	for _, tc := range tests {
		got := evaluateRef(tc.raw, dir, allowed)
		if got.Rewrote != tc.rewrote {
			t.Errorf("evaluateRef(%q).Rewrote = %v, want %v", tc.raw, got.Rewrote, tc.rewrote)
		}
		if tc.rewrote && got.NewValue != tc.want {
			t.Errorf("evaluateRef(%q).NewValue = %q, want %q", tc.raw, got.NewValue, tc.want)
		}
		hasSkip := got.SkipReason != ""
		if hasSkip != tc.skip {
			t.Errorf("evaluateRef(%q).SkipReason = %q (skip=%v, want %v)", tc.raw, got.SkipReason, hasSkip, tc.skip)
		}
	}
}

func TestWarningTaggedByFilename(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Use ref basenames that do NOT match any sibling so we get warnings.
	files := []fixtureFile{
		{name: "owner_a.yaml", content: "x:\n  $ref: '../missing/zzz.yaml#/X'\n"},
		{name: "owner_b.yaml", content: "x:\n  $ref: '../missing/qqq.yaml#/X'\n"},
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte(f.content), 0o644); err != nil {
			t.Fatalf("write %s: %v", f.name, err)
		}
	}

	report, err := RewriteLocalRefs(dir, nil, Options{Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(report.Warnings) != 2 {
		t.Fatalf("warnings = %d, want 2 (%v)", len(report.Warnings), report.Warnings)
	}
	if report.RefsRewritten != 0 {
		t.Errorf("expected no rewrites, got %d", report.RefsRewritten)
	}

	// Order depends on directory iteration; just confirm one warning per file.
	names := []string{report.Warnings[0], report.Warnings[1]}
	sort.Strings(names)
	if !strings.HasPrefix(names[0], "owner_a.yaml") || !strings.HasPrefix(names[1], "owner_b.yaml") {
		t.Errorf("warnings not properly tagged by filename: %v", names)
	}
}

// TestNonRegularEntriesSkipped covers the scan's lstat blind spot: os.ReadDir
// reports a symlink with IsDir() == false, so a link carrying a spec extension
// used to reach os.ReadFile. Pointing at a directory that failed the whole run
// with "is a directory"; pointing at a regular file the write-back followed the
// link and rewrote a file outside the project tree.
func TestNonRegularEntriesSkipped(t *testing.T) {
	t.Parallel()

	base := t.TempDir()

	// Both link targets live outside the scanned directory, as a symlinked
	// sibling repo clone would.
	outsideDir := filepath.Join(base, "outside", "specs")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	outsideSpec := filepath.Join(base, "outside", "shared.yaml")
	outsideContent := "x:\n  $ref: '../common/common.yaml#/X'\n"

	if err := os.WriteFile(outsideSpec, []byte(outsideContent), 0o644); err != nil {
		t.Fatalf("write outside spec: %v", err)
	}

	dir := filepath.Join(base, "v1")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// A sibling the links' refs could resolve against, so a followed link would
	// actually be rewritten rather than merely skipped.
	if err := os.WriteFile(filepath.Join(dir, "common.yaml"), []byte("X:\n  type: object\n"), 0o644); err != nil {
		t.Fatalf("write sibling: %v", err)
	}

	if err := os.Symlink(outsideDir, filepath.Join(dir, "dir-link.yaml")); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}

	if err := os.Symlink(outsideSpec, filepath.Join(dir, "file-link.yaml")); err != nil {
		t.Fatalf("symlink file: %v", err)
	}

	report, err := RewriteLocalRefs(dir, nil, Options{Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.FilesModified != 0 || report.RefsRewritten != 0 {
		t.Errorf("links must not be rewritten: FilesModified=%d, RefsRewritten=%d",
			report.FilesModified, report.RefsRewritten)
	}

	got, err := os.ReadFile(outsideSpec)
	if err != nil {
		t.Fatalf("read outside spec: %v", err)
	}

	if string(got) != outsideContent {
		t.Errorf("file outside the scanned tree was rewritten through the link:\n%s", got)
	}

	// Each skipped link is reported: silently ignoring a spec-looking entry would
	// leave the author wondering why their refs stayed external.
	names := append([]string{}, report.Warnings...)
	sort.Strings(names)

	if len(names) != 2 ||
		!strings.HasPrefix(names[0], "dir-link.yaml") ||
		!strings.HasPrefix(names[1], "file-link.yaml") {
		t.Errorf("expected one warning per link, got %v", report.Warnings)
	}
}

// formatLine renders a Logf invocation back into a single string for tests.
func formatLine(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
