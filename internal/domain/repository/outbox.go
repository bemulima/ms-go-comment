package repository

import (
	"context"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/google/uuid"
)

type OutboxRepository interface {
	Add(ctx context.Context, event domain.OutboxEvent) error
	// ClaimPending locks eligible rows with SKIP LOCKED within the caller's transaction.
	ClaimPending(ctx context.Context, now time.Time, limit int) ([]domain.OutboxEvent, error)
	MarkPublished(ctx context.Context, eventID uuid.UUID, publishedAt time.Time) error
	MarkFailed(ctx context.Context, eventID uuid.UUID, nextAttemptAt time.Time, message string) error
}
