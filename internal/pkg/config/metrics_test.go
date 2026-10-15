package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetrics_IsEnabled(t *testing.T) {
	if (Metrics{}).IsEnabled() {
		t.Error("an absent metrics section must leave the feature off")
	}

	if !(Metrics{Path: "./metrics-map.yaml"}).IsEnabled() {
		t.Error("a configured path must switch the feature on")
	}
}

func TestMetrics_IsValid(t *testing.T) {
	baseDir := t.TempDir()

	localMap := filepath.Join(baseDir, "metrics-map.yaml")
	if err := os.WriteFile(localMap, []byte("metrics: []"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	tests := []struct {
		name    string
		metrics Metrics
		wantOK  bool
		wantMsg string
	}{
		{
			name:    "absent section is valid",
			metrics: Metrics{},
			wantOK:  true,
		},
		{
			name:    "existing local map",
			metrics: Metrics{Path: "./metrics-map.yaml"},
			wantOK:  true,
		},
		{
			name:    "missing local map",
			metrics: Metrics{Path: "./nope.yaml"},
			wantOK:  false,
			wantMsg: "metrics.path: Invalid path: ./nope.yaml",
		},
		{
			// Remote maps are fetched at generation time, so existence is not
			// checked here — only the URI itself.
			name:    "git+https URI",
			metrics: Metrics{Path: "git+https://github.com/org/contracts.git@v1#common/metrics-map.yaml"},
			wantOK:  true,
		},
		{
			name:    "https URL",
			metrics: Metrics{Path: "https://example.com/metrics-map.yaml"},
			wantOK:  true,
		},
		{
			name:    "git URI without a subpath",
			metrics: Metrics{Path: "git+https://github.com/org/contracts.git@v1"},
			wantOK:  false,
			wantMsg: "subpath",
		},
		{
			name:    "scp-style git URL is rejected",
			metrics: Metrics{Path: "git@github.com:org/contracts.git"},
			wantOK:  false,
			wantMsg: "metrics.path:",
		},
		{
			name:    "unknown scheme",
			metrics: Metrics{Path: "ftp://example.com/metrics-map.yaml"},
			wantOK:  false,
			wantMsg: "metrics.path:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, msg := tt.metrics.IsValid(baseDir)

			if ok != tt.wantOK {
				t.Errorf("IsValid() ok = %v, want %v (msg: %s)", ok, tt.wantOK, msg)
			}

			if tt.wantMsg != "" && !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("IsValid() msg = %q, want it to contain %q", msg, tt.wantMsg)
			}
		})
	}
}
