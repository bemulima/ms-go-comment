package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AttachmentStatus int16

const (
	AttachmentStatusPending AttachmentStatus = iota + 1
	AttachmentStatusProcessing
	AttachmentStatusReady
	AttachmentStatusFailed
	AttachmentStatusDeleted
)

func (s AttachmentStatus) Valid() bool {
	return s >= AttachmentStatusPending && s <= AttachmentStatusDeleted
}

type Attachment struct {
	ID                      uuid.UUID
	ThreadID                uuid.UUID
	CommentID               *uuid.UUID
	UploaderID              uuid.UUID
	FileStorageID           uuid.UUID
	Status                  AttachmentStatus
	MIMEType                string
	SizeBytes               int64
	Width                   int
	Height                  int
	OriginalFilename        string
	ExpiresAt               time.Time
	ActivatedAt             *time.Time
	DeletedAt               *time.Time
	ActivationAttempts      int
	ActivationNextAttemptAt *time.Time
	DeleteAttempts          int
	DeleteNextAttemptAt     *time.Time
	LastError               string
	StorageDeletedAt        *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

func (a Attachment) Validate(policy Policy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	if !policy.AllowImages || policy.MaxAttachments == 0 {
		return ErrImagesDisabled
	}
	if a.ID == uuid.Nil || a.ThreadID == uuid.Nil || a.UploaderID == uuid.Nil || a.FileStorageID == uuid.Nil {
		return fmt.Errorf("%w: attachment identity and ownership are required", ErrInvalidAttachment)
	}
	if !a.Status.Valid() {
		return fmt.Errorf("%w: unsupported status", ErrInvalidAttachment)
	}
	if a.MIMEType != "image/jpeg" && a.MIMEType != "image/png" && a.MIMEType != "image/webp" {
		return fmt.Errorf("%w: unsupported image MIME type", ErrInvalidAttachment)
	}
	if a.SizeBytes < 1 || a.SizeBytes > policy.MaxImageBytes {
		return fmt.Errorf("%w: image size is outside policy", ErrInvalidAttachment)
	}
	if a.Width < 1 || a.Width > 32768 || a.Height < 1 || a.Height > 32768 {
		return fmt.Errorf("%w: image dimensions are outside limits", ErrInvalidAttachment)
	}
	if strings.TrimSpace(a.OriginalFilename) == "" {
		return fmt.Errorf("%w: original filename is required", ErrInvalidAttachment)
	}
	if !a.ExpiresAt.After(a.CreatedAt) {
		return fmt.Errorf("%w: expiry must follow creation", ErrInvalidAttachment)
	}
	if a.Status == AttachmentStatusReady && (a.CommentID == nil || a.ActivatedAt == nil) {
		return fmt.Errorf("%w: ready attachment must be bound and activated", ErrInvalidAttachment)
	}
	if a.Status == AttachmentStatusDeleted && a.DeletedAt == nil {
		return fmt.Errorf("%w: deleted attachment requires deletion time", ErrInvalidAttachment)
	}
	if a.ActivationAttempts < 0 || a.DeleteAttempts < 0 {
		return fmt.Errorf("%w: delivery attempts must be non-negative", ErrInvalidAttachment)
	}
	return nil
}
