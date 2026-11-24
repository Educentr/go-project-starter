package config

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
)

const (
	digestA = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	digestB = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	archURL = "https://example.com/tool-linux-${TARGETARCH}"
)

// TestAppDocker_ParseRuntimeAdditions covers the keys that let a project add to
// the runtime stage of its image without a hand-written Dockerfile: `copy`,
// `env`, `fetch` and `stages`.
func TestAppDocker_ParseRuntimeAdditions(t *testing.T) {
	tests := []struct {
		name    string
		docker  AppDocker
		want    ds.AppDockerConfig
		wantErr error
	}{
		{
			name:   "no addition is a zero value",
			docker: AppDocker{},
			want:   ds.AppDockerConfig{},
		},
		{
			name: "project directory and the variable pointing at it",
			docker: AppDocker{
				Copy: []AppDockerCopyEntry{{From: "template", To: "/opt/api/template"}},
				Env:  []AppDockerEnvEntry{{Name: "API_TEMPLATE_DIR", Value: "/opt/api/template"}},
			},
			want: ds.AppDockerConfig{
				Copy: []ds.AppDockerCopy{{From: "template", To: "/opt/api/template"}},
				Env:  []ds.AppDockerEnv{{Name: "API_TEMPLATE_DIR", Value: "/opt/api/template"}},
			},
		},
		{
			name: "one digest for a url that does not depend on the platform",
			docker: AppDocker{Fetch: []AppDockerFetchEntry{{
				Name: "tool", URL: "https://example.com/tool", To: "/usr/local/bin/tool",
				Mode: "0700", Checksum: digestA,
			}}},
			want: ds.AppDockerConfig{Fetch: []ds.AppDockerFetch{{
				Name: "tool", URL: "https://example.com/tool", To: "/usr/local/bin/tool",
				Mode: "0700", Checksum: digestA,
			}}},
		},
		{
			name: "a digest per platform, in the order the platforms are declared",
			docker: AppDocker{
				Platforms: []string{"linux/arm64", "linux/amd64"},
				Fetch: []AppDockerFetchEntry{{
					Name: "tool", URL: archURL, To: "/usr/local/bin/tool",
					Checksum: map[string]any{"linux/amd64": digestA, "linux/arm64": digestB},
				}},
			},
			want: ds.AppDockerConfig{
				Platforms: []string{"linux/arm64", "linux/amd64"},
				Fetch: []ds.AppDockerFetch{{
					Name: "tool", URL: archURL, To: "/usr/local/bin/tool",
					Variants: []ds.AppDockerFetchVariant{
						{Arch: "arm64", Digest: digestB},
						{Arch: "amd64", Digest: digestA},
					},
				}},
			},
		},
		{
			name: "no checksum at all",
			docker: AppDocker{Fetch: []AppDockerFetchEntry{{
				Name: "tool", URL: archURL, To: "/usr/local/bin/tool",
			}}},
			want: ds.AppDockerConfig{Fetch: []ds.AppDockerFetch{{
				Name: "tool", URL: archURL, To: "/usr/local/bin/tool",
			}}},
		},
		{
			name: "auxiliary stage",
			docker: AppDocker{Stages: []AppDockerStageEntry{{
				Name: "npmtools", Image: "node:22-alpine", Workdir: "/opt/npm",
				Run:  []string{"npm install -g --prefix /opt/npm cli@1.0.0"},
				Copy: []AppDockerCopyEntry{{From: "/opt/npm/bin", To: "/usr/local/bin/npmtools"}},
			}}},
			want: ds.AppDockerConfig{Stages: []ds.AppDockerStage{{
				Name: "npmtools", Image: "node:22-alpine", Workdir: "/opt/npm",
				Run:  []string{"npm install -g --prefix /opt/npm cli@1.0.0"},
				Copy: []ds.AppDockerCopy{{From: "/opt/npm/bin", To: "/usr/local/bin/npmtools"}},
			}}},
		},
		{
			name:    "copy source outside the project",
			docker:  AppDocker{Copy: []AppDockerCopyEntry{{From: "../shared", To: "/opt/shared"}}},
			wantErr: ErrAppDockerCopyFrom,
		},
		{
			name:    "copy source is absolute",
			docker:  AppDocker{Copy: []AppDockerCopyEntry{{From: "/etc/template", To: "/opt/template"}}},
			wantErr: ErrAppDockerCopyFrom,
		},
		{
			name:    "copy destination is relative",
			docker:  AppDocker{Copy: []AppDockerCopyEntry{{From: "template", To: "opt/template"}}},
			wantErr: ErrAppDockerPath,
		},
		{
			name:    "copy destination is the image root",
			docker:  AppDocker{Copy: []AppDockerCopyEntry{{From: "template", To: "/"}}},
			wantErr: ErrAppDockerPath,
		},
		{
			name:    "copy destination is written by the image itself",
			docker:  AppDocker{Copy: []AppDockerCopyEntry{{From: "template", To: "/public"}}},
			wantErr: ErrAppDockerDestReserved,
		},
		{
			name: "two entries write the same destination",
			docker: AppDocker{Copy: []AppDockerCopyEntry{
				{From: "template", To: "/opt/data"},
				{From: "assets", To: "/opt/data"},
			}},
			wantErr: ErrAppDockerDestDuplicate,
		},
		{
			name: "a fetch entry and a stage write the same destination",
			docker: AppDocker{
				Fetch: []AppDockerFetchEntry{{Name: "tool", URL: "https://example.com/tool", To: "/usr/local/bin/tool"}},
				Stages: []AppDockerStageEntry{{
					Name: "tools", Image: "alpine", Run: []string{"true"},
					Copy: []AppDockerCopyEntry{{From: "/out/tool", To: "/usr/local/bin/tool"}},
				}},
			},
			wantErr: ErrAppDockerDestDuplicate,
		},
		{
			name:    "environment variable name that is not one",
			docker:  AppDocker{Env: []AppDockerEnvEntry{{Name: "1DIR", Value: "/opt"}}},
			wantErr: ErrAppDockerEnvName,
		},
		{
			name:    "environment value that would break the instruction",
			docker:  AppDocker{Env: []AppDockerEnvEntry{{Name: "DIR", Value: `/opt/"x"`}}},
			wantErr: ErrAppDockerEnvValue,
		},
		{
			name: "the same variable twice",
			docker: AppDocker{Env: []AppDockerEnvEntry{
				{Name: "DIR", Value: "/opt/a"},
				{Name: "DIR", Value: "/opt/b"},
			}},
			wantErr: ErrAppDockerEnvDuplicate,
		},
		{
			name: "plaintext download",
			docker: AppDocker{Fetch: []AppDockerFetchEntry{{
				Name: "tool", URL: "http://example.com/tool", To: "/usr/local/bin/tool",
			}}},
			wantErr: ErrAppDockerFetchURL,
		},
		{
			name: "url references a variable nothing gives a value",
			docker: AppDocker{Fetch: []AppDockerFetchEntry{{
				Name: "tool", URL: "https://example.com/tool-${VERSION}", To: "/usr/local/bin/tool",
			}}},
			wantErr: ErrAppDockerFetchURLVar,
		},
		{
			name: "fetch name that is not a stage name",
			docker: AppDocker{Fetch: []AppDockerFetchEntry{{
				Name: "My Tool", URL: "https://example.com/tool", To: "/usr/local/bin/tool",
			}}},
			wantErr: ErrAppDockerName,
		},
		{
			name: "two fetch entries with one name",
			docker: AppDocker{Fetch: []AppDockerFetchEntry{
				{Name: "tool", URL: "https://example.com/a", To: "/usr/local/bin/a"},
				{Name: "tool", URL: "https://example.com/b", To: "/usr/local/bin/b"},
			}},
			wantErr: ErrAppDockerNameDuplicate,
		},
		{
			name: "file mode that is not one",
			docker: AppDocker{Fetch: []AppDockerFetchEntry{{
				Name: "tool", URL: "https://example.com/tool", To: "/usr/local/bin/tool", Mode: "rwx",
			}}},
			wantErr: ErrAppDockerFetchMode,
		},
		{
			name: "digest that is not a sha256 one",
			docker: AppDocker{Fetch: []AppDockerFetchEntry{{
				Name: "tool", URL: "https://example.com/tool", To: "/usr/local/bin/tool", Checksum: "deadbeef",
			}}},
			wantErr: ErrAppDockerFetchChecksum,
		},
		{
			name: "one digest for a url that depends on the platform",
			docker: AppDocker{Fetch: []AppDockerFetchEntry{{
				Name: "tool", URL: archURL, To: "/usr/local/bin/tool", Checksum: digestA,
			}}},
			wantErr: ErrAppDockerFetchChecksumVar,
		},
		{
			name: "a digest per platform without declared platforms",
			docker: AppDocker{Fetch: []AppDockerFetchEntry{{
				Name: "tool", URL: archURL, To: "/usr/local/bin/tool",
				Checksum: map[string]any{"linux/amd64": digestA},
			}}},
			wantErr: ErrAppDockerFetchChecksumPlatforms,
		},
		{
			name: "a digest per platform that misses one of them",
			docker: AppDocker{
				Platforms: []string{"linux/amd64", "linux/arm64"},
				Fetch: []AppDockerFetchEntry{{
					Name: "tool", URL: archURL, To: "/usr/local/bin/tool",
					Checksum: map[string]any{"linux/amd64": digestA, "linux/ppc64le": digestB},
				}},
			},
			wantErr: ErrAppDockerFetchChecksumPlatforms,
		},
		{
			name: "a digest per platform for a url that does not depend on it",
			docker: AppDocker{
				Platforms: []string{"linux/amd64"},
				Fetch: []AppDockerFetchEntry{{
					Name: "tool", URL: "https://example.com/tool", To: "/usr/local/bin/tool",
					Checksum: map[string]any{"linux/amd64": digestA},
				}},
			},
			wantErr: ErrAppDockerFetchChecksumConst,
		},
		{
			name: "stage image that would break the FROM",
			docker: AppDocker{Stages: []AppDockerStageEntry{{
				Name: "tools", Image: "node:22 alpine", Run: []string{"true"},
				Copy: []AppDockerCopyEntry{{From: "/out", To: "/opt/out"}},
			}}},
			wantErr: ErrAppDockerStageImage,
		},
		{
			name: "stage that runs nothing",
			docker: AppDocker{Stages: []AppDockerStageEntry{{
				Name: "tools", Image: "alpine",
				Copy: []AppDockerCopyEntry{{From: "/out", To: "/opt/out"}},
			}}},
			wantErr: ErrAppDockerStageRun,
		},
		{
			name: "stage command spanning two lines",
			docker: AppDocker{Stages: []AppDockerStageEntry{{
				Name: "tools", Image: "alpine", Run: []string{"npm ci\nnpm run build"},
				Copy: []AppDockerCopyEntry{{From: "/out", To: "/opt/out"}},
			}}},
			wantErr: ErrAppDockerStageRun,
		},
		{
			name: "stage command with a template variable",
			docker: AppDocker{Stages: []AppDockerStageEntry{{
				Name: "tools", Image: "alpine", Run: []string{"install {{ .Spec }}"},
				Copy: []AppDockerCopyEntry{{From: "/out", To: "/opt/out"}},
			}}},
			wantErr: ErrAppDockerStageRunVar,
		},
		{
			name: "stage nothing is copied out of",
			docker: AppDocker{Stages: []AppDockerStageEntry{{
				Name: "tools", Image: "alpine", Run: []string{"true"},
			}}},
			wantErr: ErrAppDockerStageCopy,
		},
		{
			name: "stage source is a project path",
			docker: AppDocker{Stages: []AppDockerStageEntry{{
				Name: "tools", Image: "alpine", Run: []string{"true"},
				Copy: []AppDockerCopyEntry{{From: "out", To: "/opt/out"}},
			}}},
			wantErr: ErrAppDockerPath,
		},
		{
			name: "stage workdir is relative",
			docker: AppDocker{Stages: []AppDockerStageEntry{{
				Name: "tools", Image: "alpine", Workdir: "opt", Run: []string{"true"},
				Copy: []AppDockerCopyEntry{{From: "/out", To: "/opt/out"}},
			}}},
			wantErr: ErrAppDockerPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.docker.Parse()

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Parse() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// TestAppDocker_ParseChecksumTypes covers the shapes viper can produce for the
// checksum of a fetch entry — a string, a map it decoded into map[string]any,
// and anything else.
func TestAppDocker_ParseChecksumTypes(t *testing.T) {
	base := func(checksum any) AppDocker {
		return AppDocker{
			Platforms: []string{"linux/amd64"},
			Fetch: []AppDockerFetchEntry{{
				Name: "tool", URL: archURL, To: "/usr/local/bin/tool", Checksum: checksum,
			}},
		}
	}

	if _, err := base(map[string]string{"linux/amd64": digestA}).Parse(); err != nil {
		t.Errorf("a map[string]string checksum is rejected: %v", err)
	}

	if _, err := base(map[string]any{"linux/amd64": 42}).Parse(); !errors.Is(err, ErrAppDockerFetchChecksum) {
		t.Errorf("a non-string digest error = %v, want %v", err, ErrAppDockerFetchChecksum)
	}

	if _, err := base([]any{digestA}).Parse(); !errors.Is(err, ErrAppDockerFetchChecksum) {
		t.Errorf("a list checksum error = %v, want %v", err, ErrAppDockerFetchChecksum)
	}
}
