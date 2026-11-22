package test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Educentr/go-project-starter/internal/pkg/meta"
)

// readMeta reads the manifest of a generated project.
func readMeta(t *testing.T, targetDir string) meta.Meta {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(targetDir, ".project-config", "meta.yaml"))
	if err != nil {
		t.Fatalf("read meta.yaml: %v", err)
	}

	var m meta.Meta

	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse meta.yaml: %v\n%s", err, data)
	}

	return m
}

// configDigest is the digest of a file in a test config directory — the bytes
// the source served, which is what the provenance of a copy has to carry.
func configDigest(t *testing.T, configName string, parts ...string) string {
	t.Helper()

	curDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(append([]string{curDir, "..", "test", "docker-integration", "configs", configName}, parts...)...)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return meta.HashContent(content)
}

func recordOf(t *testing.T, m meta.Meta, path string) meta.FileRecord {
	t.Helper()

	rec, ok := m.Manifest.Index()[path]
	if !ok {
		t.Fatalf("manifest has no record for %s:\n%+v", path, m.Manifest)
	}

	return rec
}

// TestManifestRecordsProvenanceOfEveryCopiedSpec covers the request behind
// TOOLS-102: every file that went through the source resolver is recorded with
// where it came from, not just the specs of the `contract:` section. Without it
// a consumer reading the manifest can prove which tree of contracts a service
// was generated from for contracts and for nothing else.
func TestManifestRecordsProvenanceOfEveryCopiedSpec(t *testing.T) {
	tmpDir := generateFromConfig(t, "custom-generator", "Generate project with contracts and a REST spec")

	m := readMeta(t, tmpDir)

	if m.Version != meta.CurrentVersion {
		t.Errorf("manifest version = %d, want %d", m.Version, meta.CurrentVersion)
	}

	cases := []struct {
		path   string
		source string
		file   string
	}{
		{
			// The REST channel: before this change its record carried the path
			// and the hash of the copy, and nothing about the source.
			path:   "api/rest/api/v1/api.swagger.yml",
			source: "./api.swagger.yml",
			file:   "api.swagger.yml",
		},
		{
			path:   "api/contract/runtime_limits.yaml",
			source: "./runtime_limits.yaml",
			file:   "runtime_limits.yaml",
		},
		{
			path:   "api/contract/codex_broker.swagger.yml",
			source: "./codex_broker.swagger.yml",
			file:   "codex_broker.swagger.yml",
		},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			rec := recordOf(t, m, tc.path)

			if rec.Kind != meta.KindCopied {
				t.Errorf("kind = %q, want %q", rec.Kind, meta.KindCopied)
			}

			if rec.SourceKind != meta.SourceKindLocal {
				t.Errorf("source_kind = %q, want %q", rec.SourceKind, meta.SourceKindLocal)
			}

			if rec.SourcePath != tc.source {
				t.Errorf("source_path = %q, want %q", rec.SourcePath, tc.source)
			}

			// A local source has no repository, ref or commit to speak of.
			if rec.SourceRepo != "" || rec.Ref != "" || rec.Commit != "" {
				t.Errorf("record = %+v, want no repository/ref/commit for a local source", rec.Provenance)
			}

			if want := configDigest(t, "custom-generator", tc.file); rec.SourceSHA256 != want {
				t.Errorf("source_sha256 = %q, want the digest of the source file %q", rec.SourceSHA256, want)
			}

			// Copied as-is and not rewritten, so the declared rule holds: the
			// digest of the source and the hash of the copy are the same.
			if rec.Hash != rec.SourceSHA256 {
				t.Errorf("hash %q and source_sha256 %q must agree for a file copied as is",
					rec.Hash, rec.SourceSHA256)
			}
		})
	}

	// A file produced from a template keeps exactly the shape it had before:
	// no provenance, because it has no source.
	gen := recordOf(t, m, "internal/app/constant/psg_constant_gen.go")

	if !gen.Provenance.IsZero() {
		t.Errorf("generated record = %+v, want no provenance", gen.Provenance)
	}
}

// The digest of a rewritten spec is the point of recording it separately:
// `rewrite_refs` edits the copy inside the project, so its hash cannot be
// compared with the blob in the contract repository, while the digest of the
// bytes the source served can.
func TestManifestSourceDigestSurvivesRefRewrite(t *testing.T) {
	tmpDir := generateFromConfig(t, "rest-only-rewrite-refs", "Generate project with rewritten spec refs")

	m := readMeta(t, tmpDir)

	rec := recordOf(t, m, "api/rest/api/v1/api.swagger.yml")

	want := configDigest(t, "rest-only-rewrite-refs", "orchestrator", "api.swagger.yml")

	if rec.SourceSHA256 != want {
		t.Errorf("source_sha256 = %q, want the digest of the spec as the source served it (%q)",
			rec.SourceSHA256, want)
	}

	if rec.Hash == rec.SourceSHA256 {
		t.Error("the spec had its refs rewritten, so the hash of the copy must differ from the source digest")
	}

	if rec.SourcePath != "./orchestrator/api.swagger.yml" {
		t.Errorf("source_path = %q, want the path as project.yaml writes it", rec.SourcePath)
	}
}

// Inputs the generator reads without copying them into the project — the
// metrics map, the OnlineConf key map, the lint overrides, the CLI and
// queue-worker specs — leave no file record, so the `inputs` section is the
// only place their provenance can live. The question a consumer asks is about
// them just as much as about a copied spec.
func TestManifestRecordsInlineInputs(t *testing.T) {
	cases := []struct {
		config  string
		msg     string
		channel string
		source  string
		file    []string
	}{
		{
			config:  "metrics-map",
			msg:     "Generate project with a metrics map",
			channel: "metrics.path",
			source:  "./metrics-map.yaml",
			file:    []string{"metrics-map.yaml"},
		},
		{
			config:  "onlineconf-map",
			msg:     "Generate project with an OnlineConf key map",
			channel: "onlineconf.path",
			source:  "./onlineconf-key-map.yaml",
			file:    []string{"onlineconf-key-map.yaml"},
		},
		{
			config:  "lint-config-overrides",
			msg:     "Generate project with lint overrides",
			channel: "tools.lint_config.overrides",
			source:  "./golangci-overrides.yml",
			file:    []string{"golangci-overrides.yml"},
		},
		{
			config:  "worker-queue",
			msg:     "Generate project with a queue worker",
			channel: "workers.task_processor.path",
			source:  "./queues.yaml",
			file:    []string{"queues.yaml"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.config, func(t *testing.T) {
			tmpDir := generateFromConfig(t, tc.config, tc.msg)

			m := readMeta(t, tmpDir)

			var found *meta.InputRecord

			for i, rec := range m.Inputs {
				if rec.Channel == tc.channel {
					found = &m.Inputs[i]

					break
				}
			}

			if found == nil {
				t.Fatalf("inputs = %+v, want a record for channel %q", m.Inputs, tc.channel)
			}

			if found.SourceKind != meta.SourceKindLocal || found.SourcePath != tc.source {
				t.Errorf("input = %+v, want the local source %q", found.Provenance, tc.source)
			}

			if want := configDigest(t, tc.config, tc.file...); found.SourceSHA256 != want {
				t.Errorf("source_sha256 = %q, want the digest of the input file (%q)", found.SourceSHA256, want)
			}
		})
	}
}

// A project that reads nothing through the resolver beyond its specs has no
// `inputs` section at all: the key is omitted rather than written empty, so the
// manifest of such a project keeps the shape it had.
func TestManifestWithoutInlineInputs(t *testing.T) {
	tmpDir := generateFromConfig(t, "rest-only", "Generate project without inline inputs")

	if inputs := readMeta(t, tmpDir).Inputs; len(inputs) != 0 {
		t.Errorf("inputs = %+v, want none", inputs)
	}

	if data := readGenerated(t, tmpDir, ".project-config", "meta.yaml"); strings.Contains(data, "inputs:") {
		t.Errorf("meta.yaml should carry no inputs section:\n%s", data)
	}
}

// A project generated before provenance was split into separate fields must
// upgrade on its own: the records of version 5 carry a single `source` string
// (and no digest of the source), and nothing about the tree changed — so the
// run that reads them has to go through, not report drift or a conflict, and
// write the current format.
func TestManifestUpgradesFromV5(t *testing.T) {
	configDir := writableConfigDir(t, "custom-generator")
	tmpDir := t.TempDir()

	if out, err := generateInto(t, configDir, tmpDir, "Generate project to downgrade its manifest"); err != nil {
		t.Fatalf("first generation failed: %s\n%s", err, out)
	}

	metaPath := filepath.Join(tmpDir, ".project-config", "meta.yaml")

	// Roll the manifest back to the shape version 5 wrote: one `source` string
	// on the copied records, no source_kind/source_path/source_sha256.
	legacy := regexp.MustCompile(`(?m)^(\s+- \{path: [^,]+, kind: copied, hash: [0-9a-f]+), source_kind: [^}]*\}$`)

	editFile(t, metaPath, func(s string) string {
		s = strings.Replace(s, "version: 6", "version: 5", 1)

		return legacy.ReplaceAllString(s, "$1, source: ./legacy.yaml}")
	})

	out, err := generateInto(t, configDir, tmpDir, "Regenerate project from a version 5 manifest")
	if err != nil {
		t.Fatalf("regeneration from version 5 failed: %s\n%s", err, out)
	}

	for _, unwanted := range []string{"edited inside the project", "conflict:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("the upgrade must not report %q:\n%s", unwanted, out)
		}
	}

	m := readMeta(t, tmpDir)

	if m.Version != meta.CurrentVersion {
		t.Errorf("version = %d, want %d", m.Version, meta.CurrentVersion)
	}

	rec := recordOf(t, m, "api/contract/runtime_limits.yaml")

	if rec.SourcePath != "./runtime_limits.yaml" || rec.SourceSHA256 == "" {
		t.Errorf("record = %+v, want the provenance of this run", rec.Provenance)
	}
}
