//go:build e2e

package e2e

import (
	"context"
	"crypto/sha256"
	"net/url"
	"strings"
	"sync"
	"testing"
)

type mailbox struct {
	mu       sync.Mutex
	messages map[string]string
}

var testMail = mailbox{messages: map[string]string{}}

func (m *mailbox) Send(_ context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[to] = body
	return nil
}
func (m *mailbox) message(to string) string { m.mu.Lock(); defer m.mu.Unlock(); return m.messages[to] }

func TestRecoveryAndRevocation(t *testing.T) {
	u := registerUser(t)
	anonymous := client{t: t}
	anonymous.do("POST", "/v1/auth/forgot-password", map[string]any{"email": "unknown-" + unique() + "@example.com"}, 202, nil)
	anonymous.do("POST", "/v1/auth/forgot-password", map[string]any{"email": u.User.Email}, 202, nil)
	body := testMail.message(u.User.Email)
	fields := strings.Fields(body)
	if len(fields) == 0 {
		t.Fatal("recovery email missing")
	}
	link, err := url.Parse(fields[len(fields)-1])
	if err != nil {
		t.Fatal(err)
	}
	token := link.Query().Get("token")
	if token == "" {
		t.Fatal("recovery token missing")
	}
	password := "replacement-password-123"
	anonymous.do("POST", "/v1/auth/reset-password", map[string]any{"token": token, "password": password}, 204, nil)
	u.do("GET", "/v1/me", nil, 401, nil)
	// Simulate an old-credential login finishing after reset-session revocation.
	stale := "stale-session-" + unique() + unique()
	staleHash := sha256.Sum256([]byte(stale))
	if _, err = pool.Exec(context.Background(), "INSERT INTO refresh_sessions(user_id,token_hash,expires_at,auth_version) VALUES($1,$2,now()+interval '1 hour',0)", u.User.ID, staleHash[:]); err != nil {
		t.Fatal(err)
	}
	anonymous.do("POST", "/v1/auth/refresh", map[string]any{"refresh_token": stale}, 401, nil)

	anonymous.do("POST", "/v1/auth/refresh", map[string]any{"refresh_token": u.RefreshToken}, 401, nil)
	anonymous.do("POST", "/v1/auth/reset-password", map[string]any{"token": token, "password": password}, 422, nil)
	anonymous.do("POST", "/v1/auth/login", map[string]any{"email": u.User.Email, "password": testPassword}, 401, nil)
	var s session
	anonymous.do("POST", "/v1/auth/login", map[string]any{"email": u.User.Email, "password": password}, 200, &s)
	current := client{t: t, token: s.AccessToken}
	current.do("GET", "/v1/me", nil, 200, nil)
	current.do("DELETE", "/v1/me", map[string]any{"password": "incorrect"}, 403, nil)
	current.do("DELETE", "/v1/me", map[string]any{"password": password}, 204, nil)
	current.do("GET", "/v1/me", nil, 401, nil)
	anonymous.do("POST", "/v1/auth/refresh", map[string]any{"refresh_token": s.RefreshToken}, 401, nil)
}
