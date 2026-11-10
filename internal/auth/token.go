package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	DefaultIssuer    = "petfinder-api"
	DefaultAudience  = "petfinder"
	DefaultAccessTTL = 15 * time.Minute
	MinSecretBytes   = 32
)

var (
	ErrSecretTooShort  = fmt.Errorf("token signing secret must be at least %d bytes", MinSecretBytes)
	ErrInvalidTTL      = errors.New("access token TTL must be positive")
	ErrSubjectRequired = errors.New("token subject is required")
	ErrInvalidToken    = errors.New("invalid access token")
)

// TokenConfig configures access-token issuance and validation. A zero
// AccessTTL selects DefaultAccessTTL. Clock is intended for deterministic
// tests; production callers should leave it nil.
type TokenConfig struct {
	Secret    []byte
	Issuer    string
	Audience  string
	AccessTTL time.Duration
	Leeway    time.Duration
	Clock     func() time.Time
}

// Claims are the authenticated identity carried by an access token.
type Claims struct {
	Roles []Role `json:"roles"`
	jwt.RegisteredClaims
}

func (c Claims) HasRole(want Role) bool {
	for _, role := range c.Roles {
		if role == want {
			return true
		}
	}
	return false
}

// Validate performs application-specific claim validation in addition to the
// registered claim validation performed by jwt.Parser.
func (c Claims) Validate() error {
	if c.Subject == "" {
		return ErrSubjectRequired
	}
	return validateRoles(c.Roles)
}

type TokenManager struct {
	secret   []byte
	issuer   string
	audience string
	ttl      time.Duration
	leeway   time.Duration
	now      func() time.Time
}

func NewTokenManager(config TokenConfig) (*TokenManager, error) {
	if len(config.Secret) < MinSecretBytes {
		return nil, ErrSecretTooShort
	}
	if config.AccessTTL < 0 {
		return nil, ErrInvalidTTL
	}
	if config.Leeway < 0 {
		return nil, errors.New("token leeway cannot be negative")
	}

	ttl := config.AccessTTL
	if ttl == 0 {
		ttl = DefaultAccessTTL
	}
	issuer := config.Issuer
	if issuer == "" {
		issuer = DefaultIssuer
	}
	audience := config.Audience
	if audience == "" {
		audience = DefaultAudience
	}
	now := config.Clock
	if now == nil {
		now = time.Now
	}

	return &TokenManager{
		secret:   append([]byte(nil), config.Secret...),
		issuer:   issuer,
		audience: audience,
		ttl:      ttl,
		leeway:   config.Leeway,
		now:      now,
	}, nil
}

// Issue creates a signed HS256 access token for subject. Duplicate roles are
// removed before the claims are signed.
func (m *TokenManager) Issue(subject string, roles []Role) (string, error) {
	if subject == "" {
		return "", ErrSubjectRequired
	}
	if err := validateRoles(roles); err != nil {
		return "", err
	}

	now := m.now().UTC()
	id, err := newTokenID()
	if err != nil {
		return "", fmt.Errorf("create token ID: %w", err)
	}
	claims := Claims{
		Roles: uniqueRoles(roles),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{m.audience},
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        id,
		},
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// Parse verifies an access token and returns its typed claims. Token text is
// deliberately excluded from returned errors.
func (m *TokenManager) Parse(encoded string) (Claims, error) {
	if encoded == "" {
		return Claims{}, ErrInvalidToken
	}

	claims := Claims{}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithAudience(m.audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(m.leeway),
		jwt.WithTimeFunc(m.now),
	)
	token, err := parser.ParseWithClaims(encoded, &claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	})
	if err != nil || !token.Valid {
		return Claims{}, ErrInvalidToken
	}

	claims.Roles = append([]Role(nil), claims.Roles...)
	return claims, nil
}

func newTokenID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(id[:]), nil
}
