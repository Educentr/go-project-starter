package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/spf13/viper"
)

// buildArgsString renders parsed build args in a comparable form:
// "<name>|<default>|<stage>,<stage>".
func buildArgsString(args []ds.DockerBuildArg) string {
	parts := make([]string, 0, len(args))

	for _, a := range args {
		parts = append(parts, a.Name+"|"+a.Default+"|"+strings.Join(a.Stages, ","))
	}

	return strings.Join(parts, " ")
}

// labelsString renders parsed labels in a comparable form: "<key>=<value>".
func labelsString(labels []ds.DockerLabel) string {
	parts := make([]string, 0, len(labels))

	for _, l := range labels {
		parts = append(parts, l.Key+"="+l.Value)
	}

	return strings.Join(parts, " ")
}

func TestDocker_ParseBuildArgs(t *testing.T) {
	tests := []struct {
		name      string
		buildArgs []any
		want      string
		wantErr   string
	}{
		{
			name:      "omitted section",
			buildArgs: nil,
			want:      "",
		},
		{
			name:      "short form defaults to the final stage",
			buildArgs: []any{"IMAGE_VERSION"},
			want:      "IMAGE_VERSION||final",
		},
		{
			name: "full form keeps default and stages",
			buildArgs: []any{map[string]any{
				"name":    "IMAGE_REVISION",
				"default": "$(shell git rev-parse HEAD)",
				"stages":  []any{"builder", "final"},
			}},
			want: "IMAGE_REVISION|$(shell git rev-parse HEAD)|builder,final",
		},
		{
			name: "stages are emitted in dockerfile order and deduplicated",
			buildArgs: []any{map[string]any{
				"name":   "A",
				"stages": []any{"final", "builder", "final"},
			}},
			want: "A||builder,final",
		},
		{
			name:      "mixed forms keep config order",
			buildArgs: []any{"B", map[string]any{"name": "A"}},
			want:      "B||final A||final",
		},
		{
			name:      "name with invalid characters is rejected",
			buildArgs: []any{"IMAGE-VERSION"},
			wantErr:   "docker.build_args: name must start with a letter",
		},
		{
			name:      "empty name is rejected",
			buildArgs: []any{""},
			wantErr:   "docker.build_args: name must start with a letter",
		},
		{
			name:      "name reserved by the dockerfile template is rejected",
			buildArgs: []any{"binary"},
			wantErr:   "docker.build_args: name is already used",
		},
		{
			name:      "name reserved by the makefile template is rejected",
			buildArgs: []any{"DOCKER_TAG"},
			wantErr:   "docker.build_args: name is already used",
		},
		{
			name:      "duplicate name is rejected",
			buildArgs: []any{"A", map[string]any{"name": "A"}},
			wantErr:   "docker.build_args: duplicate name",
		},
		{
			name:      "unknown key is rejected",
			buildArgs: []any{map[string]any{"name": "A", "stage": "final"}},
			wantErr:   "docker.build_args: unknown key",
		},
		{
			name:      "non-string name is rejected",
			buildArgs: []any{map[string]any{"name": 1}},
			wantErr:   "docker.build_args: name must start with a letter",
		},
		{
			name:      "neither a name nor an object is rejected",
			buildArgs: []any{42},
			wantErr:   "docker.build_args: expected a name or a {name, default, stages} object",
		},
		{
			name:      "non-string default is rejected",
			buildArgs: []any{map[string]any{"name": "A", "default": 1}},
			wantErr:   "docker.build_args: default must be a single-line string",
		},
		{
			// A '#' would turn the rest of the Makefile assignment into a comment.
			name:      "default with a comment character is rejected",
			buildArgs: []any{map[string]any{"name": "A", "default": "v1 # nope"}},
			wantErr:   "docker.build_args: default must be a single-line string",
		},
		{
			// The recipe wraps the value in double quotes.
			name:      "default with a double quote is rejected",
			buildArgs: []any{map[string]any{"name": "A", "default": `v"1`}},
			wantErr:   "docker.build_args: default must be a single-line string",
		},
		{
			name:      "multiline default is rejected",
			buildArgs: []any{map[string]any{"name": "A", "default": "v1\nv2"}},
			wantErr:   "docker.build_args: default must be a single-line string",
		},
		{
			name:      "unknown stage is rejected",
			buildArgs: []any{map[string]any{"name": "A", "stages": []any{"runtime"}}},
			wantErr:   "docker.build_args: unknown stage",
		},
		{
			name:      "empty stage list is rejected",
			buildArgs: []any{map[string]any{"name": "A", "stages": []any{}}},
			wantErr:   "docker.build_args: 'stages' must be a list",
		},
		{
			name:      "scalar stages is rejected",
			buildArgs: []any{map[string]any{"name": "A", "stages": "final"}},
			wantErr:   "docker.build_args: 'stages' must be a list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Docker{BuildArgs: tt.buildArgs}.Parse()

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Parse() error = nil, want error containing %q", tt.wantErr)
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Parse() error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}

			if buildArgsString(got.BuildArgs) != tt.want {
				t.Errorf("Parse() build args = %q, want %q", buildArgsString(got.BuildArgs), tt.want)
			}
		})
	}
}

func TestDocker_ParseLabels(t *testing.T) {
	tests := []struct {
		name      string
		buildArgs []any
		labels    map[string]any
		want      string
		wantErr   string
	}{
		{
			name:   "omitted section",
			labels: nil,
			want:   "",
		},
		{
			name:   "literal label",
			labels: map[string]any{"org.opencontainers.image.vendor": "Educentr"},
			want:   "org.opencontainers.image.vendor=Educentr",
		},
		{
			// viper splits a dotted key into nested maps; Parse joins it back.
			name:   "nested maps are flattened back into a dotted key",
			labels: map[string]any{"org": map[string]any{"vendor": "Educentr"}},
			want:   "org.vendor=Educentr",
		},
		{
			name: "labels are sorted by key",
			labels: map[string]any{
				"b.label": "second",
				"a.label": "first",
			},
			want: "a.label=first b.label=second",
		},
		{
			name:      "build arg reference declared in the final stage",
			buildArgs: []any{"IMAGE_VERSION"},
			labels:    map[string]any{"version": "${IMAGE_VERSION}"},
			want:      "version=${IMAGE_VERSION}",
		},
		{
			name:      "braceless build arg reference",
			buildArgs: []any{"IMAGE_VERSION"},
			labels:    map[string]any{"version": "v$IMAGE_VERSION"},
			want:      "version=v$IMAGE_VERSION",
		},
		{
			// $$ is the dockerfile escape of a literal dollar, not a reference.
			name:   "escaped dollar is not a reference",
			labels: map[string]any{"price": "$$IMAGE_VERSION"},
			want:   "price=$$IMAGE_VERSION",
		},
		{
			name:    "undeclared build arg reference is rejected",
			labels:  map[string]any{"version": "${IMAGE_VERSION}"},
			wantErr: `docker.labels: value references a build argument that is not declared`,
		},
		{
			// The builder stage value is not visible in the final stage, so the
			// label would render empty.
			name:      "build arg declared only in the builder stage is rejected",
			buildArgs: []any{map[string]any{"name": "IMAGE_VERSION", "stages": []any{"builder"}}},
			labels:    map[string]any{"version": "${IMAGE_VERSION}"},
			wantErr:   `docker.labels: value references a build argument that is not declared`,
		},
		{
			name:    "uppercase key is rejected",
			labels:  map[string]any{"MyLabel": "x"},
			wantErr: "docker.labels: key must be lowercase",
		},
		{
			name:    "key with invalid characters is rejected",
			labels:  map[string]any{"my label": "x"},
			wantErr: "docker.labels: key must be lowercase",
		},
		{
			name:    "non-string value is rejected",
			labels:  map[string]any{"version": 1.0},
			wantErr: "docker.labels: value must be a string",
		},
		{
			name:    "value with a double quote is rejected",
			labels:  map[string]any{"version": `v"1`},
			wantErr: "docker.labels: value must be a single-line string",
		},
		{
			name:    "multiline value is rejected",
			labels:  map[string]any{"version": "v1\nv2"},
			wantErr: "docker.labels: value must be a single-line string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Docker{BuildArgs: tt.buildArgs, Labels: tt.labels}.Parse()

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Parse() error = nil, want error containing %q", tt.wantErr)
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Parse() error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}

			if labelsString(got.Labels) != tt.want {
				t.Errorf("Parse() labels = %q, want %q", labelsString(got.Labels), tt.want)
			}
		})
	}
}

func TestDocker_IsValid(t *testing.T) {
	ok, msg := Docker{ImagePrefix: "registry.example/org"}.IsValid()
	if !ok {
		t.Errorf("IsValid() for a section without build args = false (%s), want true", msg)
	}

	ok, msg = Docker{BuildArgs: []any{"GOPROXY"}}.IsValid()
	if ok {
		t.Error("IsValid() for a reserved build arg name = true, want false")
	}

	if !strings.Contains(msg, "GOPROXY") {
		t.Errorf("IsValid() msg = %q, want it to name the offending argument", msg)
	}
}

// TestDocker_ViperDecoding pins the shapes viper actually produces for the
// docker section: the short and the full build arg form, and the dotted label
// keys viper turns into nested maps. Parse must keep matching them.
func TestDocker_ViperDecoding(t *testing.T) {
	const projectYAML = `
docker:
  image_prefix: registry.example/org
  build_args:
    - IMAGE_VERSION
    - name: IMAGE_REVISION
      default: $(shell git rev-parse HEAD)
      stages: [builder, final]
  labels:
    org.opencontainers.image.version: ${IMAGE_VERSION}
    org.opencontainers.image.revision: ${IMAGE_REVISION}
`

	path := filepath.Join(t.TempDir(), "project.yaml")
	if err := os.WriteFile(path, []byte(projectYAML), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	v := viper.New()
	v.SetConfigFile(path)

	if err := v.ReadInConfig(); err != nil {
		t.Fatalf("read config: %v", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	got, err := cfg.Docker.Parse()
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if got.ImagePrefix != "registry.example/org" {
		t.Errorf("Parse() image prefix = %q, want %q", got.ImagePrefix, "registry.example/org")
	}

	wantArgs := "IMAGE_VERSION||final IMAGE_REVISION|$(shell git rev-parse HEAD)|builder,final"
	if buildArgsString(got.BuildArgs) != wantArgs {
		t.Errorf("Parse() build args = %q, want %q", buildArgsString(got.BuildArgs), wantArgs)
	}

	wantLabels := "org.opencontainers.image.revision=${IMAGE_REVISION} " +
		"org.opencontainers.image.version=${IMAGE_VERSION}"
	if labelsString(got.Labels) != wantLabels {
		t.Errorf("Parse() labels = %q, want %q", labelsString(got.Labels), wantLabels)
	}
}
