package rest

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoundTripperFunc(t *testing.T) {
	expectedResp := &http.Response{StatusCode: http.StatusOK}

	rt := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return expectedResp, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := rt.RoundTrip(req)

	if err != nil {
		t.Errorf("RoundTrip() error = %v, want nil", err)
	}

	if resp != expectedResp {
		t.Errorf("RoundTrip() response = %v, want %v", resp, expectedResp)
	}
}

func TestRoundTripperFunc_Error(t *testing.T) {
	expectedErr := errors.New("transport error")

	rt := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, expectedErr
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := rt.RoundTrip(req)

	if err != expectedErr {
		t.Errorf("RoundTrip() error = %v, want %v", err, expectedErr)
	}

	if resp != nil {
		t.Errorf("RoundTrip() response = %v, want nil", resp)
	}
}

func TestChain(t *testing.T) {
	callOrder := []string{}

	baseRT := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		callOrder = append(callOrder, "base")
		return &http.Response{StatusCode: http.StatusOK}, nil
	})

	middleware1 := func(rt http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			callOrder = append(callOrder, "mw1-before")
			resp, err := rt.RoundTrip(req)
			callOrder = append(callOrder, "mw1-after")
			return resp, err
		})
	}

	middleware2 := func(rt http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			callOrder = append(callOrder, "mw2-before")
			resp, err := rt.RoundTrip(req)
			callOrder = append(callOrder, "mw2-after")
			return resp, err
		})
	}

	chained := Chain(baseRT, middleware1, middleware2)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	_, err := chained.RoundTrip(req)

	if err != nil {
		t.Errorf("Chain() RoundTrip error = %v, want nil", err)
	}

	// Middlewares should be applied in reverse order (last added wraps first)
	// So execution order is: mw1-before -> mw2-before -> base -> mw2-after -> mw1-after
	expectedOrder := []string{"mw1-before", "mw2-before", "base", "mw2-after", "mw1-after"}

	if len(callOrder) != len(expectedOrder) {
		t.Errorf("Chain() call order length = %d, want %d", len(callOrder), len(expectedOrder))
		return
	}

	for i, call := range callOrder {
		if call != expectedOrder[i] {
			t.Errorf("Chain() call order[%d] = %q, want %q", i, call, expectedOrder[i])
		}
	}
}

func TestChain_Empty(t *testing.T) {
	baseRT := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK}, nil
	})

	chained := Chain(baseRT)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := chained.RoundTrip(req)

	if err != nil {
		t.Errorf("Chain() with no middlewares error = %v, want nil", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Chain() with no middlewares status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestChain_SingleMiddleware(t *testing.T) {
	baseRT := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK}, nil
	})

	middlewareCalled := false
	middleware := func(rt http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			middlewareCalled = true
			return rt.RoundTrip(req)
		})
	}

	chained := Chain(baseRT, middleware)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	_, _ = chained.RoundTrip(req)

	if !middlewareCalled {
		t.Error("Chain() single middleware should be called")
	}
}
