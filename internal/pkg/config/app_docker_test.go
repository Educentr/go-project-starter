package config

import (
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
)

func TestAppDocker_Parse(t *testing.T) {
	tests := []struct {
		name    string
		docker  AppDocker
		want    ds.AppDockerConfig
		wantErr string
	}{
		{
			name:   "omitted section is a zero value",
			docker: AppDocker{},
			want:   ds.AppDockerConfig{},
		},
		{
			name:   "base image without packages needs no package manager",
			docker: AppDocker{BaseImage: "gcr.io/distroless/static:nonroot"},
			want:   ds.AppDockerConfig{BaseImage: "gcr.io/distroless/static:nonroot"},
		},
		{
			name:   "package manager of the default base image",
			docker: AppDocker{Packages: []string{"git", "ca-certificates"}},
			want: ds.AppDockerConfig{
				PackageManager: ds.DockerPackageManagerAPK,
				Packages:       []string{"git", "ca-certificates"},
			},
		},
		{
			name:   "package manager resolved from the base image",
			docker: AppDocker{BaseImage: "ubuntu:24.04", Packages: []string{"git"}},
			want: ds.AppDockerConfig{
				BaseImage:      "ubuntu:24.04",
				PackageManager: ds.DockerPackageManagerAPT,
				Packages:       []string{"git"},
			},
		},
		{
			name:   "package manager resolved through a registry and a port",
			docker: AppDocker{BaseImage: "registry.example:5000/mirror/debian:bookworm", Packages: []string{"curl"}},
			want: ds.AppDockerConfig{
				BaseImage:      "registry.example:5000/mirror/debian:bookworm",
				PackageManager: ds.DockerPackageManagerAPT,
				Packages:       []string{"curl"},
			},
		},
		{
			name:   "explicit package manager wins over the base image",
			docker: AppDocker{BaseImage: "ubuntu:24.04", PackageManager: ds.DockerPackageManagerDNF, Packages: []string{"git"}},
			want: ds.AppDockerConfig{
				BaseImage:      "ubuntu:24.04",
				PackageManager: ds.DockerPackageManagerDNF,
				Packages:       []string{"git"},
			},
		},
		{
			name:   "workdir and entrypoint are kept",
			docker: AppDocker{Workdir: "/workspace", Entrypoint: []string{"/service", "--verbose"}},
			want: ds.AppDockerConfig{
				Workdir:    "/workspace",
				Entrypoint: []string{"/service", "--verbose"},
			},
		},
		{
			name:   "version pinned package",
			docker: AppDocker{BaseImage: "ubuntu:24.04", Packages: []string{"curl=8.5.0-2ubuntu10.6"}},
			want: ds.AppDockerConfig{
				BaseImage:      "ubuntu:24.04",
				PackageManager: ds.DockerPackageManagerAPT,
				Packages:       []string{"curl=8.5.0-2ubuntu10.6"},
			},
		},
		{
			name:    "base image with whitespace is rejected",
			docker:  AppDocker{BaseImage: "ubuntu:24.04 AS runtime"},
			wantErr: "applications.docker.base_image",
		},
		{
			name:    "base image with a newline is rejected",
			docker:  AppDocker{BaseImage: "alpine\nRUN rm -rf /"},
			wantErr: "applications.docker.base_image",
		},
		{
			name:    "package name with a shell separator is rejected",
			docker:  AppDocker{Packages: []string{"git; rm -rf /"}},
			wantErr: "applications.docker.packages",
		},
		{
			name:    "unknown package manager is rejected",
			docker:  AppDocker{PackageManager: "brew", Packages: []string{"git"}},
			wantErr: "applications.docker.package_manager: unknown package manager",
		},
		{
			name:    "unknown base image needs an explicit package manager",
			docker:  AppDocker{BaseImage: "gcr.io/distroless/static:nonroot", Packages: []string{"git"}},
			wantErr: "applications.docker.package_manager: cannot be resolved from base_image",
		},
		{
			name:    "package manager without packages is rejected",
			docker:  AppDocker{PackageManager: ds.DockerPackageManagerAPT},
			wantErr: "applications.docker.package_manager: declared without packages",
		},
		{
			name:    "relative workdir is rejected",
			docker:  AppDocker{Workdir: "workspace"},
			wantErr: "applications.docker.workdir",
		},
		{
			name:    "workdir with a quote is rejected",
			docker:  AppDocker{Workdir: `/work"dir`},
			wantErr: "applications.docker.workdir",
		},
		{
			name:    "empty entrypoint element is rejected",
			docker:  AppDocker{Entrypoint: []string{"/service", ""}},
			wantErr: "applications.docker.entrypoint",
		},
		{
			name:    "entrypoint element with a quote is rejected",
			docker:  AppDocker{Entrypoint: []string{`/bin/sh -c "id"`}},
			wantErr: "applications.docker.entrypoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.docker.Parse()

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Parse() error = nil, want %q", tt.wantErr)
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Parse() error = %q, want it to contain %q", err, tt.wantErr)
				}

				if ok, msg := tt.docker.IsValid(); ok || !strings.Contains(msg, tt.wantErr) {
					t.Errorf("IsValid() = %v, %q, want false with %q", ok, msg, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			if got.BaseImage != tt.want.BaseImage {
				t.Errorf("BaseImage = %q, want %q", got.BaseImage, tt.want.BaseImage)
			}

			if got.PackageManager != tt.want.PackageManager {
				t.Errorf("PackageManager = %q, want %q", got.PackageManager, tt.want.PackageManager)
			}

			if strings.Join(got.Packages, ",") != strings.Join(tt.want.Packages, ",") {
				t.Errorf("Packages = %v, want %v", got.Packages, tt.want.Packages)
			}

			if got.Workdir != tt.want.Workdir {
				t.Errorf("Workdir = %q, want %q", got.Workdir, tt.want.Workdir)
			}

			if strings.Join(got.Entrypoint, ",") != strings.Join(tt.want.Entrypoint, ",") {
				t.Errorf("Entrypoint = %v, want %v", got.Entrypoint, tt.want.Entrypoint)
			}

			if ok, msg := tt.docker.IsValid(); !ok {
				t.Errorf("IsValid() = false, %q, want true", msg)
			}
		})
	}
}

func TestDockerImageName(t *testing.T) {
	tests := []struct {
		image string
		want  string
	}{
		{image: "alpine", want: "alpine"},
		{image: "alpine:3.20", want: "alpine"},
		{image: "Ubuntu:24.04", want: "ubuntu"},
		{image: "library/debian:bookworm-slim", want: "debian"},
		{image: "registry.example:5000/mirror/ubuntu", want: "ubuntu"},
		{image: "ubuntu@sha256:abc123", want: "ubuntu"},
		{image: "gcr.io/distroless/static:nonroot", want: "static"},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			if got := dockerImageName(tt.image); got != tt.want {
				t.Errorf("dockerImageName(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}
