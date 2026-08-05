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
	ListByComment(ctx context.Context, threadID, commentID uuid.UUID) ([]domain.Attachment, error)
	BindToComment(ctx context.Context, attachmentID, threadID, commentID, uploaderID uuid.UUID) error
	UpdateStatus(ctx context.Context, attachment domain.Attachment) error
	ListExpired(ctx context.Context, before time.Time, limit int) ([]domain.Attachment, error)
}
