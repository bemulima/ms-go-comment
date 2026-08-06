package access

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	"github.com/google/uuid"
)

const (
	grantBytes        = 32
	defaultMaximumTTL = 5 * time.Minute
)

type Service struct {
	Spaces     repository.SpaceRepository
	Threads    repository.ThreadRepository
	Grants     repository.AccessGrantRepository
	Now        func() time.Time
	Random     io.Reader
	MaximumTTL time.Duration
	NewID      func() uuid.UUID
}

type CreateGrantInput struct {
	Issuer      string
	UserID      uuid.UUID
	SpaceKey    string
	Resource    domain.ResourceReference
	Permissions domain.AccessPermission
	TTL         time.Duration
}

type MintedGrant struct {
	Grant       string
	ExpiresAt   time.Time
	Permissions domain.AccessPermission
}

type ThreadView struct {
	Thread domain.Thread
	Policy domain.Policy
}

func (s Service) CreateGrant(ctx context.Context, input CreateGrantInput) (MintedGrant, error) {
	if input.UserID == uuid.Nil || input.TTL < time.Second || input.TTL > s.maximumTTL() || !input.Permissions.Valid() {
		return MintedGrant{}, fmt.Errorf("%w: user, permissions, and bounded TTL are required", domain.ErrInvalidAccessGrant)
	}
	if err := input.Resource.Validate(); err != nil {
		return MintedGrant{}, fmt.Errorf("%w: resource is invalid", domain.ErrInvalidAccessGrant)
	}
	if err := domain.ValidateSpaceKey(input.SpaceKey); err != nil {
		return MintedGrant{}, fmt.Errorf("%w: space key is invalid", domain.ErrInvalidAccessGrant)
	}
	space, err := s.Spaces.GetByKey(ctx, input.SpaceKey)
	if err != nil {
		return MintedGrant{}, mapNotFound(err, domain.ErrSpaceNotFound)
	}
	if space.Status != domain.SpaceStatusActive || space.AccessMode != domain.AccessModeContextGrant {
		return MintedGrant{}, domain.ErrSpaceNotFound
	}
	secret := make([]byte, grantBytes)
	reader := s.Random
	if reader == nil {
		reader = rand.Reader
	}
	if _, err := io.ReadFull(reader, secret); err != nil {
		return MintedGrant{}, fmt.Errorf("generate access grant: %w", err)
	}
	now := s.now()
	hash := sha256.Sum256(secret)
	item := domain.AccessGrant{
		GrantHash: hash[:], Issuer: input.Issuer, UserID: input.UserID,
		SpaceID: space.ID, Resource: input.Resource, Permissions: input.Permissions,
		CreatedAt: now, ExpiresAt: now.Add(input.TTL),
	}
	if err := item.Validate(); err != nil {
		return MintedGrant{}, err
	}
	if err := s.Grants.Store(ctx, item); err != nil {
		return MintedGrant{}, err
	}
	return MintedGrant{
		Grant: base64.RawURLEncoding.EncodeToString(secret), ExpiresAt: item.ExpiresAt,
		Permissions: item.Permissions,
	}, nil
}

func (s Service) Permissions(
	ctx context.Context,
	actor domain.Actor,
	space domain.Space,
	resource domain.ResourceReference,
) (domain.AccessPermission, error) {
	if err := actor.Validate(); err != nil {
		return 0, err
	}
	if space.Status != domain.SpaceStatusActive {
		return 0, domain.ErrSpaceNotFound
	}
	if space.AccessMode == domain.AccessModeAuthenticated {
		return domain.FullAccessPermissions(), nil
	}
	if space.AccessMode != domain.AccessModeContextGrant {
		return 0, domain.ErrAccessRequired
	}
	token, ok := GrantTokenFromContext(ctx)
	if !ok {
		return 0, domain.ErrAccessRequired
	}
	secret, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(secret) != grantBytes {
		return 0, domain.ErrAccessRequired
	}
	hash := sha256.Sum256(secret)
	item, err := s.Grants.Resolve(ctx, hash[:], s.now())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return 0, domain.ErrAccessRequired
		}
		return 0, err
	}
	if item.Validate() != nil || item.UserID != actor.UserID || item.SpaceID != space.ID ||
		item.Resource != resource || !item.ExpiresAt.After(s.now()) {
		return 0, domain.ErrAccessRequired
	}
	return item.Permissions, nil
}

func (s Service) EnsureThread(ctx context.Context, spaceKey string, resource domain.ResourceReference) (ThreadView, error) {
	space, err := s.privateSpace(ctx, spaceKey, resource)
	if err != nil {
		return ThreadView{}, err
	}
	now := s.now()
	item, err := s.Threads.Ensure(ctx, domain.Thread{
		ID: s.newID(), SpaceID: space.ID, Resource: resource,
		Status: domain.ThreadStatusOpen, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return ThreadView{}, err
	}
	policy, err := domain.ApplyPolicy(space.Policy, item.PolicyOverrides)
	return ThreadView{Thread: item, Policy: policy}, err
}

func (s Service) GetThreadByResource(ctx context.Context, spaceKey string, resource domain.ResourceReference) (ThreadView, error) {
	space, err := s.privateSpace(ctx, spaceKey, resource)
	if err != nil {
		return ThreadView{}, err
	}
	item, err := s.Threads.GetByResource(ctx, space.ID, resource)
	if err != nil {
		return ThreadView{}, mapNotFound(err, domain.ErrThreadNotFound)
	}
	policy, err := domain.ApplyPolicy(space.Policy, item.PolicyOverrides)
	return ThreadView{Thread: item, Policy: policy}, err
}

func (s Service) DeleteExpired(ctx context.Context, limit int) (int, error) {
	if limit < 1 {
		return 0, fmt.Errorf("%w: cleanup limit must be positive", domain.ErrValidation)
	}
	return s.Grants.DeleteExpired(ctx, s.now(), limit)
}

func (s Service) privateSpace(ctx context.Context, key string, resource domain.ResourceReference) (domain.Space, error) {
	if err := domain.ValidateSpaceKey(key); err != nil {
		return domain.Space{}, err
	}
	if err := resource.Validate(); err != nil {
		return domain.Space{}, err
	}
	space, err := s.Spaces.GetByKey(ctx, key)
	if err != nil {
		return domain.Space{}, mapNotFound(err, domain.ErrSpaceNotFound)
	}
	if space.Status != domain.SpaceStatusActive || space.AccessMode != domain.AccessModeContextGrant {
		return domain.Space{}, domain.ErrSpaceNotFound
	}
	return space, nil
}

func (s Service) maximumTTL() time.Duration {
	if s.MaximumTTL >= time.Second {
		return s.MaximumTTL
	}
	return defaultMaximumTTL
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) newID() uuid.UUID {
	if s.NewID != nil {
		return s.NewID()
	}
	return uuid.New()
}

func mapNotFound(err, target error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return target
	}
	return err
}
