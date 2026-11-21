package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestAppDocker_ParsePlatforms(t *testing.T) {
	tests := []struct {
		name      string
		platforms []string
		want      string
		wantErr   error
	}{
		{
			name:      "omitted section",
			platforms: nil,
			want:      "",
		},
		{
			name:      "empty list is the same as omitted",
			platforms: []string{},
			want:      "",
		},
		{
			name:      "one platform",
			platforms: []string{"linux/arm64"},
			want:      "linux/arm64",
		},
		{
			name:      "several platforms keep config order",
			platforms: []string{"linux/arm64", "linux/amd64"},
			want:      "linux/arm64,linux/amd64",
		},
		{
			name:      "another linux architecture",
			platforms: []string{"linux/ppc64le"},
			want:      "linux/ppc64le",
		},
		{
			// The runtime stage targets linux: an image for another OS would
			// not run whatever the declaration says.
			name:      "non-linux os",
			platforms: []string{"darwin/arm64"},
			wantErr:   ErrAppDockerPlatform,
		},
		{
			name:      "architecture alone",
			platforms: []string{"arm64"},
			wantErr:   ErrAppDockerPlatform,
		},
		{
			// A variant does not reach the Go build: the builder stage takes
			// the architecture from TARGETARCH and nothing maps v7 to GOARM.
			name:      "variant",
			platforms: []string{"linux/arm/v7"},
			wantErr:   ErrAppDockerPlatform,
		},
		{
			name:      "uppercase",
			platforms: []string{"Linux/AMD64"},
			wantErr:   ErrAppDockerPlatform,
		},
		{
			name:      "empty entry",
			platforms: []string{""},
			wantErr:   ErrAppDockerPlatform,
		},
		{
			name:      "shell metacharacters",
			platforms: []string{"linux/amd64 && rm -rf /"},
			wantErr:   ErrAppDockerPlatform,
		},
		{
			name:      "duplicate",
			platforms: []string{"linux/amd64", "linux/amd64"},
			wantErr:   ErrAppDockerPlatformDuplicate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AppDocker{Platforms: tt.platforms}.Parse()

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Parse() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}

			if joined := strings.Join(got.Platforms, ","); joined != tt.want {
				t.Errorf("Parse() platforms = %q, want %q", joined, tt.want)
			}
		})
	}
}

// TestAppDocker_ViperDecodingPlatforms pins the shape viper actually produces for the
// per-application docker section. Parse must keep matching it.
func TestAppDocker_ViperDecodingPlatforms(t *testing.T) {
	const projectYAML = `
applications:
  - name: api
  - name: cli
    docker:
      platforms: [linux/amd64, linux/arm64]
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

	if len(cfg.Applications) != 2 {
		t.Fatalf("decoded %d applications, want 2", len(cfg.Applications))
	}

	// An application without the section declares no platform, so its image is
	// built exactly as before the key existed.
	plain, err := cfg.Applications[0].Docker.Parse()
	if err != nil {
		t.Fatalf("Parse() of the application without a docker section: %v", err)
	}

	if plain.HasPlatforms() {
		t.Errorf("application without a docker section has platforms %v", plain.Platforms)
	}

	got, err := cfg.Applications[1].Docker.Parse()
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if want := "linux/amd64,linux/arm64"; got.PlatformList() != want {
		t.Errorf("Parse() platforms = %q, want %q", got.PlatformList(), want)
	}
}
