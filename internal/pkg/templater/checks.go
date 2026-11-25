package templater

// ChecksRender is the context the shared checksJob fragment renders with: the
// root template parameters plus which of the two workflows is being rendered.
//
// The parameters are embedded rather than reached through a field, so the body
// of the fragment reads the same as it did when it was inline in ci_cd.yml
// (.GithubCI, .RegistryType, .GoLangVersion), and so that it can hand itself
// to the privateModulesAccess fragment, which wants .GoPrivate and
// .PrivateReposTokenSecretName.
type ChecksRender struct {
	GeneratorParams
	// PullRequest is true when the fragment renders into pr-checks.yml. It is
	// the only thing that differs between the two renderings, which is what
	// keeps the checks themselves from drifting apart.
	PullRequest bool
}

// prCheckWorkflowPath is the generated workflow that runs the declared checks
// on a pull request. It exists only for a project that declares checks: see
// GetMainTemplates, which drops it from the output set otherwise.
const prCheckWorkflowPath = ".github/workflows/pr-checks.yml"

// registryLoginIfPush is the condition of the registry login steps of the
// checks job on push: the matrix leg says whether this check needs the
// registry, and a push carries the repository's secrets by definition.
const registryLoginIfPush = "matrix.registry_login"

// registryLoginIfPullRequest is the same condition on a pull request, where
// the credential may be absent (a fork sees no secrets). It reads the
// credential through the job variable the fragment declares — see the comment
// there on why not secrets directly.
const registryLoginIfPullRequest = "matrix.registry_login && env.REGISTRY_CREDENTIAL != ''"

// The secrets the registry login steps authenticate with, one per
// main.registry_type. The fragment reads one of them only to tell whether this
// run has credentials at all, so for a registry whose login takes several it
// is the one without which none of the others is usable.
//
//nolint:gosec // G101: these are the names of CI secrets, not credentials
const (
	ghcrTokenSecret        = "GHCR_TOKEN"
	awsAccessKeyIDSecret   = "AWS_ACCESS_KEY_ID"
	registryPasswordSecret = "REGISTRY_PASSWORD"
)

// ChecksOnPush returns the render context of the checks job of ci_cd.yml.
func (g GeneratorParams) ChecksOnPush() ChecksRender {
	return ChecksRender{GeneratorParams: g}
}

// ChecksOnPullRequest returns the render context of the checks job of
// pr-checks.yml.
func (g GeneratorParams) ChecksOnPullRequest() ChecksRender {
	return ChecksRender{GeneratorParams: g, PullRequest: true}
}

// RegistryLoginIf returns the condition of the registry login steps of the
// checks job, as the expression to put inside ${{ }}.
func (c ChecksRender) RegistryLoginIf() string {
	if c.PullRequest {
		return registryLoginIfPullRequest
	}

	return registryLoginIfPush
}

// RegistryCredentialSecret returns the name of the secret whose presence the
// pull-request checks job takes as "this run can log in to the registry".
//
// Unknown registry types fall back to the self-hosted name: main.registry_type
// is validated on config load, so reaching this with anything else would mean
// a new type was added without being wired in here — and the fallback keeps
// the generated workflow valid YAML while the login step it guards is one the
// same unknown type would not have rendered.
func (c ChecksRender) RegistryCredentialSecret() string {
	switch c.RegistryType {
	case "github":
		return ghcrTokenSecret
	case "aws":
		return awsAccessKeyIDSecret
	default:
		return registryPasswordSecret
	}
}
