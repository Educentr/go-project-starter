package mw

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Educentr/go-project-starter-runtime/pkg/ds"
	"github.com/Educentr/go-project-starter-runtime/pkg/reqctx"
	"github.com/prometheus/client_golang/prometheus"
)

// mockActor implements ds.Actor interface for testing
type mockActor struct {
	id int64
}

func (m *mockActor) GetID() int64 {
	return m.id
}

// mockAuthorizer implements ds.Authorizer for testing
type mockAuthorizer struct {
	actor ds.Actor
	err   error
}

func (m *mockAuthorizer) Init(_ context.Context, _ []ds.Runnable, _ *prometheus.Registry) (ds.Authorizer, error) {
	return m, nil
}

func (m *mockAuthorizer) AuthRest(_ *http.Request) (ds.Actor, error) {
	return m.actor, m.err
}

func (m *mockAuthorizer) CheckCSRF(_ *http.Request) (bool, error) {
	return true, nil
}

// mockErrorHandler implements rest.RestErrorHandler for testing
type mockErrorHandler struct {
	notAuthorizedCalled bool
	notFoundCalled      bool
}

func (m *mockErrorHandler) UnexpectedError(_ context.Context, w http.ResponseWriter, _ *http.Request, _ error) {
	w.WriteHeader(http.StatusInternalServerError)
}

func (m *mockErrorHandler) NotAuthorizedError(w http.ResponseWriter, _ *http.Request) {
	m.notAuthorizedCalled = true
	w.WriteHeader(http.StatusUnauthorized)
}

func (m *mockErrorHandler) NotFoundError(w http.ResponseWriter, _ *http.Request) {
	m.notFoundCalled = true
	w.WriteHeader(http.StatusNotFound)
}

func TestHTTPServerMiddlewareAuth_Success(t *testing.T) {
	testActor := &mockActor{id: 12345}

	auth := &mockAuthorizer{actor: testActor, err: nil}
	errHandler := &mockErrorHandler{}

	middleware := HTTPServerMiddlewareAuth(context.Background(), auth, errHandler)

	var capturedActor ds.Actor
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedActor, _ = reqctx.GetActor(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	if capturedActor == nil {
		t.Error("captured actor should not be nil")
		return
	}

	if capturedActor.GetID() != testActor.GetID() {
		t.Errorf("actor ID = %d, want %d", capturedActor.GetID(), testActor.GetID())
	}
}

func TestHTTPServerMiddlewareAuth_AuthError(t *testing.T) {
	auth := &mockAuthorizer{
		actor: nil,
		err:   errors.New("invalid token"),
	}
	errHandler := &mockErrorHandler{}

	middleware := HTTPServerMiddlewareAuth(context.Background(), auth, errHandler)

	nextCalled := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	if nextCalled {
		t.Error("next handler should not be called on auth error")
	}
}
