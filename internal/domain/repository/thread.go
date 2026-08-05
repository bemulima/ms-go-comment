package repository

import (
	"context"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/google/uuid"
)

type ThreadListQuery struct {
	SpaceID *uuid.UUID
	Status  *domain.ThreadStatus
	Limit   int
	Offset  int
}

type ThreadRepository interface {
	// Ensure returns the existing thread for the exact resource tuple or creates
	// the supplied thread atomically.
	Ensure(ctx context.Context, thread domain.Thread) (domain.Thread, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.Thread, error)
	GetByResource(ctx context.Context, spaceID uuid.UUID, resource domain.ResourceReference) (domain.Thread, error)
	List(ctx context.Context, query ThreadListQuery) ([]domain.Thread, error)
	Update(ctx context.Context, thread domain.Thread) error
	// NextSequence locks the thread row and returns its next durable sequence.
	NextSequence(ctx context.Context, threadID uuid.UUID) (int64, error)
	NextSequenceAnyState(ctx context.Context, threadID uuid.UUID) (int64, error)
	RecordCommentCreated(ctx context.Context, threadID uuid.UUID, root bool) error
}
