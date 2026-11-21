package specsource

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Provenance says where the bytes of one resolve came from, in a form a
// consumer can compare against the source repository without parsing anything.
//
// It is produced by the Resolver itself rather than by the channel that asked
// for the file: every channel resolves its sources through Resolver.Resolve, so
// a channel added later is recorded by the same code that records the existing
// ones.
//
// The identity of the source and the path inside it are separate fields, and
// none of them ever carries a token: a token only enters the process as the
// value of the environment variable named by `token_env`, and it is injected
// into the clone URL in memory (see gitClone), never into a Provenance.
type Provenance struct {
	// Kind is the source kind the fields below are filled for: local, http or
	// git. It is recorded explicitly so a reader knows which fields to expect
	// instead of inferring it from which ones are empty.
	Kind Kind
	// Repo identifies the source. For a git source it is the repository URL
	// with any credentials stripped (see redactRepoURL); for an http source,
	// scheme and host; for a local source it is empty — a path on the machine
	// that ran the generator identifies nothing.
	Repo string
	// Path is the path of the file inside Repo: the #subpath for git, the URL
	// path (with query, which can select the content) for http, and the path
	// as written in project.yaml for local.
	Path string
	// Ref is the effective git ref the file was fetched at — the one actually
	// used, so an override from --sourceRef is recorded rather than the config
	// default. Git sources only.
	Ref string
	// Commit is the SHA the git clone resolved to. Git sources only: a ref says
	// what was asked for, the SHA says what arrived, and the two stop agreeing
	// the moment the branch moves.
	Commit string
	// SHA256 is the hex-encoded digest of the bytes received from the source,
	// before anything downstream rewrites them ($ref rewriting in particular).
	// That is what makes it comparable with the blob in the source repository,
	// which the hash of the copy inside the project is not.
	SHA256 string
}

// IsZero reports whether nothing was recorded.
func (p Provenance) IsZero() bool {
	return p == Provenance{}
}

// Describe renders the provenance the way the source is written in
// project.yaml, for messages: "<repo>@<ref>#<path>" for git, the URL for http,
// the path for local.
func (p Provenance) Describe() string {
	switch p.Kind {
	case KindGit:
		return fmt.Sprintf("%s@%s#%s", p.Repo, p.Ref, p.Path)
	case KindHTTP:
		return p.Repo + p.Path
	case KindLocal:
		return p.Path
	default:
		return p.Path
	}
}

// provenanceOf builds the provenance of src, with the digest of the bytes that
// were received from it. Fields that the kind of source has no answer for stay
// empty.
func provenanceOf(src SpecSource, commit, sha256sum string) Provenance {
	p := Provenance{Kind: src.Kind(), Commit: commit, SHA256: sha256sum}

	switch s := src.(type) {
	case GitSource:
		p.Repo = redactRepoURL(s.Repo)
		p.Path = s.Subpath
		p.Ref = s.Ref
	case HTTPSource:
		p.Repo, p.Path = splitHTTPURL(s.URL)
	case LocalSource:
		p.Path = s.RawPath
	}

	return p
}

// redactRepoURL drops credentials from a repository URL while keeping it
// usable for a re-clone. Which half of the userinfo is a credential depends on
// the transport, so the scheme decides:
//
//   - ssh: the username ("git@") is not a secret and the URL does not clone
//     without it, so only a password is dropped;
//   - anything else (https in practice): the userinfo is dropped whole. A
//     username alone is the standard way a token is written into an https git
//     URL ("https://<token>@github.com/org/repo.git"), so keeping it would
//     record the token — and an http source already keeps only scheme and host
//     (see splitHTTPURL), which is the same rule.
func redactRepoURL(repo string) string {
	u, err := url.Parse(repo)
	if err != nil || u.User == nil {
		return repo
	}

	if u.Scheme != "ssh" {
		u.User = nil

		return u.String()
	}

	if _, hasPassword := u.User.Password(); !hasPassword {
		return repo
	}

	u.User = url.User(u.User.Username())

	return u.String()
}

// splitHTTPURL splits an http(s) URL into its origin and the path within it,
// keeping the query on the path side: a query can select which content the URL
// serves, so it belongs to the identity of the file rather than of the host.
func splitHTTPURL(raw string) (origin, path string) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", raw
	}

	origin = u.Scheme + "://" + u.Host

	path = u.EscapedPath()
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}

	return origin, path
}

// hashFile returns the hex-encoded SHA-256 of the file at path.
func hashFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return hashBytes(content), nil
}

func hashBytes(content []byte) string {
	sum := sha256.Sum256(content)

	return hex.EncodeToString(sum[:])
}

// String renders a provenance for logs. Kept close to Describe, with the
// commit appended because that is the part a log reader is after.
func (p Provenance) String() string {
	out := p.Describe()

	if p.Commit != "" {
		out += " (" + p.Commit + ")"
	}

	return strings.TrimSpace(out)
}
