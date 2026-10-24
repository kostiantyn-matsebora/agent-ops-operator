package controlplane

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthEmptyTokenRefusesWith503(t *testing.T) {
	called := false
	h := Auth("", func(w http.ResponseWriter, r *http.Request) { called = true })

	req := httptest.NewRequest("GET", "/control-plane/probes/x", nil)
	req.Header.Set("Authorization", "Bearer anything")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if called {
		t.Fatal("next handler must not run while the token is unconfigured")
	}
}

func TestAuthWrongTokenRefusesWith401(t *testing.T) {
	called := false
	h := Auth("the-right-token", func(w http.ResponseWriter, r *http.Request) { called = true })

	req := httptest.NewRequest("GET", "/control-plane/probes/x", nil)
	req.Header.Set("Authorization", "Bearer the-wrong-token")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if called {
		t.Fatal("next handler must not run on a wrong token")
	}
}

func TestAuthMissingAuthorizationHeaderRefusesWith401(t *testing.T) {
	h := Auth("the-right-token", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler must not run with no Authorization header")
	})

	req := httptest.NewRequest("GET", "/control-plane/probes/x", nil)
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthCorrectTokenPassesThrough(t *testing.T) {
	called := false
	h := Auth("the-right-token", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/control-plane/probes/x", nil)
	req.Header.Set("Authorization", "Bearer the-right-token")
	rec := httptest.NewRecorder()
	h(rec, req)

	if !called {
		t.Fatal("next handler must run on a correct token")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
