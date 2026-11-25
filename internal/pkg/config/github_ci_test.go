package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/spf13/viper"
)

// intPtr returns a pointer to v, mirroring how viper decodes an optional int.
func intPtr(v int) *int {
	return &v
}

func TestGithubCIConfig_ParseRunsOn(t *testing.T) {
	tests := []struct {
		name    string
		runsOn  any
		want    ds.RunsOnSpec
		wantErr string
	}{
		{
			name:   "omitted section yields the zero spec",
			runsOn: nil,
			want:   ds.RunsOnSpec{},
		},
		{
			name:   "single label",
			runsOn: "self-hosted",
			want:   ds.RunsOnSpec{Labels: []string{"self-hosted"}},
		},
		{
			name:   "label list",
			runsOn: []any{"self-hosted", "linux"},
			want:   ds.RunsOnSpec{Labels: []string{"self-hosted", "linux"}},
		},
		{
			name:   "group only",
			runsOn: map[string]any{"group": "my-runners"},
			want:   ds.RunsOnSpec{Group: "my-runners"},
		},
		{
			name:   "group with labels",
			runsOn: map[string]any{"group": "my-runners", "labels": []any{"linux"}},
			want:   ds.RunsOnSpec{Group: "my-runners", Labels: []string{"linux"}},
		},
		{
			name:    "empty label",
			runsOn:  "",
			wantErr: "invalid runner label",
		},
		{
			name:    "label with a flow separator",
			runsOn:  "self-hosted,linux",
			wantErr: "invalid runner label",
		},
		{
			name:    "empty label list",
			runsOn:  []any{},
			wantErr: "label list is empty",
		},
		{
			name:    "non-string label",
			runsOn:  []any{42},
			wantErr: "label must be a string",
		},
		{
			name:    "group missing",
			runsOn:  map[string]any{"labels": []any{"linux"}},
			wantErr: "'group' is required",
		},
		{
			name:    "unknown key in object form",
			runsOn:  map[string]any{"group": "my-runners", "tags": []any{"linux"}},
			wantErr: `unknown key, expected 'group' or 'labels': "tags"`,
		},
		{
			name:    "labels is not a list",
			runsOn:  map[string]any{"group": "my-runners", "labels": "linux"},
			wantErr: "'labels' must be a list",
		},
		{
			name:    "labels is an empty list",
			runsOn:  map[string]any{"group": "my-runners", "labels": []any{}},
			wantErr: "'labels' is empty",
		},
		{
			name:    "unsupported type",
			runsOn:  42,
			wantErr: "expected a label, a list of labels or a {group, labels} object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GithubCIConfig{RunsOn: tt.runsOn}.ParseRunsOn()

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseRunsOn() error = nil, want error containing %q", tt.wantErr)
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ParseRunsOn() error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseRunsOn() unexpected error: %v", err)
			}

			if got.Group != tt.want.Group || strings.Join(got.Labels, ",") != strings.Join(tt.want.Labels, ",") {
				t.Errorf("ParseRunsOn() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestGithubCIConfig_IsValid(t *testing.T) {
	tests := []struct {
		name         string
		cfg          GithubCIConfig
		registryType string
		wantOK       bool
		wantMsg      string
	}{
		{
			name:   "omitted section is valid",
			cfg:    GithubCIConfig{},
			wantOK: true,
		},
		{
			name: "all fields set",
			cfg: GithubCIConfig{
				RunsOn:            map[string]any{"group": "my-runners"},
				JobTimeoutMinutes: intPtr(30),
				DoctlVersion:      "1.170.0",
			},
			wantOK: true,
		},
		{
			name:         "explicit install mode is valid",
			cfg:          GithubCIConfig{Doctl: "install", DoctlVersion: "1.170.0"},
			registryType: "digitalocean",
			wantOK:       true,
		},
		{
			name:         "image mode with a digitalocean registry is valid",
			cfg:          GithubCIConfig{Doctl: "image"},
			registryType: "digitalocean",
			wantOK:       true,
		},
		{
			name:         "image mode together with doctl_version is rejected",
			cfg:          GithubCIConfig{Doctl: "image", DoctlVersion: "1.170.0"},
			registryType: "digitalocean",
			wantMsg:      `github_ci.doctl: "image" takes doctl from the runner image, so github_ci.doctl_version (1.170.0) cannot be applied`,
		},
		{
			name:         "image mode outside the digitalocean registry is rejected",
			cfg:          GithubCIConfig{Doctl: "image"},
			registryType: "github",
			wantMsg:      `github_ci.doctl: "image" only applies to main.registry_type: digitalocean, got: github`,
		},
		{
			name:         "unknown doctl mode is rejected",
			cfg:          GithubCIConfig{Doctl: "skip"},
			registryType: "digitalocean",
			wantMsg:      `github_ci.doctl must be "install" or "image", got: skip`,
		},
		{
			// The install mode is the default, so it stays valid for every
			// registry type: the doctl steps simply are not generated.
			name:         "install mode outside the digitalocean registry is valid",
			cfg:          GithubCIConfig{Doctl: "install"},
			registryType: "github",
			wantOK:       true,
		},
		{
			name:    "zero timeout is rejected",
			cfg:     GithubCIConfig{JobTimeoutMinutes: intPtr(0)},
			wantMsg: "github_ci.job_timeout_minutes must be greater than 0, got 0",
		},
		{
			name:    "negative timeout is rejected",
			cfg:     GithubCIConfig{JobTimeoutMinutes: intPtr(-5)},
			wantMsg: "github_ci.job_timeout_minutes must be greater than 0, got -5",
		},
		{
			name:    "doctl version with yaml metacharacters is rejected",
			cfg:     GithubCIConfig{DoctlVersion: "1.170.0 # cmd"},
			wantMsg: "github_ci.doctl_version must be a version string, got: 1.170.0 # cmd",
		},
		{
			name:    "invalid runs_on is reported",
			cfg:     GithubCIConfig{RunsOn: []any{"linux", ""}},
			wantMsg: "invalid runner label",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, msg := tt.cfg.IsValid(tt.registryType)

			if ok != tt.wantOK {
				t.Fatalf("IsValid() ok = %v, want %v (msg: %s)", ok, tt.wantOK, msg)
			}

			if tt.wantMsg != "" && !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("IsValid() msg = %q, want it to contain %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestGithubCIConfig_JobTimeout(t *testing.T) {
	if got := (GithubCIConfig{}).JobTimeout(); got != 0 {
		t.Errorf("JobTimeout() for unset value = %d, want 0", got)
	}

	if got := (GithubCIConfig{JobTimeoutMinutes: intPtr(15)}).JobTimeout(); got != 15 {
		t.Errorf("JobTimeout() = %d, want 15", got)
	}
}

// TestGithubCIConfig_DoctlMode checks the normalization the templates branch
// on: an omitted key must keep the install mode, i.e. the historical output.
func TestGithubCIConfig_DoctlMode(t *testing.T) {
	tests := []struct {
		name string
		cfg  GithubCIConfig
		want ds.DoctlMode
	}{
		{
			name: "omitted key means install",
			cfg:  GithubCIConfig{},
			want: ds.DoctlModeInstall,
		},
		{
			name: "explicit install",
			cfg:  GithubCIConfig{Doctl: "install"},
			want: ds.DoctlModeInstall,
		},
		{
			name: "image",
			cfg:  GithubCIConfig{Doctl: "image"},
			want: ds.DoctlModeImage,
		},
		{
			name: "an unknown value is passed through for IsValid to report",
			cfg:  GithubCIConfig{Doctl: "skip"},
			want: ds.DoctlMode("skip"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.DoctlMode(); got != tt.want {
				t.Errorf("DoctlMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGithubCIConfig_ViperDecoding pins the shapes viper actually produces for
// the three accepted runs_on forms: the type switch in ParseRunsOn must keep
// matching them.
func TestGithubCIConfig_ViperDecoding(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want ds.RunsOnSpec
	}{
		{
			name: "scalar label",
			yaml: "github_ci:\n  runs_on: self-hosted\n",
			want: ds.RunsOnSpec{Labels: []string{"self-hosted"}},
		},
		{
			name: "label sequence",
			yaml: "github_ci:\n  runs_on: [self-hosted, linux]\n",
			want: ds.RunsOnSpec{Labels: []string{"self-hosted", "linux"}},
		},
		{
			name: "group mapping",
			yaml: "github_ci:\n  runs_on:\n    group: my-runners\n    labels: [linux]\n",
			want: ds.RunsOnSpec{Group: "my-runners", Labels: []string{"linux"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "project.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o600); err != nil {
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

			got, err := cfg.GithubCI.ParseRunsOn()
			if err != nil {
				t.Fatalf("ParseRunsOn() unexpected error: %v", err)
			}

			if got.Group != tt.want.Group || strings.Join(got.Labels, ",") != strings.Join(tt.want.Labels, ",") {
				t.Errorf("ParseRunsOn() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestGithubCIConfig_ValidatePushBranches(t *testing.T) {
	tests := []struct {
		name     string
		branches []string
		wantErr  string
	}{
		{
			name:     "omitted key keeps the defaults",
			branches: nil,
		},
		{
			name:     "plain branch names",
			branches: []string{"main", "staging", "trunk"},
		},
		{
			name:     "filter patterns",
			branches: []string{"main", "release/**", "v1.*", "feature/?", "releases/**", "!releases/**-alpha"},
		},
		{
			name:     "character range pattern",
			branches: []string{"release/[0-9]*"},
		},
		{
			name:     "explicit empty list is rejected",
			branches: []string{},
			wantErr:  "list is empty, omit the key instead",
		},
		{
			name:     "empty branch filter is rejected",
			branches: []string{"main", ""},
			wantErr:  "invalid branch filter",
		},
		{
			name:     "branch filter with a space is rejected",
			branches: []string{"my branch"},
			wantErr:  "invalid branch filter",
		},
		{
			// A quote would have to be escaped in the generated scalar, and no
			// git branch name may carry one.
			name:     "branch filter with a quote is rejected",
			branches: []string{"main'; rm -rf /"},
			wantErr:  "invalid branch filter",
		},
		{
			name:     "branch filter with a newline is rejected",
			branches: []string{"main\n      - injected"},
			wantErr:  "invalid branch filter",
		},
		{
			name:     "duplicate branch filter is rejected",
			branches: []string{"main", "staging", "main"},
			wantErr:  "duplicate branch filter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := GithubCIConfig{PushBranches: tt.branches}.validatePushBranches()

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validatePushBranches() unexpected error: %v", err)
				}

				return
			}

			if err == nil {
				t.Fatalf("validatePushBranches() = nil, want error containing %q", tt.wantErr)
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("validatePushBranches() error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestGithubCIConfig_IsValidPushBranches pins that push_branches is reachable
// through the validation entry point the config loader calls.
func TestGithubCIConfig_IsValidPushBranches(t *testing.T) {
	cfg := GithubCIConfig{PushBranches: []string{"main", "main"}}

	ok, msg := cfg.IsValid("github")
	if ok {
		t.Fatal("IsValid() accepted duplicate push branches")
	}

	if !strings.Contains(msg, "duplicate branch filter") {
		t.Errorf("IsValid() message = %q, want it to mention the duplicate branch filter", msg)
	}
}

// TestGithubCIConfig_PushBranchesViperDecoding pins the distinction the
// validation relies on: an omitted key decodes to a nil slice (defaults
// apply), an explicit empty list to a non-nil empty one (rejected).
func TestGithubCIConfig_PushBranchesViperDecoding(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    []string
		wantNil bool
	}{
		{
			name:    "omitted key",
			yaml:    "github_ci:\n  job_timeout_minutes: 30\n",
			wantNil: true,
		},
		{
			name: "explicit empty list",
			yaml: "github_ci:\n  push_branches: []\n",
			want: []string{},
		},
		{
			name: "branch list",
			yaml: "github_ci:\n  push_branches:\n    - main\n    - 'release/**'\n",
			want: []string{"main", "release/**"},
		},
		{
			name: "flow branch list",
			yaml: "github_ci:\n  push_branches: [main, staging]\n",
			want: []string{"main", "staging"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "project.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o600); err != nil {
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

			got := cfg.GithubCI.PushBranches

			if tt.wantNil {
				if got != nil {
					t.Fatalf("PushBranches = %+v, want nil", got)
				}

				return
			}

			if got == nil {
				t.Fatal("PushBranches = nil, want a non-nil slice")
			}

			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("PushBranches = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestGithubCIConfig_ParseBuildEnv covers the build_env entries: what the
// generated step may be handed and what it may not.
func TestGithubCIConfig_ParseBuildEnv(t *testing.T) {
	tests := []struct {
		name     string
		buildEnv []GithubCIEnvEntry
		want     []ds.GithubCIEnvVar
		wantErr  string
	}{
		{
			name: "omitted key yields no variables",
			want: nil,
		},
		{
			// Declaration order is the order the step's env: gets them, so it
			// has to survive the parse.
			name: "two variables keep their order and their value verbatim",
			buildEnv: []GithubCIEnvEntry{
				{Name: "IMAGE_REGISTRY", Value: "${{ vars.IMAGE_REGISTRY || format('{0}/{1}', secrets.A, vars.B) }}"},
				{Name: "PACKAGES_PUBLIC_BASE_URL", Value: "${{ vars.PACKAGES_PUBLIC_BASE_URL }}"},
			},
			want: []ds.GithubCIEnvVar{
				{Name: "IMAGE_REGISTRY", Value: "${{ vars.IMAGE_REGISTRY || format('{0}/{1}', secrets.A, vars.B) }}"},
				{Name: "PACKAGES_PUBLIC_BASE_URL", Value: "${{ vars.PACKAGES_PUBLIC_BASE_URL }}"},
			},
		},
		{
			name:     "lowercase name is accepted",
			buildEnv: []GithubCIEnvEntry{{Name: "_build_rev", Value: "1"}},
			want:     []ds.GithubCIEnvVar{{Name: "_build_rev", Value: "1"}},
		},
		{
			name:     "empty name is rejected",
			buildEnv: []GithubCIEnvEntry{{Name: "", Value: "x"}},
			wantErr:  "github_ci.build_env: name must start with a letter or underscore",
		},
		{
			name:     "name starting with a digit is rejected",
			buildEnv: []GithubCIEnvEntry{{Name: "1IMAGE", Value: "x"}},
			wantErr:  `name must start with a letter or underscore and contain only letters, digits and underscores: "1IMAGE"`,
		},
		{
			name:     "name with a dash is rejected",
			buildEnv: []GithubCIEnvEntry{{Name: "IMAGE-REGISTRY", Value: "x"}},
			wantErr:  `"IMAGE-REGISTRY"`,
		},
		{
			// A second REGISTRY_SERVER key in the step's env: would win over
			// the one the template sets, silently changing where the image is
			// pushed.
			name:     "name the step already sets is rejected",
			buildEnv: []GithubCIEnvEntry{{Name: "REGISTRY_SERVER", Value: "x"}},
			wantErr:  `name is already set by the generated "Build and push" step: "REGISTRY_SERVER"`,
		},
		{
			name:     "GITHUB_TOKEN is rejected as well",
			buildEnv: []GithubCIEnvEntry{{Name: "GITHUB_TOKEN", Value: "x"}},
			wantErr:  `"GITHUB_TOKEN"`,
		},
		{
			name: "duplicate name is rejected",
			buildEnv: []GithubCIEnvEntry{
				{Name: "IMAGE_REGISTRY", Value: "a"},
				{Name: "IMAGE_REGISTRY", Value: "b"},
			},
			wantErr: `github_ci.build_env: duplicate name: "IMAGE_REGISTRY"`,
		},
		{
			name:     "empty value is rejected",
			buildEnv: []GithubCIEnvEntry{{Name: "IMAGE_REGISTRY", Value: ""}},
			wantErr:  "github_ci.build_env: value must be a non-empty single-line string",
		},
		{
			// The tail of a multiline value would be read as workflow YAML of
			// its own.
			name:     "multiline value is rejected",
			buildEnv: []GithubCIEnvEntry{{Name: "IMAGE_REGISTRY", Value: "a\n          INJECTED: b"}},
			wantErr:  "value must be a non-empty single-line string",
		},
		{
			name:     "value with a control character is rejected",
			buildEnv: []GithubCIEnvEntry{{Name: "IMAGE_REGISTRY", Value: "a\tb"}},
			wantErr:  "value must be a non-empty single-line string",
		},
		{
			// The characters a workflow expression legitimately carries stay
			// in the value: the template quotes them.
			name:     "value with yaml metacharacters is accepted",
			buildEnv: []GithubCIEnvEntry{{Name: "TAG", Value: "* it: '#1' \"q\""}},
			want:     []ds.GithubCIEnvVar{{Name: "TAG", Value: "* it: '#1' \"q\""}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GithubCIConfig{BuildEnv: tt.buildEnv}.ParseBuildEnv()

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseBuildEnv() = %+v, want error containing %q", got, tt.wantErr)
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ParseBuildEnv() error = %q, want it to contain %q", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseBuildEnv() unexpected error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("ParseBuildEnv() = %+v, want %+v", got, tt.want)
			}

			for i, want := range tt.want {
				if got[i] != want {
					t.Errorf("ParseBuildEnv()[%d] = %+v, want %+v", i, got[i], want)
				}
			}
		})
	}
}

// TestGithubCIConfig_IsValidBuildEnv pins that build_env is reachable through
// the validation entry point the config loader calls.
func TestGithubCIConfig_IsValidBuildEnv(t *testing.T) {
	cfg := GithubCIConfig{BuildEnv: []GithubCIEnvEntry{{Name: "GITHUB_TOKEN", Value: "x"}}}

	ok, msg := cfg.IsValid("github")
	if ok {
		t.Fatal("IsValid() accepted a build_env variable the generated step already sets")
	}

	if !strings.Contains(msg, "github_ci.build_env") {
		t.Errorf("IsValid() message = %q, want it to mention github_ci.build_env", msg)
	}
}

// TestGithubCIConfig_BuildEnvViperDecoding pins the reason the key is a list of
// {name, value} and not a map: viper lowercases config keys, so a map key
// would arrive as a name the build never reads.
func TestGithubCIConfig_BuildEnvViperDecoding(t *testing.T) {
	yamlCfg := "github_ci:\n" +
		"  build_env:\n" +
		"    - name: IMAGE_REGISTRY\n" +
		"      value: \"${{ vars.IMAGE_REGISTRY }}\"\n"

	path := filepath.Join(t.TempDir(), "project.yaml")
	if err := os.WriteFile(path, []byte(yamlCfg), 0o600); err != nil {
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

	want := []GithubCIEnvEntry{{Name: "IMAGE_REGISTRY", Value: "${{ vars.IMAGE_REGISTRY }}"}}

	if len(cfg.GithubCI.BuildEnv) != 1 || cfg.GithubCI.BuildEnv[0] != want[0] {
		t.Fatalf("BuildEnv = %+v, want %+v", cfg.GithubCI.BuildEnv, want)
	}
}
