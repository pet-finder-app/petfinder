package api

import (
	"sync"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pet-finder-app/petfinder-api/internal/database"
)

const (
	chatMessageCreated = "message.created"
	chatMessageAck     = "message.ack"
	chatError          = "error"

	// chatBuffer bounds the events queued for one connection. A client that
	// falls this far behind is disconnected instead of slowing other clients.
	chatBuffer = 32
)

// chatEvent is a server-to-client WebSocket frame.
type chatEvent struct {
	Type     string            `json:"type" enum:"message.created,message.ack,error"`
	ClientID string            `json:"client_id,omitempty" doc:"Echoes the client_id of the command being answered."`
	Message  *database.Message `json:"message,omitempty"`
	Warning  string            `json:"warning,omitempty"`
	Error    *Problem          `json:"error,omitempty"`
}

// chatHub fans out conversation events to the connections of this process.
// Running several API replicas requires a shared broker instead.
type chatHub struct {
	mu       sync.Mutex
	rooms    map[[16]byte]map[*chatSubscriber]struct{}
	done     chan struct{}
	shutdown sync.Once
}

type chatSubscriber struct {
	events  chan chatEvent
	dropped chan struct{}
	drop    sync.Once
}

func newChatHub() *chatHub {
	return &chatHub{rooms: map[[16]byte]map[*chatSubscriber]struct{}{}, done: make(chan struct{})}
}

func (h *chatHub) subscribe(conversationID pgtype.UUID) *chatSubscriber {
	sub := &chatSubscriber{events: make(chan chatEvent, chatBuffer), dropped: make(chan struct{})}
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[conversationID.Bytes]
	if room == nil {
		room = map[*chatSubscriber]struct{}{}
		h.rooms[conversationID.Bytes] = room
	}
	room[sub] = struct{}{}
	return sub
}

func (h *chatHub) unsubscribe(conversationID pgtype.UUID, sub *chatSubscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[conversationID.Bytes]
	delete(room, sub)
	if len(room) == 0 {
		delete(h.rooms, conversationID.Bytes)
	}
}

// publish delivers event to every subscriber of the conversation except one,
// usually the connection that originated it and receives an acknowledgement.
func (h *chatHub) publish(conversationID pgtype.UUID, event chatEvent, except *chatSubscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.rooms[conversationID.Bytes] {
		if sub != except {
			sub.deliver(event)
		}
	}
}

// close asks every connection to end, as the server is shutting down.
func (h *chatHub) close() {
	h.shutdown.Do(func() { close(h.done) })
}

// deliver never blocks; a full buffer marks the subscriber as dropped.
func (s *chatSubscriber) deliver(event chatEvent) {
	select {
	case s.events <- event:
	default:
		s.drop.Do(func() { close(s.dropped) })
	}
}
