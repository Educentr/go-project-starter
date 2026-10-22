package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// contractDir is a config dir with one local spec, so Contract.IsValid's
// existence check has something to find.
func contractDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "limits.yaml"), []byte("limits: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestCustomGenerator_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		gen     CustomGenerator
		wantOK  bool
		wantMsg string
	}{
		{
			name:   "all three commands",
			gen:    CustomGenerator{Name: "limits_ts", Install: "npm ci --prefix web", Cmd: "node gen.mjs {{ .Spec }} --out {{ .Target }}", Lint: "npx spec-lint {{ .Spec }}"},
			wantOK: true,
		},
		{
			name:   "cmd only — install and lint are optional",
			gen:    CustomGenerator{Name: "limits-ts", Cmd: "go run ./cmd/gen {{ .Spec }}"},
			wantOK: true,
		},
		{
			name:   "cmd without variables is fine",
			gen:    CustomGenerator{Name: "static", Cmd: "make -C web build"},
			wantOK: true,
		},
		{
			name:    "empty name",
			gen:     CustomGenerator{Cmd: "x"},
			wantMsg: errMsgEmptyGeneratorName,
		},
		{
			name:    "uppercase name",
			gen:     CustomGenerator{Name: "LimitsTS", Cmd: "x"},
			wantMsg: "must match",
		},
		{
			name:    "name with a dot",
			gen:     CustomGenerator{Name: "limits.ts", Cmd: "x"},
			wantMsg: "must match",
		},
		{
			name:    "name collides with a built-in make target",
			gen:     CustomGenerator{Name: "ogen", Cmd: "x"},
			wantMsg: "collides with a built-in make target",
		},
		{
			name:    "name uses a reserved prefix",
			gen:     CustomGenerator{Name: "test-env-api", Cmd: "x"},
			wantMsg: "reserved prefix",
		},
		{
			name:    "empty cmd",
			gen:     CustomGenerator{Name: "limits_ts", Install: "npm ci"},
			wantMsg: "empty cmd",
		},
		{
			name:    "blank cmd",
			gen:     CustomGenerator{Name: "limits_ts", Cmd: "   "},
			wantMsg: "empty cmd",
		},
		{
			name:    "install takes no variables",
			gen:     CustomGenerator{Name: "limits_ts", Install: "npm ci {{ .Spec }}", Cmd: "x"},
			wantMsg: "install takes no template variables",
		},
		{
			name:    "unparsable cmd template",
			gen:     CustomGenerator{Name: "limits_ts", Cmd: "node gen.mjs {{ .Spec"},
			wantMsg: "invalid template",
		},
		{
			name:    "unknown variable in cmd",
			gen:     CustomGenerator{Name: "limits_ts", Cmd: "node gen.mjs {{ .Source }}"},
			wantMsg: "unknown generator command variable: .Source",
		},
		{
			name:    "unknown variable in lint",
			gen:     CustomGenerator{Name: "limits_ts", Cmd: "x", Lint: "lint {{ .Copy }}"},
			wantMsg: "unknown generator command variable: .Copy",
		},
		{
			name:    "nested field on a string variable",
			gen:     CustomGenerator{Name: "limits_ts", Cmd: "node gen.mjs {{ .Spec.Path }}"},
			wantMsg: "has no field .Path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, msg := tt.gen.IsValid()

			if ok != tt.wantOK {
				t.Fatalf("IsValid() ok = %v, want %v (msg: %s)", ok, tt.wantOK, msg)
			}

			if tt.wantMsg != "" && !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("IsValid() msg = %q, want it to contain %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestContract_IsValid(t *testing.T) {
	dir := contractDir(t)

	tests := []struct {
		name     string
		contract Contract
		wantOK   bool
		wantMsg  string
	}{
		{
			name:     "local spec with target",
			contract: Contract{Path: "./limits.yaml", Copy: "api/contract/limits.yaml", Target: "web/src/limits.ts", GeneratorName: "limits_ts"},
			wantOK:   true,
		},
		{
			name:     "remote git spec is not checked on disk",
			contract: Contract{Path: "git+ssh://git@github.com/org/contracts.git#limits/limits.yaml", Copy: "api/contract/limits.yaml", GeneratorName: "limits_ts"},
			wantOK:   true,
		},
		{
			name:     "empty path",
			contract: Contract{Copy: "api/x.yaml", GeneratorName: "g"},
			wantMsg:  errMsgEmptyContractPath,
		},
		{
			name:     "missing local spec",
			contract: Contract{Path: "./nope.yaml", Copy: "api/x.yaml", GeneratorName: "g"},
			wantMsg:  "Invalid path",
		},
		{
			name:     "missing copy",
			contract: Contract{Path: "./limits.yaml", GeneratorName: "g"},
			wantMsg:  "copy is required",
		},
		{
			name:     "absolute copy",
			contract: Contract{Path: "./limits.yaml", Copy: "/etc/limits.yaml", GeneratorName: "g"},
			wantMsg:  "must be relative to the project root",
		},
		{
			name:     "copy escaping the project root",
			contract: Contract{Path: "./limits.yaml", Copy: "../limits.yaml", GeneratorName: "g"},
			wantMsg:  "must not escape the project root",
		},
		{
			name:     "unclean copy",
			contract: Contract{Path: "./limits.yaml", Copy: "./api/limits.yaml", GeneratorName: "g"},
			wantMsg:  "must be a clean path",
		},
		{
			name:     "absolute target",
			contract: Contract{Path: "./limits.yaml", Copy: "api/limits.yaml", Target: "/srv/out.ts", GeneratorName: "g"},
			wantMsg:  "must be relative to the project root",
		},
		{
			name:     "missing generator_name",
			contract: Contract{Path: "./limits.yaml", Copy: "api/limits.yaml"},
			wantMsg:  "generator_name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, msg := tt.contract.IsValid(dir)

			if ok != tt.wantOK {
				t.Fatalf("IsValid() ok = %v, want %v (msg: %s)", ok, tt.wantOK, msg)
			}

			if tt.wantMsg != "" && !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("IsValid() msg = %q, want it to contain %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestValidateGenerators(t *testing.T) {
	dir := contractDir(t)

	withTarget := CustomGenerator{Name: "limits_ts", Cmd: "node gen.mjs {{ .Spec }} --out {{ .Target }}"}
	specOnly := CustomGenerator{Name: "limits_ts", Cmd: "node gen.mjs {{ .Spec }}"}
	lintWantsTarget := CustomGenerator{Name: "limits_ts", Cmd: "node gen.mjs {{ .Spec }}", Lint: "lint {{ .Target }}"}

	tests := []struct {
		name       string
		generators CustomGeneratorList
		contracts  ContractList
		wantOK     bool
		wantMsg    string
	}{
		{
			name:   "both sections empty",
			wantOK: true,
		},
		{
			name:       "generator with two contracts",
			generators: CustomGeneratorList{withTarget},
			contracts: ContractList{
				{Path: "./limits.yaml", Copy: "api/a.yaml", Target: "web/a.ts", GeneratorName: "limits_ts"},
				{Path: "./limits.yaml", Copy: "api/b.yaml", Target: "web/b.ts", GeneratorName: "limits_ts"},
			},
			wantOK: true,
		},
		{
			name:       "generator declared but unused",
			generators: CustomGeneratorList{specOnly},
			wantOK:     true,
		},
		{
			name:       "duplicate generator name",
			generators: CustomGeneratorList{specOnly, specOnly},
			wantMsg:    "duplicate generator name: limits_ts",
		},
		{
			name:       "unknown generator_name",
			generators: CustomGeneratorList{specOnly},
			contracts:  ContractList{{Path: "./limits.yaml", Copy: "api/a.yaml", GeneratorName: "other"}},
			wantMsg:    "unknown generator_name: other",
		},
		{
			name:       "duplicate copy path",
			generators: CustomGeneratorList{specOnly},
			contracts: ContractList{
				{Path: "./limits.yaml", Copy: "api/a.yaml", GeneratorName: "limits_ts"},
				{Path: "./limits.yaml", Copy: "api/a.yaml", GeneratorName: "limits_ts"},
			},
			wantMsg: "duplicate contract copy path 'api/a.yaml'",
		},
		{
			name:       "cmd reads .Target but contract has none",
			generators: CustomGeneratorList{withTarget},
			contracts:  ContractList{{Path: "./limits.yaml", Copy: "api/a.yaml", GeneratorName: "limits_ts"}},
			wantMsg:    "must set 'target'",
		},
		{
			name:       "lint reads .Target but contract has none",
			generators: CustomGeneratorList{lintWantsTarget},
			contracts:  ContractList{{Path: "./limits.yaml", Copy: "api/a.yaml", GeneratorName: "limits_ts"}},
			wantMsg:    "must set 'target'",
		},
		{
			name:       "contract without target is fine when no command reads it",
			generators: CustomGeneratorList{specOnly},
			contracts:  ContractList{{Path: "./limits.yaml", Copy: "api/a.yaml", GeneratorName: "limits_ts"}},
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, msg := ValidateGenerators(dir, tt.generators, tt.contracts)

			if ok != tt.wantOK {
				t.Fatalf("ValidateGenerators() ok = %v, want %v (msg: %s)", ok, tt.wantOK, msg)
			}

			if tt.wantMsg != "" && !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("ValidateGenerators() msg = %q, want it to contain %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestRenderGeneratorCommand(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		spec    string
		target  string
		want    string
		wantErr bool
	}{
		{
			name:   "both variables",
			cmd:    "node gen.mjs {{ .Spec }} --out {{ .Target }}",
			spec:   "api/contract/limits.yaml",
			target: "web/src/limits.ts",
			want:   "node gen.mjs api/contract/limits.yaml --out web/src/limits.ts",
		},
		{
			name: "no variables",
			cmd:  "make -C web build",
			spec: "api/contract/limits.yaml",
			want: "make -C web build",
		},
		{
			name: "surrounding whitespace is trimmed",
			cmd:  "  go run ./cmd/gen {{ .Spec }}  ",
			spec: "api/a.yaml",
			want: "go run ./cmd/gen api/a.yaml",
		},
		{
			name:    "multi-line command would break the recipe",
			cmd:     "go run ./a {{ .Spec }}\ngo run ./b {{ .Spec }}",
			spec:    "api/a.yaml",
			wantErr: true,
		},
		{
			name:    "command rendering to nothing",
			cmd:     "{{ .Target }}",
			spec:    "api/a.yaml",
			wantErr: true,
		},
		{
			name:    "unparsable template",
			cmd:     "go run {{ .Spec",
			spec:    "api/a.yaml",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderGeneratorCommand(tt.cmd, tt.spec, tt.target)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("RenderGeneratorCommand() = %q, want error", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("RenderGeneratorCommand() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("RenderGeneratorCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGeneratorCommandVars(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		want    map[string]bool
		wantErr bool
	}{
		{
			name: "both",
			cmd:  "gen {{ .Spec }} {{ .Target }}",
			want: map[string]bool{GeneratorVarSpec: true, GeneratorVarTarget: true},
		},
		{
			name: "none",
			cmd:  "gen --all",
			want: map[string]bool{},
		},
		{
			name: "inside a conditional — still counts as referenced",
			cmd:  `gen {{ if .Target }}--out {{ .Target }}{{ end }}`,
			want: map[string]bool{GeneratorVarTarget: true},
		},
		{
			name: "through a pipeline",
			cmd:  `gen {{ .Spec | printf "%s" }}`,
			want: map[string]bool{GeneratorVarSpec: true},
		},
		{
			name:    "unknown variable",
			cmd:     "gen {{ .Nope }}",
			wantErr: true,
		},
		{
			name:    "unknown variable inside a conditional",
			cmd:     "gen {{ if .Nope }}x{{ end }}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generatorCommandVars(tt.cmd)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("generatorCommandVars() = %v, want error", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("generatorCommandVars() error = %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("generatorCommandVars() = %v, want %v", got, tt.want)
			}

			for name := range tt.want {
				if !got[name] {
					t.Errorf("generatorCommandVars() missing %s (got %v)", name, got)
				}
			}
		})
	}
}
