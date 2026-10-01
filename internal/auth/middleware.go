package auth

import (
	"net/http"
	"strings"
)

// Authenticate validates a bearer access token and adds its claims to the
// request context. Authentication failures intentionally return no details.
func (m *TokenManager) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoded, ok := bearerToken(r.Header.Values("Authorization"))
		if !ok {
			writeAuthError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		claims, err := m.Parse(encoded)
		if err != nil {
			writeAuthError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
	})
}

// RequireRole permits requests whose authenticated identity has role.
func RequireRole(role Role) func(http.Handler) http.Handler {
	return RequireAnyRole(role)
}

// RequireAnyRole permits requests whose authenticated identity has at least
// one of the supplied roles. It must be mounted after Authenticate.
func RequireAnyRole(allowed ...Role) func(http.Handler) http.Handler {
	roles := append([]Role(nil), allowed...)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				writeAuthError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			for _, role := range roles {
				if role.Valid() && claims.HasRole(role) {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeAuthError(w, http.StatusForbidden, "permission denied")
		})
	}
}

func bearerToken(values []string) (string, bool) {
	if len(values) != 1 {
		return "", false
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + message + `"}`))
}
