package ds

import (
	"testing"
)

func TestChecksConfig_Enabled(t *testing.T) {
	tests := []struct {
		name   string
		checks ChecksConfig
		want   bool
	}{
		{
			name:   "no checks declared",
			checks: ChecksConfig{},
			want:   false,
		},
		{
			name:   "empty but non-nil slice",
			checks: ChecksConfig{Checks: []Check{}},
			want:   false,
		},
		{
			name:   "one check",
			checks: ChecksConfig{Checks: []Check{{Name: "lint", Run: "make lint"}}},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.checks.Enabled(); got != tt.want {
				t.Errorf("ChecksConfig.Enabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChecksConfig_HasRegistryLogin(t *testing.T) {
	tests := []struct {
		name   string
		checks ChecksConfig
		want   bool
	}{
		{
			name:   "no checks declared",
			checks: ChecksConfig{},
			want:   false,
		},
		{
			name: "no check needs the registry",
			checks: ChecksConfig{Checks: []Check{
				{Name: "lint", Run: "make lint"},
				{Name: "unit-tests", Run: "make unit-tests"},
			}},
			want: false,
		},
		{
			name: "last check needs the registry",
			checks: ChecksConfig{Checks: []Check{
				{Name: "lint", Run: "make lint"},
				{Name: "guards", Run: "make lint-guards", RegistryLogin: true},
			}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.checks.HasRegistryLogin(); got != tt.want {
				t.Errorf("ChecksConfig.HasRegistryLogin() = %v, want %v", got, tt.want)
			}
		})
	}
}
