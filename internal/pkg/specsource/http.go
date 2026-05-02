package specsource

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
)

// HTTPSource fetches a single file over HTTP(S). URL is the request URL with
// the token_env query parameter (if any) already stripped. TokenEnv, if set,
// is the name of an environment variable holding a Bearer token.
type HTTPSource struct {
	URL      string
	TokenEnv string
}

func (s HTTPSource) Kind() Kind { return KindHTTP }

func (s HTTPSource) TargetFilename() string {
	u, err := url.Parse(s.URL)
	if err != nil || u.Path == "" {
		return ""
	}
	return path.Base(u.Path)
}

func (s HTTPSource) Describe() string {
	if s.TokenEnv != "" {
		return fmt.Sprintf("%s (token_env=%s)", s.URL, s.TokenEnv)
	}
	return s.URL
}

// fetch downloads the source body and writes it to dst. The token is read
// from envLookup (so tests can inject a deterministic env) and only used in
// the Authorization header — never logged.
func (s HTTPSource) fetch(ctx context.Context, client *http.Client, envLookup func(string) string, dst io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return fmt.Errorf("specsource: build http request: %w", err)
	}

	if s.TokenEnv != "" {
		token := envLookup(s.TokenEnv)
		if token == "" {
			return fmt.Errorf("%w: %s", ErrTokenEnvMissing, s.TokenEnv)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("specsource: http GET %s: %w", s.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%w: %d %s for %s", ErrHTTPStatus, resp.StatusCode, http.StatusText(resp.StatusCode), s.URL)
	}

	if _, err := io.Copy(dst, resp.Body); err != nil {
		return fmt.Errorf("specsource: read http body for %s: %w", s.URL, err)
	}
	return nil
}
