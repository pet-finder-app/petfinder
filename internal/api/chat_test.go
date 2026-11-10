package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pet-finder-app/petfinder-api/internal/database"
)

func testConversationID() pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.New(), Valid: true}
}

func TestChatHubPublishSkipsOriginAndOtherRooms(t *testing.T) {
	hub := newChatHub()
	room, other := testConversationID(), testConversationID()
	origin, peer, outsider := hub.subscribe(room), hub.subscribe(room), hub.subscribe(other)

	hub.publish(room, chatEvent{Type: chatMessageCreated}, origin)

	if len(peer.events) != 1 {
		t.Fatalf("peer received %d events, want 1", len(peer.events))
	}
	if len(origin.events) != 0 || len(outsider.events) != 0 {
		t.Fatal("event delivered to the origin or another conversation")
	}
	hub.unsubscribe(room, peer)
	hub.publish(room, chatEvent{Type: chatMessageCreated}, nil)
	if len(peer.events) != 1 {
		t.Fatal("unsubscribed connection still receives events")
	}
}

func TestChatSubscriberDroppedWhenBufferFull(t *testing.T) {
	sub := newChatHub().subscribe(testConversationID())
	for range chatBuffer + 1 {
		sub.deliver(chatEvent{Type: chatMessageCreated})
	}
	select {
	case <-sub.dropped:
	default:
		t.Fatal("slow subscriber was not dropped")
	}
}

func TestSubprotocolToken(t *testing.T) {
	tests := []struct {
		header string
		want   string
		ok     bool
	}{
		{chatSubprotocol + ", bearer.a.b.c", "a.b.c", true},
		{chatSubprotocol, "", false},
		{"bearer.", "", false},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Sec-WebSocket-Protocol", tt.header)
		got, ok := subprotocolToken(r)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%q: got (%q, %v), want (%q, %v)", tt.header, got, ok, tt.want, tt.ok)
		}
	}
}

// startChat serves one conversation through the production middleware on a
// server whose timeouts are much shorter than the test connection, which
// must not apply once the connection is upgraded.
func startChat(t *testing.T, s *Server, id pgtype.UUID, expiresAt time.Time) *websocket.Conn {
	t.Helper()
	ts := httptest.NewUnstartedServer(commonMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.serveChat(w, r, id, expiresAt)
	})))
	ts.Config.ReadTimeout = 50 * time.Millisecond
	ts.Config.WriteTimeout = 50 * time.Millisecond
	ts.Start()
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http"), &websocket.DialOptions{Subprotocols: []string{chatSubprotocol}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	if conn.Subprotocol() != chatSubprotocol {
		t.Fatalf("negotiated %q", conn.Subprotocol())
	}
	return conn
}

func readEvent(t *testing.T, conn *websocket.Conn) chatEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var event chatEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	return event
}

func TestChatSocketRelaysEventsAndRejectsInvalidFrames(t *testing.T) {
	s := &Server{chat: newChatHub()}
	id := testConversationID()
	conn := startChat(t, s, id, time.Time{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"typing","client_id":"c1"}`)); err != nil {
		t.Fatal(err)
	}
	if event := readEvent(t, conn); event.Type != chatError || event.ClientID != "c1" || event.Error.Status != http.StatusBadRequest {
		t.Fatalf("unexpected event %+v", event)
	}

	// Outlive the server timeouts before publishing.
	time.Sleep(150 * time.Millisecond)
	message := database.Message{Body: "olá"}
	s.chat.publish(id, chatEvent{Type: chatMessageCreated, Message: &message}, nil)
	if event := readEvent(t, conn); event.Type != chatMessageCreated || event.Message.Body != "olá" {
		t.Fatalf("unexpected event %+v", event)
	}
}

func TestChatSocketClosesWhenTokenExpires(t *testing.T) {
	conn := startChat(t, &Server{chat: newChatHub()}, testConversationID(), time.Now().Add(100*time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err := conn.Read(ctx)
	if status := websocket.CloseStatus(err); status != chatStatusTokenExpired {
		t.Fatalf("close status %v, err %v", status, err)
	}
}

func TestChatSocketClosesOnShutdown(t *testing.T) {
	s := &Server{chat: newChatHub()}
	conn := startChat(t, s, testConversationID(), time.Time{})
	s.CloseChat()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err := conn.Read(ctx)
	if status := websocket.CloseStatus(err); status != websocket.StatusGoingAway {
		t.Fatalf("close status %v, err %v", status, err)
	}
}
