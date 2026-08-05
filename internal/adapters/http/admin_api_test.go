package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	adminuc "github.com/bemulima/ms-go-comment/internal/usecase/admin"
	"github.com/google/uuid"
)

func TestRouter_AdminRoutesRequireActor(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/v1/space/list", nil)
	NewRouter(RouterDependencies{AdminService: &stubAdminService{}}).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"error":"authentication_required"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRouter_AdminConfigurationRoutes(t *testing.T) {
	t.Parallel()

	spaceID, threadID := uuid.New(), uuid.New()
	policy := `"policy":{"allow_images":false,"allow_links":true,"max_depth":10,"max_body_length":10000,"max_attachments":4,"max_image_bytes":5242880,"edit_window_seconds":900}`
	tests := []struct {
		name, method, path, body, call string
		status                         int
	}{
		{name: "create space", method: http.MethodPost, path: "/admin/v1/space/create", body: `{"key":"course","name":"Course","access_mode":"authenticated","allowed_origins":["https://app.example"],` + policy + `}`, call: "create-space", status: http.StatusCreated},
		{name: "get space", method: http.MethodGet, path: "/admin/v1/space/get/" + spaceID.String(), call: "get-space", status: http.StatusOK},
		{name: "list spaces", method: http.MethodGet, path: "/admin/v1/space/list?limit=20&offset=0", call: "list-spaces", status: http.StatusOK},
		{name: "update space", method: http.MethodPut, path: "/admin/v1/space/update/" + spaceID.String(), body: `{"name":"Course","status":"active","access_mode":"authenticated","allowed_origins":["https://app.example"],` + policy + `}`, call: "update-space", status: http.StatusOK},
		{name: "disable space", method: http.MethodDelete, path: "/admin/v1/space/delete/" + spaceID.String(), call: "disable-space", status: http.StatusOK},
		{name: "list threads", method: http.MethodGet, path: "/admin/v1/thread/list?space_id=" + spaceID.String(), call: "list-threads", status: http.StatusOK},
		{name: "update thread", method: http.MethodPut, path: "/admin/v1/thread/update/" + threadID.String(), body: `{"status":"read_only","policy_overrides":{"allow_images":true}}`, call: "update-thread", status: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &stubAdminService{spaceID: spaceID, threadID: threadID}
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			request.Header.Set("X-User-ID", uuid.NewString())
			request.Header.Set("X-User-Role", "ADMIN")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			NewRouter(RouterDependencies{AdminService: service}).ServeHTTP(response, request)
			if response.Code != tt.status || service.call != tt.call {
				t.Fatalf("status=%d call=%q body=%s", response.Code, service.call, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "AllowImages") {
				t.Fatalf("response leaked Go field casing: %s", response.Body.String())
			}
		})
	}
}

type stubAdminService struct {
	spaceID, threadID uuid.UUID
	call              string
}

func (s *stubAdminService) space(actor domain.Actor) domain.Space {
	return domain.Space{ID: s.spaceID, Key: "course", Name: "Course", Status: domain.SpaceStatusActive,
		AccessMode: domain.AccessModeAuthenticated, Policy: domain.DefaultPolicy(), CreatedBy: actor.UserID,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
}
func (s *stubAdminService) CreateSpace(_ context.Context, actor domain.Actor, _ adminuc.CreateSpaceInput) (domain.Space, error) {
	s.call = "create-space"
	return s.space(actor), nil
}
func (s *stubAdminService) GetSpace(_ context.Context, actor domain.Actor, _ uuid.UUID) (domain.Space, error) {
	s.call = "get-space"
	return s.space(actor), nil
}
func (s *stubAdminService) ListSpaces(_ context.Context, actor domain.Actor, _ repository.SpaceListQuery) ([]domain.Space, error) {
	s.call = "list-spaces"
	return []domain.Space{s.space(actor)}, nil
}
func (s *stubAdminService) UpdateSpace(_ context.Context, actor domain.Actor, _ adminuc.UpdateSpaceInput) (domain.Space, error) {
	s.call = "update-space"
	return s.space(actor), nil
}
func (s *stubAdminService) DisableSpace(_ context.Context, actor domain.Actor, _ uuid.UUID) (domain.Space, error) {
	s.call = "disable-space"
	item := s.space(actor)
	item.Status = domain.SpaceStatusDisabled
	return item, nil
}
func (s *stubAdminService) ListThreads(_ context.Context, _ domain.Actor, _ repository.ThreadListQuery) ([]adminuc.ThreadView, error) {
	s.call = "list-threads"
	return []adminuc.ThreadView{s.threadView()}, nil
}
func (s *stubAdminService) UpdateThread(_ context.Context, _ domain.Actor, _ adminuc.UpdateThreadInput) (adminuc.ThreadView, error) {
	s.call = "update-thread"
	return s.threadView(), nil
}
func (s *stubAdminService) threadView() adminuc.ThreadView {
	return adminuc.ThreadView{Thread: domain.Thread{ID: s.threadID, SpaceID: s.spaceID,
		Resource: domain.ResourceReference{Type: "lesson", ID: "lesson-1"}, Status: domain.ThreadStatusOpen}, Policy: domain.DefaultPolicy()}
}
