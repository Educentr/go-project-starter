package test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// gitlabJob is the subset of a GitLab CI job this test inspects.
type gitlabJob struct {
	Script []string `yaml:"script"`
}

// notifyEnv is the environment the notify job reads: the CI variables GitLab
// provides plus the artifact lists prepare-env passes through build.env.
var notifyEnv = map[string]string{
	"TELEGRAM_BOT_TOKEN": "test-token",
	"TELEGRAM_CHAT_ID":   "-1001234567890",
	"CI_COMMIT_MESSAGE":  "fix: escape the lists (TOOLS-105)",
	"CI_PROJECT_PATH":    "org/checkstest",
	"CI_PIPELINE_URL":    "https://gitlab.example.com/org/checkstest/-/pipelines/7",
	"CI_COMMIT_BRANCH":   "main",
	// make docker-list output.
	"CONTAINERS": `["docker-api", "docker-worker"]`,
	// make artifact-list output.
	"ARTIFACTS": `["checkstest-api:1.2.3", "checkstest-worker:1.2.3"]`,
}

// gitlabNotifyScript reads the generated .gitlab-ci.yml and returns the shell
// script of the notify job. The notify block is unmarshalled on its own rather
// than with the whole pipeline, so that this test keeps reporting on the
// escaping of the notify job and not on an unrelated defect elsewhere in the
// generated file; the YAML parser is what dedents the script block scalar.
func gitlabNotifyScript(t *testing.T, targetDir string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(targetDir, ".gitlab-ci.yml"))
	if err != nil {
		t.Fatalf("Error reading .gitlab-ci.yml: %v", err)
	}

	block := gitlabTopLevelBlock(t, string(content), "notify")

	var jobs map[string]gitlabJob
	if err := yaml.Unmarshal([]byte(block), &jobs); err != nil {
		t.Fatalf("notify job is not valid YAML: %v\n%s", err, block)
	}

	job := jobs["notify"]
	if len(job.Script) != 1 {
		t.Fatalf("notify job has %d script entries, want 1: %#v", len(job.Script), job.Script)
	}

	return job.Script[0]
}

// gitlabTopLevelBlock cuts the block of one top-level key out of a GitLab
// pipeline: from the line that declares it to the next line starting in the
// first column (another key, a comment or the generator disclaimer).
func gitlabTopLevelBlock(t *testing.T, content, key string) string {
	t.Helper()

	lines := strings.Split(content, "\n")

	start := -1

	for i, line := range lines {
		if line == key+":" {
			start = i

			break
		}
	}

	if start < 0 {
		t.Fatalf(".gitlab-ci.yml has no %s job:\n%s", key, content)
	}

	for i := start + 1; i < len(lines); i++ {
		if line := lines[i]; line != "" && !strings.HasPrefix(line, " ") {
			return strings.Join(lines[start:i], "\n")
		}
	}

	return strings.Join(lines[start:], "\n")
}

// notifyTelegramText runs the notify job script with curl replaced by a stub
// that records the form fields it is given, and returns the value of the text
// field — the one Telegram parses as MarkdownV2. env overrides notifyEnv.
func notifyTelegramText(t *testing.T, script string, env map[string]string) string {
	t.Helper()

	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh is not available: %v", err)
	}

	dir := t.TempDir()

	// The stub writes every -d value to its own file: the text field contains
	// newlines, so a single file with a line-based separator would be
	// ambiguous. The target directory arrives in an environment variable, so
	// that a temp path with a shell metacharacter in it cannot break the stub.
	const stub = `#!/bin/sh
i=0
while [ $# -gt 0 ]; do
  if [ "$1" = "-d" ]; then
    i=$((i+1))
    printf '%s' "$2" > "$NOTIFY_FIELD_DIR/field.$i"
    shift 2
  else
    shift
  fi
done
`

	if err := os.WriteFile(filepath.Join(dir, "curl"), []byte(stub), 0o755); err != nil {
		t.Fatalf("Error writing curl stub: %v", err)
	}

	scriptPath := filepath.Join(dir, "notify.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatalf("Error writing notify script: %v", err)
	}

	// A clean environment: the test runner itself may run under CI and carry
	// its own CI_* variables, which would shadow the ones under test.
	cmdEnv := []string{
		"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"NOTIFY_FIELD_DIR=" + dir,
	}

	for name, value := range notifyEnv {
		if override, ok := env[name]; ok {
			value = override
		}

		cmdEnv = append(cmdEnv, name+"="+value)
	}

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = cmdEnv

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("notify script failed: %v\n%s", err, out)
	}

	for i := 1; ; i++ {
		field, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("field.%d", i)))
		if err != nil {
			break
		}

		if text, ok := strings.CutPrefix(string(field), "text="); ok {
			return text
		}
	}

	t.Fatal("notify script did not pass a text field to curl")

	return ""
}

// TestGenerateGitlabCINotifyEscapesMarkdownV2 verifies that the notify job of
// the generated .gitlab-ci.yml escapes every value it interpolates into the
// MarkdownV2 message. An unescaped reserved character makes Telegram answer
// 400 Can't parse entities, which curl -s hides: the job stays green and the
// notification is never delivered.
func TestGenerateGitlabCINotifyEscapesMarkdownV2(t *testing.T) {
	// ci-checks declares both docker and deb artifacts, so the notify job
	// carries the image list and the package list.
	targetDir := generateProject(t, "ci-checks")
	script := gitlabNotifyScript(t, targetDir)

	t.Run("artifact lists", func(t *testing.T) {
		text := notifyTelegramText(t, script, nil)

		// - and . are reserved; : is not, and escaping it would reach the
		// message as a literal backslash.
		for _, want := range []string{
			`docker\-api`,
			`docker\-worker`,
			`checkstest\-api:1\.2\.3`,
			`checkstest\-worker:1\.2\.3`,
		} {
			if !strings.Contains(text, want) {
				t.Errorf("notify message does not contain %q:\n%s", want, text)
			}
		}

		for _, unwanted := range []string{"docker-api", "checkstest-api:1.2.3", `\:`} {
			if strings.Contains(text, unwanted) {
				t.Errorf("notify message contains unescaped %q:\n%s", unwanted, text)
			}
		}
	})

	t.Run("branch", func(t *testing.T) {
		for branch, want := range map[string]string{
			"release/1.2": `release/1\.2`,
			"hotfix-x":    `hotfix\-x`,
			"feature_x":   `feature\_x`,
		} {
			t.Run(branch, func(t *testing.T) {
				text := notifyTelegramText(t, script, map[string]string{"CI_COMMIT_BRANCH": branch})

				if !strings.Contains(text, "на ветке *"+want+"*") {
					t.Errorf("notify message does not contain the escaped branch %q:\n%s", want, text)
				}

				if strings.Contains(text, branch) {
					t.Errorf("notify message contains the unescaped branch %q:\n%s", branch, text)
				}
			})
		}
	})
}
