package realtime_test

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
	realtimeuc "github.com/bemulima/ms-go-comment/internal/usecase/realtime"
	"github.com/google/uuid"
)

func TestTicketService_MintAndConsumeIsSingleUse(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	space, thread := realtimeFixture(now)
	tickets := newFakeTickets()
	service := realtimeuc.TicketService{
		Spaces: fakeSpaces{item: space}, Threads: fakeThreads{item: thread}, Tickets: tickets,
		Now: func() time.Time { return now }, Random: bytes.NewReader(bytes.Repeat([]byte{7}, 32)),
	}
	lastSequence := int64(8)
	actor := domain.Actor{UserID: uuid.New(), Role: "STUDENT"}
	minted, err := service.Mint(context.Background(), actor, realtimeuc.MintTicketInput{
		ThreadID: thread.ID, LastSequence: &lastSequence,
	})
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	if minted.Ticket == "" || minted.Protocol != "comment.v1" || !minted.ExpiresAt.Equal(now.Add(30*time.Second)) {
		t.Fatalf("minted = %#v", minted)
	}
	stored := tickets.only(t)
	if bytes.Contains(stored.TicketHash, []byte(minted.Ticket)) || stored.Permissions != 7 {
		t.Fatalf("stored ticket = %#v", stored)
	}
	session, err := service.Consume(context.Background(), minted.Ticket)
	if err != nil || session.UserID != actor.UserID || session.ThreadID != thread.ID ||
		session.CurrentSequence != thread.LastSequence || session.RequestedLastSequence == nil || *session.RequestedLastSequence != lastSequence {
		t.Fatalf("session=%#v error=%v", session, err)
	}
	if _, err := service.Consume(context.Background(), minted.Ticket); !errors.Is(err, domain.ErrRealtimeTicketInvalid) {
		t.Fatalf("second Consume() error = %v", err)
	}
}

func TestTicketService_RejectsExpiredTicket(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	space, thread := realtimeFixture(now)
	tickets := newFakeTickets()
	clock := now
	service := realtimeuc.TicketService{
		Spaces: fakeSpaces{item: space}, Threads: fakeThreads{item: thread}, Tickets: tickets,
		Now: func() time.Time { return clock }, Random: bytes.NewReader(bytes.Repeat([]byte{9}, 32)), TTL: time.Second,
	}
	minted, err := service.Mint(context.Background(), domain.Actor{UserID: uuid.New(), Role: "STUDENT"}, realtimeuc.MintTicketInput{ThreadID: thread.ID})
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	clock = now.Add(2 * time.Second)
	if _, err := service.Consume(context.Background(), minted.Ticket); !errors.Is(err, domain.ErrRealtimeTicketInvalid) {
		t.Fatalf("expired Consume() error = %v", err)
	}
	deleted, err := service.DeleteExpired(context.Background(), 10)
	if err != nil || deleted != 1 {
		t.Fatalf("DeleteExpired() deleted=%d error=%v", deleted, err)
	}
}

func TestTicketService_ConsumeAttenuatesPermissionsAfterThreadCloses(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	space, thread := realtimeFixture(now)
	thread.Status = domain.ThreadStatusReadOnly
	secret := bytes.Repeat([]byte{11}, 32)
	hash := sha256.Sum256(secret)
	tickets := newFakeTickets()
	_ = tickets.Store(context.Background(), domain.RealtimeTicket{
		TicketHash: hash[:], UserID: uuid.New(), ThreadID: thread.ID,
		Permissions: domain.RealtimePermissionRead | domain.RealtimePermissionWrite | domain.RealtimePermissionUpload,
		CreatedAt:   now, ExpiresAt: now.Add(time.Minute),
	})
	service := realtimeuc.TicketService{
		Spaces: fakeSpaces{item: space}, Threads: fakeThreads{item: thread}, Tickets: tickets,
		Now: func() time.Time { return now },
	}
	session, err := service.Consume(context.Background(), base64.RawURLEncoding.EncodeToString(secret))
	if err != nil || session.Permissions != domain.RealtimePermissionRead {
		t.Fatalf("session=%#v error=%v", session, err)
	}
}

type fakeTickets struct {
	items map[string]domain.RealtimeTicket
}

func newFakeTickets() *fakeTickets {
	return &fakeTickets{items: make(map[string]domain.RealtimeTicket)}
}
func (f *fakeTickets) Store(_ context.Context, item domain.RealtimeTicket) error {
	f.items[string(item.TicketHash)] = item
	return nil
}
func (f *fakeTickets) Consume(_ context.Context, hash []byte, now time.Time) (domain.RealtimeTicket, error) {
	item, ok := f.items[string(hash)]
	if !ok || !item.ExpiresAt.After(now) {
		return domain.RealtimeTicket{}, domain.ErrNotFound
	}
	delete(f.items, string(hash))
	return item, nil
}
func (f *fakeTickets) DeleteExpired(_ context.Context, now time.Time, limit int) (int, error) {
	deleted := 0
	for key, item := range f.items {
		if deleted == limit {
			break
		}
		if !item.ExpiresAt.After(now) {
			delete(f.items, key)
			deleted++
		}
	}
	return deleted, nil
}
func (f *fakeTickets) only(t *testing.T) domain.RealtimeTicket {
	t.Helper()
	if len(f.items) != 1 {
		t.Fatalf("ticket count = %d", len(f.items))
	}
	for _, item := range f.items {
		return item
	}
	return domain.RealtimeTicket{}
}

type fakeSpaces struct{ item domain.Space }

func (f fakeSpaces) Create(context.Context, domain.Space) error { return nil }
func (f fakeSpaces) GetByID(_ context.Context, id uuid.UUID) (domain.Space, error) {
	if id != f.item.ID {
		return domain.Space{}, domain.ErrNotFound
	}
	return f.item, nil
}
func (f fakeSpaces) GetByKey(context.Context, string) (domain.Space, error) {
	return domain.Space{}, domain.ErrNotFound
}
func (f fakeSpaces) Update(context.Context, domain.Space) error { return nil }
func (f fakeSpaces) List(context.Context, repository.SpaceListQuery) ([]domain.Space, error) {
	return nil, nil
}

type fakeThreads struct{ item domain.Thread }

func (f fakeThreads) Ensure(context.Context, domain.Thread) (domain.Thread, error) {
	return f.item, nil
}
func (f fakeThreads) GetByID(_ context.Context, id uuid.UUID) (domain.Thread, error) {
	if id != f.item.ID {
		return domain.Thread{}, domain.ErrNotFound
	}
	return f.item, nil
}
func (f fakeThreads) GetByResource(context.Context, uuid.UUID, domain.ResourceReference) (domain.Thread, error) {
	return domain.Thread{}, domain.ErrNotFound
}
func (f fakeThreads) List(context.Context, repository.ThreadListQuery) ([]domain.Thread, error) {
	return []domain.Thread{f.item}, nil
}
func (f fakeThreads) Update(context.Context, domain.Thread) error                    { return nil }
func (f fakeThreads) NextSequence(context.Context, uuid.UUID) (int64, error)         { return 0, nil }
func (f fakeThreads) NextSequenceAnyState(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (f fakeThreads) RecordCommentCreated(context.Context, uuid.UUID, bool) error    { return nil }

func realtimeFixture(now time.Time) (domain.Space, domain.Thread) {
	space := domain.Space{
		ID: uuid.New(), Key: "course.comments", Name: "Comments", Status: domain.SpaceStatusActive,
		AccessMode: domain.AccessModeAuthenticated, AllowedOrigins: []string{"https://app.example"},
		Policy: domain.DefaultPolicy(), CreatedBy: uuid.New(), CreatedAt: now, UpdatedAt: now,
	}
	space.Policy.AllowImages = true
	thread := domain.Thread{
		ID: uuid.New(), SpaceID: space.ID, Resource: domain.ResourceReference{Type: "lesson", ID: "42"},
		Status: domain.ThreadStatusOpen, LastSequence: 12, CreatedAt: now, UpdatedAt: now,
	}
	return space, thread
}
