//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const chatSubprotocol = "petfinder.chat.v1"

type chatMessage struct {
	ID               string `json:"id"`
	SenderID         string `json:"sender_id"`
	Body             string `json:"body"`
	ModerationStatus string `json:"moderation_status"`
}

type chatEvent struct {
	Type     string       `json:"type"`
	ClientID string       `json:"client_id"`
	Message  *chatMessage `json:"message"`
	Warning  string       `json:"warning"`
	Error    *struct {
		Status int `json:"status"`
	} `json:"error"`
}

func dialChat(t *testing.T, conversationID string, opts *websocket.DialOptions) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(baseURL, "http")+"/v1/conversations/"+conversationID+"/ws", opts)
	if conn != nil {
		t.Cleanup(func() { _ = conn.CloseNow() })
	}
	return conn, res, err
}

// connectChat opens a connection and proves it is subscribed: the server
// subscribes before reading commands, so an acknowledged message means later
// events for the conversation reach this connection.
func connectChat(t *testing.T, conversationID, clientID string, opts *websocket.DialOptions) *websocket.Conn {
	t.Helper()
	conn, _, err := dialChat(t, conversationID, opts)
	if err != nil {
		t.Fatal(err)
	}
	if conn.Subprotocol() != chatSubprotocol {
		t.Fatalf("negotiated subprotocol %q", conn.Subprotocol())
	}
	sendChat(t, conn, clientID, "Olá, "+clientID)
	expectEvent(t, conn, "message.ack", clientID)
	return conn
}

func sendChat(t *testing.T, conn *websocket.Conn, clientID, body string) {
	t.Helper()
	data, err := json.Marshal(map[string]string{"type": "message.send", "client_id": clientID, "body": body})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatal(err)
	}
}

func expectEvent(t *testing.T, conn *websocket.Conn, typ, clientID string) chatEvent {
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
	if event.Type != typ || event.ClientID != clientID {
		t.Fatalf("event %s, want type %q and client_id %q", data, typ, clientID)
	}
	return event
}

func TestChatBetweenAdopterAndOrganization(t *testing.T) {
	s := newAdoptionSetup(t)

	// Native clients authenticate with a header, browsers with a subprotocol.
	adopter := connectChat(t, s.conversationID, "adopter-hello", &websocket.DialOptions{
		Subprotocols: []string{chatSubprotocol},
		HTTPHeader:   http.Header{"Authorization": {"Bearer " + s.adopter.AccessToken}},
	})
	org := connectChat(t, s.conversationID, "org-hello", &websocket.DialOptions{
		Subprotocols: []string{chatSubprotocol, "bearer." + s.owner.AccessToken},
	})
	if event := expectEvent(t, adopter, "message.created", ""); event.Message.SenderID != s.owner.User.ID {
		t.Fatalf("adopter received %+v, want the organization's greeting", event.Message)
	}

	sendChat(t, adopter, "risky", "Me chama no whatsapp para combinar o pix")
	if ack := expectEvent(t, adopter, "message.ack", "risky"); ack.Warning == "" || ack.Message.ModerationStatus != "flagged" {
		t.Fatalf("flagged message acknowledged as %+v", ack)
	}
	if event := expectEvent(t, org, "message.created", ""); event.Message.ModerationStatus != "flagged" {
		t.Fatalf("organization received %+v", event.Message)
	}

	sendChat(t, adopter, "blank", "   ")
	if event := expectEvent(t, adopter, "error", "blank"); event.Error == nil || event.Error.Status != http.StatusUnprocessableEntity {
		t.Fatalf("blank message answered with %+v", event)
	}

	// Messages sent over REST reach every open connection, the sender's included.
	var sent struct {
		Message chatMessage `json:"message"`
	}
	s.owner.do(http.MethodPost, "/v1/conversations/"+s.conversationID+"/messages", map[string]any{"body": "Podemos agendar uma visita?"}, http.StatusCreated, &sent)
	for _, conn := range []*websocket.Conn{adopter, org} {
		if event := expectEvent(t, conn, "message.created", ""); event.Message.ID != sent.Message.ID {
			t.Fatalf("received %+v, want the REST message %s", event.Message, sent.Message.ID)
		}
	}

	var history struct {
		Items []chatMessage `json:"items"`
	}
	s.adopter.do(http.MethodGet, "/v1/conversations/"+s.conversationID+"/messages", nil, http.StatusOK, &history)
	if len(history.Items) != 4 || history.Items[0].ID != sent.Message.ID {
		t.Fatalf("history = %+v, want 4 messages, newest first", history.Items)
	}
}

func TestChatRejectsOutsiders(t *testing.T) {
	s := newAdoptionSetup(t)
	outsider := registerUser(t)

	for _, tt := range []struct {
		name         string
		subprotocols []string
		want         int
	}{
		{"anonymous", []string{chatSubprotocol}, http.StatusUnauthorized},
		{"outsider", []string{chatSubprotocol, "bearer." + outsider.AccessToken}, http.StatusForbidden},
	} {
		_, res, err := dialChat(t, s.conversationID, &websocket.DialOptions{Subprotocols: tt.subprotocols})
		if err == nil || res == nil || res.StatusCode != tt.want {
			t.Errorf("%s: dial err %v, response %v, want status %d", tt.name, err, res, tt.want)
		}
	}
	outsider.do(http.MethodGet, "/v1/conversations/"+s.conversationID+"/messages", nil, http.StatusForbidden, nil)
	outsider.do(http.MethodPost, "/v1/conversations/"+s.conversationID+"/messages", map[string]any{"body": "intrometido"}, http.StatusForbidden, nil)
}
