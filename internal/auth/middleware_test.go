package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuthenticate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)
	manager := mustTokenManager(t, TokenConfig{Secret: testSecret, Clock: func() time.Time { return now }})
	encoded, err := manager.Issue("account-123", []Role{RoleAdopter, RoleAdvertiser})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	called := false
	handler := manager.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		subject, ok := SubjectFromContext(r.Context())
		if !ok || subject != "account-123" {
			t.Errorf("SubjectFromContext() = %q, %v", subject, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "bearer "+encoded)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, req)

	if !called {
		t.Fatal("next handler was not called")
	}
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestAuthenticateRejectsInvalidAuthorization(t *testing.T) {
	t.Parallel()

	manager := mustTokenManager(t, TokenConfig{Secret: testSecret})
	tests := []struct {
		name    string
		headers []string
	}{
		{name: "missing"},
		{name: "basic", headers: []string{"Basic abc"}},
		{name: "missing token", headers: []string{"Bearer"}},
		{name: "extra field", headers: []string{"Bearer abc def"}},
		{name: "malformed token", headers: []string{"Bearer abc"}},
		{name: "multiple headers", headers: []string{"Bearer abc", "Bearer def"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			called := false
			handler := manager.Authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
			req := httptest.NewRequest(http.MethodGet, "/private", nil)
			for _, value := range tt.headers {
				req.Header.Add("Authorization", value)
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, req)

			if called {
				t.Fatal("next handler was called")
			}
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", response.Header().Get("Cache-Control"))
			}
			if response.Body.String() != `{"error":"authentication required"}` {
				t.Errorf("body = %q", response.Body.String())
			}
		})
	}
}

func TestRequireAnyRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		claims     *Claims
		allowed    []Role
		wantStatus int
		wantCalled bool
	}{
		{name: "allowed first role", claims: &Claims{Roles: []Role{RoleAdopter}}, allowed: []Role{RoleAdopter, RoleAdmin}, wantStatus: http.StatusNoContent, wantCalled: true},
		{name: "allowed second role", claims: &Claims{Roles: []Role{RoleAdmin}}, allowed: []Role{RoleAdopter, RoleAdmin}, wantStatus: http.StatusNoContent, wantCalled: true},
		{name: "forbidden", claims: &Claims{Roles: []Role{RoleAdvertiser}}, allowed: []Role{RoleAdmin}, wantStatus: http.StatusForbidden},
		{name: "no allowed roles", claims: &Claims{Roles: []Role{RoleAdmin}}, wantStatus: http.StatusForbidden},
		{name: "missing claims", allowed: []Role{RoleAdmin}, wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			called := false
			handler := RequireAnyRole(tt.allowed...)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			}))
			req := httptest.NewRequest(http.MethodGet, "/private", nil)
			if tt.claims != nil {
				req = req.WithContext(WithClaims(req.Context(), *tt.claims))
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, req)

			if called != tt.wantCalled {
				t.Fatalf("called = %v, want %v", called, tt.wantCalled)
			}
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tt.wantStatus)
			}
		})
	}
}
