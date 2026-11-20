package ds

import "testing"

// TestMetricsExport_Predicates pins the one invariant templates rely on: the
// predicates are mutually exclusive and exactly one of them holds for every
// channel the generator resolves.
func TestMetricsExport_Predicates(t *testing.T) {
	for _, tt := range []struct {
		export MetricsExport
		pull   bool
		push   bool
		none   bool
	}{
		{MetricsExportPull, true, false, false},
		{MetricsExportPush, false, true, false},
		{MetricsExportNone, false, false, true},
	} {
		t.Run(tt.export.String(), func(t *testing.T) {
			if got := tt.export.IsPull(); got != tt.pull {
				t.Errorf("IsPull() = %v, want %v", got, tt.pull)
			}

			if got := tt.export.IsPush(); got != tt.push {
				t.Errorf("IsPush() = %v, want %v", got, tt.push)
			}

			if got := tt.export.IsNone(); got != tt.none {
				t.Errorf("IsNone() = %v, want %v", got, tt.none)
			}

			// The comment lands in the generated main as a single line: a
			// newline in it would comment out the code that follows.
			comment := tt.export.Comment()
			if comment == "" {
				t.Error("Comment() must describe the channel")
			}

			for _, r := range comment {
				if r == '\n' || r == '\r' {
					t.Errorf("Comment() must stay on one line, got %q", comment)
				}
			}
		})
	}
}
