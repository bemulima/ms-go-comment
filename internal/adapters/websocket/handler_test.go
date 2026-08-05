package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	realtimeuc "github.com/bemulima/ms-go-comment/internal/usecase/realtime"
	"github.com/google/uuid"
	gorillaws "github.com/gorilla/websocket"
)

func TestHandler_HandshakeReconcilesAndRejectsDurableCommand(t *testing.T) {
	t.Parallel()

	threadID := uuid.New()
	lastSequence := int64(4)
	tickets := &fakeTicketConsumer{session: realtimeuc.Session{
		UserID: uuid.New(), ThreadID: threadID, Permissions: domain.RealtimePermissionRead | domain.RealtimePermissionWrite,
		RequestedLastSequence: &lastSequence, CurrentSequence: 7, AllowedOrigins: []string{"https://app.example"},
	}}
	typing := &fakeTypingPublisher{}
	server := httptest.NewServer(Handler{Tickets: tickets, Hub: NewHub(3, 8), Typing: typing, PingInterval: time.Hour})
	defer server.Close()

	connection, response, err := dialWebSocket(server.URL, "https://app.example")
	if err != nil {
		t.Fatalf("dial status=%v error=%v", responseStatus(response), err)
	}
	defer connection.Close()
	if connection.Subprotocol() != commentProtocol {
		t.Fatalf("subprotocol = %q", connection.Subprotocol())
	}
	ready := readFrame(t, connection)
	resync := readFrame(t, connection)
	if ready["type"] != "connection.ready" || resync["type"] != "resync_required" || resync["current_sequence"] != float64(7) {
		t.Fatalf("ready=%#v resync=%#v", ready, resync)
	}
	if err := connection.WriteJSON(map[string]any{"v": 1, "type": "comment.create", "request_id": "req-1"}); err != nil {
		t.Fatalf("write command: %v", err)
	}
	protocolError := readFrame(t, connection)
	if protocolError["type"] != "error" || protocolError["request_id"] != "req-1" {
		t.Fatalf("protocol error = %#v", protocolError)
	}
	if err := connection.WriteJSON(map[string]any{"v": 1, "type": "typing.start", "request_id": "req-2"}); err != nil {
		t.Fatalf("write typing: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && typing.count() == 0 {
		time.Sleep(time.Millisecond)
	}
	if typing.count() != 1 || !strings.Contains(string(typing.payload()), `"typing.started"`) {
		t.Fatalf("typing count=%d payload=%s", typing.count(), typing.payload())
	}
}

func TestHandler_RejectsOriginAndQueryCredentials(t *testing.T) {
	t.Parallel()

	tickets := &fakeTicketConsumer{session: realtimeuc.Session{
		UserID: uuid.New(), ThreadID: uuid.New(), Permissions: domain.RealtimePermissionRead,
		AllowedOrigins: []string{"https://allowed.example"},
	}}
	server := httptest.NewServer(Handler{Tickets: tickets, Hub: NewHub(2, 4), PingInterval: time.Hour})
	defer server.Close()

	_, response, err := dialWebSocket(server.URL, "https://evil.example")
	if err == nil || responseStatus(response) != http.StatusForbidden {
		t.Fatalf("origin response=%v error=%v", responseStatus(response), err)
	}
	dialer := gorillaws.Dialer{Subprotocols: []string{commentProtocol, "ticket.secret"}}
	_, response, err = dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"?access_token=secret", http.Header{"Origin": []string{"https://allowed.example"}})
	if err == nil || responseStatus(response) != http.StatusBadRequest {
		t.Fatalf("query response=%v error=%v", responseStatus(response), err)
	}
}

func TestHandler_EnforcesConnectionAndFrameLimits(t *testing.T) {
	t.Parallel()

	session := realtimeuc.Session{
		UserID: uuid.New(), ThreadID: uuid.New(), Permissions: domain.RealtimePermissionRead,
		AllowedOrigins: []string{"https://app.example"},
	}
	server := httptest.NewServer(Handler{
		Tickets: &fakeTicketConsumer{session: session}, Hub: NewHub(1, 4),
		MaxFrameBytes: 64, PingInterval: time.Hour,
	})
	defer server.Close()
	first, response, err := dialWebSocket(server.URL, "https://app.example")
	if err != nil {
		t.Fatalf("first dial status=%d error=%v", responseStatus(response), err)
	}
	defer first.Close()
	_ = readFrame(t, first)
	_, response, err = dialWebSocket(server.URL, "https://app.example")
	if err == nil || responseStatus(response) != http.StatusTooManyRequests {
		t.Fatalf("second dial status=%d error=%v", responseStatus(response), err)
	}
	if err := first.WriteMessage(gorillaws.TextMessage, []byte(`{"v":1,"type":"pong","padding":"`+strings.Repeat("x", 100)+`"}`)); err != nil {
		t.Fatalf("write oversized frame: %v", err)
	}
	_ = first.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := first.ReadMessage(); err == nil {
		t.Fatal("oversized frame did not close the connection")
	}
}

func TestHub_BroadcastsLifecycleOnlyToMatchingThread(t *testing.T) {
	t.Parallel()

	hub := NewHub(5, 2)
	threadID := uuid.New()
	matching, err := hub.register(realtimeuc.Session{UserID: uuid.New(), ThreadID: threadID, Permissions: domain.RealtimePermissionRead})
	if err != nil {
		t.Fatal(err)
	}
	other, err := hub.register(realtimeuc.Session{UserID: uuid.New(), ThreadID: uuid.New(), Permissions: domain.RealtimePermissionRead})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { hub.Close() })
	eventID := uuid.New()
	payload, _ := json.Marshal(map[string]any{
		"schema_version": 1, "event_id": eventID, "thread_id": threadID,
		"sequence": 3, "occurred_at": time.Now().UTC(), "comment_id": uuid.New(),
	})
	hub.BroadcastLifecycle(domain.EventCommentCreated, payload)
	initial := hub.activate(matching, []byte(`{"type":"connection.ready"}`))
	if len(initial) != 2 || string(initial[0]) != `{"type":"connection.ready"}` {
		t.Fatalf("initial frames = %q", initial)
	}
	var envelope map[string]any
	if json.Unmarshal(initial[1], &envelope) != nil || envelope["type"] != "comment.created" || envelope["sequence"] != float64(3) {
		t.Fatalf("envelope = %s", initial[1])
	}
	if frames := hub.activate(other); len(frames) != 0 {
		t.Fatalf("other thread received %q", frames)
	}
}

type fakeTicketConsumer struct {
	session realtimeuc.Session
	err     error
}

func (f *fakeTicketConsumer) Consume(context.Context, string) (realtimeuc.Session, error) {
	return f.session, f.err
}
func (f *fakeTicketConsumer) CurrentSequence(context.Context, uuid.UUID) (int64, error) {
	return f.session.CurrentSequence, f.err
}

type fakeTypingPublisher struct {
	mu       sync.Mutex
	messages [][]byte
}

func (f *fakeTypingPublisher) PublishTyping(_ context.Context, _ string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, append([]byte(nil), payload...))
	return nil
}
func (f *fakeTypingPublisher) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.messages) }
func (f *fakeTypingPublisher) payload() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.messages) == 0 {
		return nil
	}
	return append([]byte(nil), f.messages[0]...)
}

func dialWebSocket(serverURL, origin string) (*gorillaws.Conn, *http.Response, error) {
	dialer := gorillaws.Dialer{Subprotocols: []string{commentProtocol, "ticket.secret"}}
	return dialer.Dial("ws"+strings.TrimPrefix(serverURL, "http"), http.Header{"Origin": []string{origin}})
}

func readFrame(t *testing.T, connection *gorillaws.Conn) map[string]any {
	t.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	var result map[string]any
	if err := connection.ReadJSON(&result); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}
	return result
}

func responseStatus(response *http.Response) int {
	if response == nil {
		return 0
	}
	return response.StatusCode
}
