package ds

import (
	"strings"
	"testing"
)

func TestAppDockerConfig_Image(t *testing.T) {
	tests := []struct {
		name string
		cfg  AppDockerConfig
		want string
	}{
		{name: "zero value falls back to the default", cfg: AppDockerConfig{}, want: DefaultDockerBaseImage},
		{name: "declared image", cfg: AppDockerConfig{BaseImage: "ubuntu:24.04"}, want: "ubuntu:24.04"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.Image(); got != tt.want {
				t.Errorf("Image() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppDockerConfig_PackagesDirective(t *testing.T) {
	tests := []struct {
		name string
		cfg  AppDockerConfig
		want []string
		none bool
	}{
		{
			name: "no packages, no instruction",
			cfg:  AppDockerConfig{BaseImage: "ubuntu:24.04"},
			none: true,
		},
		{
			name: "apk",
			cfg:  AppDockerConfig{PackageManager: DockerPackageManagerAPK, Packages: []string{"git", "curl"}},
			want: []string{"RUN apk add --no-cache git curl"},
		},
		{
			name: "apt drops its index in the same instruction",
			cfg:  AppDockerConfig{PackageManager: DockerPackageManagerAPT, Packages: []string{"git", "curl"}},
			want: []string{
				"apt-get update",
				"apt-get install -y --no-install-recommends git curl",
				"rm -rf /var/lib/apt/lists/*",
			},
		},
		{
			name: "dnf cleans its cache in the same instruction",
			cfg:  AppDockerConfig{PackageManager: DockerPackageManagerDNF, Packages: []string{"git"}},
			want: []string{"dnf install -y git", "dnf clean all"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.PackagesDirective()

			if tt.none {
				if got != "" {
					t.Fatalf("PackagesDirective() = %q, want empty", got)
				}

				return
			}

			if !strings.HasPrefix(got, "RUN ") {
				t.Errorf("PackagesDirective() = %q, want it to be a single RUN instruction", got)
			}

			// A continuation keeps the install in one layer, so every line but
			// the last has to end with a backslash.
			lines := strings.Split(got, "\n")
			for i, line := range lines[:len(lines)-1] {
				if !strings.HasSuffix(line, "\\") {
					t.Errorf("line %d of the install instruction does not continue: %q", i, line)
				}
			}

			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("PackagesDirective() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestAppDockerConfig_WorkdirDirective(t *testing.T) {
	if got := (AppDockerConfig{}).WorkdirDirective(); got != "" {
		t.Errorf("WorkdirDirective() = %q, want empty", got)
	}

	if got := (AppDockerConfig{Workdir: "/workspace"}).WorkdirDirective(); got != "WORKDIR /workspace" {
		t.Errorf("WorkdirDirective() = %q, want %q", got, "WORKDIR /workspace")
	}
}

func TestAppDockerConfig_StartDirective(t *testing.T) {
	tests := []struct {
		name string
		cfg  AppDockerConfig
		want string
	}{
		{
			name: "default command of the service binary",
			cfg:  AppDockerConfig{},
			want: `CMD ["/service"]`,
		},
		{
			name: "entrypoint replaces the command",
			cfg:  AppDockerConfig{Entrypoint: []string{"/service", "--verbose"}},
			want: `ENTRYPOINT ["/service", "--verbose"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.StartDirective()

			if got != tt.want {
				t.Errorf("StartDirective() = %q, want %q", got, tt.want)
			}

			// Exactly one start instruction: a CMD left next to an ENTRYPOINT
			// would be passed to it as an argument.
			if strings.Contains(got, "CMD") && strings.Contains(got, "ENTRYPOINT") {
				t.Errorf("StartDirective() = %q, want either CMD or ENTRYPOINT", got)
			}
		})
	}
}
