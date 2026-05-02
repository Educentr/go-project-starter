// Package specsource parses and resolves spec sources referenced from
// project.yaml `path:` entries. A source is one of: a local filesystem path
// (backward-compatible), an HTTP(S) URL, or a git repository URI. The Resolver
// materializes every source into a real file under a per-run staging
// directory so that downstream code (CopySpecs/CopySchemas) can treat all
// sources uniformly as local files.
package specsource

import "errors"

// Kind enumerates the supported source kinds.
type Kind int

const (
	KindLocal Kind = iota
	KindHTTP
	KindGit
)

func (k Kind) String() string {
	switch k {
	case KindLocal:
		return "local"
	case KindHTTP:
		return "http"
	case KindGit:
		return "git"
	default:
		return "unknown"
	}
}

// SpecSource is a parsed source URI. Implementations are pure data; resolving
// them into local files is the Resolver's job.
type SpecSource interface {
	Kind() Kind
	// TargetFilename returns the basename under which the resolved file should
	// be saved (e.g. "users.proto" extracted from a git subpath or HTTP URL).
	TargetFilename() string
	// Describe returns a token-redacted, human-readable representation suitable
	// for log messages and error wrapping. Implementations MUST NOT include
	// raw secrets.
	Describe() string
}

// ResolvedSpec is the result of Resolver.Resolve.
type ResolvedSpec struct {
	// LocalPath is an absolute path on the local filesystem that downstream
	// code can copy or read.
	LocalPath string
	// Source is the original parsed source.
	Source SpecSource
}

// Sentinel errors. Use errors.Is to distinguish.
var (
	ErrUnknownScheme    = errors.New("specsource: unknown URI scheme")
	ErrSCPStyleURL      = errors.New("specsource: SCP-style git URL not supported, rewrite as git+ssh://")
	ErrSubpathRequired  = errors.New("specsource: git source requires a #subpath fragment")
	ErrSubpathDirectory = errors.New("specsource: directory subpaths are not supported yet")
	ErrTokenEnvMissing  = errors.New("specsource: token_env variable is empty")
	ErrSubpathNotFound  = errors.New("specsource: subpath not found in repository")
	ErrGitNotInstalled  = errors.New("specsource: 'git' executable not found in PATH")
	ErrCloneFailed      = errors.New("specsource: git clone failed")
	ErrHTTPStatus       = errors.New("specsource: HTTP request returned non-2xx status")
)
