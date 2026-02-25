package templater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
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
		filesDiff, err := GetUserCodeFromFiles(tmpDir, nil)
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

		_, err := GetUserCodeFromFiles(tmpDir, nil)
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

		filesDiff, err := GetUserCodeFromFiles(tmpDir, nil)
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

		filesDiff, err := GetUserCodeFromFiles(tmpDir, files)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := filesDiff.ObsoleteFiles[destName]; ok {
			t.Errorf("file in template set should NOT be in ObsoleteFiles")
		}
	})
}
