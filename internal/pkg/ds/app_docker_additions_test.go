package ds

import "testing"

func TestAppDockerConfig_EnvDirective(t *testing.T) {
	tests := []struct {
		name string
		cfg  AppDockerConfig
		want string
	}{
		{
			name: "nothing declared, no instruction",
			cfg:  AppDockerConfig{},
			want: "",
		},
		{
			name: "one variable",
			cfg:  AppDockerConfig{Env: []AppDockerEnv{{Name: "DIR", Value: "/opt/x"}}},
			want: `ENV DIR="/opt/x"`,
		},
		{
			// One instruction, so one layer; the declaration order is kept.
			name: "several variables in one instruction",
			cfg: AppDockerConfig{Env: []AppDockerEnv{
				{Name: "DIR", Value: "/opt/x"},
				{Name: "EMPTY", Value: ""},
			}},
			want: "ENV DIR=\"/opt/x\" \\\n    EMPTY=\"\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.EnvDirective(); got != tt.want {
				t.Errorf("EnvDirective() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppDockerFetch_Shape(t *testing.T) {
	fetch := AppDockerFetch{Name: "tool", URL: "https://example.com/tool-${TARGETARCH}", To: "/usr/local/bin/tool"}

	if got, want := fetch.StageName(), "fetch-tool"; got != want {
		t.Errorf("StageName() = %q, want %q", got, want)
	}

	if got, want := fetch.Path(), "/fetched/tool"; got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}

	// The default is executable: ADD from a URL leaves the file without the
	// bit, and a fetch entry carries a tool binary.
	if got, want := fetch.FileMode(), DefaultDockerFetchMode; got != want {
		t.Errorf("FileMode() = %q, want %q", got, want)
	}

	if got, want := (AppDockerFetch{Mode: "0600"}).FileMode(), "0600"; got != want {
		t.Errorf("FileMode() of a declared mode = %q, want %q", got, want)
	}

	if fetch.PerPlatform() {
		t.Error("PerPlatform() is true without variants")
	}

	// No digest: no --checksum flag, but the mode is always set.
	if got, want := fetch.AddDirective(""),
		"ADD --chmod=0755 https://example.com/tool-${TARGETARCH} /fetched/tool"; got != want {
		t.Errorf("AddDirective(\"\") = %q, want %q", got, want)
	}

	if got, want := fetch.AddDirective("sha256:abc"),
		"ADD --checksum=sha256:abc --chmod=0755 https://example.com/tool-${TARGETARCH} /fetched/tool"; got != want {
		t.Errorf("AddDirective(digest) = %q, want %q", got, want)
	}
}

// TestAppDockerFetch_Variants covers the selector a per-architecture digest is
// generated as: a Dockerfile cannot choose between digests inside one stage, so
// the architecture picks the stage instead.
func TestAppDockerFetch_Variants(t *testing.T) {
	fetch := AppDockerFetch{
		Name: "tool",
		URL:  "https://example.com/tool-${TARGETARCH}",
		Variants: []AppDockerFetchVariant{
			{Arch: "amd64", Digest: "sha256:a"},
			{Arch: "arm64", Digest: "sha256:b"},
		},
	}

	if !fetch.PerPlatform() {
		t.Error("PerPlatform() is false with variants")
	}

	if got, want := fetch.VariantStageName("amd64"), "fetch-tool-amd64"; got != want {
		t.Errorf("VariantStageName() = %q, want %q", got, want)
	}

	if got, want := fetch.SelectorImage(), "fetch-tool-$TARGETARCH"; got != want {
		t.Errorf("SelectorImage() = %q, want %q", got, want)
	}
}

func TestAppDockerStage_Shape(t *testing.T) {
	stage := AppDockerStage{Name: "npmtools", Image: "node:22-alpine", Workdir: "/opt/npm"}

	if got, want := stage.StageName(), "stage-npmtools"; got != want {
		t.Errorf("StageName() = %q, want %q", got, want)
	}

	// Pinned to the build host: a multi-platform build would otherwise run the
	// stage once per target platform, under emulation.
	if got, want := stage.StageFrom(), "FROM --platform=$BUILDPLATFORM node:22-alpine AS stage-npmtools"; got != want {
		t.Errorf("StageFrom() = %q, want %q", got, want)
	}

	if got, want := stage.WorkdirDirective(), "WORKDIR /opt/npm"; got != want {
		t.Errorf("WorkdirDirective() = %q, want %q", got, want)
	}

	if got := (AppDockerStage{Name: "x", Image: "alpine"}).WorkdirDirective(); got != "" {
		t.Errorf("WorkdirDirective() without a workdir = %q, want an empty string", got)
	}
}
