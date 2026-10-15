package grafana

import "testing"

func TestResolveDatasourceUID(t *testing.T) {
	tests := []struct {
		name     string
		uid      string
		dsName   string
		expected string
	}{
		{
			name:     "explicit uid wins over the generated one",
			uid:      "victoriametrics",
			dsName:   "Prometheus",
			expected: "victoriametrics",
		},
		{
			name:     "empty uid falls back to ds-<name>",
			uid:      "",
			dsName:   "Prometheus",
			expected: "ds-prometheus",
		},
		{
			name:     "fallback lowercases and dashes spaces",
			uid:      "",
			dsName:   "My Loki",
			expected: "ds-my-loki",
		},
		{
			// An explicit uid is passed through verbatim: unlike the fallback it
			// is not lowercased, because Grafana treats uids case-sensitively and
			// the shared instance may provision a mixed-case one.
			name:     "explicit uid keeps its case",
			uid:      "VictoriaMetrics",
			dsName:   "Prometheus",
			expected: "VictoriaMetrics",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveDatasourceUID(tt.uid, tt.dsName); got != tt.expected {
				t.Errorf("ResolveDatasourceUID(%q, %q) = %q, want %q", tt.uid, tt.dsName, got, tt.expected)
			}
		})
	}
}

func TestValidateDatasourceUID(t *testing.T) {
	tests := []struct {
		name  string
		uid   string
		valid bool
	}{
		{name: "lowercase letters", uid: "victoriametrics", valid: true},
		{name: "mixed case with digits", uid: "VictoriaMetrics2", valid: true},
		{name: "dashes and underscores", uid: "ds_shared-metrics", valid: true},
		{name: "single character", uid: "a", valid: true},
		{name: "exactly 40 characters", uid: "0123456789012345678901234567890123456789", valid: true},
		{name: "empty", uid: "", valid: false},
		{name: "41 characters", uid: "01234567890123456789012345678901234567890", valid: false},
		{name: "dot is not allowed", uid: "victoria.metrics", valid: false},
		{name: "space is not allowed", uid: "victoria metrics", valid: false},
		{name: "slash is not allowed", uid: "victoria/metrics", valid: false},
		{name: "non-latin letters are not allowed", uid: "виктория", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateDatasourceUID(tt.uid); got != tt.valid {
				t.Errorf("ValidateDatasourceUID(%q) = %v, want %v", tt.uid, got, tt.valid)
			}
		})
	}
}

// TestGenerateDatasourceUIDPassesValidation guards the default branch: a
// generated uid must itself be acceptable to Grafana, otherwise configs that
// never set uid would start failing validation.
func TestGenerateDatasourceUIDPassesValidation(t *testing.T) {
	for _, name := range []string{"Prometheus", "Loki", "My Shared Prometheus"} {
		uid := GenerateDatasourceUID(name)
		if !ValidateDatasourceUID(uid) {
			t.Errorf("GenerateDatasourceUID(%q) = %q, which fails ValidateDatasourceUID", name, uid)
		}
	}
}
