package repository

import (
	"context"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/google/uuid"
)

type CommentCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

type CommentListQuery struct {
	ThreadID uuid.UUID
	ParentID *uuid.UUID
	After    *CommentCursor
	Limit    int
}

type CommentChangeQuery struct {
	ThreadID      uuid.UUID
	AfterSequence int64
	Limit         int
}

type CommentRepository interface {
	Create(ctx context.Context, comment domain.Comment) error
	GetByID(ctx context.Context, commentID uuid.UUID) (domain.Comment, error)
	GetByIDForUpdate(ctx context.Context, commentID uuid.UUID) (domain.Comment, error)
	GetByIdempotencyKey(ctx context.Context, authorID, key uuid.UUID) (domain.Comment, error)
	LockIdempotencyKey(ctx context.Context, authorID, key uuid.UUID) error
	List(ctx context.Context, query CommentListQuery) ([]domain.Comment, error)
	ListChanges(ctx context.Context, query CommentChangeQuery) ([]domain.Comment, error)
	UpdateContent(ctx context.Context, comment domain.Comment, expectedVersion int) error
	MarkDeleted(ctx context.Context, comment domain.Comment, expectedVersion int) error
	UpdateModerationStatus(ctx context.Context, comment domain.Comment, expectedStatus domain.CommentStatus, expectedVersion int) error
	IncrementReplyCount(ctx context.Context, threadID, commentID uuid.UUID) error
	AdvanceSequence(ctx context.Context, commentID uuid.UUID, sequence int64, updatedAt time.Time) (domain.Comment, error)
}
