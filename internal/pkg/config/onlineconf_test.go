package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnlineConf_IsEnabled(t *testing.T) {
	if (OnlineConf{}).IsEnabled() {
		t.Error("an absent onlineconf section must leave the feature off")
	}

	if !(OnlineConf{Path: "./onlineconf-key-map.yaml"}).IsEnabled() {
		t.Error("a configured path must switch the feature on")
	}
}

func TestOnlineConf_IsValid(t *testing.T) {
	baseDir := t.TempDir()

	localMap := filepath.Join(baseDir, "onlineconf-key-map.yaml")
	if err := os.WriteFile(localMap, []byte("keys: []"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	tests := []struct {
		name       string
		onlineConf OnlineConf
		wantOK     bool
		wantMsg    string
	}{
		{
			name:       "absent section is valid",
			onlineConf: OnlineConf{},
			wantOK:     true,
		},
		{
			name:       "existing local map",
			onlineConf: OnlineConf{Path: "./onlineconf-key-map.yaml"},
			wantOK:     true,
		},
		{
			name:       "missing local map",
			onlineConf: OnlineConf{Path: "./nope.yaml"},
			wantOK:     false,
			wantMsg:    "onlineconf.path: Invalid path: ./nope.yaml",
		},
		{
			// Remote maps are fetched at generation time, so existence is not
			// checked here — only the URI itself. The form is the one
			// metrics.path and rest[].path take, so a contract repository
			// serves both from the same reference.
			name:       "git+https URI",
			onlineConf: OnlineConf{Path: "git+https://github.com/org/contracts.git@v1#common/onlineconf-key-map.yaml"},
			wantOK:     true,
		},
		{
			name:       "git+ssh URI",
			onlineConf: OnlineConf{Path: "git+ssh://git@github.com/org/contracts.git@v1#common/onlineconf-key-map.yaml"},
			wantOK:     true,
		},
		{
			name:       "https URL",
			onlineConf: OnlineConf{Path: "https://example.com/onlineconf-key-map.yaml"},
			wantOK:     true,
		},
		{
			name:       "git URI without a subpath",
			onlineConf: OnlineConf{Path: "git+https://github.com/org/contracts.git@v1"},
			wantOK:     false,
			wantMsg:    "subpath",
		},
		{
			name:       "scp-style git URL is rejected",
			onlineConf: OnlineConf{Path: "git@github.com:org/contracts.git"},
			wantOK:     false,
			wantMsg:    "onlineconf.path:",
		},
		{
			name:       "unknown scheme",
			onlineConf: OnlineConf{Path: "ftp://example.com/onlineconf-key-map.yaml"},
			wantOK:     false,
			wantMsg:    "onlineconf.path:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, msg := tt.onlineConf.IsValid(baseDir)

			if ok != tt.wantOK {
				t.Errorf("IsValid() ok = %v, want %v (msg: %s)", ok, tt.wantOK, msg)
			}

			if tt.wantMsg != "" && !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("IsValid() msg = %q, want it to contain %q", msg, tt.wantMsg)
			}
		})
	}
}
