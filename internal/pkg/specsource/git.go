package specsource

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// GitTransport is the underlying transport used to talk to the remote.
type GitTransport string

const (
	GitTransportSSH   GitTransport = "ssh"
	GitTransportHTTPS GitTransport = "https"
)

// GitSource describes a file inside a git repository.
//
//	Repo      e.g. "ssh://git@github.com/org/repo.git" or "https://github.com/org/repo.git"
//	Ref       branch / tag / commit SHA. Defaults to "HEAD" when absent.
//	Subpath   path to the file inside the repo (after the URI fragment).
//	Transport ssh or https.
//	TokenEnv  name of env var with a token (https only).
type GitSource struct {
	Repo      string
	Ref       string
	Subpath   string
	Transport GitTransport
	TokenEnv  string
}

func (s GitSource) Kind() Kind { return KindGit }

func (s GitSource) TargetFilename() string {
	return path.Base(s.Subpath)
}

func (s GitSource) Describe() string {
	repo := s.Repo
	// Strip any embedded userinfo from the displayed URL just in case.
	if u, err := url.Parse(repo); err == nil && u.User != nil {
		u.User = nil
		repo = u.String()
	}
	parts := []string{repo, "@", s.Ref, "#", s.Subpath}
	out := strings.Join(parts, "")
	if s.TokenEnv != "" {
		out += fmt.Sprintf(" (token_env=%s)", s.TokenEnv)
	}
	return out
}

// cloneKey is the per-run cache key for a (repo, ref) pair.
func (s GitSource) cloneKey() string {
	return s.Repo + "\x00" + s.Ref
}

// gitClone clones repo@ref into target. It expects target to NOT exist (git
// clone creates it). The token, if any, is read from envLookup and injected
// into the URL only in-memory; it is also passed to the redacting writer so
// that any echo from git's stderr is scrubbed.
//
// When --branch fails (which happens for commit SHAs since git refuses to
// clone into a SHA directly), we retry with a full clone followed by a
// checkout.
func gitClone(ctx context.Context, src GitSource, envLookup func(string) string, target string, stderrTo io.Writer) error {
	cloneURL := src.Repo
	var token string
	if src.Transport == GitTransportHTTPS && src.TokenEnv != "" {
		token = envLookup(src.TokenEnv)
		if token == "" {
			return fmt.Errorf("%w: %s", ErrTokenEnvMissing, src.TokenEnv)
		}
		injected, err := injectUserInfo(cloneURL, "oauth2", token)
		if err != nil {
			return err
		}
		cloneURL = injected
	}

	stderrBuf := &bytes.Buffer{}
	stderr := io.MultiWriter(stderrBuf, redactingWriter(stderrTo, token))

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", src.Ref, "--single-branch", cloneURL, target)
	cmd.Env = os.Environ()
	cmd.Stderr = stderr
	cmd.Stdout = io.Discard

	err := cmd.Run()
	if err == nil {
		return nil
	}

	// Retry strategy: --branch refuses commit SHAs. Do a full clone + checkout.
	if strings.Contains(stderrBuf.String(), "Remote branch") || strings.Contains(stderrBuf.String(), "not found") {
		// Could be either: ref doesn't exist, OR ref is a SHA. Try full clone + checkout.
		if rmErr := os.RemoveAll(target); rmErr != nil {
			return fmt.Errorf("%w: cleanup before retry: %w (original error: %w)", ErrCloneFailed, rmErr, err)
		}
		if cloneErr := gitFullCloneAndCheckout(ctx, cloneURL, src.Ref, target, token, stderrTo); cloneErr != nil {
			return fmt.Errorf("%w: %s: %w", ErrCloneFailed, src.Describe(), cloneErr)
		}
		return nil
	}

	return fmt.Errorf("%w: %s: %w", ErrCloneFailed, src.Describe(), classifyGitError(stderrBuf.String(), err))
}

func gitFullCloneAndCheckout(ctx context.Context, cloneURL, ref, target, token string, stderrTo io.Writer) error {
	stderr := redactingWriter(stderrTo, token)

	clone := exec.CommandContext(ctx, "git", "clone", cloneURL, target)
	clone.Env = os.Environ()
	clone.Stderr = stderr
	clone.Stdout = io.Discard
	if err := clone.Run(); err != nil {
		return fmt.Errorf("full clone: %w", err)
	}

	checkout := exec.CommandContext(ctx, "git", "-C", target, "checkout", "--detach", ref)
	checkout.Env = os.Environ()
	checkout.Stderr = stderr
	checkout.Stdout = io.Discard
	if err := checkout.Run(); err != nil {
		return fmt.Errorf("checkout %s: %w", ref, err)
	}
	return nil
}

// classifyGitError adds a hint based on common stderr patterns.
func classifyGitError(stderr string, err error) error {
	switch {
	case strings.Contains(stderr, "Authentication failed"),
		strings.Contains(stderr, "could not read Username"),
		strings.Contains(stderr, "Permission denied"):
		return fmt.Errorf("authentication failed (check ssh-agent or token_env): %w", err)
	case strings.Contains(stderr, "Repository not found"):
		return fmt.Errorf("repository not found: %w", err)
	default:
		return err
	}
}

// injectUserInfo inserts user:pass into an HTTP(S) URL. Returns the original
// URL unchanged if scheme is not http(s).
func injectUserInfo(rawURL, user, pass string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("specsource: parse repo URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return rawURL, nil
	}
	u.User = url.UserPassword(user, pass)
	return u.String(), nil
}

// redactingWriter wraps w so any occurrences of token are replaced with
// "<redacted>" before reaching w. If token is empty, w is returned as-is.
func redactingWriter(w io.Writer, token string) io.Writer {
	if token == "" || w == nil {
		if w == nil {
			return io.Discard
		}
		return w
	}
	return &redactor{w: w, needle: []byte(token)}
}

type redactor struct {
	w      io.Writer
	needle []byte
}

func (r *redactor) Write(p []byte) (int, error) {
	if len(r.needle) == 0 {
		return r.w.Write(p)
	}
	scrubbed := bytes.ReplaceAll(p, r.needle, []byte("<redacted>"))
	if _, err := r.w.Write(scrubbed); err != nil {
		return 0, err
	}
	return len(p), nil
}

// findSubpath locates srcSubpath inside cloneDir. If the file is missing it
// returns ErrSubpathNotFound, optionally with up to 5 same-basename hints.
func findSubpath(cloneDir, subpath string) (string, error) {
	full := filepath.Join(cloneDir, subpath)
	if _, err := os.Stat(full); err == nil {
		return full, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	hints := collectSimilarFiles(cloneDir, filepath.Base(subpath), 5)
	if len(hints) == 0 {
		return "", fmt.Errorf("%w: %s", ErrSubpathNotFound, subpath)
	}
	return "", fmt.Errorf("%w: %s (similar: %s)", ErrSubpathNotFound, subpath, strings.Join(hints, ", "))
}

// collectSimilarFiles walks root and returns up to limit files whose basename
// matches name. Returns relative paths. Walk errors are intentionally ignored
// because hints are best-effort: an unreadable subtree just yields fewer
// suggestions, not a failure of the surrounding clone+resolve.
func collectSimilarFiles(root, name string, limit int) []string {
	var hits []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error { //nolint:nilerr // best-effort walk; partial results are fine
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Base(p) == name {
			rel, relErr := filepath.Rel(root, p)
			if relErr == nil {
				hits = append(hits, rel)
			}
			if len(hits) >= limit {
				return filepath.SkipAll
			}
		}
		return nil
	})
	return hits
}
