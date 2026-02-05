package mw

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetCSRFTokenParam(t *testing.T) {
	tests := []struct {
		name        string
		queryToken  string
		headerToken string
		want        string
	}{
		{
			name:        "token from query param",
			queryToken:  "query-token-123",
			headerToken: "",
			want:        "query-token-123",
		},
		{
			name:        "token from header when query is empty",
			queryToken:  "",
			headerToken: "header-token-456",
			want:        "header-token-456",
		},
		{
			name:        "query param takes precedence over header",
			queryToken:  "query-token",
			headerToken: "header-token",
			want:        "query-token",
		},
		{
			name:        "empty when both are missing",
			queryToken:  "",
			headerToken: "",
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/test"
			if tt.queryToken != "" {
				url += "?token=" + tt.queryToken
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			if tt.headerToken != "" {
				req.Header.Set("X-CSRF-TOKEN", tt.headerToken)
			}

			got := getCSRFTokenParam(req)

			if got != tt.want {
				t.Errorf("getCSRFTokenParam() = %q, want %q", got, tt.want)
			}
		})
	}
}
