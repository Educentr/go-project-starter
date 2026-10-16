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
		name    string
		cfg     GithubCIConfig
		wantOK  bool
		wantMsg string
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
			ok, msg := tt.cfg.IsValid()

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
