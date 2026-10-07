package nats

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

// Embedding the unimplemented mutation API makes any accidental AddStream or
// UpdateStream call panic. Application startup may only inspect configuration.
type streamInfoOnly struct {
	natsgo.JetStreamContext
	info  *natsgo.StreamInfo
	err   error
	reads int
}

func (reader *streamInfoOnly) StreamInfo(name string, _ ...natsgo.JSOpt) (*natsgo.StreamInfo, error) {
	if name != LifecycleStream {
		panic("unexpected stream")
	}
	reader.reads++
	return reader.info, reader.err
}

func lifecycleFixtureConfig() natsgo.StreamConfig {
	return natsgo.StreamConfig{Name: LifecycleStream, Subjects: append([]string(nil), lifecycleSubjects...),
		Retention: natsgo.LimitsPolicy, Storage: natsgo.FileStorage, MaxAge: 7 * 24 * time.Hour, Duplicates: 10 * time.Minute}
}

func TestEnsureLifecycleStreamOnlyInspectsProvisionedConfiguration(t *testing.T) {
	for _, test := range []struct {
		name      string
		info      *natsgo.StreamInfo
		err       error
		wantError bool
	}{
		{name: "compatible", info: &natsgo.StreamInfo{Config: lifecycleFixtureConfig()}},
		{name: "missing", err: natsgo.ErrStreamNotFound, wantError: true},
		{name: "unavailable", err: context.DeadlineExceeded, wantError: true},
		{name: "empty response", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := &streamInfoOnly{info: test.info, err: test.err}
			client := &Client{Conn: &natsgo.Conn{}, js: reader}
			err := client.EnsureLifecycleStream(context.Background())
			if (err != nil) != test.wantError {
				t.Fatalf("error=%v wantError=%v", err, test.wantError)
			}
			if test.err != nil && !errors.Is(err, test.err) {
				t.Fatalf("broker error not preserved: %v", err)
			}
			if reader.reads != 1 {
				t.Fatalf("StreamInfo calls=%d", reader.reads)
			}
		})
	}
}

func TestEnsureLifecycleStreamRejectsDriftWithoutRepair(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*natsgo.StreamConfig)
	}{
		{name: "wrong identity", mutate: func(c *natsgo.StreamConfig) { c.Name = "OTHER" }},
		{name: "missing required subject", mutate: func(c *natsgo.StreamConfig) { c.Subjects = c.Subjects[:7] }},
		{name: "wrong retention", mutate: func(c *natsgo.StreamConfig) { c.Retention = natsgo.WorkQueuePolicy }},
		{name: "wrong storage", mutate: func(c *natsgo.StreamConfig) { c.Storage = natsgo.MemoryStorage }},
		{name: "short retention", mutate: func(c *natsgo.StreamConfig) { c.MaxAge = time.Hour }},
		{name: "short deduplication", mutate: func(c *natsgo.StreamConfig) { c.Duplicates = time.Minute }},
		{name: "broad subject", mutate: func(c *natsgo.StreamConfig) { c.Subjects = append(c.Subjects, "comment.>") }},
		{name: "ephemeral typing", mutate: func(c *natsgo.StreamConfig) { c.Subjects = append(c.Subjects, "comment.realtime.typing.fixture") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := lifecycleFixtureConfig()
			test.mutate(&cfg)
			reader := &streamInfoOnly{info: &natsgo.StreamInfo{Config: cfg}}
			client := &Client{Conn: &natsgo.Conn{}, js: reader}
			if err := client.EnsureLifecycleStream(context.Background()); err == nil {
				t.Fatal("incompatible stream accepted")
			}
			if reader.reads != 1 {
				t.Fatalf("StreamInfo calls=%d", reader.reads)
			}
		})
	}
}

func TestLifecycleValidationAllowsCompatibleExactAdditions(t *testing.T) {
	cfg := lifecycleFixtureConfig()
	cfg.Subjects = append(cfg.Subjects, "comment.future")
	cfg.MaxAge = 0
	if err := validateLifecycleStream(cfg); err != nil {
		t.Fatal(err)
	}
	for _, subject := range lifecycleSubjects {
		if strings.ContainsAny(subject, "*>") || strings.HasPrefix(subject, "comment.realtime.") {
			t.Fatal("invalid retained subject")
		}
	}
}
