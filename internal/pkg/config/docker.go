package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/pkg/errors"
)

// Docker contains the settings of the built images: the registry prefix, the
// build arguments the images accept and the labels of the final image.
//
// A build argument is what lets the image carry its own identity: LABEL is
// static and cannot run git, so the version and the revision have to arrive
// through `--build-arg`, and the stage that uses them has to declare `ARG`.
// Both are generated from this section, the Makefile recipe passes the
// arguments, and the Makefile defaults keep a local build working without
// passing anything by hand.
//
// YAML example:
//
//	docker:
//	  image_prefix: registry.example/org
//	  build_args:
//	    - IMAGE_VERSION                            # short form: name only
//	    - name: IMAGE_REVISION                     # full form
//	      default: $(shell git rev-parse HEAD)     # Makefile default
//	      stages: [final]                          # default; [builder, final] also valid
//	  labels:
//	    org.opencontainers.image.version: ${IMAGE_VERSION}
//	    org.opencontainers.image.revision: ${IMAGE_REVISION}
//
// See docs/configuration/docker.md for full documentation.
type Docker struct {
	// ImagePrefix is the registry prefix of the built images. Defaults to
	// "educentr" (see GetConfig).
	ImagePrefix string `mapstructure:"image_prefix"`
	// BuildArgs declares the build arguments of the images. Each entry is
	// either a name ("IMAGE_VERSION") or an object with `name`, optional
	// `default` and optional `stages`. Read via Parse.
	BuildArgs []any `mapstructure:"build_args"`
	// Labels declares the labels of the final image. Keys may contain dots
	// (`org.opencontainers.image.version`), which viper turns into nested maps,
	// hence `any` values — Parse flattens them back. Read via Parse.
	Labels map[string]any `mapstructure:"labels"`
}

// dockerArgNamePattern is the shape of a build argument name: the intersection
// of what a Dockerfile `ARG` and a Makefile variable accept. Shared by the name
// check and the reference scan of the label values.
const dockerArgNamePattern = `[A-Za-z_][A-Za-z0-9_]*`

// Sentinel errors of the docker section. Use errors.Is to distinguish.
var (
	ErrDockerBuildArgType = errors.New("docker.build_args: expected a name or a {name, default, stages} object")
	ErrDockerBuildArgName = errors.New(
		"docker.build_args: name must start with a letter or underscore and contain only letters, digits and underscores",
	)
	ErrDockerBuildArgReserved   = errors.New("docker.build_args: name is already used by the generated Dockerfile or Makefile")
	ErrDockerBuildArgDuplicate  = errors.New("docker.build_args: duplicate name")
	ErrDockerBuildArgUnknown    = errors.New("docker.build_args: unknown key, expected 'name', 'default' or 'stages'")
	ErrDockerBuildArgDefault    = errors.New("docker.build_args: default must be a single-line string without '\"', '#' and '\\'")
	ErrDockerBuildArgStages     = errors.New("docker.build_args: 'stages' must be a list of \"builder\" and/or \"final\"")
	ErrDockerBuildArgStage      = errors.New("docker.build_args: unknown stage, expected \"builder\" or \"final\"")
	ErrDockerLabelKey           = errors.New("docker.labels: key must be lowercase and contain only letters, digits, '.', '-' and '_'")
	ErrDockerLabelValueType     = errors.New("docker.labels: value must be a string (quote it in YAML)")
	ErrDockerLabelValue         = errors.New("docker.labels: value must be a single-line string without '\"' and '\\'")
	ErrDockerLabelArgUndeclared = errors.New(
		"docker.labels: value references a build argument that is not declared in docker.build_args with stage \"final\"",
	)
)

// dockerBuildArgNameRx matches the accepted build argument names.
var dockerBuildArgNameRx = regexp.MustCompile(`^` + dockerArgNamePattern + `$`)

// dockerLabelKeyRx matches the accepted label keys — reverse-DNS notation as
// used by the OCI annotations. Uppercase is rejected because viper lowercases
// config keys, so an uppercase key would silently change on the way in.
var dockerLabelKeyRx = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// dockerArgRefRx matches a build argument reference inside a label value:
// `$NAME` or `${NAME}`. A doubled `$$` is the Dockerfile escape of a literal
// dollar and is skipped by consuming it in the first alternative.
var dockerArgRefRx = regexp.MustCompile(
	`\$\$|\$\{(` + dockerArgNamePattern + `)\}|\$(` + dockerArgNamePattern + `)`,
)

// dockerUnsafeValueRx matches the characters that would break the generated
// Dockerfile LABEL or the Makefile assignment: a line break, the double quote
// the values are wrapped in, and the line continuation.
var dockerUnsafeValueRx = regexp.MustCompile(`[\r\n"\\]`)

// dockerReservedArgNames lists the names a build argument may not take: the
// `ARG`s the Dockerfile template already declares and the variables the
// Makefile template already assigns. Taking one of them would redeclare it and
// silently change the value used by the rest of the generated build.
var dockerReservedArgNames = map[string]struct{}{ //nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards
	// Dockerfile template (app/files/Dockerfile.tmpl).
	"binary": {},
	// Makefile template (main/Makefile.tmpl).
	"ARGENVERSION": {}, "BUF_VERSION": {}, "BUILD_DATE": {}, "CGO_ENABLED": {},
	"COMMIT_BRANCH": {}, "COVERAGE_DIR": {}, "COVERAGE_FILTER": {},
	"DOCKER_BUILDKIT": {}, "DOCKER_TAG": {}, "DOCS_PREFIX": {}, "DOCS_REGION": {},
	"ENVIRONMENT_PREFIX": {}, "FLAGS": {}, "GOATSERVICESVERSION": {},
	"GOATVERSION": {}, "GOCOVERDIR": {}, "GOLANGCI_BIN": {},
	"GOLANGCI_BUILD_GO_VERSION": {}, "GOLANGCI_GLOBAL_BIN": {},
	"GOLANGCI_TAG": {}, "GOPATH": {}, "GOPRIVATE": {}, "GOPROXY": {},
	"GO_JSONSCHEMA_VERSION": {}, "GO_MOD_VERSION": {}, "LAST_COMMIT_HASH": {},
	"LD_FLAGS": {}, "LINT_ARGS": {}, "LOCAL_BIN": {}, "MIN_GO_VERSION": {},
	"MKDOCS_IMAGE": {}, "MKDOCS_RUN": {}, "NFPM_VERSION": {}, "OGENVERSION": {},
	"ONLINECONF_UPDATER_VERSION": {}, "OSNAME": {}, "PACKAGE": {},
	"PKG_LIST_FOR_TEST": {}, "PKG_LIST_FOR_UNIT_TEST": {}, "PKG_RELEASE": {},
	"PKG_VERSION": {}, "PWD": {}, "REGISTRY_SERVER": {}, "RELEASE": {},
	"SERVICE_NAME": {}, "SOURCE_REF": {}, "TAGS": {}, "TEST_TIMEOUT": {},
	"UPLOAD_PREFIX": {}, "UPLOAD_REGION": {}, "VERSION": {},
}

// Parse normalizes the docker section into ds.DockerConfig: it validates the
// build arguments and labels and resolves the short forms (a bare name, an
// omitted stage list). It also enforces the invariant the section exists for —
// every build argument a label references is declared in the final stage, so a
// label never renders with an empty value.
func (d Docker) Parse() (ds.DockerConfig, error) {
	buildArgs, err := d.parseBuildArgs()
	if err != nil {
		return ds.DockerConfig{}, err
	}

	labels, err := parseDockerLabels(d.Labels)
	if err != nil {
		return ds.DockerConfig{}, err
	}

	if err = checkDockerLabelRefs(labels, buildArgs); err != nil {
		return ds.DockerConfig{}, err
	}

	return ds.DockerConfig{
		ImagePrefix: d.ImagePrefix,
		BuildArgs:   buildArgs,
		Labels:      labels,
	}, nil
}

// IsValid validates the docker section.
func (d Docker) IsValid() (bool, string) {
	if _, err := d.Parse(); err != nil {
		return false, err.Error()
	}

	return true, ""
}

// parseBuildArgs validates docker.build_args and resolves its short form.
func (d Docker) parseBuildArgs() ([]ds.DockerBuildArg, error) {
	args := make([]ds.DockerBuildArg, 0, len(d.BuildArgs))
	seen := make(map[string]struct{}, len(d.BuildArgs))

	for _, raw := range d.BuildArgs {
		arg, err := parseDockerBuildArg(raw)
		if err != nil {
			return nil, err
		}

		if _, dup := seen[arg.Name]; dup {
			return nil, fmt.Errorf(errFmtGotValue, ErrDockerBuildArgDuplicate, arg.Name)
		}

		seen[arg.Name] = struct{}{}

		args = append(args, arg)
	}

	return args, nil
}

// parseDockerBuildArg validates one docker.build_args entry: either a bare
// name or a {name, default, stages} object.
func parseDockerBuildArg(raw any) (ds.DockerBuildArg, error) {
	switch v := raw.(type) {
	case string:
		return newDockerBuildArg(v, "", nil)
	case map[string]any:
		for key := range v {
			if key != "name" && key != "default" && key != "stages" {
				return ds.DockerBuildArg{}, fmt.Errorf(errFmtGotValue, ErrDockerBuildArgUnknown, key)
			}
		}

		name, ok := v["name"].(string)
		if !ok {
			return ds.DockerBuildArg{}, fmt.Errorf(errFmtGotType, ErrDockerBuildArgName, v["name"])
		}

		def := ""

		if rawDef, ex := v["default"]; ex {
			if def, ok = rawDef.(string); !ok {
				return ds.DockerBuildArg{}, fmt.Errorf(errFmtGotType, ErrDockerBuildArgDefault, rawDef)
			}
		}

		return newDockerBuildArg(name, def, v["stages"])
	default:
		return ds.DockerBuildArg{}, fmt.Errorf(errFmtGotType, ErrDockerBuildArgType, raw)
	}
}

// newDockerBuildArg validates the parts of one build argument and applies the
// default stage list.
func newDockerBuildArg(name, def string, rawStages any) (ds.DockerBuildArg, error) {
	if !dockerBuildArgNameRx.MatchString(name) {
		return ds.DockerBuildArg{}, fmt.Errorf(errFmtGotValue, ErrDockerBuildArgName, name)
	}

	if _, reserved := dockerReservedArgNames[name]; reserved {
		return ds.DockerBuildArg{}, fmt.Errorf(errFmtGotValue, ErrDockerBuildArgReserved, name)
	}

	if dockerUnsafeValueRx.MatchString(def) || strings.Contains(def, "#") {
		return ds.DockerBuildArg{}, fmt.Errorf(errFmtGotValue, ErrDockerBuildArgDefault, def)
	}

	stages, err := parseDockerStages(rawStages)
	if err != nil {
		return ds.DockerBuildArg{}, err
	}

	return ds.DockerBuildArg{Name: name, Default: def, Stages: stages}, nil
}

// parseDockerStages validates the stage list of a build argument. Omitting it
// means the final stage: that is where the labels live, and declaring an `ARG`
// in the builder stage invalidates the build cache of every layer below it on
// every value change.
func parseDockerStages(raw any) ([]string, error) {
	if raw == nil {
		return []string{ds.DockerStageFinal}, nil
	}

	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil, fmt.Errorf(errFmtGotType, ErrDockerBuildArgStages, raw)
	}

	var builder, final bool

	for _, item := range list {
		stage, isStr := item.(string)
		if !isStr {
			return nil, fmt.Errorf(errFmtGotType, ErrDockerBuildArgStages, item)
		}

		switch stage {
		case ds.DockerStageBuilder:
			builder = true
		case ds.DockerStageFinal:
			final = true
		default:
			return nil, fmt.Errorf(errFmtGotValue, ErrDockerBuildArgStage, stage)
		}
	}

	// Emitted in Dockerfile order regardless of the order in the config; a
	// repeated stage collapses, so the same ARG is never declared twice.
	stages := make([]string, 0, ds.MaxDockerStages)

	if builder {
		stages = append(stages, ds.DockerStageBuilder)
	}

	if final {
		stages = append(stages, ds.DockerStageFinal)
	}

	return stages, nil
}

// parseDockerLabels validates docker.labels and flattens the nested maps viper
// produces for dotted keys back into the original dotted key. The result is
// sorted by key, so the generated LABEL is stable across runs.
func parseDockerLabels(raw map[string]any) ([]ds.DockerLabel, error) {
	labels := make([]ds.DockerLabel, 0, len(raw))

	if err := flattenDockerLabels("", raw, &labels); err != nil {
		return nil, err
	}

	sort.Slice(labels, func(i, j int) bool { return labels[i].Key < labels[j].Key })

	return labels, nil
}

// flattenDockerLabels walks the label map, joining the nested keys viper split
// on the dots of a reverse-DNS label key back into one key.
func flattenDockerLabels(prefix string, raw map[string]any, labels *[]ds.DockerLabel) error {
	for key, value := range raw {
		if !dockerLabelKeyRx.MatchString(key) {
			return fmt.Errorf(errFmtGotValue, ErrDockerLabelKey, prefix+key)
		}

		full := prefix + key

		if nested, ok := value.(map[string]any); ok {
			if err := flattenDockerLabels(full+".", nested, labels); err != nil {
				return err
			}

			continue
		}

		str, ok := value.(string)
		if !ok {
			return fmt.Errorf("%w: %q got %T", ErrDockerLabelValueType, full, value)
		}

		if dockerUnsafeValueRx.MatchString(str) {
			return fmt.Errorf("%w: %q = %q", ErrDockerLabelValue, full, str)
		}

		*labels = append(*labels, ds.DockerLabel{Key: full, Value: str})
	}

	return nil
}

// checkDockerLabelRefs verifies that every build argument a label value
// references is declared in the final stage. Nothing else provides a value
// there, so an undeclared reference would render the label empty — exactly the
// failure this section exists to prevent.
func checkDockerLabelRefs(labels []ds.DockerLabel, args []ds.DockerBuildArg) error {
	finalArgs := make(map[string]struct{}, len(args))

	for _, arg := range args {
		if arg.UsedInStage(ds.DockerStageFinal) {
			finalArgs[arg.Name] = struct{}{}
		}
	}

	for _, label := range labels {
		for _, match := range dockerArgRefRx.FindAllStringSubmatch(label.Value, -1) {
			// match[0] == "$$": an escaped dollar, no reference.
			name := match[1]
			if name == "" {
				name = match[2]
			}

			if name == "" {
				continue
			}

			if _, ok := finalArgs[name]; !ok {
				return fmt.Errorf("%w: %q references ${%s}", ErrDockerLabelArgUndeclared, label.Key, name)
			}
		}
	}

	return nil
}
