package repository

import (
	"context"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/google/uuid"
)

type AttachmentRepository interface {
	Create(ctx context.Context, attachment domain.Attachment) error
	GetByID(ctx context.Context, id uuid.UUID) (domain.Attachment, error)
	GetByIDForUpdate(ctx context.Context, id uuid.UUID) (domain.Attachment, error)
	ListByComment(ctx context.Context, threadID, commentID uuid.UUID) ([]domain.Attachment, error)
	CountPendingByUploader(ctx context.Context, threadID, uploaderID uuid.UUID, now time.Time) (int, error)
	BindToComment(ctx context.Context, attachmentID, threadID, commentID, uploaderID uuid.UUID) error
	UpdateStatus(ctx context.Context, attachment domain.Attachment) error
	ListExpired(ctx context.Context, before time.Time, limit int) ([]domain.Attachment, error)
	ListForActivation(ctx context.Context, now time.Time, limit int) ([]domain.Attachment, error)
	MarkReadyIfProcessing(ctx context.Context, id uuid.UUID, now time.Time) (domain.Attachment, bool, error)
	RecordActivationFailure(ctx context.Context, id uuid.UUID, next time.Time, message string, maxAttempts int) (domain.Attachment, bool, error)
	ListForDeletion(ctx context.Context, now time.Time, limit int) ([]domain.Attachment, error)
	MarkStorageDeleted(ctx context.Context, id uuid.UUID, now time.Time) error
	RecordDeleteFailure(ctx context.Context, id uuid.UUID, next time.Time, message string) error
}
