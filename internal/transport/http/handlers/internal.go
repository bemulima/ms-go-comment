package handlers

import (
	"net/http"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	accessuc "github.com/bemulima/ms-go-comment/internal/usecase/access"
	commentuc "github.com/bemulima/ms-go-comment/internal/usecase/comment"
	"github.com/google/uuid"
)

type InternalHandler struct{ Service InternalService }

type accessPermissionsRequest struct {
	Read   bool `json:"read"`
	Write  bool `json:"write"`
	Upload bool `json:"upload"`
}

type createAccessGrantRequest struct {
	Issuer           string                   `json:"issuer"`
	UserID           uuid.UUID                `json:"user_id"`
	SpaceKey         string                   `json:"space_key"`
	ResourceType     string                   `json:"resource_type"`
	ResourceID       string                   `json:"resource_id"`
	Permissions      accessPermissionsRequest `json:"permissions"`
	ExpiresInSeconds int                      `json:"expires_in_seconds"`
}

type internalThreadRequest struct {
	SpaceKey     string `json:"space_key"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
}

func (h InternalHandler) CreateAccessGrant(w http.ResponseWriter, r *http.Request) {
	var request createAccessGrantRequest
	if decodeJSON(w, r, &request) != nil {
		WriteError(w, domain.ErrInvalidAccessGrant)
		return
	}
	// Validate the absolute configuration ceiling before converting to
	// time.Duration so an extreme JSON integer cannot wrap during multiplication.
	if request.ExpiresInSeconds < 1 || request.ExpiresInSeconds > 900 {
		WriteError(w, domain.ErrInvalidAccessGrant)
		return
	}
	result, err := h.Service.CreateGrant(r.Context(), accessuc.CreateGrantInput{
		Issuer: request.Issuer, UserID: request.UserID, SpaceKey: request.SpaceKey,
		Resource:    domain.ResourceReference{Type: request.ResourceType, ID: request.ResourceID},
		Permissions: request.Permissions.domain(), TTL: time.Duration(request.ExpiresInSeconds) * time.Second,
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"grant": result.Grant, "expires_at": result.ExpiresAt,
		"permissions": projectAccessPermissions(result.Permissions),
	})
}

func (h InternalHandler) EnsureThread(w http.ResponseWriter, r *http.Request) {
	var request internalThreadRequest
	if decodeJSON(w, r, &request) != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	view, err := h.Service.EnsureThread(r.Context(), request.SpaceKey,
		domain.ResourceReference{Type: request.ResourceType, ID: request.ResourceID})
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectAccessThread(view))
}

func (h InternalHandler) GetThreadByResource(w http.ResponseWriter, r *http.Request) {
	request := internalThreadRequest{
		SpaceKey: r.URL.Query().Get("space_key"), ResourceType: r.URL.Query().Get("resource_type"),
		ResourceID: r.URL.Query().Get("resource_id"),
	}
	view, err := h.Service.GetThreadByResource(r.Context(), request.SpaceKey,
		domain.ResourceReference{Type: request.ResourceType, ID: request.ResourceID})
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectAccessThread(view))
}

func (p accessPermissionsRequest) domain() domain.AccessPermission {
	var result domain.AccessPermission
	if p.Read {
		result |= domain.AccessPermissionRead
	}
	if p.Write {
		result |= domain.AccessPermissionWrite
	}
	if p.Upload {
		result |= domain.AccessPermissionUpload
	}
	return result
}

func projectAccessPermissions(value domain.AccessPermission) map[string]bool {
	return map[string]bool{
		"read":   value.Includes(domain.AccessPermissionRead),
		"write":  value.Includes(domain.AccessPermissionWrite),
		"upload": value.Includes(domain.AccessPermissionUpload),
	}
}

func projectAccessThread(view accessuc.ThreadView) threadResponse {
	return projectThread(commentuc.ThreadView{Thread: view.Thread, Policy: view.Policy})
}
