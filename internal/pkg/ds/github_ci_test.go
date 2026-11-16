package ds

import (
	"testing"
)

func TestRunsOnSpec_YAML(t *testing.T) {
	tests := []struct {
		name string
		spec RunsOnSpec
		want string
	}{
		{
			name: "not configured falls back to default runner",
			spec: RunsOnSpec{},
			want: "ubuntu-latest",
		},
		{
			name: "single label renders as bare scalar",
			spec: RunsOnSpec{Labels: []string{"self-hosted"}},
			want: "self-hosted",
		},
		{
			name: "multiple labels render as flow sequence",
			spec: RunsOnSpec{Labels: []string{"self-hosted", "linux", "x64"}},
			want: "[self-hosted, linux, x64]",
		},
		{
			name: "group renders as flow mapping",
			spec: RunsOnSpec{Group: "my-runners"},
			want: "{group: my-runners}",
		},
		{
			name: "group with labels renders both keys",
			spec: RunsOnSpec{Group: "my-runners", Labels: []string{"linux", "x64"}},
			want: "{group: my-runners, labels: [linux, x64]}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.YAML(); got != tt.want {
				t.Errorf("RunsOnSpec.YAML() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGithubCIConfig_JobHeader(t *testing.T) {
	tests := []struct {
		name string
		cfg  GithubCIConfig
		want string
	}{
		{
			name: "zero config keeps the historical output",
			cfg:  GithubCIConfig{},
			want: "runs-on: ubuntu-latest",
		},
		{
			name: "timeout adds a second line with job-level indentation",
			cfg:  GithubCIConfig{JobTimeoutMinutes: 30},
			want: "runs-on: ubuntu-latest\n    timeout-minutes: 30",
		},
		{
			name: "runner and timeout combined",
			cfg: GithubCIConfig{
				RunsOn:            RunsOnSpec{Group: "my-runners"},
				JobTimeoutMinutes: 5,
			},
			want: "runs-on: {group: my-runners}\n    timeout-minutes: 5",
		},
		{
			name: "non-positive timeout is not emitted",
			cfg:  GithubCIConfig{JobTimeoutMinutes: 0},
			want: "runs-on: ubuntu-latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.JobHeader(); got != tt.want {
				t.Errorf("GithubCIConfig.JobHeader() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGithubCIConfig_DoctlFromImage(t *testing.T) {
	tests := []struct {
		name string
		cfg  GithubCIConfig
		want bool
	}{
		{
			name: "zero config keeps the install step",
			cfg:  GithubCIConfig{},
			want: false,
		},
		{
			name: "explicit install mode keeps the install step",
			cfg:  GithubCIConfig{DoctlMode: DoctlModeInstall},
			want: false,
		},
		{
			name: "image mode drops the install step",
			cfg:  GithubCIConfig{DoctlMode: DoctlModeImage},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.DoctlFromImage(); got != tt.want {
				t.Errorf("GithubCIConfig.DoctlFromImage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGithubCIConfig_PushBranchesBlock(t *testing.T) {
	tests := []struct {
		name string
		cfg  GithubCIConfig
		want string
	}{
		{
			name: "zero config renders the historical branch list",
			cfg:  GithubCIConfig{},
			want: "- main\n      - staging",
		},
		{
			name: "configured branches replace the defaults",
			cfg:  GithubCIConfig{PushBranches: []string{"trunk"}},
			want: "- trunk",
		},
		{
			name: "glob metacharacters stay plain",
			cfg:  GithubCIConfig{PushBranches: []string{"main", "release/**", "v1.?"}},
			want: "- main\n      - release/**\n      - v1.?",
		},
		{
			name: "leading exclamation mark is quoted",
			cfg:  GithubCIConfig{PushBranches: []string{"releases/**", "!releases/**-alpha"}},
			want: "- releases/**\n      - '!releases/**-alpha'",
		},
		{
			name: "character range brackets are quoted",
			cfg:  GithubCIConfig{PushBranches: []string{"release/[0-9]*"}},
			want: "- 'release/[0-9]*'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.PushBranchesBlock(); got != tt.want {
				t.Errorf("GithubCIConfig.PushBranchesBlock() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGithubCIConfig_PushBranchesFlow(t *testing.T) {
	tests := []struct {
		name string
		cfg  GithubCIConfig
		want string
	}{
		{
			name: "zero config renders the historical branch list",
			cfg:  GithubCIConfig{},
			want: "[main, staging]",
		},
		{
			name: "configured branches replace the defaults",
			cfg:  GithubCIConfig{PushBranches: []string{"main", "release/**"}},
			want: "[main, release/**]",
		},
		{
			name: "flow indicators are quoted so the sequence stays parseable",
			cfg:  GithubCIConfig{PushBranches: []string{"release/[0-9]*", "!main"}},
			want: "['release/[0-9]*', '!main']",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.PushBranchesFlow(); got != tt.want {
				t.Errorf("GithubCIConfig.PushBranchesFlow() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGithubCIConfig_PushBranchesDefaultNotMutated guards defaultPushBranches
// against being handed out and then appended to by a caller: the package-level
// slice backs the output of every project that does not configure the key.
func TestGithubCIConfig_PushBranchesDefaultNotMutated(t *testing.T) {
	want := []string{"main", "staging"}

	cfg := GithubCIConfig{}

	if got := cfg.PushBranchesFlow(); got != "[main, staging]" {
		t.Fatalf("unexpected default rendering: %q", got)
	}

	if len(defaultPushBranches) != len(want) {
		t.Fatalf("defaultPushBranches = %v, want %v", defaultPushBranches, want)
	}

	for i, branch := range want {
		if defaultPushBranches[i] != branch {
			t.Errorf("defaultPushBranches[%d] = %q, want %q", i, defaultPushBranches[i], branch)
		}
	}
}
