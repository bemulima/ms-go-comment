package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bemulima/ms-go-comment/internal/domain"
)

type errorResponse struct {
	Error   string         `json:"error"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

func WriteError(w http.ResponseWriter, err error) {
	status, code, message := errorContract(err)
	writeJSON(w, status, errorResponse{Error: code, Message: message, Details: map[string]any{}})
}

func errorContract(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrAuthenticationRequired):
		return http.StatusUnauthorized, "authentication_required", "authenticated user is required"
	case errors.Is(err, domain.ErrAccessRequired):
		return http.StatusForbidden, "comment_access_required", "comment access grant is required"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "comment_forbidden", "comment operation is not allowed"
	case errors.Is(err, domain.ErrSpaceNotFound):
		return http.StatusNotFound, "space_not_found", "comment space was not found"
	case errors.Is(err, domain.ErrThreadNotFound):
		return http.StatusNotFound, "thread_not_found", "comment thread was not found"
	case errors.Is(err, domain.ErrCommentNotFound):
		return http.StatusNotFound, "comment_not_found", "comment was not found"
	case errors.Is(err, domain.ErrAttachmentNotFound):
		return http.StatusNotFound, "attachment_not_found", "comment attachment was not found"
	case errors.Is(err, domain.ErrAttachmentNotReady):
		return http.StatusConflict, "attachment_not_ready", "comment attachment is not ready"
	case errors.Is(err, domain.ErrThreadNotWritable):
		return http.StatusConflict, "thread_not_writable", "comment thread is not writable"
	case errors.Is(err, domain.ErrEditConflict):
		return http.StatusConflict, "comment_edit_conflict", "comment was changed by another request"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return http.StatusConflict, "idempotency_conflict", "idempotency key was reused with different input"
	case errors.Is(err, domain.ErrParentNotFound), errors.Is(err, domain.ErrCrossThreadParent):
		return http.StatusUnprocessableEntity, "parent_not_found", "reply parent is invalid or unavailable"
	case errors.Is(err, domain.ErrMaxDepthExceeded):
		return http.StatusUnprocessableEntity, "max_depth_exceeded", "maximum comment depth was exceeded"
	case errors.Is(err, domain.ErrImagesDisabled):
		return http.StatusUnprocessableEntity, "images_disabled", "images are disabled for this thread"
	case errors.Is(err, domain.ErrLinksDisabled):
		return http.StatusUnprocessableEntity, "links_disabled", "links are disabled for this thread"
	case errors.Is(err, domain.ErrInvalidCommentContent), errors.Is(err, domain.ErrValidation),
		errors.Is(err, domain.ErrInvalidSpaceKey), errors.Is(err, domain.ErrInvalidResource), errors.Is(err, domain.ErrInvalidPlacement),
		errors.Is(err, domain.ErrInvalidAttachment):
		return http.StatusUnprocessableEntity, "invalid_comment_content", "request content violates the comment contract"
	default:
		return http.StatusInternalServerError, "internal_error", "internal server error"
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
