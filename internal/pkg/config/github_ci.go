package config

import (
	"fmt"
	"regexp"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/pkg/errors"
)

// GithubCIConfig contains the settings of the generated GitHub Actions
// workflows (.github/workflows/ci_cd.yml and go-version-check.yml): the runner,
// the job timeout and the doctl version applied to every job, plus the list of
// checks that gate build. All fields are optional; omitting the whole section
// keeps the historical output (ubuntu-latest, no job timeout, latest doctl, no
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
	// Omitted means the action installs the latest release.
	DoctlVersion string `mapstructure:"doctl_version"`
	// Checks declares the checks run by the `checks` matrix job, which gates
	// build and build-packages. Names must be non-empty and unique. Omitted
	// means no check job is generated.
	Checks CheckList `mapstructure:"checks"`
	// ChecksConfig holds the settings shared by every entry of Checks.
	ChecksConfig ChecksConfig `mapstructure:"checks_config"`
}

// Formats for wrapping the sentinel errors below with the offending value.
const (
	errFmtGotType  = "%w, got %T"
	errFmtGotValue = "%w: %q"
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
)

// runsOnTokenRx matches the runner labels and runner group names accepted in
// github_ci.runs_on. The charset is restricted to what is safe to emit as a
// plain YAML scalar inside the flow-style `runs-on:` value the templates
// produce: alphanumerics, `.`, `_`, `/`, `-`, and single inner spaces.
var runsOnTokenRx = regexp.MustCompile(`^[A-Za-z0-9._/-]+( [A-Za-z0-9._/-]+)*$`)

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

// JobTimeout returns the configured job timeout in minutes, or 0 when the
// timeout is not configured.
func (g GithubCIConfig) JobTimeout() int {
	if g.JobTimeoutMinutes == nil {
		return 0
	}

	return *g.JobTimeoutMinutes
}

// IsValid validates GithubCIConfig
func (g GithubCIConfig) IsValid() (bool, string) {
	if _, err := g.ParseRunsOn(); err != nil {
		return false, err.Error()
	}

	if g.JobTimeoutMinutes != nil && *g.JobTimeoutMinutes <= 0 {
		return false, fmt.Sprintf("github_ci.job_timeout_minutes must be greater than 0, got %d", *g.JobTimeoutMinutes)
	}

	if g.DoctlVersion != "" && !doctlVersionRx.MatchString(g.DoctlVersion) {
		return false, "github_ci.doctl_version must be a version string, got: " + g.DoctlVersion
	}

	if ok, msg := g.validateChecks(); !ok {
		return false, msg
	}

	return true, ""
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
