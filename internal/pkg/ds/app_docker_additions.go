package ds

import "strings"

// AppDockerCopy is one copy into the runtime stage of an application image.
//
// It serves two keys of the section, because the instruction it becomes is the
// same in both: `copy` (From is a project-relative directory, taken from the
// build context) and the `copy` block of an auxiliary stage (From is an
// absolute path inside that stage). Which one it is follows from where the
// entry is stored, so the template never has to ask.
type AppDockerCopy struct {
	// From is the source: a project-relative directory for the application's
	// own `copy`, an absolute path inside the stage for a stage's `copy`.
	From string
	// To is the absolute destination path in the image.
	To string
}

// AppDockerEnv is one environment variable of the runtime stage, declared in
// the `env` list of the application's docker section.
//
// Declared as a list of {name, value} rather than a map because viper
// lowercases config keys: an environment variable name is conventionally
// uppercase, and a map key would silently arrive as `vt_template_dir`.
type AppDockerEnv struct {
	Name  string
	Value string
}

// AppDockerFetch is one file the image takes from a URL, declared in the
// `fetch` list of the application's docker section — a release binary pinned to
// a version, with the target architecture in its file name.
//
// It arrives through a dedicated `FROM scratch` stage and a `COPY --from` into
// the runtime stage rather than an `ADD` in the runtime stage itself: that
// keeps the runtime stage free of both `RUN` (a shell-less base image stays
// buildable) and `ARG` (no build-cache invalidation of the layers below), and
// it is the only shape in which a per-architecture checksum can be selected at
// all — see Variants.
type AppDockerFetch struct {
	// Name identifies the entry. It is the suffix of the Dockerfile stage
	// (fetch-<Name>) and the file name inside it.
	Name string
	// URL is the source of the file. It may reference the target platform
	// (${TARGETARCH}, ${TARGETOS}), which the fetch stage declares as ARGs.
	URL string
	// To is the absolute destination path in the image.
	To string
	// Mode is the octal file mode the fetched file gets ("0755"). Empty falls
	// back to DefaultDockerFetchMode.
	Mode string
	// Checksum is the expected digest of the file ("sha256:…") when one digest
	// covers every build. Empty when the entry declares none, or when the
	// digest depends on the architecture — then see Variants.
	Checksum string
	// Variants are the per-platform digests, in the order the platforms are
	// declared. Empty unless the entry declares a digest per platform.
	Variants []AppDockerFetchVariant
}

// AppDockerFetchVariant is one per-architecture variant of a fetch entry: the
// same URL (the architecture reaches it through ${TARGETARCH}) with the digest
// of that architecture's file.
type AppDockerFetchVariant struct {
	// Arch is the architecture of the variant ("amd64") — the value
	// TARGETARCH takes for it, and the suffix of the variant's stage.
	Arch string
	// Digest is the expected digest of the file for this architecture.
	Digest string
}

// AppDockerStage is one auxiliary build stage of an application image,
// declared in the `stages` list of its docker section — a tool that no package
// manager provides (a CLI from npm, an archive that has to be unpacked) is
// installed there and copied into the runtime stage.
//
// The stage is pinned to the platform of the build host, see StageFrom.
type AppDockerStage struct {
	// Name identifies the stage. It is the suffix of the Dockerfile stage
	// (stage-<Name>).
	Name string
	// Image is the base image of the stage.
	Image string
	// Workdir is the working directory of the stage. Empty means no WORKDIR
	// instruction.
	Workdir string
	// Run are the commands of the stage, one RUN instruction each, in
	// declaration order.
	Run []string
	// Copy are the paths the runtime stage takes from this stage. At least
	// one — a stage nothing is copied from would build and be discarded.
	Copy []AppDockerCopy
}

// Generated shapes of the project-declared additions to an application image.
const (
	// DefaultDockerFetchMode is the file mode a fetched file gets when the
	// entry declares none. A fetch entry carries a tool binary, and `ADD`
	// from a URL leaves the file non-executable.
	DefaultDockerFetchMode = "0755"
	// DockerFetchDir is the directory a fetch stage puts its file in. The file
	// is named after the entry, so the runtime stage knows the path without
	// parsing the URL.
	DockerFetchDir = "/fetched"
	// DockerFetchStagePrefix and DockerToolStagePrefix are the Dockerfile
	// stage names derived from the `fetch` and `stages` keys, in the same way
	// `web-<name>` is derived from the web section.
	DockerFetchStagePrefix = "fetch-"
	DockerToolStagePrefix  = "stage-"
	// dockerEnvIndent is the continuation indentation of the generated ENV
	// instruction: the width of "ENV ".
	dockerEnvIndent = "    "
	// DockerTargetArchArg and DockerTargetOSArg are the predefined BuildKit
	// build arguments the generated stages declare, so that a command or a URL
	// inside a stage can reference the architecture of the target.
	DockerTargetArchArg = "TARGETARCH"
	DockerTargetOSArg   = "TARGETOS"
)

// EnvDirective renders the ENV instruction of the runtime stage: a single
// instruction (one layer) with one variable per line, in declaration order.
// Returns an empty string when none is declared.
func (c AppDockerConfig) EnvDirective() string {
	if len(c.Env) == 0 {
		return ""
	}

	pairs := make([]string, 0, len(c.Env))

	for _, e := range c.Env {
		pairs = append(pairs, e.Name+`="`+e.Value+`"`)
	}

	return "ENV " + strings.Join(pairs, " \\\n"+dockerEnvIndent)
}

// StageName is the name of this entry's fetch stage in the Dockerfile.
func (f AppDockerFetch) StageName() string {
	return DockerFetchStagePrefix + f.Name
}

// Path is the path of the fetched file inside its stage.
func (f AppDockerFetch) Path() string {
	return DockerFetchDir + "/" + f.Name
}

// FileMode is the octal mode the fetched file gets, applying the default.
func (f AppDockerFetch) FileMode() string {
	if f.Mode == "" {
		return DefaultDockerFetchMode
	}

	return f.Mode
}

// PerPlatform reports whether the digest of the file depends on the
// architecture, i.e. whether the entry generates a stage per variant plus a
// selector instead of one stage.
func (f AppDockerFetch) PerPlatform() bool {
	return len(f.Variants) > 0
}

// VariantStageName is the stage name of one variant.
func (f AppDockerFetch) VariantStageName(arch string) string {
	return f.StageName() + "-" + arch
}

// SelectorImage renders the base of the stage that picks the variant of the
// architecture being built: the stage name with TARGETARCH in it.
//
// The predefined platform arguments are available in the global scope, which is
// where a FROM is resolved, so the selector needs no ARG of its own. BuildKit
// builds only the stage a reference reaches, so a build for one architecture
// downloads one file.
func (f AppDockerFetch) SelectorImage() string {
	return f.StageName() + "-$" + DockerTargetArchArg
}

// AddDirective renders the ADD instruction of a fetch stage for the given
// digest (empty when the entry declares none).
//
// The mode is set by the instruction itself: `ADD` from a URL leaves the file
// non-executable, and a `RUN chmod` would need a shell the stage does not have.
func (f AppDockerFetch) AddDirective(digest string) string {
	flags := ""

	if digest != "" {
		flags += " --checksum=" + digest
	}

	flags += " --chmod=" + f.FileMode()

	return "ADD" + flags + " " + f.URL + " " + f.Path()
}

// StageName is the name of this stage in the Dockerfile.
func (s AppDockerStage) StageName() string {
	return DockerToolStagePrefix + s.Name
}

// StageFrom renders the FROM instruction of an auxiliary stage.
//
// The stage is pinned to the platform of the build host: a multi-platform build
// would otherwise run it once per target platform, under emulation — the
// invariant the cross-compiling builder stage exists for. Anything
// architecture-dependent inside the stage takes the target from TARGETARCH,
// which the stage declares, and so has to pass it explicitly.
func (s AppDockerStage) StageFrom() string {
	return "FROM --platform=$BUILDPLATFORM " + s.Image + " AS " + s.StageName()
}

// WorkdirDirective renders the WORKDIR instruction of an auxiliary stage, or an
// empty string when it declares none.
func (s AppDockerStage) WorkdirDirective() string {
	if s.Workdir == "" {
		return ""
	}

	return "WORKDIR " + s.Workdir
}

// HasRuntimeAdditions reports whether the application declares any of the
// project-declared additions to its image — `copy`, `env`, `fetch` or
// `stages`. All four shape the Dockerfile of the application and nothing else,
// so declaring one for an application that builds no image generates nothing
// at all.
func (c AppDockerConfig) HasRuntimeAdditions() bool {
	return len(c.Copy) > 0 || len(c.Env) > 0 || len(c.Fetch) > 0 || len(c.Stages) > 0
}
