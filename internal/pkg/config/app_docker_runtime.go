package config

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/pkg/errors"
)

// Parsing and validation of the project-declared additions to the runtime stage
// of an application image: `copy`, `env`, `fetch` and `stages`.
//
// All four are declarative on purpose. A project-written Dockerfile fragment
// would cover the same cases, but it also takes away what makes the generated
// runtime stage safe: the generator knows there is no `RUN` in it, which is
// what lets a shell-less base image (distroless, scratch) build and what keeps
// a cross-built image from needing emulation. These keys emit only COPY and
// ENV into the runtime stage — never RUN — so both invariants hold by
// construction and stay checkable. Where a RUN is unavoidable (installing a
// tool), it lands in an auxiliary stage pinned to the platform of the build
// host.

// destSet tracks the destinations claimed in the image so far.
type destSet map[string]struct{}

const (
	// appDockerAbsPathPattern is the shape of a path safe to emit as an
	// operand of a generated ADD, COPY or WORKDIR instruction: absolute, and
	// without the whitespace and quotes that would break the instruction.
	appDockerAbsPathPattern = `^/[^\s"'\\#]*$`
	// appDockerNamePattern is the shape of a fetch or stage name — it becomes
	// a Dockerfile stage name (fetch-<name>, stage-<name>), as a web entry
	// name does.
	appDockerNamePattern = `^[a-z0-9_-]+$`
	// dockerPlatformOS is the only OS a declared platform can name, see
	// dockerPlatformRx. It is what separates the architecture from the
	// platform.
	dockerPlatformOS = "linux"

	// Error formats shared by the checks of several keys.
	errFmtFieldValue   = "%w: %s %q"
	errFmtNamedValue   = "%w: %s = %q"
	errFmtMessage      = "%w: %s"
	errFmtPlatformSets = "%w: got %s for %s"
)

// Sentinel errors of the runtime-stage additions. Use errors.Is to
// distinguish.
var (
	ErrAppDockerPath = errors.New(
		"applications[].docker: must be an absolute path without whitespace, quotes or '#', and not the image root",
	)
	ErrAppDockerDestReserved = errors.New(
		"applications[].docker: destination is written by the generated image itself",
	)
	ErrAppDockerDestDuplicate = errors.New(
		"applications[].docker: two entries write the same destination, the later one would win",
	)
	ErrAppDockerCopyFrom = errors.New("applications[].docker.copy: 'from' must be a directory inside the project")
	ErrAppDockerEnvName  = errors.New(
		"applications[].docker.env: name must start with a letter or underscore and contain only letters, digits and underscores",
	)
	ErrAppDockerEnvValue     = errors.New(`applications[].docker.env: value must be a single-line string without '"' and '\'`)
	ErrAppDockerEnvDuplicate = errors.New("applications[].docker.env: duplicate name")
	ErrAppDockerName         = errors.New("applications[].docker: name must match ^[a-z0-9_-]+$")
	// ErrAppDockerNameDuplicate reports a fetch or stage name used twice: the
	// names become Dockerfile stage names, so a repeat would redefine a stage.
	ErrAppDockerNameDuplicate = errors.New("applications[].docker: duplicate name")
	ErrAppDockerFetchURL      = errors.New(
		"applications[].docker.fetch: url must be an https URL without whitespace, quotes or '#'",
	)
	ErrAppDockerFetchURLVar = errors.New(
		"applications[].docker.fetch: url may only reference ${TARGETARCH} and ${TARGETOS}, " +
			"nothing would give another variable a value",
	)
	ErrAppDockerFetchMode     = errors.New("applications[].docker.fetch: mode must be an octal file mode, for example \"0755\"")
	ErrAppDockerFetchChecksum = errors.New(
		"applications[].docker.fetch: checksum must be \"sha256:<64 hex digits>\", " +
			"or a map of platform to such a digest",
	)
	ErrAppDockerFetchChecksumVar = errors.New(
		"applications[].docker.fetch: one checksum cannot cover a url that depends on the platform — " +
			"declare a digest per platform",
	)
	ErrAppDockerFetchChecksumConst = errors.New(
		"applications[].docker.fetch: a digest per platform needs a url that depends on the platform " +
			"(reference ${TARGETARCH} in it)",
	)
	ErrAppDockerFetchChecksumPlatforms = errors.New(
		"applications[].docker.fetch: a digest per platform requires docker.platforms, " +
			"with exactly one digest per declared platform",
	)
	ErrAppDockerStageImage = errors.New(
		"applications[].docker.stages: image must be an image reference without whitespace, quotes or '#'",
	)
	ErrAppDockerStageRun = errors.New(
		"applications[].docker.stages: 'run' must list at least one non-empty single-line command",
	)
	ErrAppDockerStageRunVar = errors.New("applications[].docker.stages: 'run' takes no template variables")
	// ErrAppDockerNoImageAddition reports an addition to an image the
	// application does not build, the same silent no-op ErrAppDockerNoImage
	// rules out for the platform list.
	ErrAppDockerNoImageAddition = errors.New(
		"applications[].docker: copy/env/fetch/stages declared for an application that builds no docker artifact",
	)
	ErrAppDockerStageCopy = errors.New(
		"applications[].docker.stages: 'copy' must list at least one {from, to}, " +
			"nothing else carries the result out of the stage",
	)
)

// appDockerNameRx is the accepted shape of a fetch or stage name — it becomes a
// Dockerfile stage name (fetch-<name>, stage-<name>), as a web entry name does.
var appDockerNameRx = regexp.MustCompile(appDockerNamePattern)

// appDockerAbsPathRx matches an absolute path safe to emit as an ADD, COPY or
// WORKDIR operand.
var appDockerAbsPathRx = regexp.MustCompile(appDockerAbsPathPattern)

// appDockerFetchURLRx matches an accepted fetch URL: https, and nothing that
// would break the generated ADD instruction. A variable reference (`${…}`) is
// part of the accepted character set and is checked separately.
var appDockerFetchURLRx = regexp.MustCompile(`^https://[^\s"'\\#]+$`)

// appDockerModeRx matches an octal file mode as `ADD --chmod` takes it.
var appDockerModeRx = regexp.MustCompile(`^0?[0-7]{3}$`)

// appDockerDigestRx matches the digest form BuildKit's `ADD --checksum`
// accepts. Only sha256: it is the algorithm release pages publish and the one
// the frontend verifies.
var appDockerDigestRx = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// appDockerVarRx matches a variable reference inside a fetch URL: `$NAME` or
// `${NAME}`. A doubled `$$` is the Dockerfile escape of a literal dollar and is
// skipped by consuming it in the first alternative, as the label scan does.
var appDockerVarRx = regexp.MustCompile(`\$\$|\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// appDockerReservedDests lists the destinations the generated image writes
// itself. A project entry pointing at one of them would be overwritten by the
// instruction that follows it, or overwrite it — either way one of the two
// copies silently does nothing.
var appDockerReservedDests = map[string]struct{}{ //nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards
	ds.DockerServiceBinary: {},
	"/public":              {},
	"/etc/onlineconf":      {},
}

// appDockerURLVars are the variables a fetch URL may reference: the target
// platform of the build, which the fetch stage declares as ARGs. Any other name
// has no value in that stage and would render empty.
var appDockerURLVars = map[string]struct{}{ //nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards
	ds.DockerTargetArchArg: {},
	ds.DockerTargetOSArg:   {},
}

// parseRuntimeAdditions validates `copy`, `env`, `fetch` and `stages` and
// normalizes them into the ds model. platforms are the already-parsed target
// platforms of the application: a per-platform checksum is checked against
// them.
//
// One destSet is shared by all four keys, so that two entries writing the same
// path fail here rather than silently losing one of the two copies in the built
// image.
func (d AppDocker) parseRuntimeAdditions(platforms []string) (ds.AppDockerConfig, error) {
	var (
		out  ds.AppDockerConfig
		err  error
		seen = destSet{}
	)

	if out.Copy, err = d.parseCopy(seen); err != nil {
		return ds.AppDockerConfig{}, err
	}

	if out.Env, err = d.parseEnv(); err != nil {
		return ds.AppDockerConfig{}, err
	}

	if out.Fetch, err = d.parseFetch(platforms, seen); err != nil {
		return ds.AppDockerConfig{}, err
	}

	if out.Stages, err = d.parseStages(seen); err != nil {
		return ds.AppDockerConfig{}, err
	}

	return out, nil
}

// claim validates one destination path and records it. The reserved paths and
// the duplicate check are the same for every key, because the failure is: an
// instruction of the generated image writes the same path and one of the two
// copies is lost.
func (s destSet) claim(field, p string) error {
	if err := validateAppDockerAbsPath(field, p); err != nil {
		return err
	}

	if _, reserved := appDockerReservedDests[p]; reserved {
		return fmt.Errorf(errFmtFieldValue, ErrAppDockerDestReserved, field, p)
	}

	if _, dup := s[p]; dup {
		return fmt.Errorf(errFmtFieldValue, ErrAppDockerDestDuplicate, field, p)
	}

	s[p] = struct{}{}

	return nil
}

// validateAppDockerAbsPath checks that p is usable as an operand of a generated
// ADD, COPY or WORKDIR instruction.
func validateAppDockerAbsPath(field, p string) error {
	if !appDockerAbsPathRx.MatchString(p) || p == "/" || path.Clean(p) != p {
		return fmt.Errorf(errFmtFieldValue, ErrAppDockerPath, field, p)
	}

	return nil
}

// parseCopy validates the project directories carried into the image.
func (d AppDocker) parseCopy(seen destSet) ([]ds.AppDockerCopy, error) {
	if len(d.Copy) == 0 {
		return nil, nil
	}

	out := make([]ds.AppDockerCopy, 0, len(d.Copy))

	for _, entry := range d.Copy {
		if ok, msg := validateProjectRelPath("from", entry.From); !ok {
			return nil, fmt.Errorf(errFmtMessage, ErrAppDockerCopyFrom, msg)
		}

		if err := seen.claim("copy.to", entry.To); err != nil {
			return nil, err
		}

		out = append(out, ds.AppDockerCopy{From: entry.From, To: entry.To})
	}

	return out, nil
}

// parseEnv validates the environment variables of the runtime stage.
func (d AppDocker) parseEnv() ([]ds.AppDockerEnv, error) {
	if len(d.Env) == 0 {
		return nil, nil
	}

	out := make([]ds.AppDockerEnv, 0, len(d.Env))
	seen := make(map[string]struct{}, len(d.Env))

	for _, entry := range d.Env {
		if !dockerBuildArgNameRx.MatchString(entry.Name) {
			return nil, fmt.Errorf(errFmtGotValue, ErrAppDockerEnvName, entry.Name)
		}

		if dockerUnsafeValueRx.MatchString(entry.Value) {
			return nil, fmt.Errorf(errFmtNamedValue, ErrAppDockerEnvValue, entry.Name, entry.Value)
		}

		if _, dup := seen[entry.Name]; dup {
			return nil, fmt.Errorf(errFmtGotValue, ErrAppDockerEnvDuplicate, entry.Name)
		}

		seen[entry.Name] = struct{}{}

		out = append(out, ds.AppDockerEnv{Name: entry.Name, Value: entry.Value})
	}

	return out, nil
}

// parseFetch validates the files the image takes from a URL.
func (d AppDocker) parseFetch(platforms []string, seen destSet) ([]ds.AppDockerFetch, error) {
	if len(d.Fetch) == 0 {
		return nil, nil
	}

	out := make([]ds.AppDockerFetch, 0, len(d.Fetch))
	names := make(map[string]struct{}, len(d.Fetch))

	for _, entry := range d.Fetch {
		fetch, err := entry.parse(platforms)
		if err != nil {
			return nil, err
		}

		if err = claimStageName(names, fetch.Name); err != nil {
			return nil, err
		}

		if err = seen.claim("fetch.to", fetch.To); err != nil {
			return nil, err
		}

		out = append(out, fetch)
	}

	return out, nil
}

// parse validates one fetch entry and resolves its checksum form.
func (e AppDockerFetchEntry) parse(platforms []string) (ds.AppDockerFetch, error) {
	if !appDockerNameRx.MatchString(e.Name) {
		return ds.AppDockerFetch{}, fmt.Errorf(errFmtFieldValue, ErrAppDockerName, "fetch.name", e.Name)
	}

	targeted, err := e.validateURL()
	if err != nil {
		return ds.AppDockerFetch{}, err
	}

	if e.Mode != "" && !appDockerModeRx.MatchString(e.Mode) {
		return ds.AppDockerFetch{}, fmt.Errorf(errFmtGotValue, ErrAppDockerFetchMode, e.Mode)
	}

	checksum, variants, err := e.parseChecksum(platforms, targeted)
	if err != nil {
		return ds.AppDockerFetch{}, err
	}

	return ds.AppDockerFetch{
		Name:     e.Name,
		URL:      e.URL,
		To:       e.To,
		Mode:     e.Mode,
		Checksum: checksum,
		Variants: variants,
	}, nil
}

// validateURL checks the source of a fetch entry and reports whether it depends
// on the target platform, which is what decides the checksum form.
//
// https only: the file lands in the image unverified otherwise, and a plaintext
// download is exactly where a pinned digest would be needed most.
func (e AppDockerFetchEntry) validateURL() (bool, error) {
	if !appDockerFetchURLRx.MatchString(e.URL) {
		return false, fmt.Errorf(errFmtGotValue, ErrAppDockerFetchURL, e.URL)
	}

	targeted := false

	for _, match := range appDockerVarRx.FindAllStringSubmatch(e.URL, -1) {
		// match[0] == "$$": an escaped dollar, no reference.
		name := match[1]
		if name == "" {
			name = match[2]
		}

		if name == "" {
			continue
		}

		if _, ok := appDockerURLVars[name]; !ok {
			return false, fmt.Errorf("%w: got ${%s}", ErrAppDockerFetchURLVar, name)
		}

		if name == ds.DockerTargetArchArg {
			targeted = true
		}
	}

	return targeted, nil
}

// parseChecksum resolves the checksum of a fetch entry: a single digest, one
// digest per platform, or none.
//
// The two forms are not interchangeable. A Dockerfile cannot choose between
// digests, so a per-platform digest is generated as a stage per architecture
// plus a selector FROM — which only works when the URL itself depends on the
// architecture. The reverse, one digest for a URL that does depend on it, would
// verify one architecture's file against another's digest and fail the build of
// every other platform.
func (e AppDockerFetchEntry) parseChecksum(platforms []string, targeted bool) (string, []ds.AppDockerFetchVariant, error) {
	switch raw := e.Checksum.(type) {
	case nil:
		return "", nil, nil
	case string:
		if !appDockerDigestRx.MatchString(raw) {
			return "", nil, fmt.Errorf(errFmtGotValue, ErrAppDockerFetchChecksum, raw)
		}

		if targeted {
			return "", nil, fmt.Errorf(errFmtGotValue, ErrAppDockerFetchChecksumVar, e.URL)
		}

		return raw, nil, nil
	default:
		digests, err := checksumMap(raw)
		if err != nil {
			return "", nil, err
		}

		variants, err := fetchVariants(digests, platforms)
		if err != nil {
			return "", nil, err
		}

		if !targeted {
			return "", nil, fmt.Errorf(errFmtGotValue, ErrAppDockerFetchChecksumConst, e.URL)
		}

		return "", variants, nil
	}
}

// checksumMap reads the per-platform checksum map out of whatever viper
// produced for it.
func checksumMap(raw any) (map[string]string, error) {
	switch m := raw.(type) {
	case map[string]string:
		return m, nil
	case map[string]any:
		out := make(map[string]string, len(m))

		for platform, value := range m {
			digest, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf(errFmtGotType, ErrAppDockerFetchChecksum, value)
			}

			out[platform] = digest
		}

		return out, nil
	default:
		return nil, fmt.Errorf(errFmtGotType, ErrAppDockerFetchChecksum, raw)
	}
}

// fetchVariants turns a per-platform checksum map into the variants of a fetch
// entry, in the order the platforms are declared.
//
// The map has to cover exactly the declared platforms: a digest for a platform
// the application is not built for is dead configuration, and a platform
// without a digest would be built from an unverified download — the generated
// selector would not even find its stage.
func fetchVariants(digests map[string]string, platforms []string) ([]ds.AppDockerFetchVariant, error) {
	if len(platforms) == 0 || len(digests) != len(platforms) {
		return nil, fmt.Errorf(errFmtPlatformSets,
			ErrAppDockerFetchChecksumPlatforms, platformKeys(digests), platformList(platforms))
	}

	variants := make([]ds.AppDockerFetchVariant, 0, len(platforms))

	for _, platform := range platforms {
		digest, ok := digests[platform]
		if !ok {
			return nil, fmt.Errorf(errFmtPlatformSets,
				ErrAppDockerFetchChecksumPlatforms, platformKeys(digests), platformList(platforms))
		}

		if !appDockerDigestRx.MatchString(digest) {
			return nil, fmt.Errorf(errFmtNamedValue, ErrAppDockerFetchChecksum, platform, digest)
		}

		variants = append(variants, ds.AppDockerFetchVariant{
			Arch:   strings.TrimPrefix(platform, dockerPlatformOS+"/"),
			Digest: digest,
		})
	}

	return variants, nil
}

// platformKeys renders the platforms a checksum map covers, sorted so that the
// error message does not change between runs.
func platformKeys(digests map[string]string) string {
	keys := make([]string, 0, len(digests))

	for platform := range digests {
		keys = append(keys, platform)
	}

	sort.Strings(keys)

	return platformList(keys)
}

// platformList renders a platform list the way the error messages name it.
func platformList(platforms []string) string {
	if len(platforms) == 0 {
		return "no platforms"
	}

	return "[" + strings.Join(platforms, " ") + "]"
}

// parseStages validates the auxiliary build stages of the image.
func (d AppDocker) parseStages(seen destSet) ([]ds.AppDockerStage, error) {
	if len(d.Stages) == 0 {
		return nil, nil
	}

	out := make([]ds.AppDockerStage, 0, len(d.Stages))
	names := make(map[string]struct{}, len(d.Stages))

	for _, entry := range d.Stages {
		stage, err := entry.parse(seen)
		if err != nil {
			return nil, err
		}

		if err = claimStageName(names, stage.Name); err != nil {
			return nil, err
		}

		out = append(out, stage)
	}

	return out, nil
}

// parse validates one auxiliary stage.
func (e AppDockerStageEntry) parse(seen destSet) (ds.AppDockerStage, error) {
	if !appDockerNameRx.MatchString(e.Name) {
		return ds.AppDockerStage{}, fmt.Errorf(errFmtFieldValue, ErrAppDockerName, "stages.name", e.Name)
	}

	if !dockerBaseImageRx.MatchString(e.Image) {
		return ds.AppDockerStage{}, fmt.Errorf(errFmtGotValue, ErrAppDockerStageImage, e.Image)
	}

	if e.Workdir != "" {
		if err := validateAppDockerAbsPath("stages.workdir", e.Workdir); err != nil {
			return ds.AppDockerStage{}, err
		}
	}

	if len(e.Run) == 0 {
		return ds.AppDockerStage{}, fmt.Errorf(errFmtGotValue, ErrAppDockerStageRun, e.Name)
	}

	for _, cmd := range e.Run {
		if strings.TrimSpace(cmd) == "" {
			return ds.AppDockerStage{}, fmt.Errorf(errFmtGotValue, ErrAppDockerStageRun, cmd)
		}

		if ok, msg := validateSingleLineCommand("run", cmd); !ok {
			return ds.AppDockerStage{}, fmt.Errorf(errFmtMessage, ErrAppDockerStageRun, msg)
		}

		// The stage is rendered from the config as is: a template variable
		// here has nothing to resolve against and would reach the Dockerfile
		// verbatim.
		if strings.Contains(cmd, "{{") {
			return ds.AppDockerStage{}, fmt.Errorf(errFmtGotValue, ErrAppDockerStageRunVar, cmd)
		}
	}

	copies, err := e.parseCopy(seen)
	if err != nil {
		return ds.AppDockerStage{}, err
	}

	return ds.AppDockerStage{
		Name:    e.Name,
		Image:   e.Image,
		Workdir: e.Workdir,
		Run:     e.Run,
		Copy:    copies,
	}, nil
}

// parseCopy validates what the runtime stage takes out of an auxiliary stage.
// Both sides are absolute: the source is a path inside the stage, not a project
// path, because the stage gets no build context.
func (e AppDockerStageEntry) parseCopy(seen destSet) ([]ds.AppDockerCopy, error) {
	if len(e.Copy) == 0 {
		return nil, fmt.Errorf(errFmtGotValue, ErrAppDockerStageCopy, e.Name)
	}

	out := make([]ds.AppDockerCopy, 0, len(e.Copy))

	for _, entry := range e.Copy {
		if err := validateAppDockerAbsPath("stages.copy.from", entry.From); err != nil {
			return nil, err
		}

		if err := seen.claim("stages.copy.to", entry.To); err != nil {
			return nil, err
		}

		out = append(out, ds.AppDockerCopy{From: entry.From, To: entry.To})
	}

	return out, nil
}

// claimStageName records a fetch or stage name, rejecting a repeat: the names
// become Dockerfile stage names, so the second one would redefine the first.
func claimStageName(seen map[string]struct{}, name string) error {
	if _, dup := seen[name]; dup {
		return fmt.Errorf(errFmtGotValue, ErrAppDockerNameDuplicate, name)
	}

	seen[name] = struct{}{}

	return nil
}
