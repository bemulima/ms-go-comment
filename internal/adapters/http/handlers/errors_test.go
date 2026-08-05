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
		{name: "access", err: domain.ErrAccessRequired, status: http.StatusForbidden, code: "comment_access_required"},
		{name: "thread state", err: domain.ErrThreadNotWritable, status: http.StatusConflict, code: "thread_not_writable"},
		{name: "edit", err: domain.ErrEditConflict, status: http.StatusConflict, code: "comment_edit_conflict"},
		{name: "parent", err: domain.ErrParentNotFound, status: http.StatusUnprocessableEntity, code: "parent_not_found"},
		{name: "content", err: domain.ErrInvalidCommentContent, status: http.StatusUnprocessableEntity, code: "invalid_comment_content"},
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
