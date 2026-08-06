package handlers

import (
	"net/http"
	"testing"

	"github.com/bemulima/ms-go-comment/internal/domain"
)

func TestErrorContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "auth", err: domain.ErrAuthenticationRequired, status: http.StatusUnauthorized, code: "authentication_required"},
		{name: "internal auth", err: domain.ErrInternalAuthentication, status: http.StatusForbidden, code: "internal_authentication_failed"},
		{name: "access", err: domain.ErrAccessRequired, status: http.StatusForbidden, code: "comment_access_required"},
		{name: "invalid access grant", err: domain.ErrInvalidAccessGrant, status: http.StatusUnprocessableEntity, code: "invalid_access_grant"},
		{name: "thread state", err: domain.ErrThreadNotWritable, status: http.StatusConflict, code: "thread_not_writable"},
		{name: "edit", err: domain.ErrEditConflict, status: http.StatusConflict, code: "comment_edit_conflict"},
		{name: "parent", err: domain.ErrParentNotFound, status: http.StatusUnprocessableEntity, code: "parent_not_found"},
		{name: "content", err: domain.ErrInvalidCommentContent, status: http.StatusUnprocessableEntity, code: "invalid_comment_content"},
		{name: "policy", err: domain.ErrInvalidPolicy, status: http.StatusUnprocessableEntity, code: "invalid_comment_content"},
		{name: "configuration conflict", err: domain.ErrConflict, status: http.StatusConflict, code: "configuration_conflict"},
		{name: "moderation conflict", err: domain.ErrModerationConflict, status: http.StatusConflict, code: "comment_moderation_conflict"},
		{name: "attachment missing", err: domain.ErrAttachmentNotFound, status: http.StatusNotFound, code: "attachment_not_found"},
		{name: "attachment processing", err: domain.ErrAttachmentNotReady, status: http.StatusConflict, code: "attachment_not_ready"},
		{name: "realtime ticket", err: domain.ErrRealtimeTicketInvalid, status: http.StatusUnauthorized, code: "realtime_ticket_invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code, _ := errorContract(tt.err)
			if status != tt.status || code != tt.code {
				t.Fatalf("contract = (%d,%s), want (%d,%s)", status, code, tt.status, tt.code)
			}
		})
	}
}
