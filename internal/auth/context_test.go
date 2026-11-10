package auth

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestClaimsContext(t *testing.T) {
	t.Parallel()

	roles := []Role{RoleAdopter}
	ctx := WithClaims(context.Background(), Claims{
		Roles: roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "account-123",
		},
	})
	roles[0] = RoleAdmin

	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		t.Fatal("ClaimsFromContext() ok = false")
	}
	if !claims.HasRole(RoleAdopter) || claims.HasRole(RoleAdmin) {
		t.Fatalf("ClaimsFromContext() roles = %v", claims.Roles)
	}
	claims.Roles[0] = RoleAdmin

	again, _ := ClaimsFromContext(ctx)
	if !again.HasRole(RoleAdopter) || again.HasRole(RoleAdmin) {
		t.Fatal("ClaimsFromContext() returned mutable context storage")
	}
	subject, ok := SubjectFromContext(ctx)
	if !ok || subject != "account-123" {
		t.Fatalf("SubjectFromContext() = %q, %v", subject, ok)
	}
}

func TestClaimsContextMissing(t *testing.T) {
	t.Parallel()

	if _, ok := ClaimsFromContext(context.Background()); ok {
		t.Fatal("ClaimsFromContext() ok = true")
	}
	if _, ok := SubjectFromContext(context.Background()); ok {
		t.Fatal("SubjectFromContext() ok = true")
	}
}
