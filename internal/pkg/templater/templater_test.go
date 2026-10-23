package templater

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"text/template"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/meta"
)

// makefileNodeKind classifies one element of a parsed make target line.
type makefileNodeKind int

// makefilePrereqNode is one element of a parsed make target line, built by
// parseMakefilePrereqLine at the bottom of this file.
type makefilePrereqNode struct {
	kind makefileNodeKind
	// src is the span of the line this node was parsed from, quoted back in
	// the failure message so it names the offending groups.
	src      string
	children []makefilePrereqNode
	// isRange marks a group whose body repeats once per element.
	isRange bool
}

// makefileTmplPath is the embedded template backing the generated Makefile,
// read directly by the prerequisite-list tests at the bottom of this file.
const makefileTmplPath = "embedded/templates/main/Makefile.tmpl"

const (
	// makefileLiteral is plain text between template actions.
	makefileLiteral makefileNodeKind = iota
	// makefileEmit is an action printing a value, e.g. {{ $a.Name }}.
	makefileEmit
	// makefileGroup is {{ if }} / {{ range }} / {{ with }} … {{ end }}, holding
	// the nodes it emits when taken.
	makefileGroup
)

var (
	// makefileTargetLineRe matches a target line, as opposed to a variable
	// assignment (`FOO:=bar`) or a recipe line (tab-indented).
	makefileTargetLineRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+:[^=]`)
	makefileActionRe     = regexp.MustCompile(`\{\{-?\s*(.*?)\s*-?\}\}`)
)

func TestIsFileIgnored(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{
			name:     "keep file is ignored",
			filename: ".keep",
			want:     true,
		},
		{
			name:     "LICENSE.txt is ignored",
			filename: "LICENSE.txt",
			want:     true,
		},
		{
			name:     "go.mod is ignored",
			filename: "go.mod",
			want:     true,
		},
		{
			name:     "JSON files are ignored",
			filename: "config.json",
			want:     true,
		},
		{
			name:     "dashboard.json is ignored",
			filename: "dashboard.json",
			want:     true,
		},
		{
			name:     "LLMS.md is ignored",
			filename: "LLMS.md",
			want:     true,
		},
		{
			name:     "CLAUDE.md is ignored",
			filename: "CLAUDE.md",
			want:     true,
		},
		{
			name:     "go file is not ignored",
			filename: "main.go",
			want:     false,
		},
		{
			name:     "yaml file is not ignored",
			filename: "config.yaml",
			want:     false,
		},
		{
			name:     "Makefile is not ignored",
			filename: "Makefile",
			want:     false,
		},
		{
			name:     "Dockerfile is not ignored",
			filename: "Dockerfile",
			want:     false,
		},
		{
			name:     "sh file is not ignored",
			filename: "script.sh",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isFileIgnored(tt.filename)

			if got != tt.want {
				t.Errorf("isFileIgnored(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestMakeComment(t *testing.T) {
	testText := "Test comment"

	tests := []struct {
		name       string
		filename   string
		wantPrefix string
		wantErr    bool
	}{
		{
			name:       "SQL file uses -- prefix",
			filename:   "schema.sql",
			wantPrefix: "-- ",
			wantErr:    false,
		},
		{
			name:       "Go file uses // prefix",
			filename:   "main.go",
			wantPrefix: "// ",
			wantErr:    false,
		},
		{
			name:       "YAML file uses # prefix",
			filename:   "config.yaml",
			wantPrefix: "# ",
			wantErr:    false,
		},
		{
			name:       "YML file uses # prefix",
			filename:   "config.yml",
			wantPrefix: "# ",
			wantErr:    false,
		},
		{
			name:       "Shell script uses # prefix",
			filename:   "script.sh",
			wantPrefix: "# ",
			wantErr:    false,
		},
		{
			name:       "Service file uses # prefix",
			filename:   "app.service",
			wantPrefix: "# ",
			wantErr:    false,
		},
		{
			name:       "Gitignore uses # prefix",
			filename:   ".gitignore",
			wantPrefix: "# ",
			wantErr:    false,
		},
		{
			name:       "Makefile uses # prefix",
			filename:   "Makefile",
			wantPrefix: "# ",
			wantErr:    false,
		},
		{
			name:       "Dockerfile uses # prefix",
			filename:   "Dockerfile",
			wantPrefix: "# ",
			wantErr:    false,
		},
		{
			name:       "pre-commit uses # prefix",
			filename:   "pre-commit",
			wantPrefix: "# ",
			wantErr:    false,
		},
		{
			name:       "env example file uses # prefix",
			filename:   ".env-local.example",
			wantPrefix: "# ",
			wantErr:    false,
		},
		{
			name:       "MD file uses HTML comment",
			filename:   "README.md",
			wantPrefix: "<!-- ",
			wantErr:    false,
		},
		{
			name:       "TXT file has no prefix",
			filename:   "LICENSE.txt",
			wantPrefix: " ",
			wantErr:    false,
		},
		{
			name:     "unknown extension returns error",
			filename: "file.xyz",
			wantErr:  true,
		},
		{
			name:     "unknown dot file returns error",
			filename: ".unknown",
			wantErr:  true,
		},
		{
			name:     "unknown no-extension file returns error",
			filename: "unknownfile",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := makeComment(tt.filename, testText)

			if tt.wantErr {
				if err == nil {
					t.Errorf("makeComment(%q) error = nil, want error", tt.filename)
				}

				return
			}

			if err != nil {
				t.Errorf("makeComment(%q) unexpected error: %v", tt.filename, err)
				return
			}

			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("makeComment(%q) = %q, want prefix %q", tt.filename, got, tt.wantPrefix)
			}

			if !strings.Contains(got, testText) {
				t.Errorf("makeComment(%q) = %q, should contain %q", tt.filename, got, testText)
			}
		})
	}
}

func TestMakeStartDisclaimer(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		wantEmpty  bool
		wantPrefix string
	}{
		{
			name:      "ignored file returns empty",
			filename:  "/path/to/config.json",
			wantEmpty: true,
		},
		{
			name:      "go.mod returns empty",
			filename:  "/path/to/go.mod",
			wantEmpty: true,
		},
		{
			name:       "go file returns disclaimer",
			filename:   "/path/to/main.go",
			wantEmpty:  false,
			wantPrefix: "// ",
		},
		{
			name:       "MD file has header prefix",
			filename:   "/path/to/README.md",
			wantEmpty:  false,
			wantPrefix: "# README.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := makeStartDisclaimer(tt.filename)

			if err != nil {
				t.Errorf("makeStartDisclaimer(%q) unexpected error: %v", tt.filename, err)
				return
			}

			if tt.wantEmpty {
				if got != "" {
					t.Errorf("makeStartDisclaimer(%q) = %q, want empty", tt.filename, got)
				}

				return
			}

			if got == "" {
				t.Errorf("makeStartDisclaimer(%q) = empty, want non-empty", tt.filename)
				return
			}

			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("makeStartDisclaimer(%q) = %q, want prefix %q", tt.filename, got, tt.wantPrefix)
			}
		})
	}
}

func TestMakeFinishDisclaimer(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		wantEmpty bool
	}{
		{
			name:      "ignored file returns empty",
			filename:  "/path/to/config.json",
			wantEmpty: true,
		},
		{
			name:      "go file returns disclaimer",
			filename:  "/path/to/main.go",
			wantEmpty: false,
		},
		{
			name:      "yaml file returns disclaimer",
			filename:  "/path/to/config.yaml",
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := makeFinishDisclaimer(tt.filename)

			if err != nil {
				t.Errorf("makeFinishDisclaimer(%q) unexpected error: %v", tt.filename, err)
				return
			}

			if tt.wantEmpty && got != "" {
				t.Errorf("makeFinishDisclaimer(%q) = %q, want empty", tt.filename, got)
			}

			if !tt.wantEmpty && got == "" {
				t.Errorf("makeFinishDisclaimer(%q) = empty, want non-empty", tt.filename)
			}
		})
	}
}

func TestSplitDisclaimer(t *testing.T) {
	tests := []struct {
		name         string
		fileContent  string
		wantGenCode  string
		wantUserCode string
		wantErr      bool
	}{
		{
			name:         "no disclaimer returns error",
			fileContent:  "package main\n\nfunc main() {}\n",
			wantGenCode:  "package main\n\nfunc main() {}\n",
			wantUserCode: "",
			wantErr:      true,
		},
		{
			name:         "disclaimer without user code",
			fileContent:  "package main\n\n// " + disclaimer + "\n",
			wantGenCode:  "package main\n\n// " + disclaimer + "\n",
			wantUserCode: "",
			wantErr:      false,
		},
		{
			name:         "disclaimer with user code",
			fileContent:  "package main\n\n// " + disclaimer + "\n\nfunc myFunc() {}\n",
			wantGenCode:  "package main\n\n// " + disclaimer + "\n",
			wantUserCode: "\nfunc myFunc() {}\n",
			wantErr:      false,
		},
		{
			name:         "disclaimer at end without newline",
			fileContent:  "package main\n\n// " + disclaimer,
			wantGenCode:  "package main\n\n// ",
			wantUserCode: "",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotGen, gotUser, err := splitDisclaimer(tt.fileContent)

			if tt.wantErr {
				if err == nil {
					t.Errorf("splitDisclaimer() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Errorf("splitDisclaimer() unexpected error: %v", err)
				return
			}

			if gotGen != tt.wantGenCode {
				t.Errorf("splitDisclaimer() genCode = %q, want %q", gotGen, tt.wantGenCode)
			}

			if gotUser != tt.wantUserCode {
				t.Errorf("splitDisclaimer() userCode = %q, want %q", gotUser, tt.wantUserCode)
			}
		})
	}
}

func TestGetTmplErrorLine(t *testing.T) {
	lines := []string{
		"line1\n",
		"line2\n",
		"line3\n",
		"line4\n",
		"line5\n",
		"line6\n",
		"line7\n",
		"line8\n",
	}

	tests := []struct {
		name      string
		lines     []string
		tmplErr   string
		wantErr   bool
		wantMatch string
	}{
		{
			name:      "error with line number",
			lines:     lines,
			tmplErr:   "template: test:5:10: some error",
			wantErr:   false,
			wantMatch: "line5",
		},
		{
			name:      "error without line number",
			lines:     lines,
			tmplErr:   "some error without line",
			wantErr:   true,
			wantMatch: "",
		},
		{
			name:      "error with line 1",
			lines:     lines,
			tmplErr:   "template: test:1: error at start",
			wantErr:   false,
			wantMatch: "line1",
		},
		{
			name:      "empty lines",
			lines:     []string{},
			tmplErr:   "template: test:1: error",
			wantErr:   true,
			wantMatch: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getTmplErrorLine(tt.lines, tt.tmplErr)

			if tt.wantErr {
				if err == nil {
					t.Errorf("getTmplErrorLine() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Errorf("getTmplErrorLine() unexpected error: %v", err)
				return
			}

			if tt.wantMatch != "" && !strings.Contains(got, tt.wantMatch) {
				t.Errorf("getTmplErrorLine() = %q, want to contain %q", got, tt.wantMatch)
			}
		})
	}
}

func TestGenerateFilenameByTmpl(t *testing.T) {
	tests := []struct {
		name       string
		file       ds.Files
		targetPath string
		lastVer    int
		wantDest   string
		wantErr    bool
	}{
		{
			name: "simple go file",
			file: ds.Files{
				DestName:   "main.go",
				ParamsTmpl: nil,
			},
			targetPath: "/target",
			lastVer:    3,
			wantDest:   "/target/psg_main_gen.go",
			wantErr:    false,
		},
		{
			name: "test go file preserves _test suffix",
			file: ds.Files{
				DestName:   "main_test.go",
				ParamsTmpl: nil,
			},
			targetPath: "/target",
			lastVer:    3,
			wantDest:   "/target/psg_main_test.go",
			wantErr:    false,
		},
		{
			name: "non-go file",
			file: ds.Files{
				DestName:   "config.yaml",
				ParamsTmpl: nil,
			},
			targetPath: "/target",
			lastVer:    3,
			wantDest:   "/target/config.yaml",
			wantErr:    false,
		},
		{
			name: "template with params",
			file: ds.Files{
				DestName:   "{{.Name}}.go",
				ParamsTmpl: struct{ Name string }{Name: "myfile"},
			},
			targetPath: "/target",
			lastVer:    3,
			wantDest:   "/target/psg_myfile_gen.go",
			wantErr:    false,
		},
		{
			name: "file in subdirectory",
			file: ds.Files{
				DestName:   "pkg/service/handler.go",
				ParamsTmpl: nil,
			},
			targetPath: "/target",
			lastVer:    3,
			wantDest:   "/target/pkg/service/psg_handler_gen.go",
			wantErr:    false,
		},
		{
			name: "migration from version 1 - no psg prefix",
			file: ds.Files{
				DestName:   "main.go",
				ParamsTmpl: nil,
			},
			targetPath: "/target",
			lastVer:    1,
			wantDest:   "/target/psg_main_gen.go",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := GenerateFilenameByTmpl(&tt.file, tt.targetPath, tt.lastVer)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GenerateFilenameByTmpl() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Errorf("GenerateFilenameByTmpl() unexpected error: %v", err)
				return
			}

			if tt.file.DestName != tt.wantDest {
				t.Errorf("GenerateFilenameByTmpl() DestName = %q, want %q", tt.file.DestName, tt.wantDest)
			}
		})
	}
}

func TestGetTemplate(t *testing.T) {
	t.Run("get existing template", func(t *testing.T) {
		// Use a template that definitely exists
		tmpl, err := GetTemplate("embedded/templates/main/Makefile.tmpl")

		if err != nil {
			t.Errorf("GetTemplate() unexpected error: %v", err)
			return
		}

		if tmpl.Name != "embedded/templates/main/Makefile.tmpl" {
			t.Errorf("GetTemplate() Name = %q, want %q", tmpl.Name, "embedded/templates/main/Makefile.tmpl")
		}

		if tmpl.Tmpl == "" {
			t.Error("GetTemplate() Tmpl is empty, want non-empty")
		}
	})

	t.Run("get non-existing template", func(t *testing.T) {
		_, err := GetTemplate("embedded/templates/nonexistent.tmpl")

		if err == nil {
			t.Error("GetTemplate() error = nil, want error for non-existing template")
		}
	})

	t.Run("template caching", func(t *testing.T) {
		// Get same template twice to test caching
		tmpl1, err1 := GetTemplate("embedded/templates/main/Makefile.tmpl")
		tmpl2, err2 := GetTemplate("embedded/templates/main/Makefile.tmpl")

		if err1 != nil || err2 != nil {
			t.Errorf("GetTemplate() errors: err1=%v, err2=%v", err1, err2)
			return
		}

		if tmpl1.Tmpl != tmpl2.Tmpl {
			t.Error("GetTemplate() cached template differs from original")
		}
	})
}

func TestIsFileIgnore(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "gitignore is ignored",
			path: ".gitignore",
			want: true,
		},
		{
			name: "go.mod is ignored",
			path: "go.mod",
			want: true,
		},
		{
			name: "go.sum is ignored",
			path: "go.sum",
			want: true,
		},
		{
			name: "LICENSE.txt is ignored",
			path: "LICENSE.txt",
			want: true,
		},
		{
			name: "README.md is ignored",
			path: "README.md",
			want: true,
		},
		{
			name: "LLMS.md is ignored",
			path: "LLMS.md",
			want: true,
		},
		{
			name: "CLAUDE.md is ignored",
			path: "CLAUDE.md",
			want: true,
		},
		{
			name: ".git directory is ignored",
			path: ".git/config",
			want: true,
		},
		{
			name: "docs directory is ignored",
			path: "docs/api.md",
			want: true,
		},
		{
			name: "etc/onlineconf/.keep is ignored",
			path: "etc/onlineconf/.keep",
			want: true,
		},
		{
			name: "public/.keep is ignored",
			path: "public/.keep",
			want: true,
		},
		{
			name: "regular go file is not ignored",
			path: "main.go",
			want: false,
		},
		{
			name: "yaml config is not ignored",
			path: "config.yaml",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isFileIgnore(tt.path)

			if got != tt.want {
				t.Errorf("isFileIgnore(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestGenerateByTmpl(t *testing.T) {
	// For Go files, disclaimer template uses AppInfo field, so use GeneratorParams
	t.Run("generate simple template for Go file", func(t *testing.T) {
		tmpl := Template{
			Name: "test.go.tmpl",
			Tmpl: "package {{.ProjectName}}",
		}
		params := GeneratorParams{ProjectName: "main", AppInfo: "test"}

		buf, err := GenerateByTmpl(tmpl, params, nil, "/path/to/main.go")

		if err != nil {
			t.Errorf("GenerateByTmpl() unexpected error: %v", err)
			return
		}

		if buf == nil {
			t.Error("GenerateByTmpl() returned nil buffer")
			return
		}

		content := buf.String()

		if !strings.Contains(content, "package main") {
			t.Errorf("GenerateByTmpl() content = %q, want to contain 'package main'", content)
		}

		// Should have disclaimer
		if !strings.Contains(content, disclaimer) {
			t.Errorf("GenerateByTmpl() Go file should have disclaimer")
		}
	})

	t.Run("generate with user code", func(t *testing.T) {
		tmpl := Template{
			Name: "test.go.tmpl",
			Tmpl: "package main",
		}
		params := GeneratorParams{AppInfo: "test"}
		userCode := []byte("\nfunc myFunc() {}\n")

		buf, err := GenerateByTmpl(tmpl, params, userCode, "/path/to/main.go")

		if err != nil {
			t.Errorf("GenerateByTmpl() unexpected error: %v", err)
			return
		}

		content := buf.String()

		if !strings.Contains(content, "func myFunc()") {
			t.Errorf("GenerateByTmpl() content should contain user code, got: %q", content)
		}
	})

	t.Run("template parse error", func(t *testing.T) {
		tmpl := Template{
			Name: "bad.go.tmpl",
			Tmpl: "{{.Invalid",
		}

		_, err := GenerateByTmpl(tmpl, nil, nil, "/path/to/config.json")

		if err == nil {
			t.Error("GenerateByTmpl() error = nil, want error for invalid template")
		}
	})

	t.Run("template execution error", func(t *testing.T) {
		tmpl := Template{
			Name: "test.go.tmpl",
			Tmpl: "{{.MissingField}}",
		}
		params := struct{ Name string }{Name: "main"}

		_, err := GenerateByTmpl(tmpl, params, nil, "/path/to/config.json")

		if err == nil {
			t.Error("GenerateByTmpl() error = nil, want error for missing field")
		}
	})

	t.Run("generate for ignored file type (JSON)", func(t *testing.T) {
		tmpl := Template{
			Name: "test.json.tmpl",
			Tmpl: `{"name": "{{.Name}}"}`,
		}
		params := struct{ Name string }{Name: "test"}

		buf, err := GenerateByTmpl(tmpl, params, nil, "/path/to/config.json")

		if err != nil {
			t.Errorf("GenerateByTmpl() unexpected error: %v", err)
			return
		}

		content := buf.String()
		// JSON files should have no disclaimer
		if strings.Contains(content, disclaimer) {
			t.Errorf("GenerateByTmpl() JSON should not have disclaimer, got: %q", content)
		}
	})
}

func TestGenerateByTmplWithTransform(t *testing.T) {
	tmpl := Template{
		Name: "configs/golangci-lint.yml.tmpl",
		Tmpl: "version: \"2\"\nlinters:\n  enable:\n    - misspell\n",
	}
	params := GeneratorParams{AppInfo: "test"}
	destPath := "/path/to/configs/golangci-lint.yml"

	// A nil transform is the no-override path, which must stay byte-for-byte
	// what the generator produced before the hook existed.
	t.Run("nil transform matches GenerateByTmpl byte for byte", func(t *testing.T) {
		plain, err := GenerateByTmpl(tmpl, params, nil, destPath)
		if err != nil {
			t.Fatalf("GenerateByTmpl() error: %v", err)
		}

		hooked, err := GenerateByTmplWithTransform(tmpl, params, nil, destPath, nil)
		if err != nil {
			t.Fatalf("GenerateByTmplWithTransform() error: %v", err)
		}

		if plain.String() != hooked.String() {
			t.Errorf("nil transform changed the output:\n--- plain ---\n%s\n--- hooked ---\n%s", plain, hooked)
		}
	})

	// The transform must see the rendered YAML and nothing else: the merge
	// parses its input as YAML, so a disclaimer comment block leaking in would
	// end up reformatted or attached to the wrong node.
	t.Run("transform receives the body without the disclaimer blocks", func(t *testing.T) {
		var got string

		_, err := GenerateByTmplWithTransform(tmpl, params, nil, destPath,
			func(body []byte) ([]byte, error) {
				got = string(body)

				return body, nil
			})
		if err != nil {
			t.Fatalf("GenerateByTmplWithTransform() error: %v", err)
		}

		if strings.Contains(got, disclaimer) || strings.Contains(got, "DO NOT EDIT") {
			t.Errorf("transform body still carries a disclaimer block: %q", got)
		}

		if !strings.HasPrefix(got, "version: \"2\"") {
			t.Errorf("transform body = %q, want it to start with the template output", got)
		}
	})

	// Both disclaimer blocks have to come back around the rewritten body: the
	// bottom marker is what the next regeneration looks for to find user code.
	t.Run("rewritten body is reassembled between the disclaimers", func(t *testing.T) {
		userCode := "\n# project note kept below the marker\n"

		buf, err := GenerateByTmplWithTransform(tmpl, params, []byte(userCode), destPath,
			func([]byte) ([]byte, error) {
				return []byte("version: \"2\"\nlinters:\n  enable:\n    - dupl\n"), nil
			})
		if err != nil {
			t.Fatalf("GenerateByTmplWithTransform() error: %v", err)
		}

		content := buf.String()

		if !strings.HasPrefix(content, "# Code generated by projectStarter generator.") {
			t.Errorf("top disclaimer lost:\n%s", content)
		}

		if !strings.Contains(content, "- dupl") || strings.Contains(content, "- misspell") {
			t.Errorf("transformed body not used:\n%s", content)
		}

		genPart, user, err := splitDisclaimer(content)
		if err != nil {
			t.Fatalf("bottom marker lost, next regeneration would fail: %v\n%s", err, content)
		}

		if user != userCode {
			t.Errorf("user code = %q, want %q", user, userCode)
		}

		if !strings.Contains(genPart, "- dupl") {
			t.Errorf("rewritten body must sit above the marker, got:\n%s", genPart)
		}
	})

	t.Run("transform error is propagated", func(t *testing.T) {
		_, err := GenerateByTmplWithTransform(tmpl, params, nil, destPath,
			func([]byte) ([]byte, error) {
				return nil, errors.New("boom")
			})
		if err == nil {
			t.Fatal("expected the transform error to propagate, got nil")
		}

		if !strings.Contains(err.Error(), "boom") {
			t.Errorf("error = %v, want it to mention the transform failure", err)
		}
	})
}

func TestTemplateFuncs(t *testing.T) {
	tests := []struct {
		name     string
		template string
		params   any
		want     string
	}{
		{
			name:     "ToLower",
			template: "{{ToLower .Name}}",
			params:   struct{ Name string }{Name: "HELLO"},
			want:     "hello",
		},
		{
			name:     "ToUpper",
			template: "{{ToUpper .Name}}",
			params:   struct{ Name string }{Name: "hello"},
			want:     "HELLO",
		},
		{
			name:     "ReplaceDash",
			template: "{{ReplaceDash .Name}}",
			params:   struct{ Name string }{Name: "my-app-name"},
			want:     "my_app_name",
		},
		{
			name:     "Capitalize",
			template: "{{Capitalize .Name}}",
			params:   struct{ Name string }{Name: "hello world"},
			want:     "Hello World",
		},
		{
			name:     "CapitalizeFirst with dash",
			template: "{{CapitalizeFirst .Name}}",
			params:   struct{ Name string }{Name: "my-app"},
			want:     "MyApp",
		},
		{
			name:     "CapitalizeFirst with underscore",
			template: "{{CapitalizeFirst .Name}}",
			params:   struct{ Name string }{Name: "my_app"},
			want:     "MyApp",
		},
		{
			name:     "add function",
			template: "{{add .A .B}}",
			params:   struct{ A, B int }{A: 5, B: 3},
			want:     "8",
		},
		{
			name:     "escapeJSON",
			template: `{{escapeJSON .Text}}`,
			params:   struct{ Text string }{Text: `text with "quotes" and \backslash`},
			want:     `text with \"quotes\" and \\backslash`,
		},
		{
			name:     "ImageNameToPullerService",
			template: "{{ImageNameToPullerService .Image}}",
			params:   struct{ Image string }{Image: "ghcr.io/org/myimage:v1.0"},
			want:     "myimage-image-puller",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := Template{
				Name: "test.go.tmpl",
				Tmpl: tt.template,
			}

			buf, err := GenerateByTmpl(tmpl, tt.params, nil, "/path/to/config.json")

			if err != nil {
				t.Errorf("GenerateByTmpl() unexpected error: %v", err)
				return
			}

			content := buf.String()

			if !strings.Contains(content, tt.want) {
				t.Errorf("Template function result = %q, want to contain %q", content, tt.want)
			}
		})
	}
}

func TestGetUserCodeFromFiles(t *testing.T) {
	disclaimerLine := "// " + disclaimer

	t.Run("obsolete file with disclaimer without user code goes to ObsoleteFiles", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a stale generated file with disclaimer but no user code
		staleFile := filepath.Join(tmpDir, "psg_old_gen.go")
		content := "package main\n\n" + disclaimerLine + "\n"
		if err := os.WriteFile(staleFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		// No files in template set — so psg_old_gen.go is not expected
		filesDiff, err := GetUserCodeFromFiles(tmpDir, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := filesDiff.ObsoleteFiles[staleFile]; !ok {
			t.Errorf("expected %s in ObsoleteFiles, got ObsoleteFiles=%v, OtherFiles=%v",
				staleFile, filesDiff.ObsoleteFiles, filesDiff.OtherFiles)
		}
	})

	t.Run("obsolete file with disclaimer and user code returns error", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a stale generated file with disclaimer AND user code
		staleFile := filepath.Join(tmpDir, "psg_old_gen.go")
		content := "package main\n\n" + disclaimerLine + "\n\nfunc myCustomCode() {}\n"
		if err := os.WriteFile(staleFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := GetUserCodeFromFiles(tmpDir, nil, nil)
		if err == nil {
			t.Fatal("expected error for stale file with user code, got nil")
		}

		if !strings.Contains(err.Error(), "found user code in stale gen file") {
			t.Errorf("error should mention stale gen file, got: %v", err)
		}
	})

	t.Run("obsolete file without disclaimer goes to OtherFiles", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a user file without disclaimer
		userFile := filepath.Join(tmpDir, "my_helper.go")
		content := "package main\n\nfunc helper() {}\n"
		if err := os.WriteFile(userFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := filesDiff.OtherFiles[userFile]; !ok {
			t.Errorf("expected %s in OtherFiles, got OtherFiles=%v", userFile, filesDiff.OtherFiles)
		}

		if _, ok := filesDiff.ObsoleteFiles[userFile]; ok {
			t.Errorf("user file should NOT be in ObsoleteFiles")
		}
	})

	t.Run("file in template set is not obsolete", func(t *testing.T) {
		tmpDir := t.TempDir()

		destName := filepath.Join(tmpDir, "psg_handler_gen.go")
		content := "package main\n\n" + disclaimerLine + "\n"
		if err := os.WriteFile(destName, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		// This file IS in the template set
		files := []ds.Files{
			{DestName: destName, OldDestName: destName},
		}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, files, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := filesDiff.ObsoleteFiles[destName]; ok {
			t.Errorf("file in template set should NOT be in ObsoleteFiles")
		}
	})

	t.Run("symlink to a directory outside the tree goes to OtherFiles", func(t *testing.T) {
		baseDir := t.TempDir()

		// The link target lives outside the walked tree, as a build/tool cache would
		outsideDir := filepath.Join(baseDir, "outside", "cache")
		if err := os.MkdirAll(outsideDir, 0755); err != nil {
			t.Fatal(err)
		}

		tmpDir := filepath.Join(baseDir, "proj")
		if err := os.Mkdir(tmpDir, 0755); err != nil {
			t.Fatal(err)
		}

		link := filepath.Join(tmpDir, "cache")
		if err := os.Symlink(outsideDir, link); err != nil {
			t.Fatal(err)
		}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := filesDiff.OtherFiles[link]; !ok {
			t.Errorf("expected %s in OtherFiles, got OtherFiles=%v, OtherDirectory=%v",
				link, filesDiff.OtherFiles, filesDiff.OtherDirectory)
		}

		if _, ok := filesDiff.ObsoleteFiles[link]; ok {
			t.Errorf("symlink should NOT be in ObsoleteFiles")
		}
	})

	t.Run("symlink to a regular file goes to OtherFiles and is not read", func(t *testing.T) {
		tmpDir := t.TempDir()

		// A link to a generated-looking file: before the fix its content was read and
		// the disclaimer made it a deletion candidate
		target := filepath.Join(tmpDir, "real.go")
		content := "package main\n\n" + disclaimerLine + "\n"
		if err := os.WriteFile(target, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		link := filepath.Join(tmpDir, "psg_link_gen.go")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := filesDiff.OtherFiles[link]; !ok {
			t.Errorf("expected %s in OtherFiles, got OtherFiles=%v", link, filesDiff.OtherFiles)
		}

		if _, ok := filesDiff.ObsoleteFiles[link]; ok {
			t.Errorf("symlink should NOT be in ObsoleteFiles — removing it is not the generator's business")
		}
	})

	t.Run("symlink on a generated path returns ErrNonRegularTarget", func(t *testing.T) {
		baseDir := t.TempDir()

		outsideFile := filepath.Join(baseDir, "outside.go")
		if err := os.WriteFile(outsideFile, []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}

		tmpDir := filepath.Join(baseDir, "proj")
		if err := os.Mkdir(tmpDir, 0755); err != nil {
			t.Fatal(err)
		}

		destName := filepath.Join(tmpDir, "psg_handler_gen.go")
		if err := os.Symlink(outsideFile, destName); err != nil {
			t.Fatal(err)
		}

		files := []ds.Files{
			{DestName: destName, OldDestName: destName},
		}

		_, err := GetUserCodeFromFiles(tmpDir, files, nil)
		if err == nil {
			t.Fatal("expected error for symlink on a generated path, got nil")
		}

		if !errors.Is(err, ErrNonRegularTarget) {
			t.Errorf("error should be ErrNonRegularTarget, got: %v", err)
		}

		if !strings.Contains(err.Error(), destName) {
			t.Errorf("error should mention the offending path %s, got: %v", destName, err)
		}
	})

	t.Run("ignored non-regular entry stays ignored", func(t *testing.T) {
		tmpDir := t.TempDir()

		// .gitignore is in ignoreIfExistsFiles: the ignore branch runs before the
		// non-regular check, so a link there is neither read nor an error
		link := filepath.Join(tmpDir, ".gitignore")
		if err := os.Symlink(filepath.Join(tmpDir, "missing-target"), link); err != nil {
			t.Fatal(err)
		}

		files := []ds.Files{
			{DestName: link, OldDestName: link},
		}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, files, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := filesDiff.IgnoreFiles[link]; !ok {
			t.Errorf("expected %s in IgnoreFiles, got IgnoreFiles=%v", link, filesDiff.IgnoreFiles)
		}
	})
}

// writeFile creates path with content, making parent directories as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// genRecord builds a manifest record for a generated file as the previous run
// would have written it.
func genRecord(t *testing.T, relPath, content string) meta.FileRecord {
	t.Helper()

	hash, err := GeneratedRegionHash(relPath, []byte(content))
	if err != nil {
		t.Fatalf("hash %s: %v", relPath, err)
	}

	return meta.FileRecord{Path: relPath, Kind: meta.KindGenerated, Hash: hash}
}

func TestGetUserCodeFromFilesWithManifest(t *testing.T) {
	disclaimerLine := "// " + disclaimer

	t.Run("foreign file with disclaimer is neither read nor deleted", func(t *testing.T) {
		tmpDir := t.TempDir()

		genContent := "package main\n\n" + disclaimerLine + "\n"
		genFile := filepath.Join(tmpDir, "psg_handler_gen.go")
		writeFile(t, genFile, genContent)

		// A code-intelligence index that happens to embed the disclaimer
		// string, plus a merge leftover of a generated file. Neither was
		// written by the generator, so neither is in the manifest.
		indexFile := filepath.Join(tmpDir, ".codegraph", "codegraph.db")
		writeFile(t, indexFile, "SQLite format 3\x00"+disclaimer+"\x00package main\n")

		orig := filepath.Join(tmpDir, "psg_handler_gen.go.orig")
		writeFile(t, orig, genContent+"\nfunc userCode() {}\n")

		files := []ds.Files{{DestName: genFile, OldDestName: genFile}}
		prev := meta.Manifest{genRecord(t, "psg_handler_gen.go", genContent)}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, files, prev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(filesDiff.ObsoleteFiles) != 0 {
			t.Errorf("foreign files must not be deleted, got ObsoleteFiles=%v", filesDiff.ObsoleteFiles)
		}

		if len(filesDiff.DriftFiles) != 0 {
			t.Errorf("foreign files must not be reported as drift, got %v", filesDiff.DriftFiles)
		}
	})

	t.Run("orphan from manifest with untouched content is obsolete", func(t *testing.T) {
		tmpDir := t.TempDir()

		staleContent := "package main\n\n" + disclaimerLine + "\n"
		staleFile := filepath.Join(tmpDir, "psg_old_gen.go")
		writeFile(t, staleFile, staleContent)

		prev := meta.Manifest{genRecord(t, "psg_old_gen.go", staleContent)}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, nil, prev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := filesDiff.ObsoleteFiles[staleFile]; !ok {
			t.Errorf("expected %s in ObsoleteFiles, got %v", staleFile, filesDiff.ObsoleteFiles)
		}
	})

	// tools.lint_config.own in an existing project: configs/golangci-lint.yml
	// is in the previous manifest and drops out of the output set, which is
	// exactly what an orphan looks like. Handing the file over to the project
	// must not delete it — including in the dangerous case where the project
	// has not edited it yet, so its hash still matches the manifest and the
	// drift branch would not save it.
	t.Run("user-owned path dropped from the output set is handed over, not deleted", func(t *testing.T) {
		tmpDir := t.TempDir()

		relPath := "configs/golangci-lint.yml"
		content := "linters:\n  enable:\n    - misspell\n\n# " + disclaimer + "\n"
		lintFile := filepath.Join(tmpDir, filepath.FromSlash(relPath))
		writeFile(t, lintFile, content)

		prev := meta.Manifest{genRecord(t, relPath, content)}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, nil, prev, relPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := filesDiff.ObsoleteFiles[lintFile]; ok {
			t.Errorf("user-owned %s must not be deleted, got ObsoleteFiles=%v", relPath, filesDiff.ObsoleteFiles)
		}

		if len(filesDiff.DriftFiles) != 0 {
			t.Errorf("a handed-over file is not drift, got %v", filesDiff.DriftFiles)
		}

		if got, err := os.ReadFile(lintFile); err != nil || string(got) != content {
			t.Errorf("file content must be left alone, got %q (err %v)", got, err)
		}
	})

	// Same path, but without the ownership declaration: the file really is an
	// orphan and the previous behavior stands. This is what makes the test
	// above about the extraIgnore argument rather than about the path.
	t.Run("same path without the ownership declaration is still an orphan", func(t *testing.T) {
		tmpDir := t.TempDir()

		relPath := "configs/golangci-lint.yml"
		content := "linters:\n  enable:\n    - misspell\n\n# " + disclaimer + "\n"
		lintFile := filepath.Join(tmpDir, filepath.FromSlash(relPath))
		writeFile(t, lintFile, content)

		prev := meta.Manifest{genRecord(t, relPath, content)}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, nil, prev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := filesDiff.ObsoleteFiles[lintFile]; !ok {
			t.Errorf("expected %s in ObsoleteFiles, got %v", lintFile, filesDiff.ObsoleteFiles)
		}
	})

	t.Run("orphan from manifest with user code returns error", func(t *testing.T) {
		tmpDir := t.TempDir()

		staleContent := "package main\n\n" + disclaimerLine + "\n"
		staleFile := filepath.Join(tmpDir, "psg_old_gen.go")
		writeFile(t, staleFile, staleContent+"\nfunc myCustomCode() {}\n")

		prev := meta.Manifest{genRecord(t, "psg_old_gen.go", staleContent)}

		_, err := GetUserCodeFromFiles(tmpDir, nil, prev)
		if err == nil {
			t.Fatal("expected error for stale file with user code, got nil")
		}

		if !strings.Contains(err.Error(), "found user code in stale gen file") {
			t.Errorf("error should mention stale gen file, got: %v", err)
		}
	})

	t.Run("orphan edited above the marker is kept and reported", func(t *testing.T) {
		tmpDir := t.TempDir()

		staleContent := "package main\n\n" + disclaimerLine + "\n"
		staleFile := filepath.Join(tmpDir, "psg_old_gen.go")
		writeFile(t, staleFile, "package main\n\n// hand written\n\n"+disclaimerLine+"\n")

		prev := meta.Manifest{genRecord(t, "psg_old_gen.go", staleContent)}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, nil, prev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := filesDiff.ObsoleteFiles[staleFile]; ok {
			t.Error("edited file must not be deleted")
		}

		if _, ok := filesDiff.DriftFiles[staleFile]; !ok {
			t.Errorf("expected %s in DriftFiles, got %v", staleFile, filesDiff.DriftFiles)
		}
	})

	t.Run("copied orphan is left on disk", func(t *testing.T) {
		tmpDir := t.TempDir()

		spec := filepath.Join(tmpDir, "api", "rest", "svc", "v1", "svc.swagger.yml")
		writeFile(t, spec, "openapi: 3.0.0\n")

		prev := meta.Manifest{{
			Path: "api/rest/svc/v1/svc.swagger.yml",
			Kind: meta.KindCopied,
			Hash: meta.HashContent([]byte("openapi: 3.0.0\n")),
		}}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, nil, prev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := filesDiff.ObsoleteFiles[spec]; ok {
			t.Error("copied file must not be deleted automatically")
		}

		if _, ok := filesDiff.OtherFiles[spec]; !ok {
			t.Errorf("expected %s in OtherFiles, got %v", spec, filesDiff.OtherFiles)
		}
	})

	t.Run("hand edit above the marker is reported, user code preserved", func(t *testing.T) {
		tmpDir := t.TempDir()

		genContent := "package main\n\n" + disclaimerLine + "\n"
		genFile := filepath.Join(tmpDir, "psg_handler_gen.go")
		writeFile(t, genFile, "package main\n\n// hand written\n\n"+disclaimerLine+"\n\nfunc userCode() {}\n")

		files := []ds.Files{{DestName: genFile, OldDestName: genFile}}
		prev := meta.Manifest{genRecord(t, "psg_handler_gen.go", genContent)}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, files, prev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := filesDiff.DriftFiles[genFile]; !ok {
			t.Errorf("expected %s in DriftFiles, got %v", genFile, filesDiff.DriftFiles)
		}

		if got := string(filesDiff.UserContent[genFile]); got != "\nfunc userCode() {}\n" {
			t.Errorf("user code must be preserved, got %q", got)
		}
	})

	t.Run("unchanged generated file is not reported as drift", func(t *testing.T) {
		tmpDir := t.TempDir()

		genContent := "package main\n\n" + disclaimerLine + "\n"
		genFile := filepath.Join(tmpDir, "psg_handler_gen.go")
		writeFile(t, genFile, genContent+"\nfunc userCode() {}\n")

		files := []ds.Files{{DestName: genFile, OldDestName: genFile}}
		prev := meta.Manifest{genRecord(t, "psg_handler_gen.go", genContent)}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, files, prev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(filesDiff.DriftFiles) != 0 {
			t.Errorf("user code below the marker is not drift, got %v", filesDiff.DriftFiles)
		}

		if _, ok := filesDiff.NewFiles[genFile]; ok {
			t.Error("existing file must not be reported as new")
		}
	})

	t.Run("new file absent on disk stays in NewFiles", func(t *testing.T) {
		tmpDir := t.TempDir()

		genFile := filepath.Join(tmpDir, "pkg", "app", "psg_new_gen.go")
		files := []ds.Files{{DestName: genFile, OldDestName: genFile}}
		prev := meta.Manifest{genRecord(t, "psg_other_gen.go", "package main\n\n"+disclaimerLine+"\n")}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, files, prev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := filesDiff.NewFiles[genFile]; !ok {
			t.Errorf("expected %s in NewFiles, got %v", genFile, filesDiff.NewFiles)
		}
	})

	t.Run("ignore-if-exists file is left untouched", func(t *testing.T) {
		tmpDir := t.TempDir()

		goMod := filepath.Join(tmpDir, "go.mod")
		writeFile(t, goMod, "module example.com/project\n")

		files := []ds.Files{{DestName: goMod, OldDestName: goMod}}
		prev := meta.Manifest{genRecord(t, "psg_other_gen.go", "package main\n\n"+disclaimerLine+"\n")}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, files, prev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := filesDiff.IgnoreFiles[goMod]; !ok {
			t.Errorf("expected %s in IgnoreFiles, got %v", goMod, filesDiff.IgnoreFiles)
		}

		if _, ok := filesDiff.NewFiles[goMod]; ok {
			t.Error("ignored file must not be rewritten")
		}
	})

	t.Run("non-regular entries in the tree are not visited", func(t *testing.T) {
		tmpDir := t.TempDir()

		// A dangling symlink and a fifo break the fallback tree walk; with a
		// manifest they are never looked at.
		if err := os.Symlink(filepath.Join(tmpDir, "missing"), filepath.Join(tmpDir, "dangling")); err != nil {
			t.Fatal(err)
		}

		genContent := "package main\n\n" + disclaimerLine + "\n"
		genFile := filepath.Join(tmpDir, "psg_handler_gen.go")
		writeFile(t, genFile, genContent)

		files := []ds.Files{{DestName: genFile, OldDestName: genFile}}
		prev := meta.Manifest{genRecord(t, "psg_handler_gen.go", genContent)}

		if _, err := GetUserCodeFromFiles(tmpDir, files, prev); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("symlink on a generated path returns ErrNonRegularTarget", func(t *testing.T) {
		baseDir := t.TempDir()

		outsideFile := filepath.Join(baseDir, "outside.go")
		writeFile(t, outsideFile, "package main\n")

		tmpDir := filepath.Join(baseDir, "proj")
		if err := os.Mkdir(tmpDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Same invariant as in the fallback walk: the generator must not write
		// through a link, so the manifest path reports the same sentinel.
		destName := filepath.Join(tmpDir, "psg_handler_gen.go")
		if err := os.Symlink(outsideFile, destName); err != nil {
			t.Fatal(err)
		}

		files := []ds.Files{{DestName: destName, OldDestName: destName}}
		prev := meta.Manifest{genRecord(t, "psg_handler_gen.go", "package main\n\n"+disclaimerLine+"\n")}

		_, err := GetUserCodeFromFiles(tmpDir, files, prev)
		if err == nil {
			t.Fatal("expected error for symlink on a generated path, got nil")
		}

		if !errors.Is(err, ErrNonRegularTarget) {
			t.Errorf("error should be ErrNonRegularTarget, got: %v", err)
		}

		if !strings.Contains(err.Error(), destName) {
			t.Errorf("error should mention the offending path %s, got: %v", destName, err)
		}
	})
}

// TestGetUserCodeFromFilesConflicts covers the manifest-mode guard against
// overwriting a file the generator never wrote: a new generator version starts
// to claim a path where the user already keeps a file of their own.
func TestGetUserCodeFromFilesConflicts(t *testing.T) {
	disclaimerLine := "// " + disclaimer

	// otherRecord keeps the manifest non-empty (manifest mode) without
	// claiming any of the paths under test.
	otherRecord := func(t *testing.T) meta.Manifest {
		t.Helper()

		return meta.Manifest{genRecord(t, "psg_other_gen.go", "package main\n\n"+disclaimerLine+"\n")}
	}

	assertConflict := func(t *testing.T, err error, paths ...string) {
		t.Helper()

		if err == nil {
			t.Fatal("expected a conflict error, got nil")
		}

		if !errors.Is(err, ErrTargetConflict) {
			t.Errorf("error should be ErrTargetConflict, got: %v", err)
		}

		var conflict *TargetConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("error should be *TargetConflictError, got %T: %v", err, err)
		}

		if len(conflict.Paths) != len(paths) {
			t.Errorf("Paths = %v, want %v", conflict.Paths, paths)
		}

		for _, path := range paths {
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error should mention %s, got: %v", path, err)
			}
		}
	}

	t.Run("user file with disclaimer is a conflict, not a silent overwrite", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Hand-written file that happens to carry the marker: before the
		// manifest everything above it was rewritten without a word.
		userFile := filepath.Join(tmpDir, "pkg", "app", "psg_handler_gen.go")
		content := "package app\n\n// hand written\n\n" + disclaimerLine + "\n\nfunc userCode() {}\n"
		writeFile(t, userFile, content)

		files := []ds.Files{{DestName: userFile, OldDestName: userFile}}

		_, err := GetUserCodeFromFiles(tmpDir, files, otherRecord(t))
		assertConflict(t, err, userFile)

		if got, rerr := os.ReadFile(userFile); rerr != nil || string(got) != content {
			t.Errorf("user file must be left untouched, got %q (err %v)", got, rerr)
		}
	})

	t.Run("user file without disclaimer is a conflict, not a split error", func(t *testing.T) {
		tmpDir := t.TempDir()

		userFile := filepath.Join(tmpDir, "pkg", "app", "psg_handler_gen.go")
		writeFile(t, userFile, "package app\n\nfunc Handler() {}\n")

		files := []ds.Files{{DestName: userFile, OldDestName: userFile}}

		_, err := GetUserCodeFromFiles(tmpDir, files, otherRecord(t))
		assertConflict(t, err, userFile)

		if strings.Contains(err.Error(), "split disclaimer") {
			t.Errorf("a foreign file must not be reported as a broken marker, got: %v", err)
		}
	})

	t.Run("user JSON file is a conflict", func(t *testing.T) {
		tmpDir := t.TempDir()

		// JSON carries no marker, so the old behavior replaced the file whole.
		userFile := filepath.Join(tmpDir, "configs", "dashboard.json")
		writeFile(t, userFile, `{"title":"my dashboard"}`)

		files := []ds.Files{{DestName: userFile, OldDestName: userFile}}

		_, err := GetUserCodeFromFiles(tmpDir, files, otherRecord(t))
		assertConflict(t, err, userFile)
	})

	t.Run("every conflicting path is reported at once", func(t *testing.T) {
		tmpDir := t.TempDir()

		first := filepath.Join(tmpDir, "pkg", "app", "psg_a_gen.go")
		second := filepath.Join(tmpDir, "configs", "b.json")

		writeFile(t, first, "package app\n")
		writeFile(t, second, "{}")

		files := []ds.Files{
			{DestName: first, OldDestName: first},
			{DestName: second, OldDestName: second},
		}

		_, err := GetUserCodeFromFiles(tmpDir, files, otherRecord(t))
		assertConflict(t, err, first, second)
	})

	t.Run("user-owned paths are not conflicts", func(t *testing.T) {
		tmpDir := t.TempDir()

		goMod := filepath.Join(tmpDir, "go.mod")
		writeFile(t, goMod, "module example.com/project\n")

		asset := filepath.Join(tmpDir, "static", "app.js")
		writeFile(t, asset, "console.log('mine')\n")

		files := []ds.Files{
			{DestName: goMod, OldDestName: goMod},
			{DestName: asset, OldDestName: asset},
		}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, files, otherRecord(t), "static/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, path := range []string{goMod, asset} {
			if _, ok := filesDiff.IgnoreFiles[path]; !ok {
				t.Errorf("expected %s in IgnoreFiles, got %v", path, filesDiff.IgnoreFiles)
			}
		}
	})

	t.Run("pending rename recorded under the old name is not a conflict", func(t *testing.T) {
		tmpDir := t.TempDir()

		genContent := "package main\n\n" + disclaimerLine + "\n"
		oldFile := filepath.Join(tmpDir, "tests", "psg_config_gen.go")
		newFile := filepath.Join(tmpDir, "tests", "api", "psg_config_gen.go")

		writeFile(t, oldFile, genContent+"\nfunc userCode() {}\n")

		files := []ds.Files{{DestName: newFile, OldDestName: oldFile}}
		prev := meta.Manifest{genRecord(t, "tests/psg_config_gen.go", genContent)}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, files, prev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got := filesDiff.RenameFiles[oldFile]; got != newFile {
			t.Errorf("expected rename %s -> %s, got %q", oldFile, newFile, got)
		}

		if got := string(filesDiff.UserContent[newFile]); got != "\nfunc userCode() {}\n" {
			t.Errorf("user code must be carried over to the new name, got %q", got)
		}
	})

	t.Run("a foreign file on the old name of a rename is a conflict", func(t *testing.T) {
		tmpDir := t.TempDir()

		oldFile := filepath.Join(tmpDir, "tests", "psg_config_gen.go")
		newFile := filepath.Join(tmpDir, "tests", "api", "psg_config_gen.go")

		writeFile(t, oldFile, "package tests\n")

		files := []ds.Files{{DestName: newFile, OldDestName: oldFile}}

		_, err := GetUserCodeFromFiles(tmpDir, files, otherRecord(t))
		assertConflict(t, err, oldFile)
	})

	t.Run("manifest record without a hash still counts as ours", func(t *testing.T) {
		tmpDir := t.TempDir()

		genFile := filepath.Join(tmpDir, "psg_handler_gen.go")
		writeFile(t, genFile, "package main\n\n"+disclaimerLine+"\n")

		files := []ds.Files{{DestName: genFile, OldDestName: genFile}}
		prev := meta.Manifest{{Path: "psg_handler_gen.go", Kind: meta.KindGenerated}}

		filesDiff, err := GetUserCodeFromFiles(tmpDir, files, prev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(filesDiff.DriftFiles) != 0 {
			t.Errorf("a record without a hash cannot drift, got %v", filesDiff.DriftFiles)
		}
	})

	t.Run("without a manifest the old behavior is kept", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Same JSON file as in the conflict case above: a project that predates
		// meta.yaml v5 has nothing to compare against, so the run goes on.
		userFile := filepath.Join(tmpDir, "configs", "dashboard.json")
		writeFile(t, userFile, `{"title":"my dashboard"}`)

		files := []ds.Files{{DestName: userFile, OldDestName: userFile}}

		if _, err := GetUserCodeFromFiles(tmpDir, files, nil); err != nil {
			t.Fatalf("unexpected error without a manifest: %v", err)
		}
	})
}

func TestTargetConflictErrorMessage(t *testing.T) {
	if err := NewTargetConflictError(nil); err != nil {
		t.Errorf("no paths means no error, got: %v", err)
	}

	single := NewTargetConflictError([]string{"api/rest/svc/v1/svc.yaml"})
	wantSingle := "conflict: api/rest/svc/v1/svc.yaml exists and was not written by the generator — rename or remove it"

	if single.Error() != wantSingle {
		t.Errorf("Error() = %q, want %q", single.Error(), wantSingle)
	}

	// Paths are sorted so the message does not depend on traversal order.
	many := NewTargetConflictError([]string{"b.json", "a.go"})
	for _, want := range []string{"2 files exist", "\n  - a.go\n  - b.json"} {
		if !strings.Contains(many.Error(), want) {
			t.Errorf("Error() = %q, want it to contain %q", many.Error(), want)
		}
	}
}

func TestIsUserOwned(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		ignore  []string
		want    bool
	}{
		{name: "go.mod is user owned once created", relPath: "go.mod", want: true},
		{name: "docs are user owned", relPath: "docs/index.md", want: true},
		{name: "static assets are user owned", relPath: "static/app.js", ignore: []string{"static/"}, want: true},
		{name: "generated source is not", relPath: "cmd/api/psg_main_gen.go", want: false},
		{name: "static prefix does not leak", relPath: "pkg/static.go", ignore: []string{"static/"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUserOwned(tt.relPath, tt.ignore...); got != tt.want {
				t.Errorf("IsUserOwned(%q, %v) = %v, want %v", tt.relPath, tt.ignore, got, tt.want)
			}
		})
	}
}

func TestGeneratedRegionHash(t *testing.T) {
	disclaimerLine := "// " + disclaimer
	genContent := "package main\n\n" + disclaimerLine + "\n"

	t.Run("user code below the marker does not change the hash", func(t *testing.T) {
		bare, err := GeneratedRegionHash("psg_handler_gen.go", []byte(genContent))
		if err != nil {
			t.Fatal(err)
		}

		withUser, err := GeneratedRegionHash("psg_handler_gen.go", []byte(genContent+"\nfunc userCode() {}\n"))
		if err != nil {
			t.Fatal(err)
		}

		if bare != withUser {
			t.Errorf("hash must cover the generated region only: %s != %s", bare, withUser)
		}
	})

	t.Run("generated region change changes the hash", func(t *testing.T) {
		bare, err := GeneratedRegionHash("psg_handler_gen.go", []byte(genContent))
		if err != nil {
			t.Fatal(err)
		}

		edited, err := GeneratedRegionHash("psg_handler_gen.go", []byte("package main\n\n// hand written\n"+disclaimerLine+"\n"))
		if err != nil {
			t.Fatal(err)
		}

		if bare == edited {
			t.Error("edit above the marker must change the hash")
		}
	})

	t.Run("file without disclaimer support is hashed whole", func(t *testing.T) {
		got, err := GeneratedRegionHash("schema.json", []byte(`{"a":1}`))
		if err != nil {
			t.Fatal(err)
		}

		if want := meta.HashContent([]byte(`{"a":1}`)); got != want {
			t.Errorf("GeneratedRegionHash() = %s, want %s", got, want)
		}
	})

	t.Run("missing marker is an error", func(t *testing.T) {
		if _, err := GeneratedRegionHash("psg_handler_gen.go", []byte("package main\n")); err == nil {
			t.Error("expected error for content without disclaimer")
		}
	})
}

// ogenClientAuthParams builds the render params the ogen_client templates
// expect for a single client transport with the given auth_params.
func ogenClientAuthParams(authTransport, authType, instantiation string) GeneratorHandlerParams {
	const projectPath = "github.com/test/authproj"

	return GeneratorHandlerParams{
		GeneratorParams: GeneratorParams{
			ProjectName: "authproj",
			ProjectPath: projectPath,
			AppInfo:     "test",
			Applications: ds.Apps{
				{Name: "api", Transports: ds.Transports{"billing": ds.Transport{Name: "billing"}}},
			},
		},
		Transport: ds.Transport{
			Name:          "billing",
			PkgName:       "billing_v1",
			ApiVersion:    "v1",
			Type:          ds.RestTransportType,
			GeneratorType: "ogen_client",
			Instantiation: instantiation,
			AuthParams:    ds.AuthParams{Transport: authTransport, Type: authType},
			Import: []string{
				`billing_v1 "` + projectPath + `/pkg/rest/billing/v1"`,
				`"` + projectPath + `/internal/app/transport/rest/billing/v1"`,
			},
			WrapperImport: `"` + projectPath + `/internal/app/transport/rest/billing/v1"`,
		},
	}
}

func renderOgenClientTmpl(t *testing.T, name string, params GeneratorHandlerParams) (string, error) {
	t.Helper()

	tmpl, err := GetTemplate("embedded/templates/transport/rest/ogen_client/files/" + name)
	if err != nil {
		t.Fatalf("GetTemplate(%s) unexpected error: %v", name, err)
	}

	buf, err := GenerateByTmpl(tmpl, params, nil, "/project/internal/app/transport/rest/billing/v1/psg_gen.go")
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// TestOgenClientAuthTemplate covers the SecuritySource wrapper the
// ogen_client templates emit per auth_params type: the method name must match
// the ogen security scheme (AuthHeader / BearerAuth / BasicAuth) and the
// credentials must be read from the documented OnlineConf paths.
func TestOgenClientAuthTemplate(t *testing.T) {
	tests := []struct {
		name         string
		authType     string
		wantMethod   string
		wantOCLeaves []string
	}{
		{
			name:         "apikey",
			authType:     "apikey",
			wantMethod:   "func (ak *SecuritySource) AuthHeader(",
			wantOCLeaves: []string{`occonf.RESTClientAPIKey(ctx, "billing_v1")`},
		},
		{
			name:         "bearer",
			authType:     "bearer",
			wantMethod:   "func (s *SecuritySource) BearerAuth(",
			wantOCLeaves: []string{`occonf.RESTClientToken(ctx, "billing_v1")`},
		},
		{
			name:         "basic",
			authType:     "basic",
			wantMethod:   "func (s *SecuritySource) BasicAuth(",
			wantOCLeaves: []string{`occonf.RESTClientUsername(ctx, "billing_v1")`, `occonf.RESTClientPassword(ctx, "billing_v1")`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := renderOgenClientTmpl(t, "auth.go.tmpl", ogenClientAuthParams("header", tt.authType, "static"))
			if err != nil {
				t.Fatalf("rendering auth.go.tmpl for %s failed: %v", tt.authType, err)
			}

			if !strings.Contains(content, tt.wantMethod) {
				t.Errorf("auth.go for %s should contain %q, got:\n%s", tt.authType, tt.wantMethod, content)
			}

			for _, leaf := range tt.wantOCLeaves {
				if !strings.Contains(content, leaf) {
					t.Errorf("auth.go for %s should read OnlineConf through %s", tt.authType, leaf)
				}
			}

			// The wrapper lives in internal/app/transport/rest/billing/v1
			// itself — importing that path here is a self-import and makes
			// the generated project fail with an import cycle.
			if strings.Contains(content, "/internal/app/transport/rest/billing/v1") {
				t.Errorf("auth.go for %s must not import its own package, got:\n%s", tt.authType, content)
			}
		})
	}
}

// TestOgenClientAuthTemplateUnsupportedType asserts the template still fails
// loudly for auth types that have no SecuritySource branch, instead of
// emitting a wrapper that cannot compile.
func TestOgenClientAuthTemplateUnsupportedType(t *testing.T) {
	_, err := renderOgenClientTmpl(t, "auth.go.tmpl", ogenClientAuthParams("header", "oauth2", "static"))
	if err == nil {
		t.Fatal("rendering auth.go.tmpl with an unsupported auth type should fail")
	}

	if !strings.Contains(err.Error(), "Unsupported auth type") {
		t.Errorf("error should mention the unsupported auth type, got: %v", err)
	}
}

// TestOgenClientPassesSecuritySource verifies that both instantiation modes
// hand the generated SecuritySource to ogen's NewClient for every supported
// auth type — a client that skips it does not compile against a spec with a
// security scheme.
func TestOgenClientPassesSecuritySource(t *testing.T) {
	for _, instantiation := range []string{"static", "dynamic"} {
		for _, authType := range []string{"apikey", "bearer", "basic"} {
			t.Run(instantiation+"/"+authType, func(t *testing.T) {
				content, err := renderOgenClientTmpl(t, "client.go.tmpl", ogenClientAuthParams("header", authType, instantiation))
				if err != nil {
					t.Fatalf("rendering client.go.tmpl failed: %v", err)
				}

				if !strings.Contains(content, "&SecuritySource{},") {
					t.Errorf("%s client with %s auth should pass &SecuritySource{} to NewClient, got:\n%s",
						instantiation, authType, content)
				}
			})
		}
	}

	t.Run("no auth params", func(t *testing.T) {
		content, err := renderOgenClientTmpl(t, "client.go.tmpl", ogenClientAuthParams("", "", "static"))
		if err != nil {
			t.Fatalf("rendering client.go.tmpl failed: %v", err)
		}

		if strings.Contains(content, "&SecuritySource{}") {
			t.Errorf("client without auth params must not reference SecuritySource, got:\n%s", content)
		}
	})
}

func TestPrivateReposTokenSecretName(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		want       string
	}{
		{
			name:       "unset falls back to the historical default",
			configured: "",
			want:       DefaultPrivateReposTokenSecret,
		},
		{
			name:       "configured value wins",
			configured: "GO_MODULES_TOKEN",
			want:       "GO_MODULES_TOKEN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := GeneratorParams{PrivateReposTokenSecret: tt.configured}

			if got := params.PrivateReposTokenSecretName(); got != tt.want {
				t.Errorf("PrivateReposTokenSecretName() = %q, want %q", got, tt.want)
			}
		})
	}

	// The fallback is what keeps already-generated workflows unchanged, so the
	// constant itself is pinned: changing it silently rewrites every project
	// that never configured the field.
	if DefaultPrivateReposTokenSecret != "GH_PAT" {
		t.Errorf("DefaultPrivateReposTokenSecret = %q, want %q", DefaultPrivateReposTokenSecret, "GH_PAT")
	}

	// Params structs embed GeneratorParams; templates rendered with them must
	// resolve the method through the embedded value.
	appParams := GeneratorAppParams{GeneratorParams: GeneratorParams{PrivateReposTokenSecret: "GO_MODULES_TOKEN"}}
	if got := appParams.PrivateReposTokenSecretName(); got != "GO_MODULES_TOKEN" {
		t.Errorf("GeneratorAppParams.PrivateReposTokenSecretName() = %q, want %q", got, "GO_MODULES_TOKEN")
	}
}

func makefileTmplSource(t *testing.T) string {
	t.Helper()

	src, err := templates.ReadFile(makefileTmplPath)
	if err != nil {
		t.Fatalf("read %s: %v", makefileTmplPath, err)
	}

	return string(src)
}

// makefileTmplLine returns the single template line declaring the given make
// target, so a test can exercise one prerequisite list without rendering the
// whole Makefile.
func makefileTmplLine(t *testing.T, target string) string {
	t.Helper()

	for _, line := range strings.Split(makefileTmplSource(t), "\n") {
		if strings.HasPrefix(line, target+":") {
			return line
		}
	}

	t.Fatalf("no %q target line in %s", target, makefileTmplPath)

	return ""
}

// TestMakefileGenerateAggregatorSeparatesPrereqs renders the `generate:`
// aggregator with every optional generator switched on and checks that each
// one lands as its own prerequisite.
//
// The sqlc switch ($pg/$sqlc) is still a literal false in the template
// ("ToDo work with databases"), so no project config can turn it on and a
// full-generation test cannot reach the sqlc + ActiveRecord combination,
// nor the sqlc + custom generator one.
// Rendering the line standalone with the switches forced on is what exercises
// it: before TOOLS-49 the sqlc group carried no trailing separator and the two
// prerequisites fused into `sqlcgenerate-argen`, a target that does not exist.
func TestMakefileGenerateAggregatorSeparatesPrereqs(t *testing.T) {
	// The line itself only reads $ogen, $sqlc and $jsonschema; $proto is
	// already a plain literal prerequisite.
	src := "{{- $ogen := true }}{{- $sqlc := true }}{{- $jsonschema := true }}" +
		makefileTmplLine(t, "generate")

	tmpl, err := template.New("generate").Parse(src)
	if err != nil {
		t.Fatalf("parse generate line: %v", err)
	}

	var rendered strings.Builder

	data := struct {
		UseActiveRecord  bool
		CustomGenerators ds.CustomGenerators
	}{
		UseActiveRecord:  true,
		CustomGenerators: ds.CustomGenerators{{Name: "runtime_limits_ts"}},
	}

	if err := tmpl.Execute(&rendered, data); err != nil {
		t.Fatalf("render generate line: %v", err)
	}

	prereqs := strings.Fields(strings.TrimPrefix(rendered.String(), "generate:"))

	for _, want := range []string{
		"proto", "generate-ogen", "sqlc", "generate-argen",
		"generate-jsonschema", "generate-runtime_limits_ts",
	} {
		if !slices.Contains(prereqs, want) {
			t.Errorf("generate: prerequisites %q must contain %q as a separate target", prereqs, want)
		}
	}
}

// parseMakefilePrereqLine splits one template line into literals, emitting
// actions and nested conditional groups. The template never spreads a group
// across lines inside a prerequisite list; an unbalanced line is folded back
// flat rather than reported, since the caller only looks for missing
// separators.
func parseMakefilePrereqLine(line string) []makefilePrereqNode {
	// stack[0] collects the top level; an opener pushes a frame and {{ end }}
	// pops it into a group node on the frame below.
	stack := [][]makefilePrereqNode{nil}

	var (
		openVerbs []string
		openAt    []int
		pos       int
	)

	push := func(n makefilePrereqNode) {
		top := len(stack) - 1
		stack[top] = append(stack[top], n)
	}

	for _, m := range makefileActionRe.FindAllStringSubmatchIndex(line, -1) {
		if m[0] > pos {
			push(makefilePrereqNode{kind: makefileLiteral, src: line[pos:m[0]]})
		}

		action := line[m[0]:m[1]]
		pos = m[1]

		verb, _, _ := strings.Cut(line[m[2]:m[3]], " ")

		switch verb {
		case "if", "range", "with":
			stack = append(stack, nil)
			openVerbs = append(openVerbs, verb)
			openAt = append(openAt, m[0])
		case "end":
			if len(openVerbs) == 0 {
				continue
			}

			verb, at := openVerbs[len(openVerbs)-1], openAt[len(openAt)-1]
			openVerbs, openAt = openVerbs[:len(openVerbs)-1], openAt[:len(openAt)-1]

			children := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			push(makefilePrereqNode{
				kind:     makefileGroup,
				src:      line[at:pos],
				children: children,
				isRange:  verb == "range",
			})
		default:
			// Anything else prints a value whose text is unknown here:
			// {{ $a.Name }}, or {{ else }}, which this template never puts in
			// a prerequisite list.
			push(makefilePrereqNode{kind: makefileEmit, src: action})
		}
	}

	if pos < len(line) {
		push(makefilePrereqNode{kind: makefileLiteral, src: line[pos:]})
	}

	for len(stack) > 1 {
		children := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		top := len(stack) - 1
		stack[top] = append(stack[top], children...)
	}

	return stack[0]
}

// makefileStartsWithSep reports whether the text a node sequence emits always
// begins with something make takes as a prerequisite separator. An emitting
// action prints an unknown value, so it never separates; a group delegates to
// its own body.
func makefileStartsWithSep(nodes []makefilePrereqNode) bool {
	if len(nodes) == 0 {
		return false
	}

	switch n := nodes[0]; n.kind {
	case makefileGroup:
		return makefileStartsWithSep(n.children)
	case makefileLiteral:
		return strings.TrimLeft(n.src, " \t") != n.src
	default:
		return false
	}
}

// makefileEndsWithSep is makefileStartsWithSep for the other end of the
// sequence.
func makefileEndsWithSep(nodes []makefilePrereqNode) bool {
	if len(nodes) == 0 {
		return false
	}

	switch n := nodes[len(nodes)-1]; n.kind {
	case makefileGroup:
		return makefileEndsWithSep(n.children)
	case makefileLiteral:
		// The colon closing the target name separates the first prerequisite
		// just as whitespace does: `target:prereq` is valid make.
		return strings.TrimRight(n.src, " \t") != n.src || strings.HasSuffix(n.src, ":")
	default:
		return false
	}
}

// makefilePrereqSepIssues reports every junction on a make target line where
// two prerequisites can fuse into one.
//
// A prerequisite list is whitespace-separated, so a conditional group has to
// carry its own separator: the group's targets appear only when the group is
// taken, and so must the space next to them. Either side does the job — a
// leading separator ({{ if $x }} foo{{ end }}) and a trailing one
// ({{ if $x }}foo {{ end }}) are both correct and the template uses both — as
// long as every junction ends up with one. Where none does, the group's
// outermost target fuses with its neighbor's and make fails with "No rule to
// make target": TOOLS-49 dropped the separator after `sqlc`, turning
// `sqlc` + `generate-argen` into `sqlcgenerate-argen`.
func makefilePrereqSepIssues(line string) []string {
	var (
		issues []string
		walk   func(nodes []makefilePrereqNode)
	)

	walk = func(nodes []makefilePrereqNode) {
		for i, n := range nodes {
			// Only a conditional group can make a junction disappear; two
			// adjacent leaves are text that is always there to be read.
			if i > 0 && (n.kind == makefileGroup || nodes[i-1].kind == makefileGroup) &&
				!makefileEndsWithSep(nodes[i-1:i]) && !makefileStartsWithSep(nodes[i:]) {
				issues = append(issues, fmt.Sprintf("%q meets %q with no separator between them",
					nodes[i-1].src, n.src))
			}

			if n.kind != makefileGroup {
				continue
			}

			// A range body repeats, so its own tail meets its own head.
			if n.isRange && !makefileEndsWithSep(n.children) && !makefileStartsWithSep(n.children) {
				issues = append(issues,
					fmt.Sprintf("%q repeats with no separator between iterations", n.src))
			}

			walk(n.children)
		}
	}

	walk(parseMakefilePrereqLine(line))

	return issues
}

// TestMakefilePrereqSeparators guards the whole class of the TOOLS-49 bug
// instead of the one line that happened to be broken: every conditional group
// in a prerequisite list must keep its targets apart from their neighbors.
// These chains keep growing — each new optional generator appends one more
// group — so the invariant is checked on every target line of the template.
func TestMakefilePrereqSeparators(t *testing.T) {
	for i, line := range strings.Split(makefileTmplSource(t), "\n") {
		if !makefileTargetLineRe.MatchString(line) || !strings.Contains(line, "{{") {
			continue
		}

		for _, issue := range makefilePrereqSepIssues(line) {
			t.Errorf("%s:%d: prerequisites fuse into one target: %s\n\t%s",
				makefileTmplPath, i+1, issue, line)
		}
	}
}

// TestMakefilePrereqSepIssues covers the checker behind
// TestMakefilePrereqSeparators on both conventions and on the fusing shapes
// the template does not currently contain.
func TestMakefilePrereqSepIssues(t *testing.T) {
	tests := []struct {
		name string
		line string
		// want is true when the line must be reported as fusing.
		want bool
	}{
		{
			name: "separator trails each group",
			line: `generate: proto {{ if $sqlc }}sqlc {{ end }}{{ if .UseActiveRecord }}generate-argen {{ end }}`,
		},
		{
			name: "separator leads the group",
			line: `lint: install-lint{{ if $cg }} lint-custom{{ end }}`,
		},
		{
			name: "colon separates the first prerequisite",
			line: `proto:{{ if $bufLocal }} ensure-buf ensure-protoc-plugins{{ end }}`,
		},
		{
			name: "nested group supplies the separator",
			line: `docker: {{ range $_, $a := .Apps }}{{ if $a.HasDocker }}docker-{{ $a.Name }} {{ end }}{{ end }}`,
		},
		{
			name: "TOOLS-49: group carries no separator at all",
			line: `generate: proto {{ if $sqlc }}sqlc{{ end }}{{ if .UseActiveRecord }}generate-argen {{ end }}`,
			want: true,
		},
		{
			name: "leading group ahead of a trailing group fuses",
			line: `lint: base{{ if $a }} first{{ end }}{{ if $b }}second {{ end }}`,
			want: true,
		},
		{
			name: "range body unseparated fuses between iterations",
			line: `build: {{ range $_, $a := .Apps }}build-{{ $a.Name }}{{ end }}`,
			want: true,
		},
		{
			name: "group fuses with the literal behind it",
			line: `install-tools: base {{ if $a }}install-a{{ end }}install-goimports`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := makefilePrereqSepIssues(tt.line)
			if got := len(issues) > 0; got != tt.want {
				t.Errorf("makefilePrereqSepIssues(%q) = %v, want any issue = %v", tt.line, issues, tt.want)
			}
		})
	}
}

func TestYAMLQuote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain command",
			in:   "make lint",
			want: "'make lint'",
		},
		{
			name: "command with a colon",
			in:   "go test ./... -run TestX:Y",
			want: "'go test ./... -run TestX:Y'",
		},
		{
			name: "command with a single quote",
			in:   "sh -c 'make lint'",
			want: "'sh -c ''make lint'''",
		},
		{
			name: "value starting with a YAML indicator",
			in:   "*lint",
			want: "'*lint'",
		},
		{
			name: "empty value",
			in:   "",
			want: "''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := YAMLQuote(tt.in); got != tt.want {
				t.Errorf("YAMLQuote(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
