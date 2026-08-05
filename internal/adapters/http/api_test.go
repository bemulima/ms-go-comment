package http

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	commentuc "github.com/bemulima/ms-go-comment/internal/usecase/comment"
	realtimeuc "github.com/bemulima/ms-go-comment/internal/usecase/realtime"
	"github.com/google/uuid"
)

func TestRouter_BusinessRoutesRequireGatewayActor(t *testing.T) {
	t.Parallel()

	router := NewRouter(RouterDependencies{CommentService: &stubCommentService{}, RealtimeService: &stubRealtimeService{}})
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "ensure", method: http.MethodPut, path: "/api/v1/thread/ensure", body: `{}`},
		{name: "thread get", method: http.MethodGet, path: "/api/v1/thread/get/" + uuid.NewString()},
		{name: "comment list", method: http.MethodGet, path: "/api/v1/comment/list"},
		{name: "comment get", method: http.MethodGet, path: "/api/v1/comment/get/" + uuid.NewString()},
		{name: "changes", method: http.MethodGet, path: "/api/v1/comment/changes"},
		{name: "create", method: http.MethodPost, path: "/api/v1/comment/create", body: `{}`},
		{name: "update", method: http.MethodPut, path: "/api/v1/comment/update/" + uuid.NewString(), body: `{}`},
		{name: "delete", method: http.MethodDelete, path: "/api/v1/comment/delete/" + uuid.NewString(), body: `{}`},
		{name: "attachment upload", method: http.MethodPost, path: "/api/v1/comment-attachment/upload"},
		{name: "attachment signed URL", method: http.MethodGet, path: "/api/v1/comment-attachment/signed-url/" + uuid.NewString()},
		{name: "attachment delete", method: http.MethodDelete, path: "/api/v1/comment-attachment/delete/" + uuid.NewString()},
		{name: "realtime ticket", method: http.MethodPost, path: "/api/v1/realtime/ticket", body: `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"error":"authentication_required"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestRouter_MintsRealtimeTicket(t *testing.T) {
	t.Parallel()

	threadID := uuid.New()
	lastSequence := int64(12)
	called := false
	service := &stubRealtimeService{mint: func(_ context.Context, _ domain.Actor, input realtimeuc.MintTicketInput) (realtimeuc.MintedTicket, error) {
		called = true
		if input.ThreadID != threadID || input.LastSequence == nil || *input.LastSequence != lastSequence {
			t.Fatalf("ticket input = %#v", input)
		}
		return realtimeuc.MintedTicket{Ticket: "opaque", Protocol: "comment.v1", ExpiresAt: time.Now().UTC()}, nil
	}}
	request := authenticatedRequest(http.MethodPost, "/api/v1/realtime/ticket",
		`{"thread_id":"`+threadID.String()+`","last_sequence":12}`)
	response := httptest.NewRecorder()
	NewRouter(RouterDependencies{RealtimeService: service}).ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !called || !strings.Contains(response.Body.String(), `"ticket":"opaque"`) {
		t.Fatalf("status=%d called=%v body=%s", response.Code, called, response.Body.String())
	}
}

func TestRouter_WebSocketRouteUsesTicketHandlerWithoutGatewayActor(t *testing.T) {
	t.Parallel()

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusSwitchingProtocols)
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	response := httptest.NewRecorder()
	NewRouter(RouterDependencies{WebSocketHandler: handler}).ServeHTTP(response, request)
	if response.Code != http.StatusSwitchingProtocols || !called {
		t.Fatalf("status=%d called=%v", response.Code, called)
	}
}

func TestRouter_UploadAttachmentParsesMultipart(t *testing.T) {
	t.Parallel()

	threadID := uuid.New()
	attachmentID := uuid.New()
	called := false
	service := &stubCommentService{uploadAttachment: func(_ context.Context, _ domain.Actor, input commentuc.UploadAttachmentInput) (domain.Attachment, error) {
		called = true
		if input.ThreadID != threadID || input.Filename != "image.png" || string(input.Data) != "image-data" {
			t.Fatalf("upload input = %#v", input)
		}
		return domain.Attachment{ID: attachmentID, Status: domain.AttachmentStatusPending}, nil
	}}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("thread_id", threadID.String())
	part, err := writer.CreateFormFile("file", "image.png")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	_, _ = part.Write([]byte("image-data"))
	_ = writer.Close()
	request := authenticatedRequest(http.MethodPost, "/api/v1/comment-attachment/upload", body.String())
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	NewRouter(RouterDependencies{CommentService: service}).ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !called || !strings.Contains(response.Body.String(), attachmentID.String()) {
		t.Fatalf("status=%d called=%v body=%s", response.Code, called, response.Body.String())
	}
}

func TestRouter_CreateRejectsServerOwnedAndUnknownFields(t *testing.T) {
	t.Parallel()

	called := false
	service := &stubCommentService{create: func(context.Context, domain.Actor, commentuc.CreateCommentInput) (commentuc.CreateCommentResult, error) {
		called = true
		return commentuc.CreateCommentResult{}, nil
	}}
	request := authenticatedRequest(http.MethodPost, "/api/v1/comment/create", `{
"thread_id":"`+uuid.NewString()+`","body":"hello","idempotency_key":"`+uuid.NewString()+`","author_id":"`+uuid.NewString()+`"}`)
	response := httptest.NewRecorder()
	NewRouter(RouterDependencies{CommentService: service}).ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || called {
		t.Fatalf("status=%d called=%v body=%s", response.Code, called, response.Body.String())
	}
}

func TestRouter_CreateReturnsCreatedThenReplayStatus(t *testing.T) {
	t.Parallel()

	commentID := uuid.New()
	threadID := uuid.New()
	now := time.Now().UTC()
	calls := 0
	service := &stubCommentService{create: func(_ context.Context, actor domain.Actor, input commentuc.CreateCommentInput) (commentuc.CreateCommentResult, error) {
		calls++
		return commentuc.CreateCommentResult{
			Created: calls == 1,
			View: commentuc.CommentView{Comment: domain.Comment{
				ID: commentID, ThreadID: threadID, AuthorID: actor.UserID, RootID: commentID,
				Path: []uuid.UUID{commentID}, Body: input.Body, Status: domain.CommentStatusActive,
				Version: 1, Sequence: 1, CreatedAt: now, UpdatedAt: now,
			}},
		}, nil
	}}
	router := NewRouter(RouterDependencies{CommentService: service})
	body := `{"thread_id":"` + threadID.String() + `","body":"hello","attachment_ids":[],"idempotency_key":"` + uuid.NewString() + `"}`
	for index, status := range []int{http.StatusCreated, http.StatusOK} {
		request := authenticatedRequest(http.MethodPost, "/api/v1/comment/create", body)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != status || !strings.Contains(response.Body.String(), commentID.String()) {
			t.Fatalf("request %d status=%d body=%s", index, response.Code, response.Body.String())
		}
	}
}

func authenticatedRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("X-User-ID", uuid.NewString())
	request.Header.Set("X-User-Role", "STUDENT")
	request.Header.Set("Content-Type", "application/json")
	return request
}

type stubCommentService struct {
	create           func(context.Context, domain.Actor, commentuc.CreateCommentInput) (commentuc.CreateCommentResult, error)
	uploadAttachment func(context.Context, domain.Actor, commentuc.UploadAttachmentInput) (domain.Attachment, error)
}

type stubRealtimeService struct {
	mint func(context.Context, domain.Actor, realtimeuc.MintTicketInput) (realtimeuc.MintedTicket, error)
}

func (s *stubRealtimeService) Mint(ctx context.Context, actor domain.Actor, input realtimeuc.MintTicketInput) (realtimeuc.MintedTicket, error) {
	if s.mint != nil {
		return s.mint(ctx, actor, input)
	}
	return realtimeuc.MintedTicket{}, nil
}

func (s *stubCommentService) EnsureThread(context.Context, domain.Actor, commentuc.EnsureThreadInput) (commentuc.ThreadView, error) {
	return commentuc.ThreadView{}, nil
}
func (s *stubCommentService) GetThread(context.Context, domain.Actor, uuid.UUID) (commentuc.ThreadView, error) {
	return commentuc.ThreadView{}, nil
}
func (s *stubCommentService) ListComments(context.Context, domain.Actor, repository.CommentListQuery) ([]commentuc.CommentView, error) {
	return nil, nil
}
func (s *stubCommentService) GetComment(context.Context, domain.Actor, uuid.UUID) (commentuc.CommentView, error) {
	return commentuc.CommentView{}, nil
}
func (s *stubCommentService) ListChanges(context.Context, domain.Actor, repository.CommentChangeQuery) ([]commentuc.CommentView, error) {
	return nil, nil
}
func (s *stubCommentService) CreateComment(ctx context.Context, actor domain.Actor, input commentuc.CreateCommentInput) (commentuc.CreateCommentResult, error) {
	if s.create != nil {
		return s.create(ctx, actor, input)
	}
	return commentuc.CreateCommentResult{}, nil
}
func (s *stubCommentService) UpdateComment(context.Context, domain.Actor, commentuc.UpdateCommentInput) (commentuc.CommentView, error) {
	return commentuc.CommentView{}, nil
}
func (s *stubCommentService) DeleteComment(context.Context, domain.Actor, commentuc.DeleteCommentInput) (commentuc.CommentView, error) {
	return commentuc.CommentView{}, nil
}

func (s *stubCommentService) UploadAttachment(ctx context.Context, actor domain.Actor, input commentuc.UploadAttachmentInput) (domain.Attachment, error) {
	if s.uploadAttachment != nil {
		return s.uploadAttachment(ctx, actor, input)
	}
	return domain.Attachment{}, nil
}
func (s *stubCommentService) GetAttachmentSignedURL(context.Context, domain.Actor, uuid.UUID) (commentuc.SignedFileURL, error) {
	return commentuc.SignedFileURL{}, nil
}
func (s *stubCommentService) DeleteAttachment(context.Context, domain.Actor, uuid.UUID) (domain.Attachment, error) {
	return domain.Attachment{}, nil
}
