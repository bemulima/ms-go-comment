package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type RealtimePermission int16

const (
	RealtimePermissionRead RealtimePermission = 1 << iota
	RealtimePermissionWrite
	RealtimePermissionUpload
	realtimePermissionMask = RealtimePermissionRead | RealtimePermissionWrite | RealtimePermissionUpload
)

func (p RealtimePermission) Includes(permission RealtimePermission) bool {
	return p&permission == permission
}

type RealtimeTicket struct {
	TicketHash            []byte
	UserID                uuid.UUID
	ThreadID              uuid.UUID
	Permissions           RealtimePermission
	RequestedLastSequence *int64
	ExpiresAt             time.Time
	CreatedAt             time.Time
}

func (t RealtimeTicket) Validate() error {
	if len(t.TicketHash) != 32 || t.UserID == uuid.Nil || t.ThreadID == uuid.Nil {
		return fmt.Errorf("%w: hash, user, and thread are required", ErrInvalidRealtimeTicket)
	}
	if t.Permissions&^realtimePermissionMask != 0 || !t.Permissions.Includes(RealtimePermissionRead) {
		return fmt.Errorf("%w: invalid permissions", ErrInvalidRealtimeTicket)
	}
	if t.RequestedLastSequence != nil && *t.RequestedLastSequence < 0 {
		return fmt.Errorf("%w: requested sequence must be non-negative", ErrInvalidRealtimeTicket)
	}
	if !t.ExpiresAt.After(t.CreatedAt) {
		return fmt.Errorf("%w: expiry must follow creation", ErrInvalidRealtimeTicket)
	}
	return nil
}
