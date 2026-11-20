package ds

import "testing"

// TestGetTargetSpecFile_DerivesNameFromURI pins the name the templates bake
// into the generated Makefile. They are rendered BEFORE the resolve pre-pass,
// so SpecTargetFiles is still empty and the name has to be derived from the raw
// `path:` entry — which is where reading a URI as a filesystem path used to
// produce "contracts.git#api.swagger.yml" and make `make generate-ogen` fail
// with "no such file or directory" (issue #24).
func TestGetTargetSpecFile_DerivesNameFromURI(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "git top-level fragment",
			path: "git+ssh://git@github.com/example/contracts.git#api.swagger.yml",
			want: "api.swagger.yml",
		},
		{
			name: "git nested fragment",
			path: "git+ssh://git@github.com/example/contracts.git#specs/api.swagger.yml",
			want: "api.swagger.yml",
		},
		{
			name: "git pinned ref with top-level fragment",
			path: "git+https://github.com/example/contracts.git@v1.2.3#common.swagger.yml",
			want: "common.swagger.yml",
		},
		{
			name: "git https with token_env and top-level fragment",
			path: "git+https://github.com/example/contracts.git@main?token_env=CONTRACTS_TOKEN#api.swagger.yml",
			want: "api.swagger.yml",
		},
		{
			name: "http url",
			path: "https://specs.example.com/openapi/users.yaml",
			want: "users.yaml",
		},
		{
			name: "http url with token_env query",
			path: "https://specs.example.com/users.yaml?token_env=SPECS_TOKEN",
			want: "users.yaml",
		},
		{
			name: "local relative path",
			path: "./api.swagger.yml",
			want: "api.swagger.yml",
		},
		{
			name: "local nested path",
			path: "../contracts/api.swagger.yml",
			want: "api.swagger.yml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := Transport{SpecPath: []string{tt.path}}

			if got := tr.GetTargetSpecFile(0); got != tt.want {
				t.Errorf("GetTargetSpecFile(0) = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGetTargetSpecFile_PrefersResolvedTarget keeps the resolve pre-pass
// authoritative: once it recorded a name, that name is used verbatim even
// though the raw path would yield the same basename for two transports.
func TestGetTargetSpecFile_PrefersResolvedTarget(t *testing.T) {
	tr := Transport{
		SpecPath:        []string{"/tmp/gps-specs-123/files/1-shared.yaml"},
		SpecTargetFiles: []string{"shared.yaml"},
	}

	if got := tr.GetTargetSpecFile(0); got != "shared.yaml" {
		t.Errorf("GetTargetSpecFile(0) = %q, want %q", got, "shared.yaml")
	}
}

// TestGetSchemaFilenames_DerivesNameFromURI covers the same defect on the JSON
// Schema channel: the names feed `make generate-jsonschema`, which reads
// api/schema/<name>/<filename>.json — the file the copy pass wrote under the
// basename of the git subpath.
func TestGetSchemaFilenames_DerivesNameFromURI(t *testing.T) {
	schema := JSONSchema{
		Name: "events",
		Path: []string{
			"git+ssh://git@github.com/example/contracts.git#order.json",
			"git+ssh://git@github.com/example/contracts.git#schemas/user.json",
			"./local.json",
		},
	}

	got := schema.GetSchemaFilenames()
	want := []string{"order", "user", "local"}

	if len(got) != len(want) {
		t.Fatalf("GetSchemaFilenames() = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GetSchemaFilenames()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
