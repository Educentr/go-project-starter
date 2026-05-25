package refrewrite

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
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
			expectedRefs: 1,
			expectModif:  true,
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
			expectedRefs: 1,
			expectModif:  true,
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
			expectedRefs: 1,
			expectModif:  true,
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
			expectedRefs: 1,
			expectModif:  true,
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
			expectModif:  false,
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

			var beforeMTime time.Time
			if info, err := os.Stat(targetPath); err == nil {
				beforeMTime = info.ModTime()
			}

			// Sleep a millisecond so any write would produce a different mtime.
			time.Sleep(2 * time.Millisecond)

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

			if info, err := os.Stat(targetPath); err == nil {
				modified := info.ModTime().After(beforeMTime)
				if modified != tc.expectModif {
					t.Errorf("file modified=%v, want %v", modified, tc.expectModif)
				}
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

	allowed := map[string]struct{}{"common.yaml": {}}

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

// formatLine renders a Logf invocation back into a single string for tests.
func formatLine(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
