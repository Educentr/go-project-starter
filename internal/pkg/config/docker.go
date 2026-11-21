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

// AppDocker is the docker section of one application: the runtime stage of its
// generated Dockerfile and the platforms its image is built for.
//
// The runtime stage used to be fixed — `FROM alpine` with `CMD ["/service"]` —
// so an application that needs another distribution or a runtime dependency
// (git, curl, ca-certificates) had no way to say so: the stage is generated,
// and the user marker only keeps what is written below it. This section
// declares it instead.
//
// The platform list is per-application and not per-project because the
// audience of the images differs. An application that is run on developer
// machines (a CLI tool shipped as an image) has to work on amd64 and on Apple
// Silicon, while the server applications of the same project usually need one
// platform and should not pay for a second build.
//
// YAML example:
//
//	applications:
//	  - name: vt
//	    docker:
//	      base_image: ubuntu:24.04      # default: alpine
//	      packages: [git, curl]         # runtime packages
//	      workdir: /workspace           # default: none (Docker default "/")
//	      entrypoint: ["/service"]      # default: CMD ["/service"]
//	      platforms: [linux/amd64, linux/arm64]  # default: the builder platform
//
// See docs/configuration/docker.md for full documentation.
type AppDocker struct {
	// BaseImage is the base image of the runtime stage. Defaults to
	// ds.DefaultDockerBaseImage.
	BaseImage string `mapstructure:"base_image"`
	// PackageManager selects how Packages are installed: "apk", "apt" or
	// "dnf". Optional — resolved from BaseImage for the known distributions.
	PackageManager string `mapstructure:"package_manager"`
	// Packages are the runtime packages installed in the runtime stage.
	Packages []string `mapstructure:"packages"`
	// Workdir is the working directory of the runtime stage. Optional.
	Workdir string `mapstructure:"workdir"`
	// Entrypoint is the ENTRYPOINT of the runtime stage in exec form.
	// Optional — replaces the default CMD when set.
	Entrypoint []string `mapstructure:"entrypoint"`
	// Platforms declares the target platforms of the application image. Each
	// entry is "linux/<arch>". Omitted (or empty) keeps the historical
	// behavior: the image is built for the platform of the builder. Read via
	// Parse.
	Platforms []string `mapstructure:"platforms"`
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
var dockerReservedArgNames = map[string]struct{}{
	// Dockerfile template (app/files/Dockerfile.tmpl).
	"binary": {},
	// Makefile template (main/Makefile.tmpl).
	"ARGENVERSION": {}, "BUF_VERSION": {}, "BUILD_DATE": {}, "BUILDX_BUILDER": {},
	"CGO_ENABLED": {}, "COMMIT_BRANCH": {}, "COVERAGE_DIR": {}, "COVERAGE_FILTER": {},
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

// Sentinel errors of the application docker section. Use errors.Is to
// distinguish.
var (
	ErrAppDockerBaseImage = errors.New(
		"applications.docker.base_image: must be an image reference without whitespace, quotes or '#'",
	)
	ErrAppDockerPackage = errors.New(
		"applications.docker.packages: package name must start with a letter or digit and " +
			"contain only letters, digits and '._+-=:~/'",
	)
	ErrAppDockerPackageManager = errors.New(
		`applications.docker.package_manager: unknown package manager, expected "apk", "apt" or "dnf"`,
	)
	ErrAppDockerPackageManagerUnknown = errors.New(
		"applications.docker.package_manager: cannot be resolved from base_image, declare it explicitly",
	)
	ErrAppDockerPackageManagerUnused = errors.New(
		"applications.docker.package_manager: declared without packages, nothing would install them",
	)
	ErrAppDockerWorkdir    = errors.New("applications.docker.workdir: must be an absolute path without whitespace or quotes")
	ErrAppDockerEntrypoint = errors.New(`applications.docker.entrypoint: every element must be a non-empty single-line string without '"' and '\'`)
	ErrAppDockerPlatform   = errors.New(
		`applications[].docker.platforms: expected "linux/<arch>", for example "linux/amd64" or "linux/arm64"`,
	)
	ErrAppDockerPlatformDuplicate = errors.New("applications[].docker.platforms: duplicate platform")
	ErrAppDockerNoImage           = errors.New(
		"applications[].docker.platforms: declared for an application that builds no docker artifact",
	)
)

// dockerBaseImageRx matches an image reference: a registry with an optional
// port, a repository path, and an optional tag or digest. Whitespace, quotes
// and '#' are what would break the generated FROM instruction.
var dockerBaseImageRx = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]*$`)

// dockerPackageRx matches a package name. The punctuation beyond a plain name
// is what a version pin needs: `curl=8.5.0-2` (apt, apk), `pkg~=1.2` and
// `pkg/bookworm-backports` (apt release pin).
var dockerPackageRx = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+\-=:~/]*$`)

// dockerPlatformRx matches an accepted platform: linux and an architecture.
//
// Only linux, because the runtime stage of the generated Dockerfile targets
// linux — an image for another OS would not run whatever the declaration says.
// Without a variant ("linux/arm/v7"), because the variant does not reach the Go
// build: the builder stage takes the architecture from TARGETARCH and a variant
// needs its own GOARM/GOAMD64 mapping, so accepting one would silently produce
// a binary for the wrong sub-architecture.
var dockerPlatformRx = regexp.MustCompile(`^linux/[a-z0-9]+$`)

// dockerWorkdirRx matches an absolute path safe to emit as a WORKDIR operand.
var dockerWorkdirRx = regexp.MustCompile(`^/[^\s"'\\#]*$`)

// dockerBaseImageManagers maps the name of a base image to the package manager
// of its distribution. Only the images whose manager is unambiguous are listed:
// for anything else the section has to declare package_manager, because a
// guessed manager produces a Dockerfile that fails at build time.
var dockerBaseImageManagers = map[string]string{
	"alpine": ds.DockerPackageManagerAPK,
	"debian": ds.DockerPackageManagerAPT,
	"ubuntu": ds.DockerPackageManagerAPT,
	"fedora": ds.DockerPackageManagerDNF,
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

// Parse normalizes the application docker section into ds.AppDockerConfig: it
// validates the values and resolves the package manager of the base image. The
// base image default is applied by ds.AppDockerConfig.Image, so an application
// without the section carries a zero value.
func (d AppDocker) Parse() (ds.AppDockerConfig, error) {
	if d.BaseImage != "" && !dockerBaseImageRx.MatchString(d.BaseImage) {
		return ds.AppDockerConfig{}, fmt.Errorf(errFmtGotValue, ErrAppDockerBaseImage, d.BaseImage)
	}

	for _, pkg := range d.Packages {
		if !dockerPackageRx.MatchString(pkg) {
			return ds.AppDockerConfig{}, fmt.Errorf(errFmtGotValue, ErrAppDockerPackage, pkg)
		}
	}

	manager, err := d.packageManager()
	if err != nil {
		return ds.AppDockerConfig{}, err
	}

	if d.Workdir != "" && !dockerWorkdirRx.MatchString(d.Workdir) {
		return ds.AppDockerConfig{}, fmt.Errorf(errFmtGotValue, ErrAppDockerWorkdir, d.Workdir)
	}

	for _, part := range d.Entrypoint {
		if part == "" || dockerUnsafeValueRx.MatchString(part) {
			return ds.AppDockerConfig{}, fmt.Errorf(errFmtGotValue, ErrAppDockerEntrypoint, part)
		}
	}

	platforms, err := d.parsePlatforms()
	if err != nil {
		return ds.AppDockerConfig{}, err
	}

	return ds.AppDockerConfig{
		BaseImage:      d.BaseImage,
		PackageManager: manager,
		Packages:       d.Packages,
		Workdir:        d.Workdir,
		Entrypoint:     d.Entrypoint,
		Platforms:      platforms,
	}, nil
}

// IsValid validates the application docker section.
func (d AppDocker) IsValid() (bool, string) {
	if _, err := d.Parse(); err != nil {
		return false, err.Error()
	}

	return true, ""
}

// packageManager resolves which package manager installs the declared
// packages: the explicit one, or the one of a known base image.
func (d AppDocker) packageManager() (string, error) {
	if d.PackageManager != "" {
		switch d.PackageManager {
		case ds.DockerPackageManagerAPK, ds.DockerPackageManagerAPT, ds.DockerPackageManagerDNF:
		default:
			return "", fmt.Errorf(errFmtGotValue, ErrAppDockerPackageManager, d.PackageManager)
		}

		// A manager without packages installs nothing — most likely the
		// package list was meant to be there and is missing.
		if len(d.Packages) == 0 {
			return "", fmt.Errorf(errFmtGotValue, ErrAppDockerPackageManagerUnused, d.PackageManager)
		}

		return d.PackageManager, nil
	}

	if len(d.Packages) == 0 {
		return "", nil
	}

	image := d.BaseImage
	if image == "" {
		image = ds.DefaultDockerBaseImage
	}

	manager, known := dockerBaseImageManagers[dockerImageName(image)]
	if !known {
		return "", fmt.Errorf(errFmtGotValue, ErrAppDockerPackageManagerUnknown, image)
	}

	return manager, nil
}

// dockerImageName reduces an image reference to the name of its repository —
// "registry.example:5000/org/ubuntu:24.04" to "ubuntu" — which is what the
// package manager is resolved by.
func dockerImageName(image string) string {
	if digest := strings.Index(image, "@"); digest >= 0 {
		image = image[:digest]
	}

	// A tag follows the last '/', a registry port precedes it.
	if tag := strings.LastIndex(image, ":"); tag > strings.LastIndex(image, "/") {
		image = image[:tag]
	}

	if slash := strings.LastIndex(image, "/"); slash >= 0 {
		image = image[slash+1:]
	}

	return strings.ToLower(image)
}

// parsePlatforms validates the declared target platforms and returns them in
// config order. An unparsable platform has to fail here: it would otherwise
// reach `docker buildx build --platform` and fail the build of the generated
// project instead of the generation.
func (d AppDocker) parsePlatforms() ([]string, error) {
	if len(d.Platforms) == 0 {
		return nil, nil
	}

	platforms := make([]string, 0, len(d.Platforms))
	seen := make(map[string]struct{}, len(d.Platforms))

	for _, platform := range d.Platforms {
		if !dockerPlatformRx.MatchString(platform) {
			return nil, fmt.Errorf(errFmtGotValue, ErrAppDockerPlatform, platform)
		}

		if _, dup := seen[platform]; dup {
			return nil, fmt.Errorf(errFmtGotValue, ErrAppDockerPlatformDuplicate, platform)
		}

		seen[platform] = struct{}{}

		platforms = append(platforms, platform)
	}

	return platforms, nil
}
