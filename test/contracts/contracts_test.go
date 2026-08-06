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
	assertContains(t, httpContract, "status: user_realtime_admin_and_internal_routes_implemented", "HTTP implementation status")
	for _, fragment := range []string{
		`admin.Post("/space/create"`, `admin.Get("/space/get/{spaceID}"`, `admin.Get("/space/list"`,
		`admin.Put("/space/update/{spaceID}"`, `admin.Delete("/space/delete/{spaceID}"`,
		`admin.Get("/thread/list"`, `admin.Put("/thread/update/{threadID}"`,
		`admin.Put("/comment/hide/{commentID}"`, `admin.Put("/comment/restore/{commentID}"`,
	} {
		assertContains(t, router, fragment, "admin router registration")
	}
	internalRouter := read(t, "internal/adapters/http/internal/router.go")
	assertContains(t, router, `router.Mount("/internal/v1"`, "internal router mount")
	for _, fragment := range []string{
		`router.Post("/access-grant/create"`, `router.Post("/thread/ensure"`, `router.Get("/thread/get-by-resource"`,
	} {
		assertContains(t, internalRouter, fragment, "internal router registration")
	}
	for _, route := range []string{
		"POST /internal/v1/access-grant/create", "POST /internal/v1/thread/ensure", "GET /internal/v1/thread/get-by-resource",
	} {
		assertContains(t, httpContract, route, "machine-readable internal route")
	}
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
		"Implemented in Backend v2", "Role and access matrix", "End-to-end private integration",
		"Deferred after Backend v2", "ms-gateway", "ms-go-filestorage",
		"comment_space", "comment_thread", "comment_outbox", "comment_access_grant", "GET /api/v1/ws", "comment.created",
		"X-Comment-Access-Grant", "X-Internal-Token", "ticket.<opaque>",
	} {
		assertContains(t, contractMap, fragment, "agent contract map")
	}
}

func TestAdminRepositoryContract(t *testing.T) {
	t.Parallel()

	repositorySource := read(t, "internal/adapters/postgres/thread_repository.go")
	for _, fragment := range []string{
		"func (r ThreadRepository) List", "$1::uuid IS NULL OR space_id=$1",
		"$2::smallint IS NULL OR status=$2", "ORDER BY created_at, id LIMIT $3 OFFSET $4",
	} {
		assertContains(t, repositorySource, fragment, "admin thread list repository")
	}
	commentRepository := read(t, "internal/adapters/postgres/comment_repository.go")
	for _, fragment := range []string{
		"func (r CommentRepository) UpdateModerationStatus", "status=$1, version=$2, sequence=$3, updated_at=$4",
		"WHERE id=$5 AND status=$6 AND version=$7", "domain.ErrModerationConflict",
	} {
		assertContains(t, commentRepository, fragment, "comment moderation repository")
	}
}

func TestHTTPGuardrailContractsStaySynchronized(t *testing.T) {
	t.Parallel()

	router := read(t, "internal/adapters/http/router.go")
	middleware := read(t, "internal/adapters/http/middleware/guardrails.go")
	config := read(t, "internal/config/config.go")
	httpContract := read(t, ".ai/contracts/http.yaml")
	for _, fragment := range []string{
		"SecurityHeaders", "RequestID", "RecoverPanics", "RateLimitActor",
	} {
		assertContains(t, router+middleware, fragment, "HTTP guardrail implementation")
	}
	for _, fragment := range []string{
		"HTTP_USER_RATE_LIMIT_RPS", "HTTP_USER_RATE_LIMIT_MAX_ACTORS", "HTTP_MAX_HEADER_BYTES",
	} {
		assertContains(t, config, fragment, "HTTP guardrail configuration")
	}
	for _, fragment := range []string{
		"bounded per-instance token bucket", "429 rate_limited", "server-generated X-Request-ID",
	} {
		assertContains(t, httpContract, fragment, "HTTP guardrail contract")
	}
}

func TestFrontendSDKContractStaysSynchronized(t *testing.T) {
	t.Parallel()

	client := read(t, "web/src/client.ts")
	realtime := read(t, "web/src/realtime.ts")
	contract := read(t, ".ai/contracts/frontend.yaml")
	for _, fragment := range []string{
		"X-Comment-Access-Grant", `credentials: "include"`, "/realtime/ticket",
	} {
		assertContains(t, client, fragment, "frontend REST implementation")
	}
	for _, fragment := range []string{"ticket.${ticket.ticket}", "listChanges", "scheduleReconnect"} {
		assertContains(t, realtime, fragment, "frontend realtime implementation")
	}
	for _, fragment := range []string{"ticket_in_url: forbidden", "identity: gateway session", "raw HTML is never rendered"} {
		assertContains(t, contract, fragment, "frontend agent contract")
	}
}

func TestWebComponentContractStaysSynchronized(t *testing.T) {
	t.Parallel()

	element := read(t, "web/src/element.ts")
	contract := read(t, ".ai/contracts/frontend.yaml")
	for _, fragment := range []string{
		"defineCommentThreadElement", "ms-comment-thread", "textContent", "accessGrant",
		"ms-comment-ready", "ms-comment-error", "ms-comment-select", "loadMore",
	} {
		assertContains(t, element, fragment, "Web Component implementation")
		assertContains(t, contract, fragment, "Web Component agent contract")
	}
	for _, fragment := range []string{
		"accessGrant property only; forbidden in markup", "nested tree rendering", "live WebSocket binding",
	} {
		assertContains(t, contract, fragment, "Web Component boundary contract")
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
