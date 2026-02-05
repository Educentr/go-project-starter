package mw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Educentr/go-project-starter-runtime/pkg/reqctx"
)

func TestHTTPServerMiddlewareXServerHeader(t *testing.T) {
	appName := "test-app"
	middleware := HTTPServerMiddlewareXServerHeader(context.Background(), appName)

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	expectedHeader := appName + " - " + hostname

	tests := []struct {
		name       string
		wantHeader string
	}{
		{
			name:       "sets X-Server header",
			wantHeader: expectedHeader,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			got := rec.Header().Get("X-Server")
			if got != tt.wantHeader {
				t.Errorf("X-Server header = %q, want %q", got, tt.wantHeader)
			}

			if !strings.Contains(got, appName) {
				t.Errorf("X-Server header should contain app name %q", appName)
			}
		})
	}
}

func TestHTTPServerMiddlewareXServerHeader_CallsNext(t *testing.T) {
	middleware := HTTPServerMiddlewareXServerHeader(context.Background(), "app")

	nextCalled := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Error("middleware should call next handler")
	}
}

func TestHTTPServerMiddlewareRequestStartTime(t *testing.T) {
	middleware := HTTPServerMiddlewareRequestStartTime()

	tests := []struct {
		name string
	}{
		{
			name: "sets request start time in context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now()

			var capturedStartTime time.Time
			var capturedErr error
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedStartTime, capturedErr = reqctx.GetRequestStartTime(r.Context())
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			after := time.Now()

			if capturedErr != nil {
				t.Errorf("GetRequestStartTime() error = %v, want nil", capturedErr)
				return
			}

			if capturedStartTime.IsZero() {
				t.Error("request start time should not be zero")
			}

			if capturedStartTime.Before(before) || capturedStartTime.After(after) {
				t.Errorf("request start time %v should be between %v and %v", capturedStartTime, before, after)
			}
		})
	}
}

func TestHTTPServerMiddlewareRequestStartTime_CallsNext(t *testing.T) {
	middleware := HTTPServerMiddlewareRequestStartTime()

	nextCalled := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Error("middleware should call next handler")
	}
}
