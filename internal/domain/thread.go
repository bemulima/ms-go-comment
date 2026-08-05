package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var resourceTypePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

type ThreadStatus int16

const (
	ThreadStatusOpen ThreadStatus = iota + 1
	ThreadStatusReadOnly
	ThreadStatusClosed
	ThreadStatusHidden
)

func (s ThreadStatus) Valid() bool {
	return s >= ThreadStatusOpen && s <= ThreadStatusHidden
}

type ThreadPolicyOverrides struct {
	AllowImages       *bool
	AllowLinks        *bool
	MaxDepth          *int16
	MaxBodyLength     *int
	MaxAttachments    *int16
	MaxImageBytes     *int64
	EditWindowSeconds *int
}

func ApplyPolicy(base Policy, overrides ThreadPolicyOverrides) (Policy, error) {
	effective := base
	if overrides.AllowImages != nil {
		effective.AllowImages = *overrides.AllowImages
	}
	if overrides.AllowLinks != nil {
		effective.AllowLinks = *overrides.AllowLinks
	}
	if overrides.MaxDepth != nil {
		effective.MaxDepth = *overrides.MaxDepth
	}
	if overrides.MaxBodyLength != nil {
		effective.MaxBodyLength = *overrides.MaxBodyLength
	}
	if overrides.MaxAttachments != nil {
		effective.MaxAttachments = *overrides.MaxAttachments
	}
	if overrides.MaxImageBytes != nil {
		effective.MaxImageBytes = *overrides.MaxImageBytes
	}
	if overrides.EditWindowSeconds != nil {
		effective.EditWindowSeconds = *overrides.EditWindowSeconds
	}

	if err := effective.Validate(); err != nil {
		return Policy{}, err
	}
	return effective, nil
}

type ResourceReference struct {
	Type string
	ID   string
}

func (r ResourceReference) Validate() error {
	if !resourceTypePattern.MatchString(r.Type) {
		return fmt.Errorf("%w: unsupported resource type", ErrInvalidResource)
	}
	if strings.TrimSpace(r.ID) == "" || utf8.RuneCountInString(r.ID) > 512 {
		return fmt.Errorf("%w: resource id must contain 1 to 512 characters", ErrInvalidResource)
	}
	return nil
}

type Thread struct {
	ID               uuid.UUID
	SpaceID          uuid.UUID
	Resource         ResourceReference
	Status           ThreadStatus
	PolicyOverrides  ThreadPolicyOverrides
	CommentCount     int64
	RootCommentCount int64
	LastSequence     int64
	LastCommentAt    *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (t Thread) Validate() error {
	if t.ID == uuid.Nil || t.SpaceID == uuid.Nil {
		return fmt.Errorf("%w: thread and space identifiers are required", ErrValidation)
	}
	if err := t.Resource.Validate(); err != nil {
		return err
	}
	if !t.Status.Valid() {
		return fmt.Errorf("%w: unsupported thread status", ErrValidation)
	}
	if t.CommentCount < 0 || t.RootCommentCount < 0 || t.RootCommentCount > t.CommentCount || t.LastSequence < 0 {
		return fmt.Errorf("%w: invalid thread counters", ErrValidation)
	}
	_, err := ApplyPolicy(DefaultPolicy(), t.PolicyOverrides)
	return err
}
