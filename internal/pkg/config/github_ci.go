package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/pkg/errors"
)

// GithubCIConfig contains the settings of the generated GitHub Actions
// workflows (.github/workflows/ci_cd.yml and go-version-check.yml): the runner,
// the job timeout and the doctl version applied to every job, the branches
// whose push triggers them, plus the list of checks that gate build. All
// fields are optional; omitting the whole section keeps the historical output
// (ubuntu-latest, no job timeout, latest doctl, push on main and staging, no
// check job).
//
// YAML example:
//
//	github_ci:
//	  runs_on:                    # label, list of labels or {group, labels}
//	    group: my-runners
//	    labels: [linux]
//	  job_timeout_minutes: 30     # timeout-minutes for every job
//	  doctl_version: 1.170.0      # version input of digitalocean/action-doctl
//	  doctl: install              # install (default) or image
//	  push_branches:              # on.push branch filters; default [main, staging]
//	    - main
//	    - 'release/**'
//	  checks:                     # matrix job gating build and build-packages
//	    - name: lint
//	      run: make lint
//	    - name: unit-tests
//	      run: make unit-tests
//	  checks_config:
//	    fail_fast: false
//
// See docs/configuration/github-ci.md for full documentation.
type GithubCIConfig struct {
	// RunsOn selects the runner for all generated jobs. Three forms are
	// accepted: a single label ("self-hosted"), a list of labels
	// ([self-hosted, linux]) and a runner group object
	// ({group: my-runners, labels: [linux]}). Omitted means ubuntu-latest.
	RunsOn any `mapstructure:"runs_on"`
	// JobTimeoutMinutes sets timeout-minutes on every generated job and must
	// be a positive integer. Omitted means the key is not emitted, so GitHub's
	// 6-hour job limit applies.
	JobTimeoutMinutes *int `mapstructure:"job_timeout_minutes"`
	// DoctlVersion pins the doctl version installed by
	// digitalocean/action-doctl in the DigitalOcean registry login steps.
	// Omitted means the action installs the latest release. Only meaningful
	// together with Doctl == "install".
	DoctlVersion string `mapstructure:"doctl_version"`
	// Doctl selects how doctl becomes available in the DigitalOcean registry
	// login jobs: "install" installs it with digitalocean/action-doctl,
	// "image" takes it from the runner image and generates no install step.
	// Omitted means "install", the historical output.
	Doctl string `mapstructure:"doctl"`
	// PushBranches replaces the branch filters of the `on.push` trigger of
	// both generated workflows. Omitted means [main, staging], the historical
	// output; an explicit empty list is rejected.
	PushBranches []string `mapstructure:"push_branches"`
	// Checks declares the checks run by the `checks` matrix job, which gates
	// build and build-packages. Names must be non-empty and unique. Omitted
	// means no check job is generated.
	Checks CheckList `mapstructure:"checks"`
	// ChecksConfig holds the settings shared by every entry of Checks.
	ChecksConfig ChecksConfig `mapstructure:"checks_config"`
	// BuildEnv declares the environment variables added to the `env:` of the
	// "Build and push" step of the build job, next to the two the template
	// sets itself. Omitted means the step keeps exactly those two, the
	// historical output.
	BuildEnv []GithubCIEnvEntry `mapstructure:"build_env"`
}

// GithubCIEnvEntry is one environment variable of a generated workflow step,
// declared by github_ci.build_env.
//
// A list of {name, value} and not a map, because viper lowercases config keys:
// `IMAGE_REGISTRY` as a map key would silently arrive as `image_registry` and
// the build would never see the variable it reads. The same reason the runtime
// stage takes its variables as a list (AppDockerEnvEntry).
type GithubCIEnvEntry struct {
	// Name is the variable name. Required.
	Name string `mapstructure:"name"`
	// Value is the value the step gets — a workflow expression
	// (`${{ vars.IMAGE_REGISTRY }}`) or a literal, emitted verbatim.
	// Required: a variable without a value is nothing the step can use.
	Value string `mapstructure:"value"`
}

// registryTypeDigitalOcean is the main.registry_type value whose login jobs
// use doctl, so it is the only one the doctl parameters apply to.
const registryTypeDigitalOcean = "digitalocean"

// Formats for wrapping the sentinel errors below with the offending value.
const (
	errFmtGotType  = "%w, got %T"
	errFmtGotValue = "%w: %q"
	// errFmtGotValueNote wraps the sentinel with the offending value and a
	// note that explains a collision the names alone do not show (sameEnvKeyNote).
	errFmtGotValueNote = "%w: %q%s"
)

// Sentinel errors of the github_ci section. Use errors.Is to distinguish.
var (
	ErrRunsOnType       = errors.New("github_ci.runs_on: expected a label, a list of labels or a {group, labels} object")
	ErrRunsOnEmptyList  = errors.New("github_ci.runs_on: label list is empty")
	ErrRunsOnLabelType  = errors.New("github_ci.runs_on: label must be a string")
	ErrRunsOnLabel      = errors.New("github_ci.runs_on: invalid runner label")
	ErrRunsOnUnknownKey = errors.New("github_ci.runs_on: unknown key, expected 'group' or 'labels'")
	ErrRunsOnGroup      = errors.New("github_ci.runs_on: 'group' is required and must be a string")
	ErrRunsOnGroupName  = errors.New("github_ci.runs_on: invalid runner group")
	ErrRunsOnLabelsType = errors.New("github_ci.runs_on: 'labels' must be a list")
	ErrRunsOnNoLabels   = errors.New("github_ci.runs_on: 'labels' is empty, omit the key instead")

	ErrPushBranchesEmpty = errors.New(
		"github_ci.push_branches: list is empty, omit the key instead to keep the default branches")
	ErrPushBranch = errors.New(
		"github_ci.push_branches: invalid branch filter, expected a branch name or a GitHub filter pattern")
	ErrPushBranchDuplicate = errors.New("github_ci.push_branches: duplicate branch filter")

	ErrBuildEnvName = errors.New(
		"github_ci.build_env: name must start with a letter or underscore and contain only letters, digits and underscores")
	ErrBuildEnvValue     = errors.New("github_ci.build_env: value must be a non-empty single-line string")
	ErrBuildEnvDuplicate = errors.New("github_ci.build_env: duplicate name")
	ErrBuildEnvReserved  = errors.New(
		"github_ci.build_env: name is already set by the generated \"Build and push\" step")
	// ErrBuildEnvDefaultVar reports a name GitHub reserves for its own default
	// environment variables. An assignment to one of those is ignored by the
	// runner, so the variable would never reach the build — a silent no-op,
	// which is what the rest of the key's validation rules out too. The
	// workflow context (`${{ github.sha }}`) is the way to read such a value;
	// it can be handed to a variable of any other name.
	ErrBuildEnvDefaultVar = errors.New(
		"github_ci.build_env: GitHub reserves the GITHUB_ and RUNNER_ prefixes for its default " +
			"environment variables and ignores an assignment to them; rename the variable and pass " +
			"the value through a workflow context expression")
	// ErrBuildEnvNoDocker reports build_env in a project that builds no docker
	// image. The variables reach the project through the "Build and push" step
	// of the build job, which ci_cd.yml generates for the docker artifact
	// only, so there would be nothing at all to render them into — the silent
	// no-op ErrAppDockerNoImageAddition rules out for the image additions.
	ErrBuildEnvNoDocker = errors.New(
		"github_ci.build_env: declared in a project that builds no docker artifact, " +
			"the variables are set on the \"Build and push\" step of the build job")
)

// runsOnTokenRx matches the runner labels and runner group names accepted in
// github_ci.runs_on. The charset is restricted to what is safe to emit as a
// plain YAML scalar inside the flow-style `runs-on:` value the templates
// produce: alphanumerics, `.`, `_`, `/`, `-`, and single inner spaces.
var runsOnTokenRx = regexp.MustCompile(`^[A-Za-z0-9._/-]+( [A-Za-z0-9._/-]+)*$`)

// pushBranchRx matches the accepted github_ci.push_branches entries: a branch
// name or a GitHub branch filter pattern. Besides the characters a git branch
// name may carry, it admits the filter metacharacters `*`, `?`, `+`, the
// character range brackets and the leading `!` of an exclusion pattern.
// Whitespace, control characters and YAML-hostile punctuation (`:`, `#`,
// quotes) are rejected here rather than quoted away, because no GitHub filter
// needs them and a branch name cannot contain them either.
var pushBranchRx = regexp.MustCompile(`^[A-Za-z0-9._/*?+!\[\]-]+$`)

// buildEnvNameRx matches the accepted github_ci.build_env names. The shape is
// that of a docker build argument (dockerArgNamePattern) on purpose: the
// documented use of the key is to hand a repository `vars` value to a
// docker.build_args argument of the same name, which the Makefile picks up
// through `?=` — a name only one of the two accepts would break that chain.
var buildEnvNameRx = regexp.MustCompile(`^` + dockerArgNamePattern + `$`)

// buildEnvUnsafeValueRx matches what a build_env value may not carry: a line
// break, which would turn the rest of the value into workflow YAML of its own,
// and the other control characters, which a YAML scalar cannot hold as is.
// Everything else — `:`, `#`, quotes — is kept and quoted by the template
// (YAMLQuote), because a workflow expression legitimately contains them
// (`format('{0}/{1}', …)`).
var buildEnvUnsafeValueRx = regexp.MustCompile(`[\x00-\x1f\x7f]`)

// buildEnvReservedNames lists the variables the "Build and push" step of
// main/.github/workflows/ci_cd.yml.tmpl sets itself, keyed by buildEnvKey.
// Taking one of them would emit the same key twice in the step's `env:`; the
// later one wins, so the project would silently lose the value the generator
// put there.
var buildEnvReservedNames = map[string]struct{}{ //nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards
	"REGISTRY_SERVER": {},
	"GITHUB_TOKEN":    {},
}

// buildEnvDefaultVarPrefixes lists the prefixes GitHub reserves for its own
// default environment variables (GITHUB_SHA, RUNNER_OS, …). Compared against
// buildEnvKey, so the case a name is written in does not matter — neither does
// it to the runner.
var buildEnvDefaultVarPrefixes = []string{"GITHUB_", "RUNNER_"} //nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards

// buildEnvKey normalizes a build_env name into the key GitHub Actions compares
// it by. The keys of an `env:` mapping are case-insensitive, so `github_token`
// and `GITHUB_TOKEN` are one key: a name that differs only in case from a
// reserved one — or from another declared one — emits the same key twice and
// the workflow stops being valid ("key is duplicated in env section ... note
// that this key is case insensitive", actionlint).
func buildEnvKey(name string) string {
	return strings.ToUpper(name)
}

// buildEnvDefaultVarPrefix reports whether the normalized name falls into the
// space GitHub keeps for its default environment variables, and with which
// prefix. Checked by prefix rather than against the list of the default
// variables themselves: that list grows on GitHub's side, and a name matching
// a future default would silently become the no-op this rules out.
func buildEnvDefaultVarPrefix(key string) (string, bool) {
	for _, prefix := range buildEnvDefaultVarPrefixes {
		if strings.HasPrefix(key, prefix) {
			return prefix, true
		}
	}

	return "", false
}

// sameEnvKeyNote explains a collision the two names do not show on their own:
// they are one `env:` key because the comparison ignores case. Empty when the
// names are identical, where the collision needs no explaining.
func sameEnvKeyNote(name, collidesWith string) string {
	if name == collidesWith {
		return ""
	}

	return fmt.Sprintf(" (the same env key as %q, GitHub Actions compares env keys case-insensitively)", collidesWith)
}

// doctlVersionRx matches the accepted github_ci.doctl_version values, e.g.
// "1.170.0". Restricted to a version-like token so the generated `version:`
// input cannot break the workflow YAML.
var doctlVersionRx = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ParseRunsOn normalizes the github_ci.runs_on value into ds.RunsOnSpec.
// A nil value yields the zero spec, which the templates render as
// ds.DefaultRunsOn.
func (g GithubCIConfig) ParseRunsOn() (ds.RunsOnSpec, error) {
	switch v := g.RunsOn.(type) {
	case nil:
		return ds.RunsOnSpec{}, nil
	case string:
		labels, err := parseRunsOnLabels([]any{v})
		if err != nil {
			return ds.RunsOnSpec{}, err
		}

		return ds.RunsOnSpec{Labels: labels}, nil
	case []any:
		if len(v) == 0 {
			return ds.RunsOnSpec{}, ErrRunsOnEmptyList
		}

		labels, err := parseRunsOnLabels(v)
		if err != nil {
			return ds.RunsOnSpec{}, err
		}

		return ds.RunsOnSpec{Labels: labels}, nil
	case map[string]any:
		return parseRunsOnGroup(v)
	default:
		return ds.RunsOnSpec{}, fmt.Errorf(errFmtGotType, ErrRunsOnType, v)
	}
}

// parseRunsOnLabels validates a runner label list.
func parseRunsOnLabels(raw []any) ([]string, error) {
	labels := make([]string, 0, len(raw))

	for _, item := range raw {
		label, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf(errFmtGotType, ErrRunsOnLabelType, item)
		}

		if !runsOnTokenRx.MatchString(label) {
			return nil, fmt.Errorf(errFmtGotValue, ErrRunsOnLabel, label)
		}

		labels = append(labels, label)
	}

	return labels, nil
}

// parseRunsOnGroup validates the object form of runs_on.
func parseRunsOnGroup(raw map[string]any) (ds.RunsOnSpec, error) {
	for key := range raw {
		if key != "group" && key != "labels" {
			return ds.RunsOnSpec{}, fmt.Errorf(errFmtGotValue, ErrRunsOnUnknownKey, key)
		}
	}

	group, ok := raw["group"].(string)
	if !ok {
		return ds.RunsOnSpec{}, ErrRunsOnGroup
	}

	if !runsOnTokenRx.MatchString(group) {
		return ds.RunsOnSpec{}, fmt.Errorf(errFmtGotValue, ErrRunsOnGroupName, group)
	}

	spec := ds.RunsOnSpec{Group: group}

	if rawLabels, ex := raw["labels"]; ex {
		list, isList := rawLabels.([]any)
		if !isList {
			return ds.RunsOnSpec{}, fmt.Errorf(errFmtGotType, ErrRunsOnLabelsType, rawLabels)
		}

		if len(list) == 0 {
			return ds.RunsOnSpec{}, ErrRunsOnNoLabels
		}

		labels, err := parseRunsOnLabels(list)
		if err != nil {
			return ds.RunsOnSpec{}, err
		}

		spec.Labels = labels
	}

	return spec, nil
}

// validatePushBranches verifies the github_ci.push_branches entries. A nil
// slice is valid and means the generated workflows keep the default branches
// (main and staging). An explicit empty list is not: the `on.push` trigger
// cannot be rendered without branch filters, and omitting the key is the way
// to ask for the defaults.
func (g GithubCIConfig) validatePushBranches() error {
	if g.PushBranches == nil {
		return nil
	}

	if len(g.PushBranches) == 0 {
		return ErrPushBranchesEmpty
	}

	seen := make(map[string]bool, len(g.PushBranches))

	for _, branch := range g.PushBranches {
		if !pushBranchRx.MatchString(branch) {
			return fmt.Errorf(errFmtGotValue, ErrPushBranch, branch)
		}

		if seen[branch] {
			return fmt.Errorf(errFmtGotValue, ErrPushBranchDuplicate, branch)
		}

		seen[branch] = true
	}

	return nil
}

// ParseBuildEnv validates github_ci.build_env and returns the variables in the
// order they were declared, which is the order the step's `env:` gets them. A
// nil result means the key was omitted and the step keeps the two variables
// the template sets itself.
func (g GithubCIConfig) ParseBuildEnv() ([]ds.GithubCIEnvVar, error) {
	if len(g.BuildEnv) == 0 {
		return nil, nil
	}

	out := make([]ds.GithubCIEnvVar, 0, len(g.BuildEnv))
	// Keyed by buildEnvKey and holding the name as it was declared, so a
	// collision that only differs in case can name both sides.
	seen := make(map[string]string, len(g.BuildEnv))

	for _, entry := range g.BuildEnv {
		if !buildEnvNameRx.MatchString(entry.Name) {
			return nil, fmt.Errorf(errFmtGotValue, ErrBuildEnvName, entry.Name)
		}

		key := buildEnvKey(entry.Name)

		if _, reserved := buildEnvReservedNames[key]; reserved {
			return nil, fmt.Errorf(errFmtGotValueNote, ErrBuildEnvReserved, entry.Name, sameEnvKeyNote(entry.Name, key))
		}

		if prefix, isDefault := buildEnvDefaultVarPrefix(key); isDefault {
			return nil, fmt.Errorf("%w: %q starts with %s", ErrBuildEnvDefaultVar, entry.Name, prefix)
		}

		if entry.Value == "" || buildEnvUnsafeValueRx.MatchString(entry.Value) {
			return nil, fmt.Errorf(errFmtNamedValue, ErrBuildEnvValue, entry.Name, entry.Value)
		}

		if first, dup := seen[key]; dup {
			return nil, fmt.Errorf(errFmtGotValueNote, ErrBuildEnvDuplicate, entry.Name, sameEnvKeyNote(entry.Name, first))
		}

		seen[key] = entry.Name

		out = append(out, ds.GithubCIEnvVar{Name: entry.Name, Value: entry.Value})
	}

	return out, nil
}

// JobTimeout returns the configured job timeout in minutes, or 0 when the
// timeout is not configured.
func (g GithubCIConfig) JobTimeout() int {
	if g.JobTimeoutMinutes == nil {
		return 0
	}

	return *g.JobTimeoutMinutes
}

// DoctlMode returns the normalized github_ci.doctl value. An omitted value
// yields ds.DoctlModeInstall, the mode that keeps the historical output. An
// unknown value is returned as is and rejected by IsValid.
func (g GithubCIConfig) DoctlMode() ds.DoctlMode {
	if g.Doctl == "" {
		return ds.DoctlModeInstall
	}

	return ds.DoctlMode(g.Doctl)
}

// IsValid validates GithubCIConfig. registryType is main.registry_type: the
// doctl parameters only reach the generated workflow through the DigitalOcean
// registry login jobs, so they are validated against it.
func (g GithubCIConfig) IsValid(registryType string) (bool, string) {
	if _, err := g.ParseRunsOn(); err != nil {
		return false, err.Error()
	}

	if g.JobTimeoutMinutes != nil && *g.JobTimeoutMinutes <= 0 {
		return false, fmt.Sprintf("github_ci.job_timeout_minutes must be greater than 0, got %d", *g.JobTimeoutMinutes)
	}

	if err := g.validatePushBranches(); err != nil {
		return false, err.Error()
	}

	if g.DoctlVersion != "" && !doctlVersionRx.MatchString(g.DoctlVersion) {
		return false, "github_ci.doctl_version must be a version string, got: " + g.DoctlVersion
	}

	if ok, msg := g.validateChecks(); !ok {
		return false, msg
	}

	if _, err := g.ParseBuildEnv(); err != nil {
		return false, err.Error()
	}

	return g.validateDoctlMode(registryType)
}

// validateChecks verifies the declared checks. Names become workflow matrix leg
// names, so they must be present and unique; a check without a command would
// generate a job step that does nothing.
func (g GithubCIConfig) validateChecks() (bool, string) {
	seen := make(map[string]bool, len(g.Checks))

	for _, check := range g.Checks {
		if ok, msg := check.IsValid(); !ok {
			return false, msg
		}

		if seen[check.Name] {
			return false, "github_ci.checks: duplicate check name: " + check.Name
		}

		seen[check.Name] = true
	}

	return true, ""
}

// validateDoctlMode checks github_ci.doctl against the known modes, the
// parameters it makes redundant and the registry type it applies to. An
// unusable combination is an error rather than a silently ignored key: the
// generated workflow would otherwise claim a doctl setup the project did not
// ask for.
func (g GithubCIConfig) validateDoctlMode(registryType string) (bool, string) {
	switch g.DoctlMode() {
	case ds.DoctlModeInstall:
		// The default mode emits the install step for the DigitalOcean
		// registry and nothing at all for the other registry types, so there
		// is nothing left to contradict.
		return true, ""
	case ds.DoctlModeImage:
		// Checked by the rules below.
	default:
		return false, fmt.Sprintf("github_ci.doctl must be %q or %q, got: %s",
			ds.DoctlModeInstall, ds.DoctlModeImage, g.Doctl)
	}

	if g.DoctlVersion != "" {
		return false, fmt.Sprintf(
			"github_ci.doctl: %q takes doctl from the runner image, so github_ci.doctl_version (%s) cannot be applied; drop one of the two",
			ds.DoctlModeImage, g.DoctlVersion)
	}

	if registryType != registryTypeDigitalOcean {
		return false, fmt.Sprintf("github_ci.doctl: %q only applies to main.registry_type: %s, got: %s",
			ds.DoctlModeImage, registryTypeDigitalOcean, registryType)
	}

	return true, ""
}
