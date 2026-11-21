package ds

import (
	"strings"
	"testing"
)

// dockerTestConfig is the config the rendering tests share: one argument of
// each stage set plus two labels.
func dockerTestConfig() DockerConfig {
	return DockerConfig{
		ImagePrefix: "registry.example/org",
		BuildArgs: []DockerBuildArg{
			{Name: "IMAGE_VERSION", Stages: []string{DockerStageFinal}},
			{
				Name:    "IMAGE_REVISION",
				Default: "$(shell git rev-parse HEAD)",
				Stages:  []string{DockerStageBuilder, DockerStageFinal},
			},
			{Name: "BUILD_FLAVOR", Default: "slim", Stages: []string{DockerStageBuilder}},
		},
		Labels: []DockerLabel{
			{Key: "org.opencontainers.image.revision", Value: "${IMAGE_REVISION}"},
			{Key: "org.opencontainers.image.version", Value: "${IMAGE_VERSION}"},
		},
	}
}

func TestDockerConfig_StageArgs(t *testing.T) {
	cfg := dockerTestConfig()

	tests := []struct {
		name string
		got  []DockerBuildArg
		want string
	}{
		{name: "builder stage", got: cfg.BuilderArgs(), want: "IMAGE_REVISION,BUILD_FLAVOR"},
		{name: "final stage", got: cfg.FinalArgs(), want: "IMAGE_VERSION,IMAGE_REVISION"},
		{name: "unknown stage", got: cfg.StageArgs("nope"), want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names := make([]string, 0, len(tt.got))

			for _, arg := range tt.got {
				names = append(names, arg.Name)
			}

			if strings.Join(names, ",") != tt.want {
				t.Errorf("stage args = %q, want %q", strings.Join(names, ","), tt.want)
			}
		})
	}
}

func TestDockerBuildArg_UsedInStage(t *testing.T) {
	arg := DockerBuildArg{Name: "A", Stages: []string{DockerStageFinal}}

	if !arg.UsedInStage(DockerStageFinal) {
		t.Error("UsedInStage(final) = false, want true")
	}

	if arg.UsedInStage(DockerStageBuilder) {
		t.Error("UsedInStage(builder) = true, want false")
	}
}

func TestDockerConfig_LabelDirective(t *testing.T) {
	// One instruction (one layer), one pair per line, continuation lines
	// aligned under the first pair.
	want := `LABEL org.opencontainers.image.revision="${IMAGE_REVISION}" \
      org.opencontainers.image.version="${IMAGE_VERSION}"`

	if got := dockerTestConfig().LabelDirective(); got != want {
		t.Errorf("LabelDirective() =\n%s\nwant\n%s", got, want)
	}

	if got := (DockerConfig{}).LabelDirective(); got != "" {
		t.Errorf("LabelDirective() without labels = %q, want empty", got)
	}
}

func TestDockerConfig_BuildArgFlags(t *testing.T) {
	want := ` --build-arg IMAGE_VERSION="$(IMAGE_VERSION)"` +
		` --build-arg IMAGE_REVISION="$(IMAGE_REVISION)"` +
		` --build-arg BUILD_FLAVOR="$(BUILD_FLAVOR)"`

	if got := dockerTestConfig().BuildArgFlags(); got != want {
		t.Errorf("BuildArgFlags() = %q, want %q", got, want)
	}

	if got := (DockerConfig{}).BuildArgFlags(); got != "" {
		t.Errorf("BuildArgFlags() without build args = %q, want empty", got)
	}
}

func TestDockerConfig_ComposeBuildArgs(t *testing.T) {
	want := "IMAGE_VERSION=${IMAGE_VERSION:-},IMAGE_REVISION=${IMAGE_REVISION:-},BUILD_FLAVOR=${BUILD_FLAVOR:-}"

	if got := strings.Join(dockerTestConfig().ComposeBuildArgs(), ","); got != want {
		t.Errorf("ComposeBuildArgs() = %q, want %q", got, want)
	}

	if got := (DockerConfig{}).ComposeBuildArgs(); len(got) != 0 {
		t.Errorf("ComposeBuildArgs() without build args = %v, want empty", got)
	}
}

func TestAppDockerConfig(t *testing.T) {
	tests := []struct {
		name          string
		platforms     []string
		hasPlatforms  bool
		multiPlatform bool
		platformFlag  string
		outputFlag    string
	}{
		{
			name:          "no platforms",
			platforms:     nil,
			hasPlatforms:  false,
			multiPlatform: false,
			platformFlag:  "",
			outputFlag:    "",
		},
		{
			// One platform is a plain image, so it lands in the local image
			// store exactly like the `docker build` the recipe replaces.
			name:          "one platform",
			platforms:     []string{"linux/arm64"},
			hasPlatforms:  true,
			multiPlatform: false,
			platformFlag:  " --platform linux/arm64",
			outputFlag:    " --load",
		},
		{
			// Several platforms are an image index, which the local store
			// cannot hold: the build stays in the builder cache and the push
			// recipe publishes the index from there.
			name:          "several platforms",
			platforms:     []string{"linux/amd64", "linux/arm64"},
			hasPlatforms:  true,
			multiPlatform: true,
			platformFlag:  " --platform linux/amd64,linux/arm64",
			outputFlag:    " --output type=cacheonly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := AppDockerConfig{Platforms: tt.platforms}

			if got := cfg.HasPlatforms(); got != tt.hasPlatforms {
				t.Errorf("HasPlatforms() = %v, want %v", got, tt.hasPlatforms)
			}

			if got := cfg.MultiPlatform(); got != tt.multiPlatform {
				t.Errorf("MultiPlatform() = %v, want %v", got, tt.multiPlatform)
			}

			if got := cfg.PlatformFlag(); got != tt.platformFlag {
				t.Errorf("PlatformFlag() = %q, want %q", got, tt.platformFlag)
			}

			if got := cfg.BuildOutputFlag(); got != tt.outputFlag {
				t.Errorf("BuildOutputFlag() = %q, want %q", got, tt.outputFlag)
			}
		})
	}
}

func TestApps_HasDockerPlatforms(t *testing.T) {
	plain := App{Name: "api"}
	cross := App{Name: "cli", Docker: AppDockerConfig{Platforms: []string{"linux/arm64"}}}

	tests := []struct {
		name string
		apps Apps
		want bool
	}{
		{name: "no applications", apps: Apps{}, want: false},
		{name: "no application declares platforms", apps: Apps{plain}, want: false},
		{name: "one of several declares platforms", apps: Apps{plain, cross}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.apps.HasDockerPlatforms(); got != tt.want {
				t.Errorf("HasDockerPlatforms() = %v, want %v", got, tt.want)
			}
		})
	}
}
