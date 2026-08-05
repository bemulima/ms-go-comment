package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bemulima/ms-go-comment/internal/domain"
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
