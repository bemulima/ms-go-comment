package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	httpmw "github.com/bemulima/ms-go-comment/internal/adapters/http/middleware"
	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	commentuc "github.com/bemulima/ms-go-comment/internal/usecase/comment"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type CommentHandler struct {
	Service CommentService
}

type ensureThreadRequest struct {
	SpaceKey     string `json:"space_key"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
}

type createCommentRequest struct {
	ThreadID       uuid.UUID   `json:"thread_id"`
	ParentID       *uuid.UUID  `json:"parent_id,omitempty"`
	Body           string      `json:"body"`
	AttachmentIDs  []uuid.UUID `json:"attachment_ids"`
	IdempotencyKey uuid.UUID   `json:"idempotency_key"`
}

type updateCommentRequest struct {
	Body    string `json:"body"`
	Version int    `json:"version"`
}

type deleteCommentRequest struct {
	Version int `json:"version"`
}

type policyResponse struct {
	AllowImages       bool  `json:"allow_images"`
	AllowLinks        bool  `json:"allow_links"`
	MaxDepth          int16 `json:"max_depth"`
	MaxBodyLength     int   `json:"max_body_length"`
	MaxAttachments    int16 `json:"max_attachments"`
	MaxImageBytes     int64 `json:"max_image_bytes"`
	EditWindowSeconds int   `json:"edit_window_seconds"`
}

type threadResponse struct {
	ID               uuid.UUID      `json:"id"`
	SpaceID          uuid.UUID      `json:"space_id"`
	ResourceType     string         `json:"resource_type"`
	ResourceID       string         `json:"resource_id"`
	Status           string         `json:"status"`
	Policy           policyResponse `json:"policy"`
	CommentCount     int64          `json:"comment_count"`
	RootCommentCount int64          `json:"root_comment_count"`
	LastSequence     int64          `json:"last_sequence"`
	LastCommentAt    *time.Time     `json:"last_comment_at,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type attachmentResponse struct {
	ID               uuid.UUID `json:"id"`
	Status           string    `json:"status"`
	MIMEType         string    `json:"mime_type"`
	SizeBytes        int64     `json:"size_bytes"`
	Width            int       `json:"width"`
	Height           int       `json:"height"`
	OriginalFilename string    `json:"original_filename"`
}

type commentResponse struct {
	ID                 uuid.UUID            `json:"id"`
	ThreadID           uuid.UUID            `json:"thread_id"`
	AuthorID           uuid.UUID            `json:"author_id"`
	ParentID           *uuid.UUID           `json:"parent_id,omitempty"`
	RootID             uuid.UUID            `json:"root_id"`
	Path               []uuid.UUID          `json:"path"`
	Depth              int16                `json:"depth"`
	Body               string               `json:"body"`
	Links              []domain.Link        `json:"links"`
	Attachments        []attachmentResponse `json:"attachments"`
	Status             string               `json:"status"`
	Version            int                  `json:"version"`
	Sequence           int64                `json:"sequence"`
	DirectRepliesCount int64                `json:"direct_replies_count"`
	EditedAt           *time.Time           `json:"edited_at,omitempty"`
	DeletedAt          *time.Time           `json:"deleted_at,omitempty"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
}

func (h CommentHandler) EnsureThread(w http.ResponseWriter, r *http.Request) {
	var request ensureThreadRequest
	if err := decodeJSON(w, r, &request); err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	view, err := h.Service.EnsureThread(r.Context(), mustActor(r), commentuc.EnsureThreadInput{
		SpaceKey: request.SpaceKey,
		Resource: domain.ResourceReference{Type: request.ResourceType, ID: request.ResourceID},
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectThread(view))
}

func (h CommentHandler) GetThread(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "threadID"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	view, err := h.Service.GetThread(r.Context(), mustActor(r), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectThread(view))
}

func (h CommentHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	threadID, err := uuid.Parse(r.URL.Query().Get("thread_id"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	parentID, err := optionalUUID(r.URL.Query().Get("parent_id"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	cursor, err := decodeCursor(r.URL.Query().Get("cursor"), threadID, parentID)
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	views, err := h.Service.ListComments(r.Context(), mustActor(r), repository.CommentListQuery{
		ThreadID: threadID, ParentID: parentID, After: cursor, Limit: limit + 1,
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	items, next, err := paginateViews(views, limit, threadID, parentID)
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": next})
}

func (h CommentHandler) GetComment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "commentID"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	view, err := h.Service.GetComment(r.Context(), mustActor(r), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectComment(view))
}

func (h CommentHandler) ListChanges(w http.ResponseWriter, r *http.Request) {
	threadID, err := uuid.Parse(r.URL.Query().Get("thread_id"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	afterSequence, err := strconv.ParseInt(defaultString(r.URL.Query().Get("after_sequence"), "0"), 10, 64)
	if err != nil || afterSequence < 0 {
		WriteError(w, domain.ErrValidation)
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	views, err := h.Service.ListChanges(r.Context(), mustActor(r), repository.CommentChangeQuery{
		ThreadID: threadID, AfterSequence: afterSequence, Limit: limit + 1,
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	hasMore := len(views) > limit
	if hasMore {
		views = views[:limit]
	}
	items := projectComments(views)
	nextSequence := afterSequence
	if len(views) > 0 {
		nextSequence = views[len(views)-1].Comment.Sequence
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "next_after_sequence": nextSequence, "has_more": hasMore,
	})
}

func (h CommentHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
	var request createCommentRequest
	if err := decodeJSON(w, r, &request); err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	result, err := h.Service.CreateComment(r.Context(), mustActor(r), commentuc.CreateCommentInput{
		ThreadID: request.ThreadID, ParentID: request.ParentID, Body: request.Body,
		AttachmentIDs: request.AttachmentIDs, IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, projectComment(result.View))
}

func (h CommentHandler) UpdateComment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "commentID"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	var request updateCommentRequest
	if err := decodeJSON(w, r, &request); err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	view, err := h.Service.UpdateComment(r.Context(), mustActor(r), commentuc.UpdateCommentInput{
		CommentID: id, Body: request.Body, ExpectedVersion: request.Version,
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectComment(view))
}

func (h CommentHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "commentID"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	var request deleteCommentRequest
	if err := decodeJSON(w, r, &request); err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	view, err := h.Service.DeleteComment(r.Context(), mustActor(r), commentuc.DeleteCommentInput{
		CommentID: id, ExpectedVersion: request.Version,
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectComment(view))
}

func mustActor(r *http.Request) domain.Actor {
	actor, _ := httpmw.ActorFromContext(r.Context())
	return actor
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return commentuc.DefaultPageLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > commentuc.MaxPageLimit {
		return 0, domain.ErrValidation
	}
	return limit, nil
}

func optionalUUID(raw string) (*uuid.UUID, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func paginateViews(views []commentuc.CommentView, limit int, threadID uuid.UUID, parentID *uuid.UUID) ([]commentResponse, *string, error) {
	hasMore := len(views) > limit
	if hasMore {
		views = views[:limit]
	}
	items := projectComments(views)
	if !hasMore || len(views) == 0 {
		return items, nil, nil
	}
	last := views[len(views)-1].Comment
	cursor, err := encodeCursor(threadID, parentID, repository.CommentCursor{CreatedAt: last.CreatedAt, ID: last.ID})
	if err != nil {
		return nil, nil, err
	}
	return items, &cursor, nil
}

func projectThread(view commentuc.ThreadView) threadResponse {
	item := view.Thread
	return threadResponse{
		ID: item.ID, SpaceID: item.SpaceID, ResourceType: item.Resource.Type,
		ResourceID: item.Resource.ID, Status: threadStatus(item.Status),
		Policy: policyResponse{
			AllowImages: view.Policy.AllowImages, AllowLinks: view.Policy.AllowLinks,
			MaxDepth: view.Policy.MaxDepth, MaxBodyLength: view.Policy.MaxBodyLength,
			MaxAttachments: view.Policy.MaxAttachments, MaxImageBytes: view.Policy.MaxImageBytes,
			EditWindowSeconds: view.Policy.EditWindowSeconds,
		},
		CommentCount: item.CommentCount, RootCommentCount: item.RootCommentCount,
		LastSequence: item.LastSequence, LastCommentAt: item.LastCommentAt,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func projectComments(views []commentuc.CommentView) []commentResponse {
	items := make([]commentResponse, 0, len(views))
	for _, view := range views {
		items = append(items, projectComment(view))
	}
	return items
}

func projectComment(view commentuc.CommentView) commentResponse {
	item := view.Comment
	attachments := make([]attachmentResponse, 0, len(view.Attachments))
	for _, attachment := range view.Attachments {
		attachments = append(attachments, projectAttachment(attachment))
	}
	links := item.Links
	if links == nil {
		links = []domain.Link{}
	}
	path := item.Path
	if path == nil {
		path = []uuid.UUID{}
	}
	return commentResponse{
		ID: item.ID, ThreadID: item.ThreadID, AuthorID: item.AuthorID,
		ParentID: item.ParentID, RootID: item.RootID, Path: path, Depth: item.Depth,
		Body: item.Body, Links: links, Attachments: attachments, Status: commentStatus(item.Status),
		Version: item.Version, Sequence: item.Sequence, DirectRepliesCount: item.DirectRepliesCount,
		EditedAt: item.EditedAt, DeletedAt: item.DeletedAt, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func projectAttachment(attachment domain.Attachment) attachmentResponse {
	return attachmentResponse{
		ID: attachment.ID, Status: attachmentStatus(attachment.Status), MIMEType: attachment.MIMEType,
		SizeBytes: attachment.SizeBytes, Width: attachment.Width, Height: attachment.Height,
		OriginalFilename: attachment.OriginalFilename,
	}
}

func threadStatus(status domain.ThreadStatus) string {
	return map[domain.ThreadStatus]string{
		domain.ThreadStatusOpen: "open", domain.ThreadStatusReadOnly: "read_only",
		domain.ThreadStatusClosed: "closed", domain.ThreadStatusHidden: "hidden",
	}[status]
}

func commentStatus(status domain.CommentStatus) string {
	return map[domain.CommentStatus]string{
		domain.CommentStatusActive: "active", domain.CommentStatusDeleted: "deleted",
		domain.CommentStatusHidden: "hidden",
	}[status]
}

func attachmentStatus(status domain.AttachmentStatus) string {
	return map[domain.AttachmentStatus]string{
		domain.AttachmentStatusPending: "pending", domain.AttachmentStatusProcessing: "processing",
		domain.AttachmentStatusReady: "ready", domain.AttachmentStatusFailed: "failed",
		domain.AttachmentStatusDeleted: "deleted",
	}[status]
}
