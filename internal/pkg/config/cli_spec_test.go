package config

import (
	"errors"
	"strings"
	"testing"
)

// TestCLISpec_Validate_Names covers the name charset rule. Names are not just
// CLI tokens: they become Go method names, struct names and map keys in the
// generated handler, so anything outside [a-zA-Z0-9_-] must be rejected at
// config load time rather than produce source that does not compile.
func TestCLISpec_Validate_Names(t *testing.T) {
	tests := []struct {
		name    string
		spec    CLISpec
		wantErr bool
	}{
		{
			name: "plain names are accepted",
			spec: CLISpec{Commands: []CLISpecCommand{
				{Name: "migrate", Flags: []CLISpecFlag{{Name: "dry-run", Type: "bool"}}},
			}},
		},
		{
			name: "snake_case and kebab-case are accepted",
			spec: CLISpec{Commands: []CLISpecCommand{
				{Name: "sync-all", Subcommands: []CLISpecSubcommand{{Name: "from_db"}}},
			}},
		},
		{
			name: "command name with a space is rejected",
			spec: CLISpec{Commands: []CLISpecCommand{
				{Name: "user create"},
			}},
			wantErr: true,
		},
		{
			name: "command name with a quote is rejected",
			spec: CLISpec{Commands: []CLISpecCommand{
				{Name: `pi"ng`},
			}},
			wantErr: true,
		},
		{
			name: "command name starting with a digit is rejected",
			spec: CLISpec{Commands: []CLISpecCommand{
				{Name: "2fa"},
			}},
			wantErr: true,
		},
		{
			name: "subcommand name with a quote is rejected",
			spec: CLISpec{Commands: []CLISpecCommand{
				{Name: "user", Subcommands: []CLISpecSubcommand{{Name: `cre"ate`}}},
			}},
			wantErr: true,
		},
		{
			name: "flag name with a space is rejected",
			spec: CLISpec{Commands: []CLISpecCommand{
				{Name: "migrate", Flags: []CLISpecFlag{{Name: "dry run", Type: "bool"}}},
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()

			switch {
			case tt.wantErr && err == nil:
				t.Fatal("Validate() = nil, want an error")
			case tt.wantErr && !errors.Is(err, ErrInvalidName):
				t.Fatalf("Validate() = %v, want it to wrap ErrInvalidName", err)
			case !tt.wantErr && err != nil:
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

// TestCLISpec_Validate_FlagDefaults covers typed defaults. A default reaches
// the generated flag declaration verbatim for non-string types, so `steps: 1e9`
// on an int flag used to emit `fs.Int("steps", 1e9, ...)` — source that does
// not compile. Rejecting it here keeps the failure on the config, where the
// message can point at the offending flag.
func TestCLISpec_Validate_FlagDefaults(t *testing.T) {
	tests := []struct {
		name     string
		flagType string
		def      string
		wantErr  bool
	}{
		{name: "empty default is always fine", flagType: "int", def: ""},
		{name: "valid int", flagType: "int", def: "100"},
		{name: "invalid int", flagType: "int", def: "many", wantErr: true},
		{name: "valid bool", flagType: "bool", def: "true"},
		{name: "invalid bool", flagType: "bool", def: "yes-please", wantErr: true},
		{name: "valid float64", flagType: "float64", def: "1.5"},
		{name: "invalid float64", flagType: "float64", def: "1,5", wantErr: true},
		{name: "valid duration", flagType: "duration", def: "5s"},
		{name: "invalid duration", flagType: "duration", def: "5 seconds", wantErr: true},
		// A string default is escaped on the way into generated source, so any
		// content is legal — including quotes and backslashes.
		{name: "string default with quotes", flagType: "string", def: `fixtures "default".json`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := CLISpec{Commands: []CLISpecCommand{{
				Name:  "seed",
				Flags: []CLISpecFlag{{Name: "opt", Type: tt.flagType, Default: tt.def}},
			}}}

			err := spec.Validate()

			switch {
			case tt.wantErr && err == nil:
				t.Fatal("Validate() = nil, want an error")
			case tt.wantErr && !strings.Contains(err.Error(), "opt"):
				t.Fatalf("Validate() = %v, want the error to name the offending flag", err)
			case !tt.wantErr && err != nil:
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

// TestCLISpec_Validate_DescriptionsAreFreeText guards the other half of the
// escaping fix: descriptions may contain quotes, backslashes and newlines, and
// validation must NOT reject them — the templates quote them instead.
func TestCLISpec_Validate_DescriptionsAreFreeText(t *testing.T) {
	spec := CLISpec{Commands: []CLISpecCommand{{
		Name:        "seed",
		Description: "Seed the database with \"fixtures\".\nSecond paragraph mentions C:\\data.",
		Flags: []CLISpecFlag{{
			Name:        "file",
			Type:        "string",
			Description: `Path to the "fixtures" file`,
		}},
	}}}

	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil — descriptions are free text", err)
	}
}
