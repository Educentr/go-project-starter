package ds

import "strings"

// DockerBuildArg is one build argument the image accepts, declared in the
// docker.build_args section of project.yaml.
//
// Values are validated on config load (see config.Docker.Parse), so they are
// safe to emit into the Dockerfile and the Makefile without further escaping.
type DockerBuildArg struct {
	// Name is the argument name, used verbatim as the Dockerfile `ARG` name,
	// as the Makefile variable name and as the `--build-arg` key.
	Name string
	// Default is the Makefile default of the argument (`<name> ?= <default>`),
	// so a local build does not have to pass it by hand. May be a make
	// function, e.g. `$(shell git rev-parse HEAD)`. Empty means the variable is
	// declared empty and only an environment variable or a command line
	// override gives it a value.
	Default string
	// Stages lists the Dockerfile stages that declare the argument, in the
	// order the stages appear in the file. Never empty — an argument declared
	// in no stage would be passed to the build and silently dropped.
	Stages []string
}

// DockerLabel is one OCI label of the final image, declared in the
// docker.labels section of project.yaml. The value may reference build
// arguments (`${IMAGE_VERSION}`); config load verifies that every referenced
// argument is declared in the final stage.
type DockerLabel struct {
	Key   string
	Value string
}

// DockerConfig holds the docker section of project.yaml: the registry prefix of
// the built images plus the build arguments and labels the generated Dockerfile
// and Makefile recipes carry.
type DockerConfig struct {
	// ImagePrefix is the registry prefix of the built images
	// (REGISTRY_SERVER in the generated Makefile).
	ImagePrefix string
	// BuildArgs are the declared build arguments, in config order.
	BuildArgs []DockerBuildArg
	// Labels are the declared image labels, sorted by key.
	Labels []DockerLabel
}

// Docker build stages a build argument can be declared in. A build argument is
// only visible in the stages that declare it, so the stage list of every
// argument decides where `ARG <name>` is emitted.
const (
	// DockerStageBuilder is the golang builder stage of the generated
	// Dockerfile — the stage that compiles the binary.
	DockerStageBuilder = "builder"
	// DockerStageFinal is the runtime stage of the generated Dockerfile — the
	// stage that carries the image labels.
	DockerStageFinal = "final"
	// MaxDockerStages is the number of stages a build argument can be declared
	// in — the generated Dockerfile has exactly two.
	MaxDockerStages = 2
	// dockerLabelIndent is the continuation indentation of the generated LABEL
	// instruction: the width of "LABEL ", so the key/value pairs line up.
	dockerLabelIndent = "      "
)

// UsedInStage reports whether the argument is declared in the given Dockerfile
// stage (DockerStageBuilder or DockerStageFinal).
func (a DockerBuildArg) UsedInStage(stage string) bool {
	for _, s := range a.Stages {
		if s == stage {
			return true
		}
	}

	return false
}

// StageArgs returns the build arguments declared in the given Dockerfile stage,
// in config order. Templates call it to emit the `ARG` block of a stage.
func (d DockerConfig) StageArgs(stage string) []DockerBuildArg {
	args := make([]DockerBuildArg, 0, len(d.BuildArgs))

	for _, arg := range d.BuildArgs {
		if arg.UsedInStage(stage) {
			args = append(args, arg)
		}
	}

	return args
}

// BuilderArgs returns the build arguments declared in the builder stage.
func (d DockerConfig) BuilderArgs() []DockerBuildArg {
	return d.StageArgs(DockerStageBuilder)
}

// FinalArgs returns the build arguments declared in the final stage.
func (d DockerConfig) FinalArgs() []DockerBuildArg {
	return d.StageArgs(DockerStageFinal)
}

// LabelDirective renders the LABEL instruction of the final stage: a single
// instruction (one layer) with one key/value pair per line. Returns an empty
// string when no label is configured, so the template emits nothing.
func (d DockerConfig) LabelDirective() string {
	if len(d.Labels) == 0 {
		return ""
	}

	pairs := make([]string, 0, len(d.Labels))

	for _, l := range d.Labels {
		pairs = append(pairs, l.Key+`="`+l.Value+`"`)
	}

	return "LABEL " + strings.Join(pairs, " \\\n"+dockerLabelIndent)
}

// BuildArgFlags renders the `--build-arg` flags of the `docker build` call in
// the generated `docker-<app>` recipe, each taking the value of the Makefile
// variable of the same name. The result starts with a space so it appends to
// the command line, and is empty when no build argument is configured.
func (d DockerConfig) BuildArgFlags() string {
	var sb strings.Builder

	for _, arg := range d.BuildArgs {
		sb.WriteString(` --build-arg ` + arg.Name + `="$(` + arg.Name + `)"`)
	}

	return sb.String()
}

// ComposeBuildArgs renders the `build.args` entries of the application service
// in docker-compose-dev.yaml. Compose cannot evaluate the Makefile defaults, so
// each argument takes the environment variable of the same name and falls back
// to empty.
func (d DockerConfig) ComposeBuildArgs() []string {
	entries := make([]string, 0, len(d.BuildArgs))

	for _, arg := range d.BuildArgs {
		entries = append(entries, arg.Name+"=${"+arg.Name+":-}")
	}

	return entries
}
