package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type EventSubject string

const (
	EventCommentCreated          EventSubject = "comment.created"
	EventCommentUpdated          EventSubject = "comment.updated"
	EventCommentDeleted          EventSubject = "comment.deleted"
	EventCommentHidden           EventSubject = "comment.hidden"
	EventCommentRestored         EventSubject = "comment.restored"
	EventCommentAttachmentReady  EventSubject = "comment.attachment.ready"
	EventCommentAttachmentFailed EventSubject = "comment.attachment.failed"
	EventCommentThreadUpdated    EventSubject = "comment.thread.updated"
)

func (s EventSubject) Valid() bool {
	switch s {
	case EventCommentCreated, EventCommentUpdated, EventCommentDeleted,
		EventCommentHidden, EventCommentRestored, EventCommentAttachmentReady,
		EventCommentAttachmentFailed, EventCommentThreadUpdated:
		return true
	default:
		return false
	}
}

type OutboxEvent struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	Subject       EventSubject
	SchemaVersion int16
	Payload       json.RawMessage
	Attempts      int
	NextAttemptAt time.Time
	PublishedAt   *time.Time
	LastError     string
	CreatedAt     time.Time
}

func (e OutboxEvent) Validate() error {
	if e.ID == uuid.Nil || e.AggregateID == uuid.Nil || strings.TrimSpace(e.AggregateType) == "" {
		return fmt.Errorf("%w: event and aggregate identity are required", ErrInvalidOutboxEvent)
	}
	if !e.Subject.Valid() || e.SchemaVersion < 1 || e.Attempts < 0 {
		return fmt.Errorf("%w: subject, version, or attempts are invalid", ErrInvalidOutboxEvent)
	}
	trimmed := bytes.TrimSpace(e.Payload)
	if !json.Valid(trimmed) || len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return fmt.Errorf("%w: payload must be a JSON object", ErrInvalidOutboxEvent)
	}
	return nil
}
