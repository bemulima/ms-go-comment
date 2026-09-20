package handlers

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	adminuc "github.com/bemulima/ms-go-comment/internal/usecase/admin"
	commentuc "github.com/bemulima/ms-go-comment/internal/usecase/comment"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AdminHandler struct{ Service AdminService }

type policyRequest struct {
	AllowImages       bool  `json:"allow_images"`
	AllowLinks        bool  `json:"allow_links"`
	MaxDepth          int16 `json:"max_depth"`
	MaxBodyLength     int   `json:"max_body_length"`
	MaxAttachments    int16 `json:"max_attachments"`
	MaxImageBytes     int64 `json:"max_image_bytes"`
	EditWindowSeconds int   `json:"edit_window_seconds"`
}

type spaceWriteRequest struct {
	Key            string        `json:"key,omitempty"`
	Name           string        `json:"name"`
	Status         string        `json:"status,omitempty"`
	AccessMode     string        `json:"access_mode"`
	AllowedOrigins []string      `json:"allowed_origins"`
	Policy         policyRequest `json:"policy"`
}

type overridesRequest struct {
	AllowImages       *bool  `json:"allow_images"`
	AllowLinks        *bool  `json:"allow_links"`
	MaxDepth          *int16 `json:"max_depth"`
	MaxBodyLength     *int   `json:"max_body_length"`
	MaxAttachments    *int16 `json:"max_attachments"`
	MaxImageBytes     *int64 `json:"max_image_bytes"`
	EditWindowSeconds *int   `json:"edit_window_seconds"`
}

type threadUpdateRequest struct {
	Status          string           `json:"status"`
	PolicyOverrides overridesRequest `json:"policy_overrides"`
}

func (h AdminHandler) CreateSpace(w http.ResponseWriter, r *http.Request) {
	var request spaceWriteRequest
	if decodeJSON(w, r, &request) != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	mode, ok := accessMode(request.AccessMode)
	if !ok || request.Status != "" {
		WriteError(w, domain.ErrValidation)
		return
	}
	item, err := h.Service.CreateSpace(r.Context(), mustActor(r), adminuc.CreateSpaceInput{
		Key: request.Key, Name: request.Name, AccessMode: mode,
		AllowedOrigins: request.AllowedOrigins, Policy: request.Policy.domain(),
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, projectSpace(item))
}

func (h AdminHandler) GetSpace(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	item, err := h.Service.GetSpace(r.Context(), mustActor(r), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectSpace(item))
}

func (h AdminHandler) ListSpaces(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := adminPage(r)
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	var status *domain.SpaceStatus
	if raw := r.URL.Query().Get("status"); raw != "" {
		parsed, ok := spaceStatus(raw)
		if !ok {
			WriteError(w, domain.ErrValidation)
			return
		}
		status = &parsed
	}
	items, err := h.Service.ListSpaces(r.Context(), mustActor(r), repository.SpaceListQuery{Status: status, Limit: limit, Offset: offset})
	if err != nil {
		WriteError(w, err)
		return
	}
	result := make([]any, 0, len(items))
	for _, item := range items {
		result = append(result, projectSpace(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result, "limit": limit, "offset": offset})
}

func (h AdminHandler) UpdateSpace(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	var request spaceWriteRequest
	if decodeJSON(w, r, &request) != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	status, statusOK := spaceStatus(request.Status)
	mode, modeOK := accessMode(request.AccessMode)
	if !statusOK || !modeOK || request.Key != "" {
		WriteError(w, domain.ErrValidation)
		return
	}
	item, err := h.Service.UpdateSpace(r.Context(), mustActor(r), adminuc.UpdateSpaceInput{ID: id,
		Name: request.Name, Status: status, AccessMode: mode, AllowedOrigins: request.AllowedOrigins, Policy: request.Policy.domain()})
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectSpace(item))
}

func (h AdminHandler) DisableSpace(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	item, err := h.Service.DisableSpace(r.Context(), mustActor(r), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectSpace(item))
}

func (h AdminHandler) ListThreads(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := adminPage(r)
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	var spaceID *uuid.UUID
	if raw := r.URL.Query().Get("space_id"); raw != "" {
		id, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			WriteError(w, domain.ErrValidation)
			return
		}
		spaceID = &id
	}
	var status *domain.ThreadStatus
	if raw := r.URL.Query().Get("status"); raw != "" {
		parsed, ok := threadStatusValue(raw)
		if !ok {
			WriteError(w, domain.ErrValidation)
			return
		}
		status = &parsed
	}
	items, err := h.Service.ListThreads(r.Context(), mustActor(r), repository.ThreadListQuery{SpaceID: spaceID, Status: status, Limit: limit, Offset: offset})
	if err != nil {
		WriteError(w, err)
		return
	}
	result := make([]threadResponse, 0, len(items))
	for _, item := range items {
		result = append(result, projectAdminThread(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result, "limit": limit, "offset": offset})
}

func (h AdminHandler) UpdateThread(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "threadID"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	var request threadUpdateRequest
	if decodeJSON(w, r, &request) != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	status, ok := threadStatusValue(request.Status)
	if !ok {
		WriteError(w, domain.ErrValidation)
		return
	}
	view, err := h.Service.UpdateThread(r.Context(), mustActor(r), adminuc.UpdateThreadInput{ID: id, Status: status, Overrides: request.PolicyOverrides.domain()})
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectAdminThread(view))
}

func (h AdminHandler) HideComment(w http.ResponseWriter, r *http.Request) {
	h.moderateComment(w, r, h.Service.HideComment)
}

func (h AdminHandler) RestoreComment(w http.ResponseWriter, r *http.Request) {
	h.moderateComment(w, r, h.Service.RestoreComment)
}

func (h AdminHandler) moderateComment(
	w http.ResponseWriter,
	r *http.Request,
	action func(context.Context, domain.Actor, uuid.UUID) (adminuc.ModerationView, error),
) {
	id, err := uuid.Parse(chi.URLParam(r, "commentID"))
	if err != nil || !emptyRequestBody(r) {
		WriteError(w, domain.ErrValidation)
		return
	}
	view, err := action(r.Context(), mustActor(r), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectModeratedComment(view))
}

func (p policyRequest) domain() domain.Policy {
	return domain.Policy{AllowImages: p.AllowImages, AllowLinks: p.AllowLinks, MaxDepth: p.MaxDepth,
		MaxBodyLength: p.MaxBodyLength, MaxAttachments: p.MaxAttachments, MaxImageBytes: p.MaxImageBytes, EditWindowSeconds: p.EditWindowSeconds}
}

func (p overridesRequest) domain() domain.ThreadPolicyOverrides {
	return domain.ThreadPolicyOverrides{AllowImages: p.AllowImages, AllowLinks: p.AllowLinks, MaxDepth: p.MaxDepth,
		MaxBodyLength: p.MaxBodyLength, MaxAttachments: p.MaxAttachments, MaxImageBytes: p.MaxImageBytes, EditWindowSeconds: p.EditWindowSeconds}
}

func projectSpace(item domain.Space) map[string]any {
	policy := policyResponse{AllowImages: item.Policy.AllowImages, AllowLinks: item.Policy.AllowLinks,
		MaxDepth: item.Policy.MaxDepth, MaxBodyLength: item.Policy.MaxBodyLength,
		MaxAttachments: item.Policy.MaxAttachments, MaxImageBytes: item.Policy.MaxImageBytes,
		EditWindowSeconds: item.Policy.EditWindowSeconds}
	return map[string]any{"id": item.ID, "key": item.Key, "name": item.Name, "status": spaceStatusName(item.Status),
		"access_mode": accessModeName(item.AccessMode), "allowed_origins": item.AllowedOrigins, "policy": policy,
		"created_by": item.CreatedBy, "created_at": item.CreatedAt, "updated_at": item.UpdatedAt}
}

func projectAdminThread(view adminuc.ThreadView) threadResponse {
	return projectThread(commentuc.ThreadView{Thread: view.Thread, Policy: view.Policy})
}

func projectModeratedComment(view adminuc.ModerationView) commentResponse {
	return projectComment(commentuc.CommentView{Comment: view.Comment, Attachments: view.Attachments})
}

func emptyRequestBody(r *http.Request) bool {
	if r.Body == nil {
		return true
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 1))
	return err == nil && len(data) == 0
}

func adminPage(r *http.Request) (int, int, error) {
	limit, err := strconv.Atoi(defaultString(r.URL.Query().Get("limit"), strconv.Itoa(adminuc.DefaultListLimit)))
	if err != nil || limit < 1 || limit > adminuc.MaxListLimit {
		return 0, 0, domain.ErrValidation
	}
	offset, err := strconv.Atoi(defaultString(r.URL.Query().Get("offset"), "0"))
	if err != nil || offset < 0 {
		return 0, 0, domain.ErrValidation
	}
	return limit, offset, nil
}

func accessMode(raw string) (domain.AccessMode, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "authenticated":
		return domain.AccessModeAuthenticated, true
	case "context_grant":
		return domain.AccessModeContextGrant, true
	default:
		return 0, false
	}
}
func accessModeName(value domain.AccessMode) string {
	if value == domain.AccessModeContextGrant {
		return "context_grant"
	}
	return "authenticated"
}
func spaceStatus(raw string) (domain.SpaceStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "active":
		return domain.SpaceStatusActive, true
	case "disabled":
		return domain.SpaceStatusDisabled, true
	default:
		return 0, false
	}
}
func spaceStatusName(value domain.SpaceStatus) string {
	if value == domain.SpaceStatusActive {
		return "active"
	}
	return "disabled"
}
func threadStatusValue(raw string) (domain.ThreadStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "open":
		return domain.ThreadStatusOpen, true
	case "read_only":
		return domain.ThreadStatusReadOnly, true
	case "closed":
		return domain.ThreadStatusClosed, true
	case "hidden":
		return domain.ThreadStatusHidden, true
	default:
		return 0, false
	}
}
