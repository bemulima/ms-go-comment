package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRouter_Health(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-ID", "client-controlled")
	NewRouter(RouterDependencies{}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"service":"ms-go-comment"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if response.Header().Get("X-Request-ID") == "" || response.Header().Get("X-Request-ID") == "client-controlled" {
		t.Fatalf("request ID = %q", response.Header().Get("X-Request-ID"))
	}
	for name, want := range map[string]string{
		"Cache-Control": "no-store", "X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY",
	} {
		if got := response.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestRouter_NotFoundUsesStableError(t *testing.T) {
	response := httptest.NewRecorder()
	NewRouter(RouterDependencies{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/missing", nil))

	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error":"not_found"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRouter_RecoversPanicsWithStableJSON(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	NewRouter(RouterDependencies{WebSocketHandler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})}).ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"error":"internal_error"`) ||
		response.Header().Get("X-Request-ID") == "" {
		t.Fatalf("status=%d request_id=%q body=%s", response.Code, response.Header().Get("X-Request-ID"), response.Body.String())
	}
}

type denyActorLimiter struct{}

func (denyActorLimiter) Allow(uuid.UUID) bool { return false }
