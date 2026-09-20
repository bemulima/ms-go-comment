package websocket

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/google/uuid"
)

const protocolVersion = 1

var websocketTypes = map[domain.EventSubject]string{
	domain.EventCommentCreated:          "comment.created",
	domain.EventCommentUpdated:          "comment.updated",
	domain.EventCommentDeleted:          "comment.deleted",
	domain.EventCommentHidden:           "comment.hidden",
	domain.EventCommentRestored:         "comment.restored",
	domain.EventCommentAttachmentReady:  "attachment.ready",
	domain.EventCommentAttachmentFailed: "attachment.failed",
	domain.EventCommentThreadUpdated:    "thread.updated",
}

func lifecycleEnvelope(subject domain.EventSubject, payload []byte) (uuid.UUID, []byte, error) {
	eventType, ok := websocketTypes[subject]
	if !ok {
		return uuid.Nil, nil, fmt.Errorf("unsupported lifecycle subject")
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(payload, &data); err != nil {
		return uuid.Nil, nil, err
	}
	threadID, err := rawUUID(data["thread_id"])
	if err != nil {
		return uuid.Nil, nil, err
	}
	eventID, err := rawUUID(data["event_id"])
	if err != nil {
		return uuid.Nil, nil, err
	}
	var sequence int64
	var occurredAt time.Time
	if json.Unmarshal(data["sequence"], &sequence) != nil || sequence < 1 ||
		json.Unmarshal(data["occurred_at"], &occurredAt) != nil || occurredAt.IsZero() {
		return uuid.Nil, nil, fmt.Errorf("invalid lifecycle sequence or timestamp")
	}
	envelope := map[string]any{
		"v": protocolVersion, "type": eventType, "thread_id": threadID,
		"event_id": eventID, "sequence": sequence, "occurred_at": occurredAt,
	}
	delete(data, "schema_version")
	delete(data, "event_id")
	delete(data, "thread_id")
	delete(data, "sequence")
	delete(data, "occurred_at")
	envelope["data"] = data
	encoded, err := json.Marshal(envelope)
	return threadID, encoded, err
}

func readyFrame(threadID uuid.UUID, currentSequence int64, now time.Time) []byte {
	return mustJSON(map[string]any{
		"v": protocolVersion, "type": "connection.ready", "thread_id": threadID,
		"current_sequence": currentSequence, "occurred_at": now,
	})
}

func resyncFrame(threadID uuid.UUID, requested, current int64, now time.Time) []byte {
	return mustJSON(map[string]any{
		"v": protocolVersion, "type": "resync_required", "thread_id": threadID,
		"last_sequence": requested, "current_sequence": current, "occurred_at": now,
	})
}

func errorFrame(requestID, code, message string) []byte {
	return mustJSON(map[string]any{
		"v": protocolVersion, "type": "error", "request_id": requestID,
		"data": map[string]string{"error": code, "message": message},
	})
}

func pingFrame(now time.Time) []byte {
	return mustJSON(map[string]any{"v": protocolVersion, "type": "ping", "occurred_at": now})
}

func typingFrame(eventType string, threadID, userID, eventID uuid.UUID, now time.Time) []byte {
	return mustJSON(map[string]any{
		"v": protocolVersion, "type": eventType, "event_id": eventID,
		"thread_id": threadID, "occurred_at": now,
		"data": map[string]any{"user_id": userID},
	})
}

func rawUUID(value json.RawMessage) (uuid.UUID, error) {
	var id uuid.UUID
	if len(value) == 0 || json.Unmarshal(value, &id) != nil || id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("invalid thread id")
	}
	return id, nil
}

func mustJSON(value any) []byte {
	result, _ := json.Marshal(value)
	return result
}
