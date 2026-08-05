package nats

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
)

func TestLifecycleMessageUsesEventIDForJetStreamDeduplication(t *testing.T) {
	t.Parallel()

	eventID := uuid.New()
	payload, _ := json.Marshal(map[string]any{"event_id": eventID, "thread_id": uuid.New(), "sequence": 1})
	event := domain.OutboxEvent{
		ID: eventID, AggregateType: "comment", AggregateID: uuid.New(),
		Subject: domain.EventCommentCreated, SchemaVersion: 1, Payload: payload,
		NextAttemptAt: time.Now().UTC(), CreatedAt: time.Now().UTC(),
	}
	message, err := lifecycleMessage(event)
	if err != nil {
		t.Fatalf("lifecycleMessage() error = %v", err)
	}
	if message.Subject != string(event.Subject) || message.Header.Get(natsgo.MsgIdHdr) != eventID.String() || string(message.Data) != string(payload) {
		t.Fatalf("message subject=%s headers=%v data=%s", message.Subject, message.Header, message.Data)
	}
}
