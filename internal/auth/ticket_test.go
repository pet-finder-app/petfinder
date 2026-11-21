package auth

import (
	"testing"
	"time"
)

func TestTicketIsScopedAndSeparateFromAccessTokens(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)
	clock := now
	manager := mustTokenManager(t, TokenConfig{Secret: testSecret, Clock: func() time.Time { return clock }})

	access, err := manager.IssueVersion("account-123", []Role{RoleAdopter}, 3)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := manager.Parse(access)
	if err != nil {
		t.Fatal(err)
	}
	ticket, expires, err := manager.IssueTicket(claims, "conversation-a")
	if err != nil {
		t.Fatalf("IssueTicket() error = %v", err)
	}
	if !expires.Equal(now.Add(TicketTTL)) {
		t.Errorf("ticket expiry = %v, want %v", expires, now.Add(TicketTTL))
	}

	if _, err := manager.Parse(ticket); err == nil {
		t.Error("ticket accepted as an access token")
	}
	if _, err := manager.ParseTicket(access, "conversation-a"); err == nil {
		t.Error("access token accepted as a ticket")
	}
	if _, err := manager.ParseTicket(ticket, "conversation-b"); err == nil {
		t.Error("ticket accepted for another conversation")
	}

	got, err := manager.ParseTicket(ticket, "conversation-a")
	if err != nil {
		t.Fatalf("ParseTicket() error = %v", err)
	}
	if got.Subject != "account-123" || got.Version != 3 || !got.HasRole(RoleAdopter) {
		t.Errorf("ticket identity = %+v", got)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(claims.ExpiresAt.Time) {
		t.Errorf("connection expiry = %v, want access expiry %v", got.ExpiresAt, claims.ExpiresAt)
	}

	clock = now.Add(TicketTTL + time.Second)
	if _, err := manager.ParseTicket(ticket, "conversation-a"); err == nil {
		t.Error("expired ticket accepted")
	}
}

func TestTicketNeverOutlivesAccessToken(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)
	manager := mustTokenManager(t, TokenConfig{Secret: testSecret, AccessTTL: 10 * time.Second, Clock: func() time.Time { return now }})
	access, _ := manager.Issue("account-123", []Role{RoleAdopter})
	claims, _ := manager.Parse(access)
	_, expires, err := manager.IssueTicket(claims, "conversation-a")
	if err != nil {
		t.Fatal(err)
	}
	if !expires.Equal(now.Add(10 * time.Second)) {
		t.Errorf("ticket expiry = %v, want access expiry", expires)
	}
}
