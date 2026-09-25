package grafana

import (
	"strings"
	"testing"
)

func TestGenerateDatasourceUID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "lowercase", in: "prometheus", want: "ds-prometheus"},
		{name: "mixed case", in: "VictoriaMetrics", want: "ds-victoriametrics"},
		{name: "spaces become dashes", in: "Main Metrics", want: "ds-main-metrics"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GenerateDatasourceUID(tt.in); got != tt.want {
				t.Errorf("GenerateDatasourceUID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveDatasourceUID(t *testing.T) {
	tests := []struct {
		name        string
		explicitUID string
		dsName      string
		want        string
	}{
		{
			name:   "no explicit uid falls back to the generated one",
			dsName: "Prometheus",
			want:   "ds-prometheus",
		},
		{
			name:        "explicit uid is used as is",
			explicitUID: "victoriametrics",
			dsName:      "Prometheus",
			want:        "victoriametrics",
		},
		{
			name:        "explicit uid keeps its case",
			explicitUID: "VictoriaMetrics",
			dsName:      "Prometheus",
			want:        "VictoriaMetrics",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveDatasourceUID(tt.explicitUID, tt.dsName)
			if got != tt.want {
				t.Errorf("ResolveDatasourceUID(%q, %q) = %q, want %q",
					tt.explicitUID, tt.dsName, got, tt.want)
			}
		})
	}
}

func TestIsValidDatasourceUID(t *testing.T) {
	tests := []struct {
		name string
		uid  string
		want bool
	}{
		{name: "plain", uid: "victoriametrics", want: true},
		{name: "generated form", uid: "ds-prometheus", want: true},
		{name: "digits, dash and underscore", uid: "vm_main-01", want: true},
		{name: "at the length limit", uid: strings.Repeat("a", MaxDatasourceUIDLen), want: true},
		{name: "empty", uid: "", want: false},
		{name: "over the length limit", uid: strings.Repeat("a", MaxDatasourceUIDLen+1), want: false},
		{name: "space", uid: "vm main", want: false},
		{name: "dot", uid: "vm.main", want: false},
		{name: "slash", uid: "vm/main", want: false},
		{name: "non-ascii", uid: "виктория", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidDatasourceUID(tt.uid); got != tt.want {
				t.Errorf("IsValidDatasourceUID(%q) = %v, want %v", tt.uid, got, tt.want)
			}
		})
	}
}
