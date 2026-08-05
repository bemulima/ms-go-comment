package contracts_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestImplementedHTTPRoutesStaySynchronized(t *testing.T) {
	t.Parallel()

	router := read(t, "internal/adapters/http/router.go")
	httpContract := read(t, ".ai/contracts/http.yaml")
	registrations := []string{
		`router.Get("/healthz"`, `api.Put("/thread/ensure"`, `api.Get("/thread/get/{threadID}"`,
		`api.Get("/comment/list"`, `api.Get("/comment/get/{commentID}"`, `api.Get("/comment/changes"`,
		`api.Post("/comment/create"`, `api.Put("/comment/update/{commentID}"`, `api.Delete("/comment/delete/{commentID}"`,
		`api.Post("/comment-attachment/upload"`, `api.Get("/comment-attachment/signed-url/{attachmentID}"`,
		`api.Delete("/comment-attachment/delete/{attachmentID}"`, `Post("/api/v1/realtime/ticket"`,
		`router.Handle("/api/v1/ws"`,
	}
	implemented := []string{
		"GET /healthz", "PUT /api/v1/thread/ensure", "GET /api/v1/thread/get/{threadID}",
		"GET /api/v1/comment/list", "GET /api/v1/comment/get/{commentID}", "GET /api/v1/comment/changes",
		"POST /api/v1/comment/create", "PUT /api/v1/comment/update/{commentID}", "DELETE /api/v1/comment/delete/{commentID}",
		"POST /api/v1/comment-attachment/upload", "GET /api/v1/comment-attachment/signed-url/{attachmentID}",
		"DELETE /api/v1/comment-attachment/delete/{attachmentID}", "POST /api/v1/realtime/ticket", "GET /api/v1/ws",
	}
	for index := range registrations {
		assertContains(t, router, registrations[index], "router registration")
		assertContains(t, httpContract, implemented[index], "machine-readable implemented route")
	}
	assertContains(t, httpContract, "status: deferred", "admin/internal implementation status")
}

func TestRealtimeContractsStaySynchronized(t *testing.T) {
	t.Parallel()

	domainEvents := read(t, "internal/domain/outbox.go")
	eventContract := read(t, ".ai/contracts/events.yaml")
	realtimeDoc := read(t, "docs/realtime-delivery.md")
	for _, subject := range []string{
		"comment.created", "comment.updated", "comment.deleted", "comment.hidden",
		"comment.restored", "comment.attachment.ready", "comment.attachment.failed", "comment.thread.updated",
	} {
		assertContains(t, domainEvents, subject, "domain event")
		assertContains(t, eventContract, subject, "event contract")
		assertContains(t, realtimeDoc, subject, "realtime documentation")
	}

	handler := read(t, "internal/adapters/websocket/handler.go")
	wsContract := read(t, ".ai/contracts/websocket.yaml")
	for _, fragment := range []string{"access_token", "ticket", "Sec-WebSocket-Protocol", "comment.v1"} {
		assertContains(t, handler+wsContract, fragment, "WebSocket credential contract")
	}
}

func TestAgentMapNamesOwnedBoundariesAndDeferredScope(t *testing.T) {
	t.Parallel()

	contractMap := read(t, "docs/agent-contract-map.md")
	for _, fragment := range []string{
		"Implemented in backend v1", "Deferred after backend v1", "ms-gateway", "ms-go-filestorage",
		"comment_space", "comment_thread", "comment_outbox", "GET /api/v1/ws", "comment.created",
	} {
		assertContains(t, contractMap, fragment, "agent contract map")
	}
}

func read(t *testing.T, name string) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve contract test path")
	}
	root := filepath.Join(filepath.Dir(source), "..", "..")
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func assertContains(t *testing.T, text, fragment, contract string) {
	t.Helper()
	if !strings.Contains(text, fragment) {
		t.Errorf("%s is missing %q", contract, fragment)
	}
}
