package admin_test

import (
	"context"
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

type fakeStore struct {
	spaces      map[uuid.UUID]domain.Space
	threads     map[uuid.UUID]domain.Thread
	updateCalls int
	outbox      []domain.OutboxEvent
}

func fixture() (*adminuc.Service, *fakeStore, domain.Actor, domain.Actor) {
	now := time.Date(2026, 8, 5, 18, 0, 0, 0, time.UTC)
	store := &fakeStore{spaces: map[uuid.UUID]domain.Space{}, threads: map[uuid.UUID]domain.Thread{}}
	service := &adminuc.Service{Spaces: fakeSpaces{store}, Threads: fakeThreads{store}, Outbox: fakeOutbox{store},
		Tx: fakeTx{}, Now: func() time.Time { return now }, NewID: uuid.New}
	return service, store, domain.Actor{UserID: uuid.New(), Role: "ADMIN"}, domain.Actor{UserID: uuid.New(), Role: "MODERATOR"}
}

type fakeSpaces struct{ *fakeStore }
type fakeThreads struct{ *fakeStore }
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
