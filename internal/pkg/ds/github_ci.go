package ds

// GithubCIEnvVar is one environment variable of a generated GitHub Actions
// step: declared by github_ci.build_env and rendered into the `env:` of the
// "Build and push" step of the build job.
//
// Name is validated on config load (config.GithubCIConfig.ParseBuildEnv), so
// it is safe to emit as a plain YAML key. Value is not restricted beyond being
// a single line — a workflow expression carries `:`, `#` and quotes — so the
// template quotes it (YAMLQuote) instead.
type GithubCIEnvVar struct {
	// Name is the variable name.
	Name string
	// Value is what the step gets, emitted verbatim: the runner evaluates the
	// workflow expressions in it, the generator does not.
	Value string
}
