package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef")

func TestTokenManagerIssueAndParse(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)
	manager := mustTokenManager(t, TokenConfig{
		Secret: testSecret,
		Clock:  func() time.Time { return now },
	})

	encoded, err := manager.Issue("account-123", []Role{RoleAdopter, RoleAdvertiser, RoleAdopter})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	claims, err := manager.Parse(encoded)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if claims.Subject != "account-123" {
		t.Errorf("Subject = %q, want account-123", claims.Subject)
	}
	if claims.Issuer != DefaultIssuer {
		t.Errorf("Issuer = %q, want %q", claims.Issuer, DefaultIssuer)
	}
	if !containsAudience(claims.Audience, DefaultAudience) {
		t.Errorf("Audience = %v, want %q", claims.Audience, DefaultAudience)
	}
	if claims.ExpiresAt == nil || !claims.ExpiresAt.Equal(now.Add(DefaultAccessTTL)) {
		t.Errorf("ExpiresAt = %v, want %v", claims.ExpiresAt, now.Add(DefaultAccessTTL))
	}
	if claims.ID == "" {
		t.Error("ID is empty")
	}
	if len(claims.Roles) != 2 || !claims.HasRole(RoleAdopter) || !claims.HasRole(RoleAdvertiser) {
		t.Errorf("Roles = %v, want unique adopter and advertiser", claims.Roles)
	}
}

func TestTokenManagerUsesConfiguredIssuerAndAudience(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)
	manager := mustTokenManager(t, TokenConfig{
		Secret:   testSecret,
		Issuer:   "custom-issuer",
		Audience: "custom-audience",
		Clock:    func() time.Time { return now },
	})
	encoded, err := manager.Issue("account-123", []Role{RoleNGOMember, RoleAdmin})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	claims, err := manager.Parse(encoded)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if claims.Issuer != "custom-issuer" || !containsAudience(claims.Audience, "custom-audience") {
		t.Fatalf("issuer/audience = %q/%v", claims.Issuer, claims.Audience)
	}
	if !claims.HasRole(RoleNGOMember) || !claims.HasRole(RoleAdmin) {
		t.Fatalf("roles = %v", claims.Roles)
	}
}

func TestTokenManagerRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config TokenConfig
		want   error
	}{
		{name: "short secret", config: TokenConfig{Secret: []byte("short")}, want: ErrSecretTooShort},
		{name: "negative TTL", config: TokenConfig{Secret: testSecret, AccessTTL: -time.Second}, want: ErrInvalidTTL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewTokenManager(tt.config); !errors.Is(err, tt.want) {
				t.Fatalf("NewTokenManager() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestTokenManagerRejectsInvalidIssueClaims(t *testing.T) {
	t.Parallel()

	manager := mustTokenManager(t, TokenConfig{Secret: testSecret})
	if _, err := manager.Issue("", []Role{RoleAdopter}); !errors.Is(err, ErrSubjectRequired) {
		t.Fatalf("Issue() empty subject error = %v", err)
	}
	if _, err := manager.Issue("account-123", []Role{"superuser"}); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("Issue() invalid role error = %v", err)
	}
}

func TestTokenManagerRejectsInvalidTokens(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)
	issuer := mustTokenManager(t, TokenConfig{
		Secret:    testSecret,
		AccessTTL: time.Minute,
		Clock:     func() time.Time { return now },
	})
	encoded, err := issuer.Issue("account-123", []Role{RoleAdopter})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	tests := []struct {
		name    string
		manager *TokenManager
		token   string
	}{
		{
			name:    "expired",
			manager: mustTokenManager(t, TokenConfig{Secret: testSecret, Clock: func() time.Time { return now.Add(2 * time.Minute) }}),
			token:   encoded,
		},
		{
			name:    "wrong key",
			manager: mustTokenManager(t, TokenConfig{Secret: []byte("abcdef0123456789abcdef0123456789"), Clock: func() time.Time { return now }}),
			token:   encoded,
		},
		{
			name:    "malformed",
			manager: issuer,
			token:   "not-a-token",
		},
		{
			name:    "empty",
			manager: issuer,
			token:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tt.manager.Parse(tt.token); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("Parse() error = %v, want %v", err, ErrInvalidToken)
			}
		})
	}
}

func TestTokenManagerRejectsUntrustedClaimsAndAlgorithm(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)
	manager := mustTokenManager(t, TokenConfig{Secret: testSecret, Clock: func() time.Time { return now }})
	base := jwt.RegisteredClaims{
		Issuer:    DefaultIssuer,
		Subject:   "account-123",
		Audience:  jwt.ClaimStrings{DefaultAudience},
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
		IssuedAt:  jwt.NewNumericDate(now),
	}

	tests := []struct {
		name   string
		claims Claims
		method jwt.SigningMethod
		key    any
	}{
		{name: "wrong issuer", claims: Claims{Roles: []Role{RoleAdopter}, RegisteredClaims: withIssuer(base, "other")}, method: jwt.SigningMethodHS256, key: testSecret},
		{name: "wrong audience", claims: Claims{Roles: []Role{RoleAdopter}, RegisteredClaims: withAudience(base, "other")}, method: jwt.SigningMethodHS256, key: testSecret},
		{name: "unknown role", claims: Claims{Roles: []Role{"superuser"}, RegisteredClaims: base}, method: jwt.SigningMethodHS256, key: testSecret},
		{name: "none algorithm", claims: Claims{Roles: []Role{RoleAdopter}, RegisteredClaims: base}, method: jwt.SigningMethodNone, key: jwt.UnsafeAllowNoneSignatureType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := jwt.NewWithClaims(tt.method, tt.claims).SignedString(tt.key)
			if err != nil {
				t.Fatalf("SignedString() error = %v", err)
			}
			if _, err := manager.Parse(encoded); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("Parse() error = %v, want %v", err, ErrInvalidToken)
			}
		})
	}
}

func withIssuer(claims jwt.RegisteredClaims, issuer string) jwt.RegisteredClaims {
	claims.Issuer = issuer
	return claims
}

func withAudience(claims jwt.RegisteredClaims, audience string) jwt.RegisteredClaims {
	claims.Audience = jwt.ClaimStrings{audience}
	return claims
}

func containsAudience(audiences jwt.ClaimStrings, want string) bool {
	for _, audience := range audiences {
		if audience == want {
			return true
		}
	}
	return false
}

func mustTokenManager(t *testing.T, config TokenConfig) *TokenManager {
	t.Helper()
	manager, err := NewTokenManager(config)
	if err != nil {
		t.Fatalf("NewTokenManager() error = %v", err)
	}
	return manager
}
