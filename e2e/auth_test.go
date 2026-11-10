//go:build e2e

package e2e

import (
	"net/http"
	"slices"
	"testing"
)

func TestAuthSessionLifecycle(t *testing.T) {
	u := registerUser(t)
	anonymous := client{t: t}

	var profile struct {
		ID    string   `json:"id"`
		Email string   `json:"email"`
		Roles []string `json:"roles"`
	}
	u.do(http.MethodGet, "/v1/me", nil, http.StatusOK, &profile)
	if profile.ID != u.User.ID || profile.Email != u.User.Email {
		t.Fatalf("profile %+v does not match the registered user %+v", profile, u.User)
	}
	if !slices.Contains(profile.Roles, "adopter") || !slices.Contains(profile.Roles, "advertiser") {
		t.Fatalf("new accounts should be adopters and advertisers, got %v", profile.Roles)
	}
	anonymous.do(http.MethodGet, "/v1/me", nil, http.StatusUnauthorized, nil)

	anonymous.do(http.MethodPost, "/v1/auth/register", map[string]any{
		"name": "Duplicada", "email": u.User.Email, "password": testPassword,
		"accepted_terms": true, "accepted_privacy_policy": true,
	}, http.StatusConflict, nil)
	anonymous.do(http.MethodPost, "/v1/auth/login", map[string]any{"email": u.User.Email, "password": "wrong-password-1"}, http.StatusUnauthorized, nil)

	var name struct {
		Name string `json:"name"`
	}
	u.do(http.MethodPatch, "/v1/me", map[string]any{"name": "Nome Atualizado"}, http.StatusOK, &name)
	if name.Name != "Nome Atualizado" {
		t.Fatalf("name = %q after update", name.Name)
	}

	var rotated session
	anonymous.do(http.MethodPost, "/v1/auth/refresh", map[string]any{"refresh_token": u.RefreshToken}, http.StatusOK, &rotated)
	if rotated.RefreshToken == u.RefreshToken {
		t.Fatal("refresh did not rotate the refresh token")
	}
	anonymous.do(http.MethodPost, "/v1/auth/refresh", map[string]any{"refresh_token": u.RefreshToken}, http.StatusUnauthorized, nil)

	current := client{t: t, token: rotated.AccessToken}
	current.do(http.MethodPost, "/v1/auth/logout", map[string]any{"refresh_token": rotated.RefreshToken}, http.StatusNoContent, nil)
	anonymous.do(http.MethodPost, "/v1/auth/refresh", map[string]any{"refresh_token": rotated.RefreshToken}, http.StatusUnauthorized, nil)
}

func TestRegisterValidatesInput(t *testing.T) {
	client{t: t}.do(http.MethodPost, "/v1/auth/register", map[string]any{
		"name": "E2E", "email": "e2e-" + unique() + "@example.com", "password": "short",
		"accepted_terms": true, "accepted_privacy_policy": false,
	}, http.StatusUnprocessableEntity, nil)
}
