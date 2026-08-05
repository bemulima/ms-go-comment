package comment_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	commentuc "github.com/bemulima/ms-go-comment/internal/usecase/comment"
	"github.com/google/uuid"
)

func TestService_CreateCommentRootAndReplay(t *testing.T) {
	t.Parallel()

	service, store, actor, thread := newFixture()
	commentID := uuid.New()
	eventID := uuid.New()
	service.NewID = idSequence(commentID, eventID)
	input := commentuc.CreateCommentInput{
		ThreadID: thread.ID, Body: "hello https://example.com/docs", IdempotencyKey: uuid.New(),
	}

	created, err := service.CreateComment(context.Background(), actor, input)
	if err != nil {
		t.Fatalf("CreateComment() error = %v", err)
	}
	if !created.Created || created.View.Comment.ID != commentID || created.View.Comment.RootID != commentID || created.View.Comment.Sequence != 1 {
		t.Fatalf("unexpected create result: %#v", created)
	}
	if got := store.threads[thread.ID]; got.CommentCount != 1 || got.RootCommentCount != 1 || got.LastSequence != 1 {
		t.Fatalf("unexpected thread counters: %#v", got)
	}
	if len(store.outbox) != 1 || store.outbox[0].Subject != domain.EventCommentCreated {
		t.Fatalf("unexpected outbox: %#v", store.outbox)
	}

	replay, err := service.CreateComment(context.Background(), actor, input)
	if err != nil {
		t.Fatalf("replay error = %v", err)
	}
	if replay.Created || replay.View.Comment.ID != commentID || len(store.outbox) != 1 {
		t.Fatalf("unexpected replay: %#v outbox=%d", replay, len(store.outbox))
	}

	input.Body = "different"
	if _, err := service.CreateComment(context.Background(), actor, input); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("conflicting replay error = %v", err)
	}
}

func TestService_CreateReplyAndRejectInvalidParent(t *testing.T) {
	t.Parallel()

	service, store, actor, thread := newFixture()
	rootID := uuid.New()
	root := domain.Comment{
		ID: rootID, ThreadID: thread.ID, AuthorID: actor.UserID, RootID: rootID,
		Path: []uuid.UUID{rootID}, Status: domain.CommentStatusActive, Version: 1,
		Sequence: 1, IdempotencyKey: uuid.New(), CreatedAt: service.Now(), UpdatedAt: service.Now(),
	}
	store.comments[root.ID] = root
	store.threads[thread.ID] = withSequence(thread, 1)
	replyID := uuid.New()
	service.NewID = idSequence(replyID, uuid.New())

	result, err := service.CreateComment(context.Background(), actor, commentuc.CreateCommentInput{
		ThreadID: thread.ID, ParentID: &rootID, Body: "reply", IdempotencyKey: uuid.New(),
	})
	if err != nil {
		t.Fatalf("CreateComment() error = %v", err)
	}
	if result.View.Comment.Depth != 1 || result.View.Comment.RootID != rootID || len(result.View.Comment.Path) != 2 {
		t.Fatalf("unexpected reply placement: %#v", result.View.Comment)
	}
	if store.comments[rootID].DirectRepliesCount != 1 {
		t.Fatalf("reply count = %d", store.comments[rootID].DirectRepliesCount)
	}

	foreignParent := uuid.New()
	if _, err := service.CreateComment(context.Background(), actor, commentuc.CreateCommentInput{
		ThreadID: thread.ID, ParentID: &foreignParent, Body: "bad", IdempotencyKey: uuid.New(),
	}); !errors.Is(err, domain.ErrParentNotFound) {
		t.Fatalf("invalid parent error = %v", err)
	}
}

func TestService_UpdateAndDeleteUseVersionAndEditWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		actor   func(domain.Actor) domain.Actor
		version int
		advance time.Duration
		wantErr error
	}{
		{name: "stale version", version: 2, wantErr: domain.ErrEditConflict},
		{name: "another author", version: 1, actor: func(a domain.Actor) domain.Actor { a.UserID = uuid.New(); return a }, wantErr: domain.ErrForbidden},
		{name: "expired window", version: 1, advance: 16 * time.Minute, wantErr: domain.ErrForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, store, actor, thread := newFixture()
			commentID := uuid.New()
			store.comments[commentID] = activeComment(commentID, thread.ID, actor.UserID, service.Now())
			if tt.actor != nil {
				actor = tt.actor(actor)
			}
			baseNow := service.Now()
			service.Now = func() time.Time { return baseNow.Add(tt.advance) }
			_, err := service.UpdateComment(context.Background(), actor, commentuc.UpdateCommentInput{
				CommentID: commentID, Body: "edited", ExpectedVersion: tt.version,
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("UpdateComment() error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	service, store, actor, thread := newFixture()
	commentID := uuid.New()
	store.comments[commentID] = activeComment(commentID, thread.ID, actor.UserID, service.Now())
	service.NewID = idSequence(uuid.New(), uuid.New())
	updated, err := service.UpdateComment(context.Background(), actor, commentuc.UpdateCommentInput{
		CommentID: commentID, Body: "edited", ExpectedVersion: 1,
	})
	if err != nil || updated.Comment.Version != 2 || updated.Comment.Sequence != 1 {
		t.Fatalf("unexpected update: %#v error=%v", updated, err)
	}
	deleted, err := service.DeleteComment(context.Background(), actor, commentuc.DeleteCommentInput{
		CommentID: commentID, ExpectedVersion: 2,
	})
	if err != nil || deleted.Comment.Status != domain.CommentStatusDeleted || deleted.Comment.Body != "" || deleted.Comment.Sequence != 2 {
		t.Fatalf("unexpected delete: %#v error=%v", deleted, err)
	}
	if len(store.outbox) != 2 {
		t.Fatalf("outbox count = %d", len(store.outbox))
	}
}

func TestService_AccessAndWritableState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*fakeStore, domain.Thread)
		write     bool
		wantErr   error
	}{
		{name: "authenticated read"},
		{name: "context grant required", configure: func(s *fakeStore, thread domain.Thread) {
			space := s.spaces[thread.SpaceID]
			space.AccessMode = domain.AccessModeContextGrant
			s.spaces[space.ID] = space
		}, wantErr: domain.ErrAccessRequired},
		{name: "read only rejects create", configure: func(s *fakeStore, thread domain.Thread) {
			thread.Status = domain.ThreadStatusReadOnly
			s.threads[thread.ID] = thread
		}, write: true, wantErr: domain.ErrThreadNotWritable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, store, actor, thread := newFixture()
			if tt.configure != nil {
				tt.configure(store, thread)
			}
			var err error
			if tt.write {
				_, err = service.CreateComment(context.Background(), actor, commentuc.CreateCommentInput{
					ThreadID: thread.ID, Body: "hello", IdempotencyKey: uuid.New(),
				})
			} else {
				_, err = service.GetThread(context.Background(), actor, thread.ID)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestService_ReadListAndReconcileViews(t *testing.T) {
	t.Parallel()

	service, store, actor, thread := newFixture()
	rootID := uuid.New()
	root := activeComment(rootID, thread.ID, actor.UserID, service.Now())
	root.Sequence = 1
	childID := uuid.New()
	child := activeComment(childID, thread.ID, actor.UserID, service.Now().Add(time.Second))
	child.ParentID = &rootID
	child.RootID = rootID
	child.Path = []uuid.UUID{rootID, childID}
	child.Depth = 1
	child.Sequence = 2
	hiddenID := uuid.New()
	hidden := activeComment(hiddenID, thread.ID, actor.UserID, service.Now().Add(2*time.Second))
	hidden.Status = domain.CommentStatusHidden
	hidden.Sequence = 3
	store.comments[rootID], store.comments[childID], store.comments[hiddenID] = root, child, hidden
	store.threads[thread.ID] = withSequence(thread, 3)
	attachmentID := uuid.New()
	store.attachments[attachmentID] = domain.Attachment{
		ID: attachmentID, ThreadID: thread.ID, CommentID: &rootID, UploaderID: actor.UserID,
		FileStorageID: uuid.New(), Status: domain.AttachmentStatusReady, MIMEType: "image/png",
		SizeBytes: 64, Width: 2, Height: 2, OriginalFilename: "proof.png",
		ExpiresAt: service.Now().Add(time.Hour), CreatedAt: service.Now(), UpdatedAt: service.Now(),
	}

	threadView, err := service.GetThread(context.Background(), actor, thread.ID)
	if err != nil || threadView.Thread.LastSequence != 3 || threadView.Policy.MaxDepth != domain.DefaultPolicy().MaxDepth {
		t.Fatalf("GetThread() = %#v, error=%v", threadView, err)
	}
	commentView, err := service.GetComment(context.Background(), actor, rootID)
	if err != nil || len(commentView.Attachments) != 1 || commentView.Attachments[0].ID != attachmentID {
		t.Fatalf("GetComment() = %#v, error=%v", commentView, err)
	}
	roots, err := service.ListComments(context.Background(), actor, repository.CommentListQuery{ThreadID: thread.ID, Limit: 20})
	if err != nil || len(roots) != 1 || roots[0].Comment.ID != rootID {
		t.Fatalf("ListComments(roots) = %#v, error=%v", roots, err)
	}
	children, err := service.ListComments(context.Background(), actor, repository.CommentListQuery{ThreadID: thread.ID, ParentID: &rootID, Limit: 20})
	if err != nil || len(children) != 1 || children[0].Comment.ID != childID {
		t.Fatalf("ListComments(children) = %#v, error=%v", children, err)
	}
	changes, err := service.ListChanges(context.Background(), actor, repository.CommentChangeQuery{ThreadID: thread.ID, AfterSequence: 1, Limit: 20})
	if err != nil || len(changes) != 1 || changes[0].Comment.ID != childID {
		t.Fatalf("ListChanges() = %#v, error=%v", changes, err)
	}
}

func TestService_ReadValidationAndVisibilityErrors(t *testing.T) {
	t.Parallel()

	service, store, actor, thread := newFixture()
	hiddenID := uuid.New()
	hidden := activeComment(hiddenID, thread.ID, actor.UserID, service.Now())
	hidden.Status = domain.CommentStatusHidden
	store.comments[hiddenID] = hidden

	if _, err := service.GetComment(context.Background(), actor, uuid.New()); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Fatalf("missing comment error = %v", err)
	}
	if _, err := service.GetComment(context.Background(), actor, hiddenID); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Fatalf("hidden comment error = %v", err)
	}
	if _, err := service.ListComments(context.Background(), actor, repository.CommentListQuery{ThreadID: thread.ID}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("invalid list limit error = %v", err)
	}
	if _, err := service.ListChanges(context.Background(), actor, repository.CommentChangeQuery{ThreadID: thread.ID, AfterSequence: -1, Limit: 20}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("invalid changes cursor error = %v", err)
	}
	foreignParent := uuid.New()
	if _, err := service.ListComments(context.Background(), actor, repository.CommentListQuery{ThreadID: thread.ID, ParentID: &foreignParent, Limit: 20}); !errors.Is(err, domain.ErrParentNotFound) {
		t.Fatalf("foreign parent error = %v", err)
	}
}

type fakeStore struct {
	spaces      map[uuid.UUID]domain.Space
	spaceKeys   map[string]uuid.UUID
	threads     map[uuid.UUID]domain.Thread
	comments    map[uuid.UUID]domain.Comment
	attachments map[uuid.UUID]domain.Attachment
	outbox      []domain.OutboxEvent
	txCalls     int
}

func newFixture() (*commentuc.Service, *fakeStore, domain.Actor, domain.Thread) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	space := domain.Space{
		ID: uuid.New(), Key: "course", Name: "Course", Status: domain.SpaceStatusActive,
		AccessMode: domain.AccessModeAuthenticated, Policy: domain.DefaultPolicy(), CreatedBy: uuid.New(),
		CreatedAt: now, UpdatedAt: now,
	}
	thread := domain.Thread{
		ID: uuid.New(), SpaceID: space.ID, Resource: domain.ResourceReference{Type: "lesson", ID: "lesson-1"},
		Status: domain.ThreadStatusOpen, CreatedAt: now, UpdatedAt: now,
	}
	store := &fakeStore{
		spaces: map[uuid.UUID]domain.Space{space.ID: space}, spaceKeys: map[string]uuid.UUID{space.Key: space.ID},
		threads: map[uuid.UUID]domain.Thread{thread.ID: thread}, comments: map[uuid.UUID]domain.Comment{},
		attachments: map[uuid.UUID]domain.Attachment{},
	}
	service := &commentuc.Service{
		Spaces: fakeSpaces{store}, Threads: fakeThreads{store}, Comments: fakeComments{store},
		Attachments: fakeAttachments{store}, Outbox: fakeOutbox{store}, Tx: fakeTx{store},
		Now: func() time.Time { return now }, NewID: uuid.New,
	}
	return service, store, domain.Actor{UserID: uuid.New(), Role: "STUDENT"}, thread
}

type fakeTx struct{ *fakeStore }
type fakeSpaces struct{ *fakeStore }
type fakeThreads struct{ *fakeStore }
type fakeComments struct{ *fakeStore }
type fakeAttachments struct{ *fakeStore }
type fakeOutbox struct{ *fakeStore }

func (s fakeTx) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	s.fakeStore.txCalls++
	return fn(ctx)
}

func (s fakeSpaces) Create(_ context.Context, item domain.Space) error {
	s.spaces[item.ID] = item
	s.spaceKeys[item.Key] = item.ID
	return nil
}

func (s fakeSpaces) GetByID(_ context.Context, id uuid.UUID) (domain.Space, error) {
	item, ok := s.spaces[id]
	if !ok {
		return domain.Space{}, domain.ErrNotFound
	}
	return item, nil
}

func (s fakeSpaces) GetByKey(_ context.Context, key string) (domain.Space, error) {
	id, ok := s.spaceKeys[key]
	if !ok {
		return domain.Space{}, domain.ErrNotFound
	}
	return s.spaces[id], nil
}

func (s fakeSpaces) Update(_ context.Context, item domain.Space) error {
	s.spaces[item.ID] = item
	return nil
}
func (s fakeSpaces) List(_ context.Context, _ repository.SpaceListQuery) ([]domain.Space, error) {
	return nil, nil
}

func (s fakeThreads) Ensure(_ context.Context, item domain.Thread) (domain.Thread, error) {
	for _, existing := range s.threads {
		if existing.SpaceID == item.SpaceID && existing.Resource == item.Resource {
			return existing, nil
		}
	}
	s.threads[item.ID] = item
	return item, nil
}

func (s fakeThreads) GetByID(_ context.Context, id uuid.UUID) (domain.Thread, error) {
	item, ok := s.threads[id]
	if !ok {
		return domain.Thread{}, domain.ErrNotFound
	}
	return item, nil
}

func (s fakeThreads) GetByResource(_ context.Context, spaceID uuid.UUID, resource domain.ResourceReference) (domain.Thread, error) {
	for _, item := range s.threads {
		if item.SpaceID == spaceID && item.Resource == resource {
			return item, nil
		}
	}
	return domain.Thread{}, domain.ErrNotFound
}

func (s fakeThreads) Update(_ context.Context, item domain.Thread) error {
	s.threads[item.ID] = item
	return nil
}
func (s fakeThreads) NextSequence(_ context.Context, id uuid.UUID) (int64, error) {
	item, ok := s.threads[id]
	if !ok {
		return 0, domain.ErrNotFound
	}
	if item.Status != domain.ThreadStatusOpen {
		return 0, domain.ErrThreadNotWritable
	}
	item.LastSequence++
	s.threads[id] = item
	return item.LastSequence, nil
}
func (s fakeThreads) NextSequenceAnyState(_ context.Context, id uuid.UUID) (int64, error) {
	item, ok := s.threads[id]
	if !ok {
		return 0, domain.ErrNotFound
	}
	item.LastSequence++
	s.threads[id] = item
	return item.LastSequence, nil
}

func (s fakeThreads) RecordCommentCreated(_ context.Context, id uuid.UUID, root bool) error {
	item := s.threads[id]
	item.CommentCount++
	if root {
		item.RootCommentCount++
	}
	s.threads[id] = item
	return nil
}

func (s fakeComments) Create(_ context.Context, item domain.Comment) error {
	s.comments[item.ID] = item
	return nil
}
func (s fakeComments) GetByID(_ context.Context, id uuid.UUID) (domain.Comment, error) {
	item, ok := s.comments[id]
	if !ok {
		return domain.Comment{}, domain.ErrNotFound
	}
	return item, nil
}

func (s fakeComments) GetByIDForUpdate(ctx context.Context, id uuid.UUID) (domain.Comment, error) {
	return s.GetByID(ctx, id)
}
func (s fakeComments) GetByIdempotencyKey(_ context.Context, authorID, key uuid.UUID) (domain.Comment, error) {
	for _, item := range s.comments {
		if item.AuthorID == authorID && item.IdempotencyKey == key {
			return item, nil
		}
	}
	return domain.Comment{}, domain.ErrNotFound
}
func (s fakeComments) LockIdempotencyKey(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (s fakeComments) List(_ context.Context, query repository.CommentListQuery) ([]domain.Comment, error) {
	items := make([]domain.Comment, 0)
	for _, item := range s.comments {
		if item.ThreadID == query.ThreadID && equalOptional(item.ParentID, query.ParentID) {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	if len(items) > query.Limit {
		items = items[:query.Limit]
	}
	return items, nil
}
func (s fakeComments) ListChanges(_ context.Context, query repository.CommentChangeQuery) ([]domain.Comment, error) {
	items := make([]domain.Comment, 0)
	for _, item := range s.comments {
		if item.ThreadID == query.ThreadID && item.Sequence > query.AfterSequence {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Sequence < items[j].Sequence })
	return items, nil
}
func (s fakeComments) UpdateContent(_ context.Context, item domain.Comment, expected int) error {
	current := s.comments[item.ID]
	if current.Version != expected {
		return domain.ErrEditConflict
	}
	s.comments[item.ID] = item
	return nil
}
func (s fakeComments) MarkDeleted(ctx context.Context, item domain.Comment, expected int) error {
	return s.UpdateContent(ctx, item, expected)
}
func (s fakeComments) IncrementReplyCount(_ context.Context, _, id uuid.UUID) error {
	item := s.comments[id]
	item.DirectRepliesCount++
	s.comments[id] = item
	return nil
}
func (s fakeComments) AdvanceSequence(_ context.Context, id uuid.UUID, sequence int64, updatedAt time.Time) (domain.Comment, error) {
	item, ok := s.comments[id]
	if !ok {
		return domain.Comment{}, domain.ErrNotFound
	}
	item.Sequence = sequence
	item.Version++
	item.UpdatedAt = updatedAt
	s.comments[id] = item
	return item, nil
}

func (s fakeAttachments) Create(_ context.Context, item domain.Attachment) error {
	s.attachments[item.ID] = item
	return nil
}
func (s fakeAttachments) GetByID(_ context.Context, id uuid.UUID) (domain.Attachment, error) {
	item, ok := s.attachments[id]
	if !ok {
		return domain.Attachment{}, domain.ErrNotFound
	}
	return item, nil
}
func (s fakeAttachments) GetByIDForUpdate(ctx context.Context, id uuid.UUID) (domain.Attachment, error) {
	return s.GetByID(ctx, id)
}
func (s fakeAttachments) ListByComment(_ context.Context, threadID, commentID uuid.UUID) ([]domain.Attachment, error) {
	items := make([]domain.Attachment, 0)
	for _, item := range s.attachments {
		if item.ThreadID == threadID && item.CommentID != nil && *item.CommentID == commentID && item.Status != domain.AttachmentStatusDeleted {
			items = append(items, item)
		}
	}
	return items, nil
}
func (s fakeAttachments) CountPendingByUploader(_ context.Context, threadID, uploaderID uuid.UUID, now time.Time) (int, error) {
	count := 0
	for _, item := range s.attachments {
		if item.ThreadID == threadID && item.UploaderID == uploaderID && item.Status == domain.AttachmentStatusPending &&
			item.CommentID == nil && item.ExpiresAt.After(now) {
			count++
		}
	}
	return count, nil
}
func (s fakeAttachments) BindToComment(_ context.Context, id, threadID, commentID, uploaderID uuid.UUID) error {
	item := s.attachments[id]
	item.ThreadID = threadID
	item.CommentID = &commentID
	item.UploaderID = uploaderID
	item.Status = domain.AttachmentStatusProcessing
	s.attachments[id] = item
	return nil
}
func (s fakeAttachments) UpdateStatus(_ context.Context, item domain.Attachment) error {
	s.attachments[item.ID] = item
	return nil
}
func (s fakeAttachments) ListExpired(_ context.Context, before time.Time, limit int) ([]domain.Attachment, error) {
	items := make([]domain.Attachment, 0)
	for _, item := range s.attachments {
		if item.Status == domain.AttachmentStatusPending && item.ExpiresAt.Before(before) {
			items = append(items, item)
		}
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}
func (s fakeAttachments) ListForActivation(_ context.Context, now time.Time, limit int) ([]domain.Attachment, error) {
	items := make([]domain.Attachment, 0)
	for _, item := range s.attachments {
		if item.Status == domain.AttachmentStatusProcessing && (item.ActivationNextAttemptAt == nil || !item.ActivationNextAttemptAt.After(now)) {
			items = append(items, item)
		}
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}
func (s fakeAttachments) MarkReadyIfProcessing(_ context.Context, id uuid.UUID, now time.Time) (domain.Attachment, bool, error) {
	item, ok := s.attachments[id]
	if !ok || item.Status != domain.AttachmentStatusProcessing {
		return domain.Attachment{}, false, nil
	}
	item.Status = domain.AttachmentStatusReady
	item.ActivatedAt = &now
	item.ActivationNextAttemptAt = nil
	item.UpdatedAt = now
	s.attachments[id] = item
	return item, true, nil
}
func (s fakeAttachments) RecordActivationFailure(_ context.Context, id uuid.UUID, next time.Time, message string, maxAttempts int) (domain.Attachment, bool, error) {
	item, ok := s.attachments[id]
	if !ok || item.Status != domain.AttachmentStatusProcessing {
		return domain.Attachment{}, false, nil
	}
	item.ActivationAttempts++
	item.LastError = message
	terminal := item.ActivationAttempts >= maxAttempts
	if terminal {
		item.Status = domain.AttachmentStatusFailed
		item.ActivationNextAttemptAt = nil
	} else {
		item.ActivationNextAttemptAt = &next
	}
	s.attachments[id] = item
	return item, terminal, nil
}
func (s fakeAttachments) ListForDeletion(_ context.Context, now time.Time, limit int) ([]domain.Attachment, error) {
	items := make([]domain.Attachment, 0)
	for _, item := range s.attachments {
		if item.Status == domain.AttachmentStatusDeleted && item.StorageDeletedAt == nil &&
			(item.DeleteNextAttemptAt == nil || !item.DeleteNextAttemptAt.After(now)) {
			items = append(items, item)
		}
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}
func (s fakeAttachments) MarkStorageDeleted(_ context.Context, id uuid.UUID, now time.Time) error {
	item, ok := s.attachments[id]
	if !ok {
		return domain.ErrNotFound
	}
	item.StorageDeletedAt = &now
	item.DeleteNextAttemptAt = nil
	s.attachments[id] = item
	return nil
}
func (s fakeAttachments) RecordDeleteFailure(_ context.Context, id uuid.UUID, next time.Time, message string) error {
	item, ok := s.attachments[id]
	if !ok {
		return domain.ErrNotFound
	}
	item.DeleteAttempts++
	item.DeleteNextAttemptAt = &next
	item.LastError = message
	s.attachments[id] = item
	return nil
}

func (s fakeOutbox) Add(_ context.Context, event domain.OutboxEvent) error {
	s.outbox = append(s.outbox, event)
	return nil
}
func (s fakeOutbox) ClaimPending(context.Context, time.Time, time.Time, int) ([]domain.OutboxEvent, error) {
	return nil, nil
}
func (s fakeOutbox) MarkPublished(context.Context, uuid.UUID, time.Time) error      { return nil }
func (s fakeOutbox) MarkFailed(context.Context, uuid.UUID, time.Time, string) error { return nil }

func activeComment(id, threadID, authorID uuid.UUID, now time.Time) domain.Comment {
	return domain.Comment{ID: id, ThreadID: threadID, AuthorID: authorID, RootID: id,
		Path: []uuid.UUID{id}, Body: "body", Status: domain.CommentStatusActive,
		Version: 1, IdempotencyKey: uuid.New(), CreatedAt: now, UpdatedAt: now}
}

func withSequence(thread domain.Thread, sequence int64) domain.Thread {
	thread.LastSequence = sequence
	return thread
}
func idSequence(ids ...uuid.UUID) func() uuid.UUID {
	index := 0
	return func() uuid.UUID { id := ids[index]; index++; return id }
}
func equalOptional(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
