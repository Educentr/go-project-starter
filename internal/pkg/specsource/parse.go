package specsource

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseSpecSource parses a `path:` entry into a typed SpecSource. The
// function is pure (no I/O) and is safe to call from validation code paths.
//
// Backward compatibility: any value without a "://" separator is treated as a
// local path and returned as a LocalSource verbatim.
func ParseSpecSource(raw string) (SpecSource, error) {
	if raw == "" {
		return nil, fmt.Errorf("specsource: empty path")
	}

	// SCP-style git URL: "git@github.com:org/repo.git". Detect early because
	// url.Parse would otherwise mis-interpret it.
	if isSCPStyle(raw) {
		return nil, fmt.Errorf("%w: %s", ErrSCPStyleURL, raw)
	}

	if !strings.Contains(raw, "://") {
		return LocalSource{RawPath: raw}, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("specsource: parse %q: %w", raw, err)
	}

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "git+ssh", "git+https":
		return parseGitURL(u, scheme, raw)
	case "http", "https":
		return parseHTTPURL(u)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownScheme, u.Scheme)
	}
}

func parseGitURL(u *url.URL, scheme, raw string) (GitSource, error) {
	transport := GitTransportSSH
	if scheme == "git+https" {
		transport = GitTransportHTTPS
	}

	subpath := strings.TrimPrefix(u.Fragment, "/")
	if subpath == "" {
		return GitSource{}, fmt.Errorf("%w: %s", ErrSubpathRequired, raw)
	}
	if strings.HasSuffix(subpath, "/") {
		return GitSource{}, fmt.Errorf("%w: %s", ErrSubpathDirectory, subpath)
	}

	repoPath, ref := splitRefFromPath(u.Path)

	tokenEnv := u.Query().Get("token_env")
	if tokenEnv != "" && transport == GitTransportSSH {
		return GitSource{}, fmt.Errorf("%w: token_env is only valid for git+https URIs", ErrUnknownScheme)
	}

	repoURL := buildGitRepoURL(u, repoPath, transport)

	return GitSource{
		Repo:      repoURL,
		Ref:       defaultRef(ref),
		Subpath:   subpath,
		Transport: transport,
		TokenEnv:  tokenEnv,
	}, nil
}

func parseHTTPURL(u *url.URL) (HTTPSource, error) {
	tokenEnv := u.Query().Get("token_env")

	clean := *u
	clean.RawQuery = stripTokenEnvQuery(u)
	clean.Fragment = ""

	return HTTPSource{
		URL:      clean.String(),
		TokenEnv: tokenEnv,
	}, nil
}

// splitRefFromPath splits "/org/repo.git@v1.0.0" into ("/org/repo.git",
// "v1.0.0"). If no "@" is present (or it's only at index 0) returns the
// original path with empty ref.
func splitRefFromPath(p string) (repoPath, ref string) {
	idx := strings.LastIndex(p, "@")
	if idx <= 0 {
		return p, ""
	}
	return p[:idx], p[idx+1:]
}

func defaultRef(ref string) string {
	if ref == "" {
		return "HEAD"
	}
	return ref
}

// buildGitRepoURL reassembles the repo URL that we will hand to `git clone`.
// We strip the "git+" prefix from the scheme and drop the fragment + query
// (token_env in particular). For SSH we keep the userinfo (typically "git").
func buildGitRepoURL(u *url.URL, repoPath string, transport GitTransport) string {
	scheme := "https"
	if transport == GitTransportSSH {
		scheme = "ssh"
	}
	out := url.URL{
		Scheme: scheme,
		User:   u.User,
		Host:   u.Host,
		Path:   repoPath,
	}
	return out.String()
}

func stripTokenEnvQuery(u *url.URL) string {
	q := u.Query()
	q.Del("token_env")
	return q.Encode()
}

// isSCPStyle detects scp-like git URLs ("git@host:path"). The heuristic
// matches strings that have no "://" but contain a colon after a non-empty
// userinfo segment (e.g. "git@github.com:org/repo.git"). Pure local paths
// like "./foo.yaml" or "C:\foo.yaml" must NOT trigger this.
func isSCPStyle(raw string) bool {
	if strings.Contains(raw, "://") {
		return false
	}
	at := strings.Index(raw, "@")
	colon := strings.Index(raw, ":")
	if at < 0 || colon < 0 || colon < at {
		return false
	}
	// Reject Windows-style "C:\..." which has colon at index 1 and no "@"
	// before it (already filtered by the at<0 check above).
	// Require something between "@" and ":" (i.e. the host).
	host := raw[at+1 : colon]
	if host == "" {
		return false
	}
	// And something after the colon (path).
	if colon == len(raw)-1 {
		return false
	}
	return true
}
