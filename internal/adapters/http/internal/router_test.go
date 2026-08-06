package internalhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	accessuc "github.com/bemulima/ms-go-comment/internal/usecase/access"
	"github.com/google/uuid"
)

func TestRouter_RequiresExactInternalToken(t *testing.T) {
	t.Parallel()

	for _, token := range []string{"", "wrong"} {
		request := httptest.NewRequest(http.MethodGet, "/thread/get-by-resource?space_key=course.private&resource_type=lesson&resource_id=1", nil)
		request.Header.Set("X-Internal-Token", token)
		response := httptest.NewRecorder()
		NewRouter(&stubInternalService{}, "secret").ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"error":"internal_authentication_failed"`) {
			t.Fatalf("token=%q status=%d body=%s", token, response.Code, response.Body.String())
		}
	}
}

func TestRouter_InternalRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, method, path, body, call string
		status                         int
	}{
		{name: "create grant", method: http.MethodPost, path: "/access-grant/create", body: `{"issuer":"ms-go-course","user_id":"` + uuid.NewString() + `","space_key":"course.private","resource_type":"lesson","resource_id":"lesson-1","permissions":{"read":true,"write":true,"upload":false},"expires_in_seconds":60}`, call: "create-grant", status: http.StatusCreated},
		{name: "ensure thread", method: http.MethodPost, path: "/thread/ensure", body: `{"space_key":"course.private","resource_type":"lesson","resource_id":"lesson-1"}`, call: "ensure-thread", status: http.StatusOK},
		{name: "get thread", method: http.MethodGet, path: "/thread/get-by-resource?space_key=course.private&resource_type=lesson&resource_id=lesson-1", call: "get-thread", status: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &stubInternalService{}
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			request.Header.Set("X-Internal-Token", "secret")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			NewRouter(service, "secret").ServeHTTP(response, request)
			if response.Code != tt.status || service.call != tt.call {
				t.Fatalf("status=%d call=%q body=%s", response.Code, service.call, response.Body.String())
			}
		})
	}
}

func TestRouter_CreateGrantRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	service := &stubInternalService{}
	request := httptest.NewRequest(http.MethodPost, "/access-grant/create", strings.NewReader(`{"issuer":"ms-go-course","unexpected":true}`))
	request.Header.Set("X-Internal-Token", "secret")
	response := httptest.NewRecorder()
	NewRouter(service, "secret").ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || service.call != "" || !strings.Contains(response.Body.String(), `"error":"invalid_access_grant"`) {
		t.Fatalf("status=%d call=%q body=%s", response.Code, service.call, response.Body.String())
	}
}

func TestRouter_CreateGrantRejectsUnboundedSecondsBeforeConversion(t *testing.T) {
	t.Parallel()

	service := &stubInternalService{}
	request := httptest.NewRequest(http.MethodPost, "/access-grant/create", strings.NewReader(`{"issuer":"ms-go-course","user_id":"`+uuid.NewString()+`","space_key":"course.private","resource_type":"lesson","resource_id":"lesson-1","permissions":{"read":true},"expires_in_seconds":9223372036854775807}`))
	request.Header.Set("X-Internal-Token", "secret")
	response := httptest.NewRecorder()
	NewRouter(service, "secret").ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || service.call != "" || !strings.Contains(response.Body.String(), `"error":"invalid_access_grant"`) {
		t.Fatalf("status=%d call=%q body=%s", response.Code, service.call, response.Body.String())
	}
}

type stubInternalService struct{ call string }

func (s *stubInternalService) CreateGrant(_ context.Context, input accessuc.CreateGrantInput) (accessuc.MintedGrant, error) {
	s.call = "create-grant"
	return accessuc.MintedGrant{Grant: "opaque", ExpiresAt: time.Now().UTC().Add(time.Minute), Permissions: input.Permissions}, nil
}
func (s *stubInternalService) EnsureThread(_ context.Context, _ string, resource domain.ResourceReference) (accessuc.ThreadView, error) {
	s.call = "ensure-thread"
	return internalThreadView(resource), nil
}
func (s *stubInternalService) GetThreadByResource(_ context.Context, _ string, resource domain.ResourceReference) (accessuc.ThreadView, error) {
	s.call = "get-thread"
	return internalThreadView(resource), nil
}
func internalThreadView(resource domain.ResourceReference) accessuc.ThreadView {
	now := time.Now().UTC()
	return accessuc.ThreadView{Thread: domain.Thread{ID: uuid.New(), SpaceID: uuid.New(), Resource: resource,
		Status: domain.ThreadStatusOpen, CreatedAt: now, UpdatedAt: now}, Policy: domain.DefaultPolicy()}
}
