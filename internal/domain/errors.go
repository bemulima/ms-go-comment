package domain

import "errors"

var (
	ErrValidation            = errors.New("validation failed")
	ErrInvalidSpaceKey       = errors.New("invalid space key")
	ErrInvalidPolicy         = errors.New("invalid comment policy")
	ErrInvalidResource       = errors.New("invalid resource reference")
	ErrInvalidCommentContent = errors.New("invalid comment content")
	ErrInvalidPlacement      = errors.New("invalid comment placement")
	ErrCrossThreadParent     = errors.New("parent belongs to another thread")
	ErrMaxDepthExceeded      = errors.New("maximum comment depth exceeded")
	ErrImagesDisabled        = errors.New("images are disabled")
	ErrLinksDisabled         = errors.New("links are disabled")
	ErrInvalidAttachment     = errors.New("invalid attachment")
	ErrInvalidOutboxEvent    = errors.New("invalid outbox event")
	ErrInvalidRealtimeTicket = errors.New("invalid realtime ticket")
	ErrNotFound              = errors.New("not found")
	ErrConflict              = errors.New("conflict")
)
