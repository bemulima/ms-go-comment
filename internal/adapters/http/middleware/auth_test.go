package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bemulima/ms-go-comment/internal/domain"
	accessuc "github.com/bemulima/ms-go-comment/internal/usecase/access"
	"github.com/google/uuid"
)

func TestRequireActor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		userID string
		role   string
		wantOK bool
	}{
		{name: "valid", userID: uuid.NewString(), role: "student", wantOK: true},
		{name: "missing id", role: "STUDENT"},
		{name: "invalid id", userID: "not-a-uuid", role: "STUDENT"},
		{name: "missing role", userID: uuid.NewString()},
		{name: "guest", userID: uuid.NewString(), role: "guest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			writerCalled := false
			handler := RequireActor(func(w http.ResponseWriter, err error) {
				writerCalled = true
				if err != domain.ErrAuthenticationRequired {
					t.Fatalf("error = %v", err)
				}
				w.WriteHeader(http.StatusUnauthorized)
			})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				actor, ok := ActorFromContext(r.Context())
				if !ok || actor.Role != "STUDENT" {
					t.Fatalf("actor = %#v, ok=%v", actor, ok)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("X-User-ID", tt.userID)
			request.Header.Set("X-User-Role", tt.role)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if called != tt.wantOK || writerCalled == tt.wantOK {
				t.Fatalf("called=%v writerCalled=%v wantOK=%v", called, writerCalled, tt.wantOK)
			}
		})
	}
}

func TestCaptureAccessGrant(t *testing.T) {
	t.Parallel()

	handler := CaptureAccessGrant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := accessuc.GrantTokenFromContext(r.Context())
		if !ok || token != "opaque-grant" {
			t.Fatalf("grant = %q, ok=%v", token, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Comment-Access-Grant", " opaque-grant ")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestRequireInternalToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, configured, supplied string
		wantOK                     bool
	}{
		{name: "exact token", configured: "internal-secret", supplied: "internal-secret", wantOK: true},
		{name: "wrong token", configured: "internal-secret", supplied: "wrong"},
		{name: "empty configured token", supplied: "anything"},
		{name: "missing supplied token", configured: "internal-secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			var received error
			handler := RequireInternalToken(tt.configured, func(w http.ResponseWriter, err error) {
				received = err
				w.WriteHeader(http.StatusForbidden)
			})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
			request.Header.Set("X-Internal-Token", tt.supplied)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if called != tt.wantOK {
				t.Fatalf("called = %v, want %v", called, tt.wantOK)
			}
			if tt.wantOK && received != nil {
				t.Fatalf("error = %v", received)
			}
			if !tt.wantOK && received != domain.ErrInternalAuthentication {
				t.Fatalf("error = %v", received)
			}
		})
	}
}
