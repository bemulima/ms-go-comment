package comment

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type TemporaryFileInput struct {
	OwnerID    uuid.UUID
	Filename   string
	MIMEType   string
	Data       []byte
	TTLMinutes int
}

type StoredFile struct {
	ID        uuid.UUID
	ExpiresAt *time.Time
}

type SignedFileURL struct {
	URL       string
	ExpiresAt time.Time
}

type FileStorage interface {
	UploadTemporary(ctx context.Context, input TemporaryFileInput) (StoredFile, error)
	Activate(ctx context.Context, fileID uuid.UUID) error
	SignedGETURL(ctx context.Context, fileID uuid.UUID, expiresMinutes int) (string, error)
	Delete(ctx context.Context, fileID uuid.UUID) error
}
