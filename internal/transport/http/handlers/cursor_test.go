package handlers

import (
	"testing"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	"github.com/google/uuid"
)

func TestCursor_RoundTripAndScope(t *testing.T) {
	t.Parallel()

	threadID := uuid.New()
	parentID := uuid.New()
	want := repository.CommentCursor{CreatedAt: time.Now().UTC().Truncate(time.Microsecond), ID: uuid.New()}
	encoded, err := encodeCursor(threadID, &parentID, want)
	if err != nil {
		t.Fatalf("encodeCursor() error = %v", err)
	}
	got, err := decodeCursor(encoded, threadID, &parentID)
	if err != nil {
		t.Fatalf("decodeCursor() error = %v", err)
	}
	if got.ID != want.ID || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("decoded cursor = %#v, want %#v", got, want)
	}
	if _, err := decodeCursor(encoded, uuid.New(), &parentID); err == nil {
		t.Fatal("cursor from another thread was accepted")
	}
	if _, err := decodeCursor("not-base64", threadID, &parentID); err == nil {
		t.Fatal("malformed cursor was accepted")
	}
}
