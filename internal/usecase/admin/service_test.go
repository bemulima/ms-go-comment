package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	adminuc "github.com/bemulima/ms-go-comment/internal/usecase/admin"
	"github.com/google/uuid"
)

func TestService_SpaceLifecycleAndPermissions(t *testing.T) {
	t.Parallel()

	service, store, admin, moderator := fixture()
	created, err := service.CreateSpace(context.Background(), admin, adminuc.CreateSpaceInput{
		Key: "articles", Name: "Articles", AccessMode: domain.AccessModeAuthenticated,
		AllowedOrigins: []string{"https://app.example"}, Policy: domain.DefaultPolicy(),
	})
	if err != nil || created.Key != "articles" || created.CreatedBy != admin.UserID {
		t.Fatalf("CreateSpace() = %#v, error=%v", created, err)
	}
	if _, err := service.CreateSpace(context.Background(), moderator, adminuc.CreateSpaceInput{}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("moderator create error = %v", err)
	}
	listed, err := service.ListSpaces(context.Background(), moderator, repository.SpaceListQuery{Limit: 20})
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListSpaces() = %#v, error=%v", listed, err)
	}
	updatedPolicy := domain.DefaultPolicy()
	updatedPolicy.AllowImages = true
	updated, err := service.UpdateSpace(context.Background(), admin, adminuc.UpdateSpaceInput{
		ID: created.ID, Name: "Article discussions", Status: domain.SpaceStatusActive,
		AccessMode: domain.AccessModeAuthenticated, AllowedOrigins: []string{"https://app.example"}, Policy: updatedPolicy,
	})
	if err != nil || !updated.Policy.AllowImages || updated.Key != created.Key || updated.CreatedBy != created.CreatedBy {
		t.Fatalf("UpdateSpace() = %#v, error=%v", updated, err)
	}
	if _, err := service.UpdateSpace(context.Background(), moderator, adminuc.UpdateSpaceInput{}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("moderator update error = %v", err)
	}
	disabled, err := service.DisableSpace(context.Background(), admin, created.ID)
	if err != nil || disabled.Status != domain.SpaceStatusDisabled || store.updateCalls != 2 {
		t.Fatalf("DisableSpace() = %#v, updates=%d error=%v", disabled, store.updateCalls, err)
	}
	if _, err := service.DisableSpace(context.Background(), admin, created.ID); err != nil || store.updateCalls != 2 {
		t.Fatalf("idempotent disable updates=%d error=%v", store.updateCalls, err)
	}
}

func TestService_ThreadListAndUpdatePolicy(t *testing.T) {
	t.Parallel()

	service, store, admin, moderator := fixture()
	space := domain.Space{ID: uuid.New(), Key: "course", Name: "Course", Status: domain.SpaceStatusActive,
		AccessMode: domain.AccessModeAuthenticated, Policy: domain.DefaultPolicy(), CreatedBy: admin.UserID,
		CreatedAt: service.Now(), UpdatedAt: service.Now()}
	store.spaces[space.ID] = space
	thread := domain.Thread{ID: uuid.New(), SpaceID: space.ID, Resource: domain.ResourceReference{Type: "lesson", ID: "lesson-1"},
		Status: domain.ThreadStatusOpen, CreatedAt: service.Now(), UpdatedAt: service.Now()}
	store.threads[thread.ID] = thread
	views, err := service.ListThreads(context.Background(), moderator, repository.ThreadListQuery{SpaceID: &space.ID, Limit: 20})
	if err != nil || len(views) != 1 || views[0].Policy.MaxDepth != space.Policy.MaxDepth {
		t.Fatalf("ListThreads() = %#v, error=%v", views, err)
	}
	depth := int16(3)
	updated, err := service.UpdateThread(context.Background(), admin, adminuc.UpdateThreadInput{
		ID: thread.ID, Status: domain.ThreadStatusReadOnly, Overrides: domain.ThreadPolicyOverrides{MaxDepth: &depth},
	})
	if err != nil || updated.Thread.Status != domain.ThreadStatusReadOnly || updated.Policy.MaxDepth != depth ||
		updated.Thread.LastSequence != 1 || len(store.outbox) != 1 || store.outbox[0].Subject != domain.EventCommentThreadUpdated {
		t.Fatalf("UpdateThread() = %#v, error=%v", updated, err)
	}
	if _, err := service.UpdateThread(context.Background(), moderator, adminuc.UpdateThreadInput{}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("moderator thread update error = %v", err)
	}
	badDepth := int16(99)
	if _, err := service.UpdateThread(context.Background(), admin, adminuc.UpdateThreadInput{
		ID: thread.ID, Status: domain.ThreadStatusOpen, Overrides: domain.ThreadPolicyOverrides{MaxDepth: &badDepth},
	}); !errors.Is(err, domain.ErrInvalidPolicy) {
		t.Fatalf("invalid override error = %v", err)
	}
}

func TestService_ModeratesCommentsTransactionally(t *testing.T) {
	t.Parallel()

	service, store, admin, moderator := fixture()
	space := domain.Space{ID: uuid.New(), Key: "course", Name: "Course", Status: domain.SpaceStatusActive,
		AccessMode: domain.AccessModeAuthenticated, Policy: domain.DefaultPolicy(), CreatedBy: admin.UserID,
		CreatedAt: service.Now(), UpdatedAt: service.Now()}
	thread := domain.Thread{ID: uuid.New(), SpaceID: space.ID, Resource: domain.ResourceReference{Type: "lesson", ID: "lesson-1"},
		Status: domain.ThreadStatusOpen, CreatedAt: service.Now(), UpdatedAt: service.Now()}
	commentID := uuid.New()
	comment := domain.Comment{ID: commentID, ThreadID: thread.ID, AuthorID: uuid.New(), RootID: commentID,
		Path: []uuid.UUID{commentID}, Body: "visible https://example.com", Links: []domain.Link{{URL: "https://example.com"}},
		Status: domain.CommentStatusActive, Version: 1, IdempotencyKey: uuid.New(), CreatedAt: service.Now(), UpdatedAt: service.Now()}
	attachmentID := uuid.New()
	store.spaces[space.ID], store.threads[thread.ID], store.comments[comment.ID] = space, thread, comment
	store.attachments[attachmentID] = domain.Attachment{ID: attachmentID, ThreadID: thread.ID, CommentID: &commentID,
		UploaderID: comment.AuthorID, FileStorageID: uuid.New(), Status: domain.AttachmentStatusReady, MIMEType: "image/png"}

	hidden, err := service.HideComment(context.Background(), moderator, comment.ID)
	if err != nil || hidden.Comment.Status != domain.CommentStatusHidden || hidden.Comment.Version != 2 ||
		hidden.Comment.Sequence != 1 || len(hidden.Attachments) != 1 || len(store.outbox) != 1 {
		t.Fatalf("HideComment() = %#v, outbox=%d error=%v", hidden, len(store.outbox), err)
	}
	assertPayloadFields(t, store.outbox[0], domain.EventCommentHidden, false)
	if _, err := service.HideComment(context.Background(), moderator, comment.ID); err != nil || len(store.outbox) != 1 || store.threads[thread.ID].LastSequence != 1 {
		t.Fatalf("idempotent hide sequence=%d outbox=%d error=%v", store.threads[thread.ID].LastSequence, len(store.outbox), err)
	}

	restored, err := service.RestoreComment(context.Background(), admin, comment.ID)
	if err != nil || restored.Comment.Status != domain.CommentStatusActive || restored.Comment.Version != 3 ||
		restored.Comment.Sequence != 2 || len(store.outbox) != 2 {
		t.Fatalf("RestoreComment() = %#v, outbox=%d error=%v", restored, len(store.outbox), err)
	}
	assertPayloadFields(t, store.outbox[1], domain.EventCommentRestored, true)
	if _, err := service.RestoreComment(context.Background(), admin, comment.ID); err != nil || len(store.outbox) != 2 || store.threads[thread.ID].LastSequence != 2 {
		t.Fatalf("idempotent restore sequence=%d outbox=%d error=%v", store.threads[thread.ID].LastSequence, len(store.outbox), err)
	}
	if _, err := service.HideComment(context.Background(), domain.Actor{UserID: uuid.New(), Role: "STUDENT"}, comment.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("student moderation error=%v", err)
	}

	comment = store.comments[comment.ID]
	comment.Status = domain.CommentStatusDeleted
	store.comments[comment.ID] = comment
	if _, err := service.RestoreComment(context.Background(), moderator, comment.ID); !errors.Is(err, domain.ErrModerationConflict) {
		t.Fatalf("deleted moderation error=%v", err)
	}
}

func assertPayloadFields(t *testing.T, event domain.OutboxEvent, subject domain.EventSubject, restored bool) {
	t.Helper()
	if event.Subject != subject {
		t.Fatalf("subject=%s want=%s", event.Subject, subject)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"body", "links", "attachments"} {
		_, exists := payload[field]
		if exists != restored {
			t.Fatalf("payload field %q exists=%v restored=%v payload=%s", field, exists, restored, event.Payload)
		}
	}
	if restored {
		var body string
		if err := json.Unmarshal(payload["body"], &body); err != nil || body != "visible https://example.com" {
			t.Fatalf("restored body=%q error=%v payload=%s", body, err, event.Payload)
		}
	}
}

type fakeStore struct {
	spaces      map[uuid.UUID]domain.Space
	threads     map[uuid.UUID]domain.Thread
	comments    map[uuid.UUID]domain.Comment
	attachments map[uuid.UUID]domain.Attachment
	updateCalls int
	outbox      []domain.OutboxEvent
}

func fixture() (*adminuc.Service, *fakeStore, domain.Actor, domain.Actor) {
	now := time.Date(2026, 8, 5, 18, 0, 0, 0, time.UTC)
	store := &fakeStore{spaces: map[uuid.UUID]domain.Space{}, threads: map[uuid.UUID]domain.Thread{},
		comments: map[uuid.UUID]domain.Comment{}, attachments: map[uuid.UUID]domain.Attachment{}}
	service := &adminuc.Service{Spaces: fakeSpaces{store}, Threads: fakeThreads{store}, Comments: fakeComments{store},
		Attachments: fakeAttachments{store}, Outbox: fakeOutbox{store},
		Tx: fakeTx{}, Now: func() time.Time { return now }, NewID: uuid.New}
	return service, store, domain.Actor{UserID: uuid.New(), Role: "ADMIN"}, domain.Actor{UserID: uuid.New(), Role: "MODERATOR"}
}

type fakeSpaces struct{ *fakeStore }
type fakeThreads struct{ *fakeStore }
type fakeComments struct{ *fakeStore }
type fakeAttachments struct{ *fakeStore }
type fakeOutbox struct{ *fakeStore }
type fakeTx struct{}

func (fakeTx) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func (f fakeSpaces) Create(_ context.Context, item domain.Space) error {
	f.spaces[item.ID] = item
	return nil
}
func (f fakeSpaces) GetByID(_ context.Context, id uuid.UUID) (domain.Space, error) {
	item, ok := f.spaces[id]
	if !ok {
		return domain.Space{}, domain.ErrNotFound
	}
	return item, nil
}
func (f fakeSpaces) GetByKey(_ context.Context, key string) (domain.Space, error) {
	for _, item := range f.spaces {
		if item.Key == key {
			return item, nil
		}
	}
	return domain.Space{}, domain.ErrNotFound
}
func (f fakeSpaces) Update(_ context.Context, item domain.Space) error {
	f.spaces[item.ID] = item
	f.updateCalls++
	return nil
}
func (f fakeSpaces) List(_ context.Context, query repository.SpaceListQuery) ([]domain.Space, error) {
	items := make([]domain.Space, 0)
	for _, item := range f.spaces {
		if query.Status == nil || item.Status == *query.Status {
			items = append(items, item)
		}
	}
	return items, nil
}
func (f fakeThreads) Ensure(_ context.Context, item domain.Thread) (domain.Thread, error) {
	f.threads[item.ID] = item
	return item, nil
}
func (f fakeThreads) GetByID(_ context.Context, id uuid.UUID) (domain.Thread, error) {
	item, ok := f.threads[id]
	if !ok {
		return domain.Thread{}, domain.ErrNotFound
	}
	return item, nil
}
func (f fakeThreads) GetByResource(context.Context, uuid.UUID, domain.ResourceReference) (domain.Thread, error) {
	return domain.Thread{}, domain.ErrNotFound
}
func (f fakeThreads) List(_ context.Context, query repository.ThreadListQuery) ([]domain.Thread, error) {
	items := make([]domain.Thread, 0)
	for _, item := range f.threads {
		if query.SpaceID != nil && item.SpaceID != *query.SpaceID {
			continue
		}
		if query.Status != nil && item.Status != *query.Status {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}
func (f fakeThreads) Update(_ context.Context, item domain.Thread) error {
	f.threads[item.ID] = item
	return nil
}
func (f fakeThreads) NextSequence(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (f fakeThreads) NextSequenceAnyState(_ context.Context, id uuid.UUID) (int64, error) {
	item, ok := f.threads[id]
	if !ok {
		return 0, domain.ErrNotFound
	}
	item.LastSequence++
	f.threads[id] = item
	return item.LastSequence, nil
}
func (f fakeThreads) RecordCommentCreated(context.Context, uuid.UUID, bool) error { return nil }
func (f fakeComments) GetByIDForUpdate(_ context.Context, id uuid.UUID) (domain.Comment, error) {
	item, ok := f.comments[id]
	if !ok {
		return domain.Comment{}, domain.ErrNotFound
	}
	return item, nil
}
func (f fakeComments) UpdateModerationStatus(_ context.Context, item domain.Comment, expectedStatus domain.CommentStatus, expectedVersion int) error {
	current, ok := f.comments[item.ID]
	if !ok || current.Status != expectedStatus || current.Version != expectedVersion {
		return domain.ErrModerationConflict
	}
	f.comments[item.ID] = item
	return nil
}
func (f fakeAttachments) ListByComment(_ context.Context, threadID, commentID uuid.UUID) ([]domain.Attachment, error) {
	items := make([]domain.Attachment, 0)
	for _, item := range f.attachments {
		if item.ThreadID == threadID && item.CommentID != nil && *item.CommentID == commentID {
			items = append(items, item)
		}
	}
	return items, nil
}
func (f fakeOutbox) Add(_ context.Context, item domain.OutboxEvent) error {
	f.outbox = append(f.outbox, item)
	return nil
}
func (f fakeOutbox) ClaimPending(context.Context, time.Time, time.Time, int) ([]domain.OutboxEvent, error) {
	return nil, nil
}
func (f fakeOutbox) MarkPublished(context.Context, uuid.UUID, time.Time) error { return nil }
func (f fakeOutbox) MarkFailed(context.Context, uuid.UUID, time.Time, string) error {
	return nil
}
