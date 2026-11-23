package test

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// githubWorkflow is the subset of a GitHub Actions workflow the github_ci
// parameters affect.
type githubWorkflow struct {
	// On carries the workflow triggers. The key is quoted in the tag because
	// `on` is also a YAML 1.1 boolean; yaml.v3 reads it as the string key the
	// generated workflow means.
	On   githubTriggers       `yaml:"on"`
	Jobs map[string]githubJob `yaml:"jobs"`
}

// githubTriggers is the subset of the `on:` section the github_ci parameters
// affect.
type githubTriggers struct {
	Push githubPushTrigger `yaml:"push"`
}

// githubPushTrigger is the subset of the `on.push` trigger the github_ci
// parameters affect. The filters are decoded as []any, not []string, because
// GitHub wants them as strings and a filter the generator emitted unquoted
// resolves to whatever YAML makes of it (`1.10` to the float 1.1). Decoding
// into []string would convert such a value back to a string and hide it.
type githubPushTrigger struct {
	Branches []any `yaml:"branches"`
}

// githubJob is the subset of a workflow job the github_ci parameters affect.
// Env is decoded because a job-level variable is part of the environment its
// steps run with: a step executed in a test without it runs a different
// script than the runner does.
type githubJob struct {
	RunsOn         any               `yaml:"runs-on"`
	TimeoutMinutes *int              `yaml:"timeout-minutes"`
	If             string            `yaml:"if"`
	Env            map[string]string `yaml:"env"`
	Steps          []githubStep      `yaml:"steps"`
}

// githubStep is the subset of a job step the github_ci parameters affect.
type githubStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
	Run  string            `yaml:"run"`
}

// exprStubs resolves a workflow expression to the value the runner would
// substitute for it. Exact matches come first; the patterns cover the
// expressions whose names depend on the project (one PORT_PREFIX per
// application and transport), which an exact table would have to repeat per
// config.
type exprStubs struct {
	exact    map[string]string
	patterns []exprStubPattern
}

// exprStubPattern is a stub for every expression matching rx.
type exprStubPattern struct {
	rx    *regexp.Regexp
	value string
	// expand makes value a template over rx's capture groups ($1), for an
	// expression whose value is derived from its own name. It is off by
	// default because a stubbed value is itself allowed to contain `$(`,
	// which the expansion would read as an empty group name and eat.
	expand bool
}

// repoStubValues are the values of the expressions whose content is chosen
// outside the template: the repository's own variables and secrets, and the
// version `make release-tag` produced. They are a struct of their own so a
// test can hand the steps text that reads as shell code — which is the whole
// subject of the injection tests — without restating the stub table.
type repoStubValues struct {
	portPrefix        string
	domain            string
	envType           string
	ocUser            string
	ocPassword        string
	ocHost            string
	ocPort            string
	release           string
	registryServer    string
	registryContainer string
	internalSubnet    string
}

// envFileEntry is a line of the env file the `Create env file` step writes:
// the key, whether the value is written inside double quotes, and the value
// the stubbed repository provides for it.
type envFileEntry struct {
	key    string
	quoted bool
	value  func(repoStubValues) string
}

// stepRun is the outcome of executing a step the way a runner executes it.
type stepRun struct {
	// dir is the working directory the step ran in: the files it wrote and
	// anything a hostile value managed to create are both in there.
	dir string
	// output are the lines the step appended to $GITHUB_OUTPUT.
	output []string
	// combined is the stdout and stderr of the step, for failure messages.
	combined string
}

// injectionScanConfig is a generated project the `run:` scan walks.
// registryType, when set, overrides main.registry_type of the fixture: the
// registry login branch of a job is rendered only for digitalocean, and no
// fixture pairs it with declared CI checks.
type injectionScanConfig struct {
	name         string
	registryType string
}

// doRegistryLoginStepName is the name of the step that logs in to the
// DigitalOcean Container Registry in both jobs that push images.
const doRegistryLoginStepName = "Log in to DO Container Registry"

// branchVarsStepName is the name of the prepare-env step that resolves the
// per-branch names of the repository variables and secrets the build reads.
const branchVarsStepName = "Get branch"

// envFileStepName is the name of the deploy step that writes the env file of
// the branch being deployed.
const envFileStepName = "Create env file"

// instanceVarsStepName is the prepare-env step that validates the deployment
// variables and secrets of the branch and hands them on as job outputs.
const instanceVarsStepName = "Check and set vars for instances"

// notifyJobName is the job of ci_cd.yml that sends the build notification.
const notifyJobName = "artifact_version"

// notifyStepName is the step of the notify job that composes the Telegram
// message and sends it.
const notifyStepName = "Check conditions and set message"

// curlArgSeparator separates the arguments the curl stub of the notify step
// records, one marker line after each.
const curlArgSeparator = "===CURL-ARG==="

// markdownV2Specials are the characters Telegram reserves in the MarkdownV2
// parse mode the notify step sends the message with: an unescaped one makes
// Telegram answer 400 instead of delivering the message.
const markdownV2Specials = "_*[]()~`>#+-=|{}.!"

// branchSanitizePattern is the shell pattern both the `Get branch` step and
// the env file name derive from the branch name.
const branchSanitizePattern = `[^A-Za-z0-9_]`

// injectionMarkerFile is the file the hostile branch names of the injection
// tests try to create. Its absence after the step is what says the branch name
// stayed a value instead of becoming a command.
const injectionMarkerFile = "pwned"

// envTypeStub is the deploy environment type the stubbed `Create env file`
// step resolves, which the step appends to the env file name.
const envTypeStub = "prod"

// The GitHub Actions workflows the generator produces.
const (
	ciCDWorkflow           = ".github/workflows/ci_cd.yml"
	goVersionCheckWorkflow = ".github/workflows/go-version-check.yml"
)

// generatedWorkflowNames lists the GitHub Actions workflows the generator
// produces; the github_ci parameters apply to every job of both.
var generatedWorkflowNames = []string{
	ciCDWorkflow,
	goVersionCheckWorkflow,
}

// injectionScanConfigs are the configs the `run:` scan generates. Together
// they render every recipe of both workflow templates — which runStepNames
// below is what asserts — and each entry is here for the recipes no other
// entry renders:
//
//   - github-ci: prepare-env, create_env_files, build, deploy, the goat tests
//     and the notification, plus the DigitalOcean login in its install mode.
//   - github-ci-doctl-image: the same login in its image mode, where it runs
//     doctl from the runner image instead of installing it.
//   - ci-checks (as digitalocean): the checks job, and its own copy of that
//     login block.
//   - private-token-secret: build-packages, and the private-module access
//     steps of both workflows.
//   - docs-s3, docs-ghpages, docs-minio: the three branches of deploy-docs.
//     docs-minio also declares documentation.headers, the one input that
//     reaches a recipe as `secrets.` only after rendering.
//   - package-upload-{minio,aws,rsync}: the three branches of upload-packages.
var injectionScanConfigs = []injectionScanConfig{
	{name: "github-ci"},
	{name: "github-ci-doctl-image"},
	{name: "ci-checks", registryType: "digitalocean"},
	{name: "private-token-secret"},
	{name: "docs-s3"},
	{name: "docs-ghpages"},
	{name: "docs-minio"},
	{name: "package-upload-minio"},
	{name: "package-upload-aws"},
	{name: "package-upload-rsync"},
}

// runStepNames is every step of the generated workflows that carries a `run:`
// recipe. A new such step belongs here, together with a config above that
// renders it — otherwise its recipe is never scanned.
var runStepNames = []string{
	// prepare-env
	"Get branch",
	"Use vars",
	"Deploy vars",
	instanceVarsStepName,
	"Version and containers",
	// checks
	"Run check ${{ matrix.name }}",
	// the DigitalOcean registry login, in checks, build and the goat tests
	doRegistryLoginStepName,
	// create_env_files
	envFileStepName,
	// run-goat-tests-<app>
	"Run GOAT tests for api",
	// build
	"Build and push",
	// build-packages
	"Install nfpm",
	"Build packages",
	// upload-packages
	"Install MinIO Client",
	"Configure MinIO Client",
	"Upload to MinIO",
	"Upload to AWS S3",
	"Setup SSH",
	"Upload via rsync",
	// deploy-docs
	"Build docs",
	"Deploy to S3",
	"Deploy to MinIO",
	"Deploy to GitHub Pages",
	// deploy
	"Modify env file",
	// artifact_version
	notifyStepName,
	// the private-module access fragment, in both workflows
	"Configure Git for private repos",
	"Set GOPRIVATE",
	// go-version-check.yml
	"Check go.mod version against direct dependencies",
}

// privateReposStepNames are the two steps that give a job access to private Go
// modules: the git credential rewrite and GOPRIVATE. Every job that downloads
// modules needs both.
var privateReposStepNames = []string{
	"Configure Git for private repos",
	"Set GOPRIVATE",
}

// githubVarNameRx matches the characters GitHub allows in the name of a
// repository variable or secret. A name outside this set cannot exist, so a
// step that builds one sends the whole run down the "branch not enabled" path.
var githubVarNameRx = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// notifyBranchRx captures the branch name as the notify step rendered it into
// the message. The greedy group stops at the last `*` of the line, which is
// the one closing the bold branch name.
var notifyBranchRx = regexp.MustCompile(`(?m)на ветке \*(.*)\*\s*$`)

// workflowExprRx matches a GitHub Actions expression, which the runner
// substitutes before the shell ever sees the recipe.
var workflowExprRx = regexp.MustCompile(`\$\{\{\s*([^{}]+?)\s*\}\}`)

// injectableRunExprRx matches the workflow contexts whose value is chosen
// outside the template: the branch or tag name, the pull request head, the
// event payload, and the repository's own variables, secrets and job outputs.
// In `run:` the runner substitutes them before the shell starts, so the value
// becomes part of the script source — `release/$(cmd)` runs a command in a job
// that holds the repository secrets, a single quote closes a `'…'` and makes
// the rest of the line read as code, and a backtick kills the step with a
// syntax error. Such a value belongs in `env:`, where it stays a value.
//
// For a repository variable or secret there is no privilege escalation (the
// content is chosen by whoever configures the repository), but a broken env
// file and a red step on a value with a quote are the same defect; a secret
// pasted into a recipe also lands in the script file on the runner instead of
// staying in the step's environment.
//
// `vars[…]` and `secrets[…]` are matched next to `vars.` and `secrets.`
// because prepare-env reads them by index, which a dotted-form pattern alone
// would miss. `needs.*.outputs.` and `steps.*.outputs.` are matched by their
// `outputs.` segment: `needs.<job>.result` is a value of a closed set
// (success/failure/canceled/skipped) and carries no text.
//
// Deliberately not matched, each because the value cannot carry anything a
// shell reads as code:
//
//   - `env.<NAME>_NAME` — the name of a branch's repository variable, built by
//     the `Get branch` step out of the branch name normalized to
//     [A-Za-z0-9_]; GitHub allows no other character in such a name.
//   - `matrix.name`, `matrix.run`, `matrix.containers` — the checks declared
//     in project.yaml and the output of `make docker-list`, both chosen by the
//     project itself rather than by the repository settings. `matrix.ips` is
//     built from a repository variable and does travel through `env:`.
//   - `github.repository`, `github.workflow`, `github.run_id`,
//     `github.workspace` — the owner/repo slug, the `name:` of this very
//     template, a number and the runner's checkout path.
var injectableRunExprRx = regexp.MustCompile(
	`github\.(ref_name|head_ref|ref|event)\b` +
		`|\bsecrets[.\[]` +
		`|\bvars[.\[]` +
		`|\bneeds\.[A-Za-z0-9_-]+\.outputs\.` +
		`|\bsteps\.[A-Za-z0-9_-]+\.outputs\.`)

// branchSanitizeRx is branchSanitizePattern as the test applies it, to derive
// the name the steps build out of a branch name.
var branchSanitizeRx = regexp.MustCompile(branchSanitizePattern)

// hostileBranchNames are branch names `git check-ref-format` accepts and the
// shell reads as code: a command substitution, a backtick command and the
// unbalanced backtick and quote that break the script outright.
var hostileBranchNames = []string{
	"x$(touch " + injectionMarkerFile + ")",
	"x`touch " + injectionMarkerFile + "`",
	"a`b",
	`a"b`,
	"a;b",
}

// hostileValues are values a repository variable, a repository secret or a git
// tag can legitimately hold and a shell reads as code, as the end of a quoted
// string, or as more than one word. `100%s` is there for the other half of the
// fix: the value is handed to printf as an argument, never as its format.
var hostileValues = []string{
	"x$(touch " + injectionMarkerFile + ")",
	"x`touch " + injectionMarkerFile + "`",
	"a'b",
	`a"b`,
	"a b",
	"a;b",
	"100%s",
}

// envFileEntries is the env file the `Create env file` step writes for the
// github-ci fixture, in order. It pins the whole file, so a key, an order or a
// quoting that changed shows up here instead of in a deployment.
var envFileEntries = []envFileEntry{
	{"PORT_PREFIX_API_TRAEFIK", true, func(r repoStubValues) string { return r.portPrefix }},
	{"DOMAIN_API", true, func(r repoStubValues) string { return r.domain }},
	{"PORT_PREFIX_API_API", true, func(r repoStubValues) string { return r.portPrefix }},
	{"PORT_PREFIX_API_SYS", true, func(r repoStubValues) string { return r.portPrefix }},
	{"ENV_TYPE", false, func(r repoStubValues) string { return r.envType }},
	{"OC_HOST", true, func(r repoStubValues) string { return r.ocHost }},
	{"OC_PORT", true, func(r repoStubValues) string { return r.ocPort }},
	{"OC_USER", true, func(r repoStubValues) string { return r.ocUser }},
	{"OC_PASSWORD", true, func(r repoStubValues) string { return r.ocPassword }},
	{"APP_VERSION", true, func(r repoStubValues) string { return r.release }},
	{"REGISTRY_SERVER", false, func(r repoStubValues) string {
		return r.registryServer + "/" + r.registryContainer
	}},
	{"INTERNAL_SUBNET", false, func(r repoStubValues) string { return r.internalSubnet }},
}

// generate renders the config and returns the target directory.
func (c injectionScanConfig) generate(t *testing.T) string {
	t.Helper()

	if c.registryType == "" {
		return generateProject(t, c.name)
	}

	configDir := writeMatrixConfig(t, c.name, c.registryType, "")
	targetDir := t.TempDir()

	out, err := generateFromConfigDir(t, configDir, targetDir,
		"Generate "+c.name+" as "+c.registryType)
	if err != nil {
		t.Fatalf("Error creating project %s as %s: %v\n%s", c.name, c.registryType, err, out)
	}

	return targetDir
}

// repoValueCases are the repository value sets the injection tests run the
// executed steps with: the ordinary one, which pins that nothing changed for
// an existing deployment, plus one per hostile value.
func repoValueCases() map[string]repoStubValues {
	cases := map[string]repoStubValues{"ordinary": ordinaryRepoValues()}

	for _, value := range hostileValues {
		cases[value] = uniformRepoValues(value)
	}

	return cases
}

// lookup returns the stubbed value of an expression.
func (s exprStubs) lookup(expr string) (string, bool) {
	if value, ok := s.exact[expr]; ok {
		return value, true
	}

	for _, p := range s.patterns {
		if !p.rx.MatchString(expr) {
			continue
		}

		if p.expand {
			return p.rx.ReplaceAllString(expr, p.value), true
		}

		return p.value, true
	}

	return "", false
}

// ordinaryRepoValues are the values an ordinary deployment has. The
// assertions about an unchanged env file are written against them.
func ordinaryRepoValues() repoStubValues {
	return repoStubValues{
		portPrefix:        "31",
		domain:            "api.test",
		envType:           envTypeStub,
		ocUser:            "oc-user",
		ocPassword:        "oc-password",
		ocHost:            "oc.test",
		ocPort:            "8080",
		release:           "1.0.0",
		registryServer:    "registry.test",
		registryContainer: "github-ci-test",
		internalSubnet:    "10.1.0.0/24",
	}
}

// uniformRepoValues puts the same value in every field, so one assertion
// covers every value the step writes.
func uniformRepoValues(value string) repoStubValues {
	return repoStubValues{
		portPrefix: value, domain: value, envType: value,
		ocUser: value, ocPassword: value, ocHost: value, ocPort: value,
		release: value, registryServer: value, registryContainer: value,
		internalSubnet: value,
	}
}

// jsonArray renders value as a one-element JSON array, the shape prepare-env
// expects of a per-environment repository variable or secret. One element
// keeps the array's length at the LEN_EXPECTED the steps check it against.
func jsonArray(value string) string {
	// The error is dropped on purpose: encoding a []string cannot fail, which
	// is exactly what errchkjson checks.
	encoded, _ := json.Marshal([]string{value})

	return string(encoded)
}

// runnerStubs are the values a runner would substitute for the workflow
// expressions of the executed steps, for a push of branch with the build
// ending in buildResult.
func runnerStubs(branch, buildResult string) exprStubs {
	return runnerStubsFor(branch, buildResult, ordinaryRepoValues())
}

// runnerStubsFor is runnerStubs with the repository-owned values given
// explicitly.
func runnerStubsFor(branch, buildResult string, repo repoStubValues) exprStubs {
	return exprStubs{
		exact: map[string]string{
			"github.ref_name":                  branch,
			"github.ref":                       "refs/heads/" + branch,
			"github.repository":                "Educentr/github-ci-test",
			"github.workflow":                  "CI Pipeline",
			"github.run_id":                    "42",
			"github.event.head_commit.message": "fix: a commit",
			"secrets.TELEGRAM_BOT_TOKEN":       "stub-token",
			"secrets.TELEGRAM_CHAT_ID":         "1",
			"secrets.REGISTRY_LOGIN_SERVER":    repo.registryServer,
			"vars.REGISTRY_CONTAINER":          repo.registryContainer,
			"needs.build.result":               buildResult,
			"needs.build-packages.result":      buildResult,

			// One deployment environment, so the arrays above hold one
			// element and `Check and set vars for instances` accepts them.
			"steps.vars_deploy.outputs.LEN_EXPECTED": "1",

			"needs.prepare-env.outputs.ARTIFACTS":                       `["registry/github-ci-test:1.0"]`,
			"needs.prepare-env.outputs.PACKAGES":                        `["github-ci-test-1.0.rpm"]`,
			"needs.prepare-env.outputs.OC_HOST":                         repo.ocHost,
			"needs.prepare-env.outputs.OC_PORT":                         repo.ocPort,
			"needs.prepare-env.outputs.RELEASE":                         repo.release,
			"needs.prepare-env.outputs.INTERNAL_SUBNET":                 repo.internalSubnet,
			"fromJSON(needs.prepare-env.outputs.ENV_TYPE)[matrix.envs]": repo.envType,
		},
		patterns: []exprStubPattern{
			{
				rx:    regexp.MustCompile(`^fromJson\(needs\.prepare-env\.outputs\.PORT_PREFIX_[A-Za-z0-9_]+\)\[matrix\.envs\]$`),
				value: repo.portPrefix,
			},
			{
				rx:    regexp.MustCompile(`^fromJson\(needs\.prepare-env\.outputs\.DOMAIN_[A-Za-z0-9_]+\)\[matrix\.envs\]$`),
				value: repo.domain,
			},
			{
				rx:    regexp.MustCompile(`^fromJson\(secrets\[needs\.prepare-env\.outputs\.OC_USER\]\)\[matrix\.envs\]$`),
				value: repo.ocUser,
			},
			{
				rx:    regexp.MustCompile(`^fromJson\(secrets\[needs\.prepare-env\.outputs\.OC_PASSWORD\]\)\[matrix\.envs\]$`),
				value: repo.ocPassword,
			},
			{
				rx:    regexp.MustCompile(`^vars\[env\.PORT_PREFIX_[A-Za-z0-9_]+\]$`),
				value: jsonArray(repo.portPrefix),
			},
			{
				rx:    regexp.MustCompile(`^vars\[env\.DOMAIN_[A-Za-z0-9_]+\]$`),
				value: jsonArray(repo.domain),
			},
			{
				rx:    regexp.MustCompile(`^vars\[env\.ENV_TYPE_NAME\]$`),
				value: jsonArray(repo.envType),
			},
			{
				rx:    regexp.MustCompile(`^secrets\[env\.OC_USER_NAME\]$`),
				value: jsonArray(repo.ocUser),
			},
			{
				rx:    regexp.MustCompile(`^secrets\[env\.OC_PASSWORD_NAME\]$`),
				value: jsonArray(repo.ocPassword),
			},
			{
				// The name of a branch's repository variable, as the `Get
				// branch` step builds it. Derived from the expression itself,
				// hence the capture-group expansion.
				rx:     regexp.MustCompile(`^env\.([A-Za-z0-9_]+)_NAME$`),
				value:  "${1}_" + strings.ToUpper(sanitizeBranch(branch)),
				expand: true,
			},
		},
	}
}

// substituteWorkflowExprs replaces every workflow expression in text with the
// value the runner would substitute, so the text can be used as what it
// becomes on a runner. An expression without a stub fails the test: a silently
// empty substitution would hide whatever the step does with that value.
func substituteWorkflowExprs(t *testing.T, what, text string, stubs exprStubs) string {
	t.Helper()

	var unknown []string

	substituted := workflowExprRx.ReplaceAllStringFunc(text, func(match string) string {
		expr := workflowExprRx.FindStringSubmatch(match)[1]

		value, ok := stubs.lookup(expr)
		if !ok {
			unknown = append(unknown, expr)
		}

		return value
	})

	if len(unknown) != 0 {
		t.Fatalf("%s uses the workflow expressions %v the test has no stub for", what, unknown)
	}

	return substituted
}

// stepShellEnv renders the environment a step runs with — the `env:` of its
// job plus its own, the step's winning — into the KEY=VALUE entries the shell
// gets, with the same expression substitution the runner does.
//
// Reading the `env:` blocks is what keeps the executed script the one the
// runner runs: a value moved out of `run:` into `env:` disappears from the
// recipe, so a test that only executed the recipe would run it with the
// variable unset and pass without asserting anything.
func stepShellEnv(t *testing.T, what string, job githubJob, step githubStep, stubs exprStubs) []string {
	t.Helper()

	vars := map[string]string{}

	for name, value := range job.Env {
		vars[name] = substituteWorkflowExprs(t, what+" job env "+name, value, stubs)
	}

	for name, value := range step.Env {
		vars[name] = substituteWorkflowExprs(t, what+" step env "+name, value, stubs)
	}

	env := make([]string, 0, len(vars))

	for name, value := range vars {
		env = append(env, name+"="+value)
	}

	slices.Sort(env)

	return env
}

// findStepByName returns the job that carries the named step and the step
// itself. The step is looked up by name across jobs because the job it lives
// in is itself conditional on the project's artifacts.
func findStepByName(t *testing.T, targetDir, stepName string) (string, githubJob, githubStep) {
	t.Helper()

	wf := parseWorkflow(t, targetDir, ciCDWorkflow)

	for _, jobName := range slices.Sorted(maps.Keys(wf.Jobs)) {
		job := wf.Jobs[jobName]

		for _, step := range job.Steps {
			if step.Name == stepName {
				return jobName, job, step
			}
		}
	}

	t.Fatalf("ci_cd.yml has no %q step, got jobs %v", stepName, slices.Sorted(maps.Keys(wf.Jobs)))

	return "", githubJob{}, githubStep{}
}

// sanitizeBranch is what both the `Get branch` step and the env file name do
// to a branch name before using it.
func sanitizeBranch(branch string) string {
	return branchSanitizeRx.ReplaceAllString(branch, "_")
}

// stepsByName indexes the steps of a job by step name.
func stepsByName(job githubJob) map[string]githubStep {
	steps := map[string]githubStep{}

	for _, step := range job.Steps {
		steps[step.Name] = step
	}

	return steps
}

// generateProject runs the generator for a config dir from
// test/docker-integration/configs and returns the target directory.
func generateProject(t *testing.T, configName string) string {
	t.Helper()

	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory: %v", err)
	}

	configDir := filepath.Join(curDir, "..", "test", "docker-integration", "configs", configName)
	tmpDir := t.TempDir()

	out, err := ExecCommand(filepath.Join(curDir, ".."), "go", []string{
		"run", filepath.Join(curDir, "..", "cmd", "go-project-starter", "main.go"),
		"--target", tmpDir,
		"--configDir", configDir,
		"--config", "project.yaml",
	}, "Generate "+configName+" project ("+tmpDir+")")
	if err != nil {
		t.Fatalf("Error creating project: %s\n%s", err, out)
	}

	return tmpDir
}

// parseWorkflow reads a generated workflow and unmarshals it, which also
// asserts that the generated YAML is valid.
func parseWorkflow(t *testing.T, targetDir, relPath string) githubWorkflow {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(targetDir, relPath))
	if err != nil {
		t.Fatalf("Error reading %s: %v", relPath, err)
	}

	var wf githubWorkflow
	if err := yaml.Unmarshal(content, &wf); err != nil {
		t.Fatalf("%s is not valid YAML: %v\n%s", relPath, err, content)
	}

	if len(wf.Jobs) == 0 {
		t.Fatalf("%s has no jobs", relPath)
	}

	return wf
}

// assertPushBranches checks the `on.push.branches` filters of both generated
// workflows against want, each as the string it was configured as. The two
// templates render the list in different YAML styles (block in ci_cd.yml, flow
// in go-version-check.yml), so both are parsed back rather than compared as
// text.
func assertPushBranches(t *testing.T, targetDir string, want []string) {
	t.Helper()

	for _, name := range generatedWorkflowNames {
		got := parseWorkflow(t, targetDir, name).On.Push.Branches

		if len(got) != len(want) {
			t.Errorf("%s: on.push.branches = %#v, want %#v", name, got, want)

			continue
		}

		for i, wantBranch := range want {
			gotBranch, isStr := got[i].(string)
			if !isStr {
				t.Errorf("%s: on.push.branches[%d] = %#v (%T), want the string %q",
					name, i, got[i], got[i], wantBranch)

				continue
			}

			if gotBranch != wantBranch {
				t.Errorf("%s: on.push.branches[%d] = %q, want %q", name, i, gotBranch, wantBranch)
			}
		}
	}
}

// doctlVersions collects the version input of every digitalocean/action-doctl
// step of a workflow. A nil entry means the step has no version input.
func doctlVersions(wf githubWorkflow) []any {
	versions := []any{}

	for _, job := range wf.Jobs {
		for _, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "digitalocean/action-doctl@") {
				versions = append(versions, step.With["version"])
			}
		}
	}

	return versions
}

// doRegistryLoginSteps collects the DigitalOcean registry login steps of a
// workflow, keyed by job name so a missing job is visible in the failure.
func doRegistryLoginSteps(wf githubWorkflow) map[string]githubStep {
	steps := map[string]githubStep{}

	for jobName, job := range wf.Jobs {
		for _, step := range job.Steps {
			if step.Name == doRegistryLoginStepName {
				steps[jobName] = step
			}
		}
	}

	return steps
}

// TestGenerateGithubCIDefaults verifies that without a github_ci section the
// generated workflows keep the historical output: every job runs on
// ubuntu-latest, no job declares a timeout and action-doctl installs the
// latest release.
func TestGenerateGithubCIDefaults(t *testing.T) {
	// github-ci-defaults is github-ci without the github_ci block, so the two
	// configs differ only in the parameters under test.
	tmpDir := generateProject(t, "github-ci-defaults")

	for _, name := range generatedWorkflowNames {
		wf := parseWorkflow(t, tmpDir, name)

		for jobName, job := range wf.Jobs {
			if job.RunsOn != "ubuntu-latest" {
				t.Errorf("%s: job %q runs-on = %#v, want \"ubuntu-latest\"", name, jobName, job.RunsOn)
			}

			if job.TimeoutMinutes != nil {
				t.Errorf("%s: job %q should not declare timeout-minutes, got %d", name, jobName, *job.TimeoutMinutes)
			}
		}
	}

	// Both workflows must keep the historical push trigger.
	assertPushBranches(t, tmpDir, []string{"main", "staging"})

	// action-doctl must keep installing the latest release.
	wf := parseWorkflow(t, tmpDir, ciCDWorkflow)

	versions := doctlVersions(wf)
	if len(versions) == 0 {
		t.Fatal("ci_cd.yml has no digitalocean/action-doctl steps, expected the digitalocean registry branch")
	}

	for i, v := range versions {
		if v != nil {
			t.Errorf("action-doctl step %d: version = %#v, want no version input", i, v)
		}
	}

	// The login step keeps taking the token from the action, not from its own
	// environment: that shape belongs to the image mode only.
	for jobName, step := range doRegistryLoginSteps(wf) {
		if _, ex := step.Env["DIGITALOCEAN_ACCESS_TOKEN"]; ex {
			t.Errorf("job %q: login step should not declare DIGITALOCEAN_ACCESS_TOKEN in the install mode", jobName)
		}
	}
}

// TestGenerateGithubCIDoctlImage verifies that github_ci.doctl: image drops
// the doctl install step from every job and makes the login step take the
// token from its own environment, after printing the version of the doctl the
// runner image provides.
func TestGenerateGithubCIDoctlImage(t *testing.T) {
	tmpDir := generateProject(t, "github-ci-doctl-image")

	wf := parseWorkflow(t, tmpDir, ciCDWorkflow)

	// No doctl may be installed: neither by action-doctl nor by a hand-rolled
	// download. A runner image without doctl has to turn the job red.
	if versions := doctlVersions(wf); len(versions) != 0 {
		t.Errorf("ci_cd.yml has %d digitalocean/action-doctl steps, want none in the image mode", len(versions))
	}

	for jobName, job := range wf.Jobs {
		for _, step := range job.Steps {
			if strings.Contains(step.Name, "Install doctl") || strings.Contains(step.Run, "doctl-") {
				t.Errorf("job %q: step %q installs doctl, want none in the image mode", jobName, step.Name)
			}
		}
	}

	// Every job that logs in to the registry does so in the image mode: the
	// two that push images (the GOAT tests job and the build matrix) plus the
	// checks job, which the config gives a check with registry_login: true.
	// The checks job arrived alongside this mode, so it is the one place where
	// a stale install step can survive unnoticed.
	loginSteps := doRegistryLoginSteps(wf)
	if len(loginSteps) != 3 {
		t.Fatalf("ci_cd.yml has %d %q steps (%v), want 3", len(loginSteps), doRegistryLoginStepName, loginSteps)
	}

	if _, ok := loginSteps["checks"]; !ok {
		t.Errorf("the checks job must log in to the registry for a check with registry_login, got jobs %v", loginSteps)
	}

	for jobName, step := range loginSteps {
		if step.Uses != "" {
			t.Errorf("job %q: login step uses %q, want a plain run step", jobName, step.Uses)
		}

		if got, want := step.Env["DIGITALOCEAN_ACCESS_TOKEN"], "${{ secrets.REGISTRY_PASSWORD }}"; got != want {
			t.Errorf("job %q: login step env DIGITALOCEAN_ACCESS_TOKEN = %q, want %q", jobName, got, want)
		}

		versionIdx := strings.Index(step.Run, "doctl version")
		loginIdx := strings.Index(step.Run, "doctl registry login")

		if versionIdx < 0 || loginIdx < 0 {
			t.Errorf("job %q: login step run = %q, want it to call `doctl version` and `doctl registry login`", jobName, step.Run)

			continue
		}

		if versionIdx > loginIdx {
			t.Errorf("job %q: login step runs `doctl version` after the login, want it first as the loud stop on a missing binary", jobName)
		}
	}
}

// TestGenerateGithubCI verifies that the github_ci parameters reach every job
// of both generated workflows and the version input of both action-doctl
// steps.
func TestGenerateGithubCI(t *testing.T) {
	tmpDir := generateProject(t, "github-ci")

	// Mirrors github-ci/project.yaml: runs_on as a runner group narrowed by
	// labels, a 30 minute job timeout and a pinned doctl version.
	wantRunsOn := map[string]any{
		"group":  "my-runners",
		"labels": []any{"linux", "x64"},
	}

	for _, name := range generatedWorkflowNames {
		wf := parseWorkflow(t, tmpDir, name)

		for jobName, job := range wf.Jobs {
			gotRunsOn, ok := job.RunsOn.(map[string]any)
			if !ok {
				t.Errorf("%s: job %q runs-on = %#v, want a group mapping", name, jobName, job.RunsOn)

				continue
			}

			if gotRunsOn["group"] != wantRunsOn["group"] {
				t.Errorf("%s: job %q runs-on.group = %#v, want %#v", name, jobName, gotRunsOn["group"], wantRunsOn["group"])
			}

			gotLabels, _ := gotRunsOn["labels"].([]any)
			if len(gotLabels) != 2 || gotLabels[0] != "linux" || gotLabels[1] != "x64" {
				t.Errorf("%s: job %q runs-on.labels = %#v, want [linux x64]", name, jobName, gotRunsOn["labels"])
			}

			if job.TimeoutMinutes == nil || *job.TimeoutMinutes != 30 {
				t.Errorf("%s: job %q timeout-minutes = %v, want 30", name, jobName, job.TimeoutMinutes)
			}
		}
	}

	// Mirrors push_branches in github-ci/project.yaml, which carries a glob
	// pattern, an exclusion pattern and a branch name YAML resolves as a
	// float: each needs quoting in the generated YAML for a different reason,
	// so a round trip through the parser proves every filter survived as the
	// string it was configured as.
	assertPushBranches(t, tmpDir, []string{"main", "release/**", "!release/**-alpha", "1.10"})

	assertNotifyFollowsEnabledBuild(t, tmpDir)

	// Both action-doctl steps live in ci_cd.yml and must carry the pinned
	// version.
	versions := doctlVersions(parseWorkflow(t, tmpDir, ciCDWorkflow))
	if len(versions) == 0 {
		t.Fatal("ci_cd.yml has no digitalocean/action-doctl steps, expected the digitalocean registry branch")
	}

	for i, v := range versions {
		if v != "1.170.0" {
			t.Errorf("action-doctl step %d: version = %#v, want \"1.170.0\"", i, v)
		}
	}
}

// branchVarNames runs the `Get branch` step of the generated prepare-env job
// with GITHUB_REF set to ref and returns the variables it exported, keyed by
// name. The step is plain shell without workflow expressions, so executing it
// is what proves the suffix it derives for a branch name GitHub does not admit
// in a variable name (`release/1.2`); reading the YAML only shows the recipe.
func branchVarNames(t *testing.T, targetDir, ref string) map[string]string {
	t.Helper()

	wf := parseWorkflow(t, targetDir, ciCDWorkflow)

	job, ok := wf.Jobs["prepare-env"]
	if !ok {
		t.Fatalf("ci_cd.yml has no prepare-env job, got jobs %v", wf.Jobs)
	}

	var step githubStep

	for _, s := range job.Steps {
		if s.Name == branchVarsStepName {
			step = s

			break
		}
	}

	// A `uses:` step cannot be it: the action used before
	// (noliran/branch-based-secrets) is exactly what pasted the branch name
	// into the variable name unchanged.
	if step.Run == "" || step.Uses != "" {
		t.Fatalf("prepare-env step %q = %#v, want a run step resolving the branch variable names",
			branchVarsStepName, step)
	}

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "get-branch.sh")
	envPath := filepath.Join(dir, "github_env")

	if err := os.WriteFile(scriptPath, []byte(step.Run), 0o600); err != nil {
		t.Fatalf("Error writing the step script: %v", err)
	}

	if err := os.WriteFile(envPath, nil, 0o600); err != nil {
		t.Fatalf("Error creating the GITHUB_ENV file: %v", err)
	}

	cmd := exec.Command("bash", scriptPath)

	cmd.Env = append(os.Environ(), "GITHUB_REF="+ref, "GITHUB_ENV="+envPath)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Error running the %q step for %s: %v\n%s", branchVarsStepName, ref, err, out)
	}

	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Error reading the GITHUB_ENV file: %v", err)
	}

	vars := map[string]string{}

	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if line == "" {
			continue
		}

		name, value, found := strings.Cut(line, "=")
		if !found {
			t.Fatalf("the %q step wrote %q to GITHUB_ENV, want NAME=VALUE", branchVarsStepName, line)
		}

		vars[name] = value
	}

	if len(vars) == 0 {
		t.Fatalf("the %q step exported nothing for %s\n%s", branchVarsStepName, ref, out)
	}

	return vars
}

// TestGenerateGithubCIBranchVarNames verifies that the names of the per-branch
// repository variables and secrets the generated prepare-env job reads are
// legal GitHub names for every branch github_ci.push_branches admits, and that
// main and staging keep the names existing projects already configured.
//
// Without it a branch with `/` or `-` in the name (`release/**`, the pattern
// the key exists for) asked for a variable that cannot exist, so the push
// triggered the workflow, every job was skipped and the run went green with
// nothing built.
func TestGenerateGithubCIBranchVarNames(t *testing.T) {
	tmpDir := generateProject(t, "github-ci")

	tests := []struct {
		ref    string
		suffix string
	}{
		{ref: "refs/heads/main", suffix: "MAIN"},
		{ref: "refs/heads/staging", suffix: "STAGING"},
		{ref: "refs/heads/release/1.2", suffix: "RELEASE_1_2"},
		{ref: "refs/heads/hotfix-x", suffix: "HOTFIX_X"},
		{ref: "refs/heads/feature/ABC-1_x", suffix: "FEATURE_ABC_1_X"},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			vars := branchVarNames(t, tmpDir, tt.ref)

			// ENABLED is the variable that gates every job of ci_cd.yml, so it
			// is the one the whole run hangs on.
			if got, want := vars["ENABLED_NAME"], "ENABLED_"+tt.suffix; got != want {
				t.Errorf("ENABLED_NAME = %q, want %q", got, want)
			}

			for name, value := range vars {
				if !githubVarNameRx.MatchString(value) {
					t.Errorf("%s = %q, which GitHub cannot have as a variable name", name, value)
				}

				if !strings.HasSuffix(value, "_"+tt.suffix) {
					t.Errorf("%s = %q, want the branch suffix %q", name, value, tt.suffix)
				}
			}
		})
	}

	assertEnvFileNameSanitized(t, tmpDir)
}

// assertNotifyFollowsEnabledBuild checks that the notification job reports
// every branch whose build is enabled instead of a hardcoded pair of branch
// names: it is gated on the same ENABLE_BUILD output as the build itself, and
// no step narrows that down to a branch literal. A literal `main`/`staging`
// check would leave a branch added through github_ci.push_branches building
// without any notification.
func assertNotifyFollowsEnabledBuild(t *testing.T, targetDir string) {
	t.Helper()

	wf := parseWorkflow(t, targetDir, ciCDWorkflow)

	job, ok := wf.Jobs[notifyJobName]
	if !ok {
		t.Fatalf("ci_cd.yml has no %s job, got jobs %v", notifyJobName, wf.Jobs)
	}

	if !strings.Contains(job.If, "ENABLE_BUILD") {
		t.Errorf("job %s if = %q, want it gated on the ENABLE_BUILD output", notifyJobName, job.If)
	}

	for _, step := range job.Steps {
		for _, branch := range []string{"main", "staging"} {
			if strings.Contains(step.Run, `"`+branch+`"`) {
				t.Errorf("job %s step %q compares the branch to the literal %q, want it to follow ENABLE_BUILD",
					notifyJobName, step.Name, branch)
			}
		}
	}
}

// assertEnvFileNameSanitized checks that the deploy step builds the env file
// name out of a sanitized branch name. With the raw name a branch containing
// `/` turned `.env.` into a path to a directory that does not exist, and the
// step died on its first redirect.
func assertEnvFileNameSanitized(t *testing.T, targetDir string) {
	t.Helper()

	wf := parseWorkflow(t, targetDir, ciCDWorkflow)

	for jobName, job := range wf.Jobs {
		for _, step := range job.Steps {
			if step.Name != envFileStepName {
				continue
			}

			if !strings.Contains(step.Run, branchSanitizePattern) {
				t.Errorf("job %q step %q builds the env file name without sanitizing the branch (%s): %s",
					jobName, envFileStepName, branchSanitizePattern, step.Run)
			}

			return
		}
	}

	t.Fatalf("ci_cd.yml has no %q step, expected it in the deploy job", envFileStepName)
}

// escapeMarkdownV2 is what the generated step's sed does to a value before
// pasting it into the message.
func escapeMarkdownV2(s string) string {
	var b strings.Builder

	for _, r := range s {
		if strings.ContainsRune(markdownV2Specials, r) {
			b.WriteRune('\\')
		}

		b.WriteRune(r)
	}

	return b.String()
}

// notifyStepScript returns the recipe of the notify step and the environment
// it runs with, both with every workflow expression replaced by the value the
// runner would substitute, so the step can be executed the way a runner
// executes it.
func notifyStepScript(t *testing.T, targetDir, branch, buildResult string) (string, []string) {
	t.Helper()

	jobName, job, step := findStepByName(t, targetDir, notifyStepName)

	if jobName != notifyJobName {
		t.Errorf("the %q step is in job %q, want it in %q", notifyStepName, jobName, notifyJobName)
	}

	if step.Run == "" {
		t.Fatalf("job %s has no recipe in its %q step, got %#v", jobName, notifyStepName, step)
	}

	stubs := runnerStubs(branch, buildResult)
	what := "the " + notifyStepName + " step"

	return substituteWorkflowExprs(t, what, step.Run, stubs), stepShellEnv(t, what, job, step, stubs)
}

// notifyMessage runs the notify step for a branch and returns the message text
// it handed to curl. Executing the step is what proves the message Telegram
// receives: reading the YAML only shows the recipe, and the recipe is where a
// value gets pasted into MarkdownV2 unescaped.
func notifyMessage(t *testing.T, targetDir, branch, buildResult string) string {
	t.Helper()

	script, stepEnv := notifyStepScript(t, targetDir, branch, buildResult)

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "notify.sh")
	argsPath := filepath.Join(dir, "curl-args")
	binDir := filepath.Join(dir, "bin")

	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("Error creating the stub bin directory: %v", err)
	}

	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("Error writing the step script: %v", err)
	}

	// The stub stands in for curl so the step's request is captured instead of
	// sent. The arguments are separated by a marker line rather than by a
	// newline: the message is itself multi-line.
	curlStub := "#!/bin/sh\nfor arg in \"$@\"; do printf '%s\\n" + curlArgSeparator + "\\n' \"$arg\" >> \"$CURL_ARGS_FILE\"; done\n"
	if err := os.WriteFile(filepath.Join(binDir, "curl"), []byte(curlStub), 0o700); err != nil {
		t.Fatalf("Error writing the curl stub: %v", err)
	}

	cmd := exec.Command("bash", scriptPath)

	// The script runs in its own directory so that anything a hostile branch
	// name manages to create lands where the test looks for it.
	cmd.Dir = dir

	// The `env:` of the step and its job carry everything the step itself
	// reads — including the secrets it gives up early without, substituted by
	// runnerStubs — so the test adds only what the runner provides around the
	// step.
	stepEnv = append(stepEnv,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CURL_ARGS_FILE="+argsPath,
		"GITHUB_ENV="+filepath.Join(dir, "github_env"),
	)

	cmd.Env = append(os.Environ(), stepEnv...)

	out, err := cmd.CombinedOutput()

	// Checked before the exit status: a branch name that executed is the worse
	// failure of the two, and it can make the step fail as well.
	assertNoInjectionMarker(t, dir, notifyStepName, branch)

	if err != nil {
		t.Fatalf("Error running the %q step for %s: %v\n%s", notifyStepName, branch, err, out)
	}

	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("the %q step sent no request for %s: %v\n%s", notifyStepName, branch, err, out)
	}

	for _, arg := range strings.Split(string(args), "\n"+curlArgSeparator+"\n") {
		if text, found := strings.CutPrefix(arg, "text="); found {
			return text
		}
	}

	t.Fatalf("the %q step sent no text for %s, got the arguments:\n%s", notifyStepName, branch, args)

	return ""
}

// TestGenerateGithubCINotifyEscapesBranch verifies that the branch name
// reaches the Telegram message escaped for MarkdownV2, the parse mode the
// notify step sends it with.
//
// Without it a branch carrying a reserved character — which `release/1.2`,
// `hotfix-x` and `feature_x` all do, and which only became reachable once the
// notify job stopped being limited to main and staging — made Telegram answer
// 400 and drop the message, invisibly: `curl -s` does not see that error, so
// the job stayed green with no notification sent.
func TestGenerateGithubCINotifyEscapesBranch(t *testing.T) {
	tmpDir := generateProject(t, "github-ci")

	// Both outcomes are checked because the message is composed twice, once
	// per branch of the status check, and each pastes the branch name itself.
	for _, buildResult := range []string{"success", "failure"} {
		for _, branch := range []string{"main", "release/1.2", "hotfix-x", "feature_x", "1.10"} {
			t.Run(buildResult+"/"+branch, func(t *testing.T) {
				message := notifyMessage(t, tmpDir, branch, buildResult)

				match := notifyBranchRx.FindStringSubmatch(message)
				if match == nil {
					t.Fatalf("the message names no branch, got:\n%s", message)
				}

				if got, want := match[1], escapeMarkdownV2(branch); got != want {
					t.Errorf("the message names the branch as %q, want %q (MarkdownV2 escaped)\n%s",
						got, want, message)
				}
			})
		}
	}
}

// assertNoInjectionMarker checks that an executed step left no marker file
// behind, which is what a value reaching the shell as code creates. subject
// is the value under test, named in the failure.
func assertNoInjectionMarker(t *testing.T, dir, stepName, subject string) {
	t.Helper()

	if _, err := os.Stat(filepath.Join(dir, injectionMarkerFile)); err == nil {
		t.Fatalf("the %q step executed %q: it created %s",
			stepName, subject, injectionMarkerFile)
	}
}

// outputValue returns the value the step reported for a $GITHUB_OUTPUT key.
func (r stepRun) outputValue(t *testing.T, stepName, key string) string {
	t.Helper()

	for _, line := range r.output {
		if value, found := strings.CutPrefix(line, key+"="); found {
			return value
		}
	}

	t.Fatalf("the %q step reported no %s, got the output:\n%s\n%s",
		stepName, key, strings.Join(r.output, "\n"), r.combined)

	return ""
}

// runWorkflowStep executes a named step of the generated ci_cd.yml the way a
// runner executes it: every workflow expression of the recipe replaced by the
// value stubs resolves it to, and the `env:` of the step and of its job as the
// environment. subject is the value under test, named in the failures.
//
// The step is executed, not read, because a value is only dangerous once a
// shell sees it, and only an executed step shows whether the value reached the
// script at all. Reading the `env:` blocks is what keeps the executed script
// the one the runner runs: a value moved out of `run:` into `env:` disappears
// from the recipe, so a test that only executed the recipe would run it with
// the variable unset and pass without asserting anything.
func runWorkflowStep(t *testing.T, targetDir, stepName, subject string, stubs exprStubs) stepRun {
	t.Helper()

	jobName, job, step := findStepByName(t, targetDir, stepName)

	if step.Run == "" {
		t.Fatalf("job %s has no recipe in its %q step, got %#v", jobName, stepName, step)
	}

	what := "the " + stepName + " step"
	script := substituteWorkflowExprs(t, what, step.Run, stubs)
	stepEnv := stepShellEnv(t, what, job, step, stubs)

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "step.sh")
	outputPath := filepath.Join(dir, "github_output")

	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("Error writing the step script: %v", err)
	}

	if err := os.WriteFile(outputPath, nil, 0o600); err != nil {
		t.Fatalf("Error creating the GITHUB_OUTPUT file: %v", err)
	}

	// `-e` is what GitHub runs a bash recipe with, so a step that fails
	// mid-script fails here the same way.
	cmd := exec.Command("bash", "-e", scriptPath)

	// The step writes its files into the working directory, which is also
	// where anything a hostile value creates lands.
	cmd.Dir = dir

	stepEnv = append(stepEnv, "GITHUB_OUTPUT="+outputPath)
	cmd.Env = append(os.Environ(), stepEnv...)

	out, err := cmd.CombinedOutput()

	// Checked before the exit status: a value that executed is the worse
	// failure of the two, and it can make the step fail as well.
	assertNoInjectionMarker(t, dir, stepName, subject)

	if err != nil {
		t.Fatalf("Error running the %q step for %q: %v\n%s", stepName, subject, err, out)
	}

	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Error reading the GITHUB_OUTPUT file: %v", err)
	}

	run := stepRun{dir: dir, combined: string(out)}

	if trimmed := strings.TrimSpace(string(output)); trimmed != "" {
		run.output = strings.Split(trimmed, "\n")
	}

	return run
}

// envFileLines runs the `Create env file` step for a branch and the given
// repository values, and returns the name of the env file it wrote — read back
// from the step output the upload step takes the path from — together with its
// lines.
func envFileLines(t *testing.T, targetDir, branch, subject string, repo repoStubValues) (string, []string) {
	t.Helper()

	stubs := runnerStubsFor(branch, "success", repo)
	run := runWorkflowStep(t, targetDir, envFileStepName, subject, stubs)

	fname := run.outputValue(t, envFileStepName, "FNAME")

	content, err := os.ReadFile(filepath.Join(run.dir, fname))
	if err != nil {
		t.Fatalf("the %q step reported %s but wrote no such file: %v\n%s",
			envFileStepName, fname, err, run.combined)
	}

	return fname, strings.Split(strings.TrimRight(string(content), "\n"), "\n")
}

// envFileName runs the `Create env file` step for a branch and returns the
// name of the env file it wrote.
func envFileName(t *testing.T, targetDir, branch string) string {
	t.Helper()

	fname, _ := envFileLines(t, targetDir, branch, branch, ordinaryRepoValues())

	return fname
}

// TestGenerateGithubCIEnvFileBranchNotInjected verifies that the deploy step
// takes the branch name as a value: it names the env file after every branch
// `git check-ref-format` accepts, including the names that read as shell code.
//
// With the name pasted into the recipe through a workflow expression, a branch
// like `x$(touch pwned)` ran a command in the job and one with an unbalanced
// backtick killed the step with a syntax error (GitHub Security hardening →
// script injection).
func TestGenerateGithubCIEnvFileBranchNotInjected(t *testing.T) {
	tmpDir := generateProject(t, "github-ci")

	// The ordinary names come first: the same assertion also says the file
	// name of an existing deploy did not change.
	branches := append([]string{"main", "release/1.2", "hotfix-x", "feature_x"}, hostileBranchNames...)

	for _, branch := range branches {
		t.Run(branch, func(t *testing.T) {
			want := ".env." + sanitizeBranch(branch) + "-" + envTypeStub

			if got := envFileName(t, tmpDir, branch); got != want {
				t.Errorf("the %q step named the env file %q, want %q", envFileStepName, got, want)
			}
		})
	}
}

// envFileWant renders those entries as the lines the step has to write for a
// set of repository values.
func envFileWant(repo repoStubValues) []string {
	lines := make([]string, 0, len(envFileEntries))

	for _, e := range envFileEntries {
		value := e.value(repo)
		if e.quoted {
			value = `"` + value + `"`
		}

		lines = append(lines, e.key+"="+value)
	}

	return lines
}

// TestGenerateGithubCIEnvFileValuesNotInjected verifies that every value the
// `Create env file` step writes reaches the env file whole, including the
// values a shell reads as code.
//
// With the value pasted into the recipe through a workflow expression, a
// single quote in it closed the `'…'` of the `echo` and the rest of the line
// read as code; `ENV_TYPE` was substituted without quotes at all, so a value
// with a space, a `;` or a `$(…)` broke the assignment or ran a command
// (GitHub Security hardening → script injection). The values come from the
// repository's own variables and secrets and from `make release-tag` — git
// allows `'` in a tag name — so there is no privilege escalation here, but a
// broken env file and a red step are the defect either way.
//
// The ordinary values go through the same assertion, which is what says the
// env file of an existing deployment did not change.
func TestGenerateGithubCIEnvFileValuesNotInjected(t *testing.T) {
	tmpDir := generateProject(t, "github-ci")

	for _, name := range slices.Sorted(maps.Keys(repoValueCases())) {
		repo := repoValueCases()[name]

		t.Run(name, func(t *testing.T) {
			_, got := envFileLines(t, tmpDir, "main", name, repo)

			if want := envFileWant(repo); !slices.Equal(got, want) {
				t.Errorf("the %q step wrote the env file\n%s\nwant\n%s",
					envFileStepName, strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

// instanceVarsWant are the lines the `Check and set vars for instances` step
// has to append to $GITHUB_OUTPUT for the github-ci fixture: the whole array
// of every per-environment variable, and the *name* of the two OnlineConf
// secrets — create_env_files indexes the secret by that name, the value itself
// never leaves prepare-env.
func instanceVarsWant(branch string, repo repoStubValues) []string {
	suffix := "_" + strings.ToUpper(sanitizeBranch(branch))

	return []string{
		"PORT_PREFIX_API_TRAEFIK=" + jsonArray(repo.portPrefix),
		"DOMAIN_API=" + jsonArray(repo.domain),
		"PORT_PREFIX_API_API=" + jsonArray(repo.portPrefix),
		"PORT_PREFIX_API_SYS=" + jsonArray(repo.portPrefix),
		"ENV_TYPE=" + jsonArray(repo.envType),
		"OC_USER=OC_USER" + suffix,
		"OC_PASSWORD=OC_PASSWORD" + suffix,
	}
}

// TestGenerateGithubCIInstanceVarsNotInjected verifies the same for the
// prepare-env step that validates those variables and secrets: a JSON value
// holding a quote or a `$(…)` passes through it whole, the step stays green
// and nothing in it runs.
//
// This step breaks one stage earlier than `Create env file` — it is where the
// value of a repository variable, and of the two OnlineConf secrets, was
// pasted into `echo '…' | jq` and into `[ -z "…" ]`.
func TestGenerateGithubCIInstanceVarsNotInjected(t *testing.T) {
	tmpDir := generateProject(t, "github-ci")

	const branch = "main"

	for _, name := range slices.Sorted(maps.Keys(repoValueCases())) {
		repo := repoValueCases()[name]

		t.Run(name, func(t *testing.T) {
			stubs := runnerStubsFor(branch, "success", repo)
			run := runWorkflowStep(t, tmpDir, instanceVarsStepName, name, stubs)

			if want := instanceVarsWant(branch, repo); !slices.Equal(run.output, want) {
				t.Errorf("the %q step wrote the output\n%s\nwant\n%s",
					instanceVarsStepName, strings.Join(run.output, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

// TestGenerateGithubCINotifyBranchNotInjected verifies that the notify step
// takes the branch name as a value: it reports every branch name
// `git check-ref-format` accepts, including the names that read as shell code,
// and runs none of them.
func TestGenerateGithubCINotifyBranchNotInjected(t *testing.T) {
	tmpDir := generateProject(t, "github-ci")

	// Both outcomes are checked because the message is composed twice, once
	// per branch of the status check, and each pastes the branch name itself.
	for _, buildResult := range []string{"success", "failure"} {
		for _, branch := range hostileBranchNames {
			t.Run(buildResult+"/"+branch, func(t *testing.T) {
				message := notifyMessage(t, tmpDir, branch, buildResult)

				match := notifyBranchRx.FindStringSubmatch(message)
				if match == nil {
					t.Fatalf("the message names no branch, got:\n%s", message)
				}

				if got, want := match[1], escapeMarkdownV2(branch); got != want {
					t.Errorf("the message names the branch as %q, want %q (MarkdownV2 escaped)\n%s",
						got, want, message)
				}
			})
		}
	}
}

// TestGenerateGithubCINoInjectableRunExpr verifies that no `run:` recipe of
// the generated workflows substitutes a workflow context whose value is chosen
// outside the template. Such a value has to travel through `env:`, where the
// shell reads it as data; in a recipe it is source code (GitHub Security
// hardening → script injection).
//
// The test is the one that catches the pattern coming back: every future step
// that pastes a branch name, a repository variable, a secret or a job output
// into a script fails it, whether or not the test suite executes that step.
//
// It scans generated projects rather than the templates, because one source of
// `secrets.` appears in no template at all: `documentation.headers` goes
// through the ToGitHubSecret helper, which turns `${NAME}` into a reference to
// that secret while rendering.
func TestGenerateGithubCINoInjectableRunExpr(t *testing.T) {
	seen := map[string]bool{}

	for _, config := range injectionScanConfigs {
		tmpDir := config.generate(t)

		for _, name := range generatedWorkflowNames {
			wf := parseWorkflow(t, tmpDir, name)

			for _, jobName := range slices.Sorted(maps.Keys(wf.Jobs)) {
				for _, step := range wf.Jobs[jobName].Steps {
					if step.Run == "" {
						continue
					}

					seen[step.Name] = true

					for _, match := range workflowExprRx.FindAllStringSubmatch(step.Run, -1) {
						if ctx := injectableRunExprRx.FindString(match[1]); ctx != "" {
							t.Errorf("%s (%s): job %s step %q runs a script with ${{ %s }} in it (%s): pass the value through env: instead",
								name, config.name, jobName, step.Name, match[1], ctx)
						}
					}
				}
			}
		}
	}

	// The scan is only worth its claim if the configs above really render
	// every recipe of the templates: a `run:` step added under a condition no
	// config sets renders nowhere, and a scan that never saw it stays green.
	for _, stepName := range runStepNames {
		if !seen[stepName] {
			t.Errorf("no scanned config renders the %q step: add a config that does, "+
				"otherwise its recipe is never checked", stepName)
		}
	}

	for stepName := range seen {
		if !slices.Contains(runStepNames, stepName) {
			t.Errorf("the %q step carries a recipe and is not listed in runStepNames: "+
				"add it, so a config that stops rendering it is noticed here", stepName)
		}
	}
}

// TestGenerateGoVersionCheckPrivateRepos verifies that the Go version policy
// workflow gets private-module access when git.private_repos is declared.
// Its script downloads the direct dependencies (`go list -m all`, `go mod
// download`), so without those steps the job fails on the first private
// dependency and the check is red on every push.
func TestGenerateGoVersionCheckPrivateRepos(t *testing.T) {
	// private-token-secret declares git.private_repos plus a non-default
	// private_repos_token_secret, so the steps must also read the configured
	// secret name rather than the GH_PAT default.
	tmpDir := generateProject(t, "private-token-secret")

	wf := parseWorkflow(t, tmpDir, goVersionCheckWorkflow)

	const jobName = "check-go-mod-version"

	job, ok := wf.Jobs[jobName]
	if !ok {
		t.Fatalf("%s has no %q job, got %v", goVersionCheckWorkflow, jobName, wf.Jobs)
	}

	steps := stepsByName(job)

	for _, name := range privateReposStepNames {
		if _, ok := steps[name]; !ok {
			t.Errorf("job %q has no %q step, got steps %v", jobName, name, job.Steps)
		}
	}

	if got, want := steps[privateReposStepNames[0]].Env["GH_TOKEN"], "${{ secrets.GO_MODULES_TOKEN }}"; got != want {
		t.Errorf("job %q: git config step env GH_TOKEN = %q, want %q", jobName, got, want)
	}

	// Mirrors private-token-secret/project.yaml: the declared glob has to reach
	// GOPRIVATE, otherwise the proxy is still asked for the private modules.
	if run := steps[privateReposStepNames[1]].Run; !strings.Contains(run, "github.com/myorg/*") {
		t.Errorf("job %q: GOPRIVATE step run = %q, want it to carry the declared private repos", jobName, run)
	}

	// Both steps have to precede the check itself: configuring access after the
	// download would not help it.
	checkIdx := -1
	lastSetupIdx := -1

	for i, step := range job.Steps {
		switch {
		case strings.HasPrefix(step.Name, "Check go.mod version"):
			checkIdx = i
		case step.Name == privateReposStepNames[0], step.Name == privateReposStepNames[1]:
			lastSetupIdx = i
		}
	}

	if checkIdx < 0 {
		t.Fatalf("job %q has no go.mod version check step, got steps %v", jobName, job.Steps)
	}

	if lastSetupIdx > checkIdx {
		t.Errorf("job %q configures private-module access after the check (step %d vs %d)", jobName, lastSetupIdx, checkIdx)
	}
}

// TestGenerateGoVersionCheckWithoutPrivateRepos verifies that a project
// without git.private_repos keeps the previous workflow: no token is read and
// no GOPRIVATE is set where there is nothing private to fetch.
func TestGenerateGoVersionCheckWithoutPrivateRepos(t *testing.T) {
	tmpDir := generateProject(t, "github-ci-defaults")

	wf := parseWorkflow(t, tmpDir, goVersionCheckWorkflow)

	for jobName, job := range wf.Jobs {
		for _, name := range privateReposStepNames {
			if _, ok := stepsByName(job)[name]; ok {
				t.Errorf("job %q has a %q step without git.private_repos", jobName, name)
			}
		}
	}
}

// TestGeneratePrivateModulesAccessPerJob pins which jobs of ci_cd.yml get
// private-module access on the runner, for a project that declares private
// modules.
//
// The block is rendered from one shared fragment, so the question it used to
// answer per copy — does this job need it — is now answered once per call
// site, and this is where that list lives. Every job that runs Go on the
// runner has both steps; `build` has neither, because it runs no Go there: it
// calls `make docker-<app>`, and the modules are downloaded inside the image
// from the BuildKit secret, which the Build-and-push step passes on its own.
//
// Whether the block is rendered at all is the second axis, and
// TestGeneratePrivateModulesAccessWithoutPrivateRepos covers it: the steps
// used to be unconditional here because GOPRIVATE always covered
// github.com/Educentr.
func TestGeneratePrivateModulesAccessPerJob(t *testing.T) {
	// private-token-secret builds a container and a deb package, declares a CI
	// check and runs goat tests, so every job that can carry the block is
	// rendered, and it configures a non-default secret name, so a job reading
	// the wrong one is visible here too.
	tmpDir := generateProject(t, "private-token-secret")

	wf := parseWorkflow(t, tmpDir, ciCDWorkflow)

	// jobs that fetch modules on the runner.
	wantAccess := []string{"build-packages", "checks", "run-goat-tests-api"}
	// jobs that do not, listed explicitly so that a new job cannot join the
	// workflow without a decision being made here.
	wantNoAccess := []string{"build", "deploy", "prepare-env"}

	for _, jobName := range append(append([]string{}, wantAccess...), wantNoAccess...) {
		if _, ok := wf.Jobs[jobName]; !ok {
			t.Fatalf("%s has no %q job, got %v", ciCDWorkflow, jobName, wf.Jobs)
		}
	}

	for jobName, job := range wf.Jobs {
		steps := stepsByName(job)
		want := slices.Contains(wantAccess, jobName)

		for _, name := range privateReposStepNames {
			if _, ok := steps[name]; ok != want {
				t.Errorf("job %q: has %q step = %v, want %v", jobName, name, ok, want)
			}
		}

		if !want {
			continue
		}

		if got, w := steps[privateReposStepNames[0]].Env["GH_TOKEN"], "${{ secrets.GO_MODULES_TOKEN }}"; got != w {
			t.Errorf("job %q: git config step env GH_TOKEN = %q, want %q", jobName, got, w)
		}

		if run := steps[privateReposStepNames[1]].Run; !strings.Contains(run, "github.com/myorg/*") {
			t.Errorf("job %q: GOPRIVATE step run = %q, want it to carry the declared private repos", jobName, run)
		}
	}

	// The image still gets the token: the build step hands it to `docker build
	// --secret`, which is what the removal of the runner-side step relies on.
	buildSteps := stepsByName(wf.Jobs["build"])

	step, ok := buildSteps["Build and push"]
	if !ok {
		t.Fatalf("job %q has no %q step, got steps %v", "build", "Build and push", wf.Jobs["build"].Steps)
	}

	if got, w := step.Env["GITHUB_TOKEN"], "${{ secrets.GO_MODULES_TOKEN }}"; got != w {
		t.Errorf("job \"build\": Build and push env GITHUB_TOKEN = %q, want %q", got, w)
	}
}

// TestGeneratePrivateModulesAccessWithoutPrivateRepos checks that a project
// without git.private_repos gets neither step, in either workflow.
//
// In ci_cd.yml they used to be unconditional, justified by GOPRIVATE always
// covering github.com/Educentr: the credential rewrite was installed in every
// generated project, so a project that depended on no private module still
// needed a token secret for its own CI to fetch the public Educentr modules
// through that rewrite. With nothing taken off the module proxy there is no
// such fetch and no such secret.
func TestGeneratePrivateModulesAccessWithoutPrivateRepos(t *testing.T) {
	tmpDir := generateProject(t, "github-ci-defaults")

	for _, relPath := range generatedWorkflowNames {
		wf := parseWorkflow(t, tmpDir, relPath)

		for jobName, job := range wf.Jobs {
			for _, name := range privateReposStepNames {
				if _, ok := stepsByName(job)[name]; ok {
					t.Errorf("%s: job %q has a %q step without git.private_repos", relPath, jobName, name)
				}
			}
		}
	}
}
