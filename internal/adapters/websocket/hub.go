package websocket

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/usecase/realtime"
	"github.com/google/uuid"
)

var errConnectionLimit = errors.New("realtime connection limit reached")

type Hub struct {
	mu                    sync.RWMutex
	threads               map[uuid.UUID]map[*client]struct{}
	userCounts            map[uuid.UUID]int
	maxConnectionsPerUser int
	queueSize             int
	closed                bool
}

type client struct {
	session   realtime.Session
	send      chan []byte
	done      chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
	active    bool
	pending   [][]byte
	queueSize int
}

func NewHub(maxConnectionsPerUser, queueSize int) *Hub {
	if maxConnectionsPerUser < 1 {
		maxConnectionsPerUser = 5
	}
	if queueSize < 1 {
		queueSize = 64
	}
	return &Hub{
		threads:               make(map[uuid.UUID]map[*client]struct{}),
		userCounts:            make(map[uuid.UUID]int),
		maxConnectionsPerUser: maxConnectionsPerUser, queueSize: queueSize,
	}
}

func (h *Hub) register(session realtime.Session) (*client, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if session.UserID == uuid.Nil || session.ThreadID == uuid.Nil || !session.Permissions.Includes(domain.RealtimePermissionRead) {
		return nil, errors.New("invalid realtime session")
	}
	if h.closed {
		return nil, errors.New("realtime hub is closed")
	}
	if h.userCounts[session.UserID] >= h.maxConnectionsPerUser {
		return nil, errConnectionLimit
	}
	item := &client{
		session: session, send: make(chan []byte, h.queueSize), done: make(chan struct{}),
		pending: make([][]byte, 0), queueSize: h.queueSize,
	}
	if h.threads[session.ThreadID] == nil {
		h.threads[session.ThreadID] = make(map[*client]struct{})
	}
	h.threads[session.ThreadID][item] = struct{}{}
	h.userCounts[session.UserID]++
	return item, nil
}

func (h *Hub) unregister(item *client) {
	if item == nil {
		return
	}
	h.mu.Lock()
	if clients := h.threads[item.session.ThreadID]; clients != nil {
		if _, exists := clients[item]; exists {
			delete(clients, item)
			h.userCounts[item.session.UserID]--
			if h.userCounts[item.session.UserID] <= 0 {
				delete(h.userCounts, item.session.UserID)
			}
			if len(clients) == 0 {
				delete(h.threads, item.session.ThreadID)
			}
		}
	}
	h.mu.Unlock()
	item.closeOnce.Do(func() { close(item.done) })
}

func (h *Hub) BroadcastLifecycle(subject domain.EventSubject, payload []byte) {
	threadID, envelope, err := lifecycleEnvelope(subject, payload)
	if err != nil {
		return
	}
	h.broadcast(threadID, envelope)
}

func (h *Hub) BroadcastEphemeral(payload []byte) {
	var event struct {
		V          int       `json:"v"`
		Type       string    `json:"type"`
		EventID    uuid.UUID `json:"event_id"`
		ThreadID   uuid.UUID `json:"thread_id"`
		OccurredAt time.Time `json:"occurred_at"`
		Data       struct {
			UserID uuid.UUID `json:"user_id"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &event) != nil || event.V != protocolVersion ||
		(event.Type != "typing.started" && event.Type != "typing.stopped") || event.EventID == uuid.Nil ||
		event.ThreadID == uuid.Nil || event.Data.UserID == uuid.Nil || event.OccurredAt.IsZero() {
		return
	}
	h.broadcast(event.ThreadID, append([]byte(nil), payload...))
}

func (h *Hub) broadcast(threadID uuid.UUID, payload []byte) {
	h.mu.RLock()
	slow := make([]*client, 0)
	for item := range h.threads[threadID] {
		if !item.push(payload) {
			slow = append(slow, item)
		}
	}
	h.mu.RUnlock()
	for _, item := range slow {
		h.unregister(item)
	}
}

func (h *Hub) activate(item *client, initial ...[]byte) [][]byte {
	item.mu.Lock()
	defer item.mu.Unlock()
	result := make([][]byte, 0, len(initial)+len(item.pending))
	for _, payload := range initial {
		if len(payload) > 0 {
			result = append(result, payload)
		}
	}
	result = append(result, item.pending...)
	item.pending = nil
	item.active = true
	return result
}

func (c *client) push(payload []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.active {
		if len(c.pending) >= c.queueSize {
			return false
		}
		c.pending = append(c.pending, payload)
		return true
	}
	select {
	case c.send <- payload:
		return true
	default:
		return false
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	clients := make([]*client, 0)
	for _, byThread := range h.threads {
		for item := range byThread {
			clients = append(clients, item)
		}
	}
	h.threads = make(map[uuid.UUID]map[*client]struct{})
	h.userCounts = make(map[uuid.UUID]int)
	h.mu.Unlock()
	for _, item := range clients {
		item.closeOnce.Do(func() { close(item.done) })
	}
}

var _ interface {
	BroadcastLifecycle(domain.EventSubject, []byte)
	BroadcastEphemeral([]byte)
} = (*Hub)(nil)
