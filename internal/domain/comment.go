package domain

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

type CommentStatus int16

const (
	CommentStatusActive CommentStatus = iota + 1
	CommentStatusDeleted
	CommentStatusHidden
)

func (s CommentStatus) Valid() bool {
	return s >= CommentStatusActive && s <= CommentStatusHidden
}

type Link struct {
	URL   string
	Title string
}

type CommentContent struct {
	Body            string
	Links           []Link
	AttachmentCount int
	ContainsRawHTML bool
}

func (c CommentContent) Validate(policy Policy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	if c.ContainsRawHTML {
		return fmt.Errorf("%w: raw HTML is not allowed", ErrInvalidCommentContent)
	}
	if utf8.RuneCountInString(c.Body) > policy.MaxBodyLength {
		return fmt.Errorf("%w: body exceeds %d characters", ErrInvalidCommentContent, policy.MaxBodyLength)
	}
	if len(c.Links) > 0 && !policy.AllowLinks {
		return ErrLinksDisabled
	}
	for _, link := range c.Links {
		parsed, err := url.ParseRequestURI(link.URL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("%w: link must be an absolute HTTP(S) URL", ErrInvalidCommentContent)
		}
	}
	if c.AttachmentCount < 0 || c.AttachmentCount > int(policy.MaxAttachments) {
		return fmt.Errorf("%w: attachment count is outside policy", ErrInvalidCommentContent)
	}
	if c.AttachmentCount > 0 && !policy.AllowImages {
		return ErrImagesDisabled
	}
	if strings.TrimSpace(c.Body) == "" && c.AttachmentCount == 0 {
		return fmt.Errorf("%w: body or attachment is required", ErrInvalidCommentContent)
	}
	return nil
}

type Comment struct {
	ID                 uuid.UUID
	ThreadID           uuid.UUID
	AuthorID           uuid.UUID
	ParentID           *uuid.UUID
	RootID             uuid.UUID
	Path               []uuid.UUID
	Depth              int16
	Body               string
	Links              []Link
	Status             CommentStatus
	Version            int
	Sequence           int64
	DirectRepliesCount int64
	IdempotencyKey     uuid.UUID
	EditedAt           *time.Time
	DeletedAt          *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type CommentPlacement struct {
	ParentID *uuid.UUID
	RootID   uuid.UUID
	Path     []uuid.UUID
	Depth    int16
}

func BuildCommentPlacement(commentID, threadID uuid.UUID, parent *Comment, maxDepth int16) (CommentPlacement, error) {
	if commentID == uuid.Nil || threadID == uuid.Nil || maxDepth < 1 || maxDepth > MaxAllowedDepth {
		return CommentPlacement{}, fmt.Errorf("%w: invalid identifiers or maximum depth", ErrInvalidPlacement)
	}
	if parent == nil {
		return CommentPlacement{RootID: commentID, Path: []uuid.UUID{commentID}}, nil
	}
	if parent.ThreadID != threadID {
		return CommentPlacement{}, ErrCrossThreadParent
	}
	if parent.Status != CommentStatusActive {
		return CommentPlacement{}, fmt.Errorf("%w: parent must be active", ErrInvalidPlacement)
	}
	if err := parent.ValidatePlacement(); err != nil {
		return CommentPlacement{}, fmt.Errorf("%w: parent: %v", ErrInvalidPlacement, err)
	}
	depth := parent.Depth + 1
	if depth > maxDepth {
		return CommentPlacement{}, ErrMaxDepthExceeded
	}
	parentID := parent.ID
	path := append(append([]uuid.UUID(nil), parent.Path...), commentID)
	return CommentPlacement{ParentID: &parentID, RootID: parent.RootID, Path: path, Depth: depth}, nil
}

func (c Comment) ValidatePlacement() error {
	if c.ID == uuid.Nil || c.ThreadID == uuid.Nil || c.RootID == uuid.Nil {
		return fmt.Errorf("%w: comment, thread, and root identifiers are required", ErrInvalidPlacement)
	}
	if c.Depth < 0 || c.Depth > MaxAllowedDepth || len(c.Path) != int(c.Depth)+1 {
		return fmt.Errorf("%w: path length must equal depth plus one", ErrInvalidPlacement)
	}
	if c.Path[0] != c.RootID || c.Path[len(c.Path)-1] != c.ID {
		return fmt.Errorf("%w: path endpoints do not match root and comment", ErrInvalidPlacement)
	}
	if c.ParentID == nil {
		if c.Depth != 0 || c.RootID != c.ID {
			return fmt.Errorf("%w: root comment shape is inconsistent", ErrInvalidPlacement)
		}
	} else if c.Depth == 0 || c.RootID == c.ID {
		return fmt.Errorf("%w: reply shape is inconsistent", ErrInvalidPlacement)
	}
	return nil
}
