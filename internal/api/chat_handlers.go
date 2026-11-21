package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pet-finder-app/petfinder-api/internal/auth"
)

const (
	chatSubprotocol = "petfinder.chat.v1"
	// bearerSubprotocol prefixes an access token offered as a subprotocol.
	bearerSubprotocol = "bearer."
	// ticketSubprotocol prefixes a conversation-scoped ticket from the
	// ws-ticket route, for browsers that keep access tokens in HttpOnly cookies.
	ticketSubprotocol = "ticket."
	chatSendCommand   = "message.send"

	chatReadLimit    = 64 << 10
	chatPingInterval = 30 * time.Second
	chatWriteTimeout = 10 * time.Second

	// chatStatusTokenExpired tells clients to refresh the access token and reconnect.
	chatStatusTokenExpired websocket.StatusCode = 4001
)

// chatCommand is a client-to-server WebSocket frame.
type chatCommand struct {
	Type     string `json:"type" enum:"message.send"`
	ClientID string `json:"client_id,omitempty" maxLength:"64" doc:"Optional correlation ID echoed in the acknowledgement or error."`
	Body     string `json:"body" minLength:"1" maxLength:"5000"`
}

var (
	errChatTokenExpired = errors.New("access token expired")
	errChatSlowClient   = errors.New("client is not keeping up")
	errChatShutdown     = errors.New("server shutting down")
)

// chatAuth accepts the access token in the Authorization header or, because
// browsers cannot set headers on WebSocket requests, as a "bearer.<token>"
// subprotocol, or a conversation-scoped "ticket.<ticket>" subprotocol issued by
// the ws-ticket route. Query strings are avoided because proxies commonly log them.
func (s *Server) chatAuth(handler http.HandlerFunc) http.Handler {
	protected := s.protected(handler)
	ticketed := s.accountActive(handler)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			if ticket, ok := subprotocolValue(r, ticketSubprotocol); ok {
				claims, err := s.tokens.ParseTicket(ticket, r.PathValue("conversationId"))
				if err != nil {
					writeProblem(w, http.StatusUnauthorized, "Unauthorized", "invalid or expired chat ticket", nil)
					return
				}
				ticketed.ServeHTTP(w, r.WithContext(auth.WithClaims(r.Context(), claims)))
				return
			}
			if token, ok := subprotocolToken(r); ok {
				r = r.Clone(r.Context())
				r.Header.Set("Authorization", "Bearer "+token)
			}
		}
		protected.ServeHTTP(w, r)
	})
}

func subprotocolToken(r *http.Request) (string, bool) {
	return subprotocolValue(r, bearerSubprotocol)
}

func subprotocolValue(r *http.Request, prefix string) (string, bool) {
	for _, value := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, protocol := range strings.Split(value, ",") {
			if token, ok := strings.CutPrefix(strings.TrimSpace(protocol), prefix); ok && token != "" {
				return token, true
			}
		}
	}
	return "", false
}

type chatTicketResponse struct {
	Ticket      string    `json:"ticket" doc:"Offer as a ticket.<ticket> subprotocol next to petfinder.chat.v1."`
	URL         string    `json:"url" doc:"WebSocket URL of the conversation."`
	Subprotocol string    `json:"subprotocol"`
	ExpiresAt   time.Time `json:"expires_at" doc:"Redeem before this instant; the connection then lasts until the access token expires."`
}

// issueChatTicket lets cookie-based browser clients open the conversation socket
// without exposing their access token to JavaScript.
func (s *Server) issueChatTicket(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "conversationId")
	if !ok {
		invalidPath(w)
		return
	}
	conversation, err := s.queries.GetConversationByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if allowed, _ := s.conversationAllowed(r, conversation); !allowed {
		writeServiceError(w, ErrForbidden)
		return
	}
	claims, _ := auth.ClaimsFromContext(r.Context())
	ticket, expires, err := s.tokens.IssueTicket(claims, uuidString(id))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, chatTicketResponse{Ticket: ticket, URL: chatSocketURL(r, uuidString(id)), Subprotocol: chatSubprotocol, ExpiresAt: expires})
}

func chatSocketURL(r *http.Request, id string) string {
	base := strings.TrimRight(os.Getenv("PUBLIC_API_URL"), "/")
	if base == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	base = strings.Replace(strings.Replace(base, "https://", "wss://", 1), "http://", "ws://", 1)
	return base + "/v1/conversations/" + id + "/ws"
}

func (s *Server) chatSocket(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "conversationId")
	if !ok {
		invalidPath(w)
		return
	}
	conversation, err := s.queries.GetConversationByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if allowed, _ := s.conversationAllowed(r, conversation); !allowed {
		writeServiceError(w, ErrForbidden)
		return
	}
	var expiresAt time.Time
	if claims, _ := auth.ClaimsFromContext(r.Context()); claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}
	s.serveChat(w, r, id, expiresAt, func() bool { _, err := s.authorizeMessage(r, id); return err == nil })
}

// serveChat upgrades an authorized request and relays the conversation until
// the client leaves, the access token expires at expiresAt, or the server stops.
func (s *Server) serveChat(w http.ResponseWriter, r *http.Request, id pgtype.UUID, expiresAt time.Time, checks ...func() bool) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{chatSubprotocol},
		// Credentials are never sent implicitly (no cookies), so a cross-origin
		// page cannot act on a user's behalf without their access token.
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return // Accept has already written the HTTP error.
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(chatReadLimit)

	sub := s.chat.subscribe(id)
	defer s.chat.unsubscribe(id, sub)
	readErr := make(chan error, 1)
	go func() { readErr <- s.readChatCommands(r, conn, id, sub) }()

	var expires <-chan time.Time
	if !expiresAt.IsZero() {
		timer := time.NewTimer(time.Until(expiresAt))
		defer timer.Stop()
		expires = timer.C
	}
	var check func() bool
	if len(checks) > 0 {
		check = checks[0]
	}
	err = s.writeChatEvents(r.Context(), conn, sub, readErr, expires, check)
	switch {
	case errors.Is(err, errChatTokenExpired):
		_ = conn.Close(chatStatusTokenExpired, err.Error())
	case errors.Is(err, errChatSlowClient):
		_ = conn.Close(websocket.StatusPolicyViolation, err.Error())
	case errors.Is(err, errChatShutdown):
		_ = conn.Close(websocket.StatusGoingAway, err.Error())
	case err != nil && websocket.CloseStatus(err) == -1 && !errors.Is(err, io.EOF):
		slog.Warn("chat connection failed", "conversation_id", uuidString(id), "error", err)
	}
}

// writeChatEvents is the only writer to conn. It returns why the connection
// should end; the reader's error means the client is already gone.
func (s *Server) writeChatEvents(ctx context.Context, conn *websocket.Conn, sub *chatSubscriber, readErr <-chan error, expires <-chan time.Time, check func() bool) error {
	ping := time.NewTicker(chatPingInterval)
	defer ping.Stop()
	for {
		select {
		case err := <-readErr:
			return err
		case <-expires:
			return errChatTokenExpired
		case <-sub.dropped:
			return errChatSlowClient
		case <-s.chat.done:
			return errChatShutdown
		case <-ctx.Done():
			return ctx.Err()
		case event := <-sub.events:
			if check != nil && !check() {
				return errChatTokenExpired
			}
			if err := writeChatFrame(ctx, conn, event); err != nil {
				return err
			}
		case <-ping.C:
			if check != nil && !check() {
				return errChatTokenExpired
			}
			pingCtx, cancel := context.WithTimeout(ctx, chatWriteTimeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}

func writeChatFrame(ctx context.Context, conn *websocket.Conn, event chatEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, chatWriteTimeout)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, data)
}

// readChatCommands handles client frames until the connection closes. Every
// send is authorized again, so closed conversations and revoked memberships
// take effect without reconnecting.
func (s *Server) readChatCommands(r *http.Request, conn *websocket.Conn, conversationID pgtype.UUID, sub *chatSubscriber) error {
	for {
		typ, data, err := conn.Read(r.Context())
		if err != nil {
			return err
		}
		if typ != websocket.MessageText {
			sub.deliver(chatErrorEvent("", newProblem(http.StatusBadRequest, "Invalid frame", "frames must be JSON text messages", nil)))
			continue
		}
		var command chatCommand
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&command); err != nil {
			sub.deliver(chatErrorEvent("", newProblem(http.StatusBadRequest, "Invalid frame", err.Error(), nil)))
			continue
		}
		if len(command.ClientID) > 64 {
			sub.deliver(chatErrorEvent("", newProblem(http.StatusUnprocessableEntity, "Validation failed", "client_id must contain at most 64 characters", nil)))
			continue
		}
		if command.Type != chatSendCommand {
			sub.deliver(chatErrorEvent(command.ClientID, newProblem(http.StatusBadRequest, "Invalid frame", "unsupported command type", nil)))
			continue
		}
		s.handleChatSend(r, conversationID, sub, command)
	}
}

func (s *Server) handleChatSend(r *http.Request, conversationID pgtype.UUID, sub *chatSubscriber, command chatCommand) {
	moderator, err := s.authorizeMessage(r, conversationID)
	if err != nil {
		sub.deliver(chatErrorEvent(command.ClientID, serviceProblem(err)))
		return
	}
	body, ok := messageBody(command.Body)
	if !ok {
		sub.deliver(chatErrorEvent(command.ClientID, newProblem(http.StatusUnprocessableEntity, "Validation failed", messageBodyRule, nil)))
		return
	}
	response, err := s.postMessage(r, conversationID, body, moderator)
	if err != nil {
		sub.deliver(chatErrorEvent(command.ClientID, serviceProblem(err)))
		return
	}
	s.chat.publish(conversationID, chatEvent{Type: chatMessageCreated, Message: &response.Message}, sub)
	sub.deliver(chatEvent{Type: chatMessageAck, ClientID: command.ClientID, Message: &response.Message, Warning: response.Warning})
}

func chatErrorEvent(clientID string, problem Problem) chatEvent {
	return chatEvent{Type: chatError, ClientID: clientID, Error: &problem}
}
