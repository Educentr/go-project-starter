// Package refrewrite rewrites cross-directory $ref values in OpenAPI/JSON
// Schema files to local sibling references. It is invoked after the generator
// copies remote/local specs into a flat target directory (e.g.
// api/rest/<svc>/<ver>/ or api/schema/<name>/): any $ref whose basename
// matches a file already in that directory is rewritten to "./<basename>#<frag>".
//
// The package supports both YAML (.yaml/.yml) and JSON (.json) inputs. YAML
// processing uses gopkg.in/yaml.v3 with yaml.Node so head/line/foot comments
// survive the round-trip. JSON processing uses a line-based regex over
// `"$ref": "..."` literals, which is safe because JSON does not allow
// multi-line strings or comments.
package refrewrite

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Options configures RewriteLocalRefs behaviour.
type Options struct {
	// Logf, when non-nil, is used for per-file diagnostics. Defaults to log.Printf.
	Logf func(format string, args ...any)
	// DryRun computes the report without writing any files.
	DryRun bool
}

// Report summarises a RewriteLocalRefs run.
type Report struct {
	FilesScanned  int
	FilesModified int
	RefsRewritten int
	RefsSkipped   int
	Warnings      []string
}

// fileRewriter parses src bytes and returns (newBytes, modified, error).
// Per-ref counters and warnings are written into report.
type fileRewriter func(src []byte, dir string, allowed map[string]struct{}, fileName string, report *Report) ([]byte, bool, error)

// RewriteLocalRefs scans dir (non-recursive) for .yaml/.yml/.json files and
// rewrites each $ref whose basename(<path>) exists in dir (and, if expected is
// non-nil, is also present in expected) into "./<basename>#<frag>".
//
// expected, when non-nil, restricts which sibling basenames are eligible —
// passing transport.SpecTargetFiles prevents accidental matches against
// user-authored files that happen to share a name with a referenced spec.
// Passing nil disables the whitelist and relies on os.Stat alone.
func RewriteLocalRefs(dir string, expected []string, opts Options) (Report, error) {
	report := Report{}

	logf := opts.Logf
	if logf == nil {
		logf = log.Printf
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return report, fmt.Errorf("refrewrite: read dir %s: %w", dir, err)
	}

	var allowed map[string]struct{}
	if expected != nil {
		allowed = make(map[string]struct{}, len(expected))
		for _, name := range expected {
			if name == "" {
				continue
			}
			allowed[filepath.Base(name)] = struct{}{}
		}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))

		var rewriter fileRewriter
		switch ext {
		case ".yaml", ".yml":
			rewriter = rewriteYAMLBytes
		case ".json":
			rewriter = rewriteJSONBytes
		default:
			continue
		}

		full := filepath.Join(dir, name)
		report.FilesScanned++

		src, err := os.ReadFile(full)
		if err != nil {
			return report, fmt.Errorf("refrewrite: read %s: %w", full, err)
		}

		beforeRefs := report.RefsRewritten
		beforeSkipped := report.RefsSkipped

		out, modified, rerr := rewriter(src, dir, allowed, name, &report)
		if rerr != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("%s: %v", name, rerr))
			continue
		}

		if !modified {
			continue
		}

		if !opts.DryRun {
			if err := os.WriteFile(full, out, 0o644); err != nil {
				return report, fmt.Errorf("refrewrite: write %s: %w", full, err)
			}
		}

		report.FilesModified++
		rewrote := report.RefsRewritten - beforeRefs
		skipped := report.RefsSkipped - beforeSkipped
		logf("rewrite refs: %s (rewrote=%d, skipped=%d)", name, rewrote, skipped)
	}

	return report, nil
}
