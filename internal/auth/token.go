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
	Roles   []Role `json:"roles"`
	Version int    `json:"version"`
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
	return m.IssueVersion(subject, roles, 0)
}

func (m *TokenManager) IssueVersion(subject string, roles []Role, version int) (string, error) {
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
		Roles: uniqueRoles(roles), Version: version,
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

// TicketTTL bounds how long a WebSocket ticket can be redeemed. The connection
// it opens lives until the issuing access token would have expired.
const TicketTTL = time.Minute

// ticketAudienceSuffix gives tickets their own audience so a ticket is never
// accepted as an access token and an access token is never accepted as a ticket.
const ticketAudienceSuffix = ".ws"

var ErrTicketScope = errors.New("ticket does not grant this resource")

// ticketClaims carry an access identity scoped to one conversation socket.
type ticketClaims struct {
	Roles            []Role `json:"roles"`
	Version          int    `json:"version"`
	Conversation     string `json:"cid"`
	SessionExpiresAt int64  `json:"sxp"`
	jwt.RegisteredClaims
}

func (c ticketClaims) Validate() error {
	if c.Subject == "" {
		return ErrSubjectRequired
	}
	if c.Conversation == "" || c.SessionExpiresAt == 0 {
		return ErrTicketScope
	}
	return validateRoles(c.Roles)
}

// IssueTicket creates a short-lived ticket that lets a browser, which cannot
// send an Authorization header or read HttpOnly cookies, open the WebSocket of
// one conversation without exposing its access token to JavaScript.
func (m *TokenManager) IssueTicket(access Claims, conversation string) (string, time.Time, error) {
	if access.Subject == "" {
		return "", time.Time{}, ErrSubjectRequired
	}
	if conversation == "" || access.ExpiresAt == nil {
		return "", time.Time{}, ErrTicketScope
	}
	now := m.now().UTC()
	expires := now.Add(TicketTTL)
	if access.ExpiresAt.Before(expires) {
		expires = access.ExpiresAt.Time
	}
	id, err := newTokenID()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create ticket ID: %w", err)
	}
	claims := ticketClaims{
		Roles: uniqueRoles(access.Roles), Version: access.Version, Conversation: conversation,
		SessionExpiresAt: access.ExpiresAt.Unix(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   access.Subject,
			Audience:  jwt.ClaimStrings{m.audience + ticketAudienceSuffix},
			ExpiresAt: jwt.NewNumericDate(expires),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        id,
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	return signed, expires, err
}

// ParseTicket verifies a ticket for conversation and returns the identity it
// carries. The returned expiry is that of the access token that issued it.
func (m *TokenManager) ParseTicket(encoded, conversation string) (Claims, error) {
	if encoded == "" || conversation == "" {
		return Claims{}, ErrInvalidToken
	}
	claims := ticketClaims{}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithAudience(m.audience+ticketAudienceSuffix),
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
	if err != nil || !token.Valid || claims.Conversation != conversation {
		return Claims{}, ErrInvalidToken
	}
	session := time.Unix(claims.SessionExpiresAt, 0)
	if !m.now().Before(session) {
		return Claims{}, ErrInvalidToken
	}
	return Claims{
		Roles: append([]Role(nil), claims.Roles...), Version: claims.Version,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: claims.Issuer, Subject: claims.Subject, Audience: jwt.ClaimStrings{m.audience},
			ExpiresAt: jwt.NewNumericDate(session), IssuedAt: claims.IssuedAt, ID: claims.ID,
		},
	}, nil
}
