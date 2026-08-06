package access_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	accessuc "github.com/bemulima/ms-go-comment/internal/usecase/access"
	"github.com/google/uuid"
)

func TestService_CreateGrantStoresOnlyHash(t *testing.T) {
	t.Parallel()

	service, store, _, space, resource := accessFixture()
	secret := bytes.Repeat([]byte{7}, 32)
	service.Random = bytes.NewReader(secret)
	userID := uuid.New()
	result, err := service.CreateGrant(context.Background(), accessuc.CreateGrantInput{
		Issuer: "ms-go-course", UserID: userID, SpaceKey: space.Key, Resource: resource,
		Permissions: domain.FullAccessPermissions(), TTL: 2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("CreateGrant() error = %v", err)
	}
	if result.Grant != base64.RawURLEncoding.EncodeToString(secret) || !result.ExpiresAt.Equal(service.Now().Add(2*time.Minute)) {
		t.Fatalf("minted grant = %#v", result)
	}
	stored := store.onlyGrant(t)
	wantHash := sha256.Sum256(secret)
	if !bytes.Equal(stored.GrantHash, wantHash[:]) || bytes.Contains(stored.GrantHash, []byte(result.Grant)) {
		t.Fatalf("stored hash = %x", stored.GrantHash)
	}
	if stored.UserID != userID || stored.SpaceID != space.ID || stored.Resource != resource || stored.Issuer != "ms-go-course" {
		t.Fatalf("stored binding = %#v", stored)
	}
}

func TestService_CreateGrantValidatesBoundary(t *testing.T) {
	t.Parallel()

	service, store, _, space, resource := accessFixture()
	base := accessuc.CreateGrantInput{
		Issuer: "ms-go-course", UserID: uuid.New(), SpaceKey: space.Key, Resource: resource,
		Permissions: domain.AccessPermissionRead, TTL: time.Minute,
	}
	tests := []struct {
		name   string
		mutate func(*accessuc.CreateGrantInput, *accessStore)
		want   error
	}{
		{name: "ttl too short", mutate: func(input *accessuc.CreateGrantInput, _ *accessStore) { input.TTL = time.Millisecond }, want: domain.ErrInvalidAccessGrant},
		{name: "ttl too long", mutate: func(input *accessuc.CreateGrantInput, _ *accessStore) { input.TTL = 6 * time.Minute }, want: domain.ErrInvalidAccessGrant},
		{name: "upload without write", mutate: func(input *accessuc.CreateGrantInput, _ *accessStore) {
			input.Permissions = domain.AccessPermissionRead | domain.AccessPermissionUpload
		}, want: domain.ErrInvalidAccessGrant},
		{name: "invalid space key", mutate: func(input *accessuc.CreateGrantInput, _ *accessStore) {
			input.SpaceKey = "Course Private"
		}, want: domain.ErrInvalidAccessGrant},
		{name: "authenticated space", mutate: func(_ *accessuc.CreateGrantInput, store *accessStore) {
			item := store.space
			item.AccessMode = domain.AccessModeAuthenticated
			store.space = item
		}, want: domain.ErrSpaceNotFound},
		{name: "disabled space", mutate: func(_ *accessuc.CreateGrantInput, store *accessStore) {
			item := store.space
			item.Status = domain.SpaceStatusDisabled
			store.space = item
		}, want: domain.ErrSpaceNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixtureService := service
			fixtureStore := *store
			fixtureStore.grants = make(map[string]domain.AccessGrant)
			fixtureService.Spaces = accessSpaces{&fixtureStore}
			fixtureService.Threads = accessThreads{&fixtureStore}
			fixtureService.Grants = accessGrants{&fixtureStore}
			input := base
			tt.mutate(&input, &fixtureStore)
			if _, err := fixtureService.CreateGrant(context.Background(), input); !errors.Is(err, tt.want) {
				t.Fatalf("CreateGrant() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestService_PermissionsEnforcesBindingAndExpiry(t *testing.T) {
	t.Parallel()

	service, store, actor, space, resource := accessFixture()
	secret := bytes.Repeat([]byte{9}, 32)
	hash := sha256.Sum256(secret)
	item := domain.AccessGrant{
		GrantHash: hash[:], Issuer: "ms-go-course", UserID: actor.UserID, SpaceID: space.ID,
		Resource: resource, Permissions: domain.AccessPermissionRead | domain.AccessPermissionWrite,
		CreatedAt: service.Now(), ExpiresAt: service.Now().Add(time.Minute),
	}
	store.grants[string(item.GrantHash)] = item
	ctx := accessuc.WithGrantToken(context.Background(), base64.RawURLEncoding.EncodeToString(secret))
	permissions, err := service.Permissions(ctx, actor, space, resource)
	if err != nil || permissions != item.Permissions {
		t.Fatalf("Permissions() = %d, error=%v", permissions, err)
	}

	tests := []struct {
		name  string
		ctx   context.Context
		actor domain.Actor
		space domain.Space
		item  domain.AccessGrant
		clock time.Time
	}{
		{name: "missing token", ctx: context.Background(), actor: actor, space: space, item: item, clock: service.Now()},
		{name: "wrong user", ctx: ctx, actor: domain.Actor{UserID: uuid.New(), Role: actor.Role}, space: space, item: item, clock: service.Now()},
		{name: "wrong space", ctx: ctx, actor: actor, space: func() domain.Space { value := space; value.ID = uuid.New(); return value }(), item: item, clock: service.Now()},
		{name: "wrong resource", ctx: ctx, actor: actor, space: space, item: func() domain.AccessGrant { value := item; value.Resource.ID = "other"; return value }(), clock: service.Now()},
		{name: "expired", ctx: ctx, actor: actor, space: space, item: item, clock: item.ExpiresAt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixtureStore := *store
			fixtureStore.grants = map[string]domain.AccessGrant{string(hash[:]): tt.item}
			fixtureService := service
			fixtureService.Grants = accessGrants{&fixtureStore}
			fixtureService.Now = func() time.Time { return tt.clock }
			if _, err := fixtureService.Permissions(tt.ctx, tt.actor, tt.space, resource); !errors.Is(err, domain.ErrAccessRequired) {
				t.Fatalf("Permissions() error = %v", err)
			}
		})
	}
}

func TestService_InternalThreadOperationsAndCleanup(t *testing.T) {
	t.Parallel()

	service, store, _, _, resource := accessFixture()
	ensured, err := service.EnsureThread(context.Background(), store.space.Key, resource)
	if err != nil || ensured.Thread.Resource != resource || ensured.Policy != store.space.Policy {
		t.Fatalf("EnsureThread() = %#v, error=%v", ensured, err)
	}
	resolved, err := service.GetThreadByResource(context.Background(), store.space.Key, resource)
	if err != nil || resolved.Thread.ID != ensured.Thread.ID {
		t.Fatalf("GetThreadByResource() = %#v, error=%v", resolved, err)
	}

	now := service.Now()
	store.grants["expired"] = domain.AccessGrant{ExpiresAt: now.Add(-time.Second)}
	store.grants["active"] = domain.AccessGrant{ExpiresAt: now.Add(time.Second)}
	deleted, err := service.DeleteExpired(context.Background(), 1)
	if err != nil || deleted != 1 || len(store.grants) != 1 {
		t.Fatalf("DeleteExpired() deleted=%d grants=%d error=%v", deleted, len(store.grants), err)
	}
	if _, err := service.DeleteExpired(context.Background(), 0); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("DeleteExpired(0) error = %v", err)
	}
}

type accessStore struct {
	space   domain.Space
	threads map[uuid.UUID]domain.Thread
	grants  map[string]domain.AccessGrant
}

func accessFixture() (accessuc.Service, *accessStore, domain.Actor, domain.Space, domain.ResourceReference) {
	now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	space := domain.Space{
		ID: uuid.New(), Key: "course.private", Name: "Course private", Status: domain.SpaceStatusActive,
		AccessMode: domain.AccessModeContextGrant, Policy: domain.DefaultPolicy(), CreatedBy: uuid.New(),
		CreatedAt: now, UpdatedAt: now,
	}
	store := &accessStore{space: space, threads: make(map[uuid.UUID]domain.Thread), grants: make(map[string]domain.AccessGrant)}
	service := accessuc.Service{
		Spaces: accessSpaces{store}, Threads: accessThreads{store}, Grants: accessGrants{store}, Now: func() time.Time { return now },
		MaximumTTL: 5 * time.Minute, NewID: uuid.New,
	}
	return service, store, domain.Actor{UserID: uuid.New(), Role: "STUDENT"}, space,
		domain.ResourceReference{Type: "lesson", ID: "lesson-1"}
}

type accessSpaces struct{ *accessStore }
type accessThreads struct{ *accessStore }
type accessGrants struct{ *accessStore }

func (s accessSpaces) Create(context.Context, domain.Space) error { return nil }
func (s accessSpaces) GetByID(_ context.Context, id uuid.UUID) (domain.Space, error) {
	if id != s.space.ID {
		return domain.Space{}, domain.ErrNotFound
	}
	return s.space, nil
}
func (s accessSpaces) GetByKey(_ context.Context, key string) (domain.Space, error) {
	if key != s.space.Key {
		return domain.Space{}, domain.ErrNotFound
	}
	return s.space, nil
}
func (s accessSpaces) Update(context.Context, domain.Space) error { return nil }
func (s accessSpaces) List(context.Context, repository.SpaceListQuery) ([]domain.Space, error) {
	return nil, nil
}
func (s accessThreads) Ensure(_ context.Context, item domain.Thread) (domain.Thread, error) {
	for _, existing := range s.threads {
		if existing.SpaceID == item.SpaceID && existing.Resource == item.Resource {
			return existing, nil
		}
	}
	s.threads[item.ID] = item
	return item, nil
}
func (s accessThreads) GetByID(_ context.Context, id uuid.UUID) (domain.Thread, error) {
	item, ok := s.threads[id]
	if !ok {
		return domain.Thread{}, domain.ErrNotFound
	}
	return item, nil
}
func (s accessThreads) GetByResource(_ context.Context, spaceID uuid.UUID, resource domain.ResourceReference) (domain.Thread, error) {
	for _, item := range s.threads {
		if item.SpaceID == spaceID && item.Resource == resource {
			return item, nil
		}
	}
	return domain.Thread{}, domain.ErrNotFound
}
func (s accessThreads) List(context.Context, repository.ThreadListQuery) ([]domain.Thread, error) {
	return nil, nil
}
func (s accessThreads) Update(context.Context, domain.Thread) error                    { return nil }
func (s accessThreads) NextSequence(context.Context, uuid.UUID) (int64, error)         { return 0, nil }
func (s accessThreads) NextSequenceAnyState(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (s accessThreads) RecordCommentCreated(context.Context, uuid.UUID, bool) error    { return nil }
func (s accessGrants) Store(_ context.Context, item domain.AccessGrant) error {
	s.grants[string(item.GrantHash)] = item
	return nil
}
func (s accessGrants) Resolve(_ context.Context, hash []byte, now time.Time) (domain.AccessGrant, error) {
	item, ok := s.grants[string(hash)]
	if !ok || !item.ExpiresAt.After(now) {
		return domain.AccessGrant{}, domain.ErrNotFound
	}
	return item, nil
}
func (s accessGrants) DeleteExpired(_ context.Context, now time.Time, limit int) (int, error) {
	deleted := 0
	for key, item := range s.grants {
		if deleted == limit {
			break
		}
		if !item.ExpiresAt.After(now) {
			delete(s.grants, key)
			deleted++
		}
	}
	return deleted, nil
}
func (s *accessStore) onlyGrant(t *testing.T) domain.AccessGrant {
	t.Helper()
	if len(s.grants) != 1 {
		t.Fatalf("grant count = %d", len(s.grants))
	}
	for _, item := range s.grants {
		return item
	}
	return domain.AccessGrant{}
}
