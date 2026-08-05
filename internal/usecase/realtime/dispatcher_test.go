package realtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	realtimeuc "github.com/bemulima/ms-go-comment/internal/usecase/realtime"
	"github.com/google/uuid"
)

func TestDispatcher_PublishesAndRetriesOutsideClaim(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	first := outboxEvent(now)
	second := outboxEvent(now)
	store := &fakeOutbox{items: []domain.OutboxEvent{first, second}}
	publisher := &fakeLifecyclePublisher{failID: second.ID}
	dispatcher := realtimeuc.Dispatcher{Outbox: store, Publisher: publisher, Now: func() time.Time { return now }, Lease: 20 * time.Second}

	result, err := dispatcher.Process(context.Background(), 10)
	if err != nil || result.Published != 1 || result.Failed != 1 {
		t.Fatalf("Process() result=%#v error=%v", result, err)
	}
	if !store.leaseUntil.Equal(now.Add(20*time.Second)) || store.published[first.ID] == nil {
		t.Fatalf("lease=%s published=%#v", store.leaseUntil, store.published)
	}
	failure := store.failed[second.ID]
	if failure.attempts != 1 || !failure.next.Equal(now.Add(5*time.Second)) || failure.message == "" {
		t.Fatalf("failure=%#v", failure)
	}
	result, err = dispatcher.Process(context.Background(), 10)
	if err != nil || result.Published != 0 || result.Failed != 0 {
		t.Fatalf("second Process() result=%#v error=%v", result, err)
	}
}

type fakeLifecyclePublisher struct{ failID uuid.UUID }

func (f *fakeLifecyclePublisher) PublishLifecycle(_ context.Context, event domain.OutboxEvent) error {
	if event.ID == f.failID {
		return errors.New("NATS unavailable")
	}
	return nil
}

type failedDelivery struct {
	attempts int
	next     time.Time
	message  string
}
type fakeOutbox struct {
	items      []domain.OutboxEvent
	leaseUntil time.Time
	published  map[uuid.UUID]*time.Time
	failed     map[uuid.UUID]failedDelivery
}

func (f *fakeOutbox) Add(context.Context, domain.OutboxEvent) error { return nil }
func (f *fakeOutbox) ClaimPending(_ context.Context, now, leaseUntil time.Time, limit int) ([]domain.OutboxEvent, error) {
	f.leaseUntil = leaseUntil
	result := make([]domain.OutboxEvent, 0, limit)
	for _, item := range f.items {
		if item.PublishedAt == nil && !item.NextAttemptAt.After(now) && len(result) < limit {
			result = append(result, item)
		}
	}
	return result, nil
}
func (f *fakeOutbox) MarkPublished(_ context.Context, id uuid.UUID, at time.Time) error {
	if f.published == nil {
		f.published = make(map[uuid.UUID]*time.Time)
	}
	f.published[id] = &at
	for index := range f.items {
		if f.items[index].ID == id {
			f.items[index].PublishedAt = &at
		}
	}
	return nil
}
func (f *fakeOutbox) MarkFailed(_ context.Context, id uuid.UUID, next time.Time, message string) error {
	if f.failed == nil {
		f.failed = make(map[uuid.UUID]failedDelivery)
	}
	attempts := 1
	for index := range f.items {
		if f.items[index].ID == id {
			f.items[index].Attempts++
			attempts = f.items[index].Attempts
			f.items[index].NextAttemptAt = next
		}
	}
	f.failed[id] = failedDelivery{attempts: attempts, next: next, message: message}
	return nil
}

func outboxEvent(now time.Time) domain.OutboxEvent {
	id := uuid.New()
	payload, _ := json.Marshal(map[string]any{
		"schema_version": 1, "event_id": id, "thread_id": uuid.New(), "sequence": 1, "occurred_at": now,
	})
	return domain.OutboxEvent{
		ID: id, AggregateType: "comment", AggregateID: uuid.New(), Subject: domain.EventCommentCreated,
		SchemaVersion: 1, Payload: payload, NextAttemptAt: now, CreatedAt: now,
	}
}
