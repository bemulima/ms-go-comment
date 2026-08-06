package realtime

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
	defaultTicketTTL = 30 * time.Second
	ticketBytes      = 32
)

type TicketService struct {
	Spaces  repository.SpaceRepository
	Threads repository.ThreadRepository
	Tickets repository.RealtimeTicketRepository
	Access  AccessAuthorizer
	Now     func() time.Time
	Random  io.Reader
	TTL     time.Duration
}

type AccessAuthorizer interface {
	Permissions(context.Context, domain.Actor, domain.Space, domain.ResourceReference) (domain.AccessPermission, error)
}

type MintTicketInput struct {
	ThreadID     uuid.UUID
	LastSequence *int64
}

type MintedTicket struct {
	Ticket    string
	ExpiresAt time.Time
	Protocol  string
}

type Session struct {
	UserID                uuid.UUID
	ThreadID              uuid.UUID
	Permissions           domain.RealtimePermission
	RequestedLastSequence *int64
	CurrentSequence       int64
	AllowedOrigins        []string
}

func (s TicketService) Mint(ctx context.Context, actor domain.Actor, input MintTicketInput) (MintedTicket, error) {
	if err := actor.Validate(); err != nil {
		return MintedTicket{}, err
	}
	if input.ThreadID == uuid.Nil || (input.LastSequence != nil && *input.LastSequence < 0) {
		return MintedTicket{}, fmt.Errorf("%w: thread and non-negative sequence are required", domain.ErrInvalidRealtimeTicket)
	}
	thread, space, policy, err := s.loadThread(ctx, input.ThreadID)
	if err != nil {
		return MintedTicket{}, err
	}
	accessPermissions, err := s.accessPermissions(ctx, actor, space, thread.Resource)
	if err != nil {
		return MintedTicket{}, err
	}
	permissions := domain.RealtimePermissionRead
	if thread.Status == domain.ThreadStatusOpen && accessPermissions.Includes(domain.AccessPermissionWrite) {
		permissions |= domain.RealtimePermissionWrite
		if accessPermissions.Includes(domain.AccessPermissionUpload) && policy.AllowImages && policy.MaxAttachments > 0 {
			permissions |= domain.RealtimePermissionUpload
		}
	}
	secret := make([]byte, ticketBytes)
	reader := s.Random
	if reader == nil {
		reader = rand.Reader
	}
	if _, err := io.ReadFull(reader, secret); err != nil {
		return MintedTicket{}, fmt.Errorf("generate realtime ticket: %w", err)
	}
	now := s.now()
	hash := sha256.Sum256(secret)
	ticket := domain.RealtimeTicket{
		TicketHash: hash[:], UserID: actor.UserID, ThreadID: thread.ID, Permissions: permissions,
		RequestedLastSequence: input.LastSequence, CreatedAt: now, ExpiresAt: now.Add(s.ttl()),
	}
	if err := ticket.Validate(); err != nil {
		return MintedTicket{}, err
	}
	if err := s.Tickets.Store(ctx, ticket); err != nil {
		return MintedTicket{}, err
	}
	return MintedTicket{
		Ticket: base64.RawURLEncoding.EncodeToString(secret), ExpiresAt: ticket.ExpiresAt, Protocol: "comment.v1",
	}, nil
}

func (s TicketService) Consume(ctx context.Context, opaque string) (Session, error) {
	secret, err := base64.RawURLEncoding.DecodeString(opaque)
	if err != nil || len(secret) != ticketBytes {
		return Session{}, domain.ErrRealtimeTicketInvalid
	}
	hash := sha256.Sum256(secret)
	ticket, err := s.Tickets.Consume(ctx, hash[:], s.now())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return Session{}, domain.ErrRealtimeTicketInvalid
		}
		return Session{}, err
	}
	thread, space, policy, err := s.loadThread(ctx, ticket.ThreadID)
	if err != nil {
		return Session{}, domain.ErrRealtimeTicketInvalid
	}
	permissions := ticket.Permissions
	if thread.Status != domain.ThreadStatusOpen {
		permissions = domain.RealtimePermissionRead
	} else if !policy.AllowImages || policy.MaxAttachments == 0 {
		permissions &^= domain.RealtimePermissionUpload
	}
	return Session{
		UserID: ticket.UserID, ThreadID: ticket.ThreadID, Permissions: permissions,
		RequestedLastSequence: ticket.RequestedLastSequence, CurrentSequence: thread.LastSequence,
		AllowedOrigins: append([]string(nil), space.AllowedOrigins...),
	}, nil
}

func (s TicketService) CurrentSequence(ctx context.Context, threadID uuid.UUID) (int64, error) {
	thread, _, _, err := s.loadThread(ctx, threadID)
	if err != nil {
		return 0, err
	}
	return thread.LastSequence, nil
}

func (s TicketService) DeleteExpired(ctx context.Context, limit int) (int, error) {
	if limit < 1 {
		return 0, fmt.Errorf("%w: cleanup limit must be positive", domain.ErrValidation)
	}
	return s.Tickets.DeleteExpired(ctx, s.now(), limit)
}

func (s TicketService) loadThread(ctx context.Context, id uuid.UUID) (domain.Thread, domain.Space, domain.Policy, error) {
	thread, err := s.Threads.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			err = domain.ErrThreadNotFound
		}
		return domain.Thread{}, domain.Space{}, domain.Policy{}, err
	}
	space, err := s.Spaces.GetByID(ctx, thread.SpaceID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			err = domain.ErrSpaceNotFound
		}
		return domain.Thread{}, domain.Space{}, domain.Policy{}, err
	}
	if space.Status != domain.SpaceStatusActive || thread.Status == domain.ThreadStatusHidden {
		return domain.Thread{}, domain.Space{}, domain.Policy{}, domain.ErrThreadNotFound
	}
	policy, err := domain.ApplyPolicy(space.Policy, thread.PolicyOverrides)
	return thread, space, policy, err
}

func (s TicketService) accessPermissions(
	ctx context.Context,
	actor domain.Actor,
	space domain.Space,
	resource domain.ResourceReference,
) (domain.AccessPermission, error) {
	if space.AccessMode == domain.AccessModeAuthenticated {
		return domain.FullAccessPermissions(), nil
	}
	if space.AccessMode != domain.AccessModeContextGrant || s.Access == nil {
		return 0, domain.ErrAccessRequired
	}
	permissions, err := s.Access.Permissions(ctx, actor, space, resource)
	if err != nil {
		return 0, err
	}
	if !permissions.Includes(domain.AccessPermissionRead) {
		return 0, domain.ErrForbidden
	}
	return permissions, nil
}

func (s TicketService) ttl() time.Duration {
	if s.TTL > 0 && s.TTL <= defaultTicketTTL {
		return s.TTL
	}
	return defaultTicketTTL
}

func (s TicketService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
