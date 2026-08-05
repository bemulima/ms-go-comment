package handlers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	"github.com/google/uuid"
)

type listCursor struct {
	ThreadID  uuid.UUID  `json:"thread_id"`
	ParentID  *uuid.UUID `json:"parent_id,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	ID        uuid.UUID  `json:"id"`
}

func encodeCursor(threadID uuid.UUID, parentID *uuid.UUID, cursor repository.CommentCursor) (string, error) {
	payload, err := json.Marshal(listCursor{
		ThreadID: threadID, ParentID: parentID, CreatedAt: cursor.CreatedAt, ID: cursor.ID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeCursor(raw string, threadID uuid.UUID, parentID *uuid.UUID) (*repository.CommentCursor, error) {
	if raw == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.New("invalid cursor encoding")
	}
	var cursor listCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.ID == uuid.Nil || cursor.CreatedAt.IsZero() {
		return nil, errors.New("invalid cursor payload")
	}
	if cursor.ThreadID != threadID || !sameOptionalUUID(cursor.ParentID, parentID) {
		return nil, errors.New("cursor does not belong to this comment list")
	}
	return &repository.CommentCursor{CreatedAt: cursor.CreatedAt, ID: cursor.ID}, nil
}

func sameOptionalUUID(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
