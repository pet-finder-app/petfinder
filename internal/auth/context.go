package auth

import "context"

type claimsContextKey struct{}

func WithClaims(ctx context.Context, claims Claims) context.Context {
	claims.Roles = append([]Role(nil), claims.Roles...)
	return context.WithValue(ctx, claimsContextKey{}, claims)
}

func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey{}).(Claims)
	if !ok {
		return Claims{}, false
	}
	claims.Roles = append([]Role(nil), claims.Roles...)
	return claims, true
}

func SubjectFromContext(ctx context.Context) (string, bool) {
	claims, ok := ClaimsFromContext(ctx)
	if !ok || claims.Subject == "" {
		return "", false
	}
	return claims.Subject, true
}
