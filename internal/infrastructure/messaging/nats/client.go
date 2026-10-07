package nats

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	realtimeuc "github.com/bemulima/ms-go-comment/internal/usecase/realtime"
	natsgo "github.com/nats-io/nats.go"
)

const LifecycleStream = "COMMENT_EVENTS"

var lifecycleSubjects = []string{
	string(domain.EventCommentCreated),
	string(domain.EventCommentUpdated),
	string(domain.EventCommentDeleted),
	string(domain.EventCommentHidden),
	string(domain.EventCommentRestored),
	string(domain.EventCommentAttachmentReady),
	string(domain.EventCommentAttachmentFailed),
	string(domain.EventCommentThreadUpdated),
}

type Client struct {
	Conn *natsgo.Conn
	mu   sync.Mutex
	js   natsgo.JetStreamContext
}

func Connect(url string) (*natsgo.Conn, error) {
	return natsgo.Connect(url,
		natsgo.Name("ms-go-comment"),
		natsgo.Timeout(5*time.Second),
		natsgo.MaxReconnects(-1),
		natsgo.ReconnectWait(time.Second),
	)
}

// EnsureLifecycleStream only validates an infrastructure-provisioned stream.
// Application startup must never create or repair shared broker configuration.
func (c *Client) EnsureLifecycleStream(ctx context.Context) error {
	js, err := c.jetStream()
	if err != nil {
		return err
	}
	info, err := js.StreamInfo(LifecycleStream, natsgo.Context(ctx))
	if err != nil {
		return fmt.Errorf("validate infrastructure-provisioned %s: %w", LifecycleStream, err)
	}
	if info == nil {
		return fmt.Errorf("infrastructure-provisioned %s returned no configuration", LifecycleStream)
	}
	return validateLifecycleStream(info.Config)
}

// Publication checks its minimum compatible stream requirements. The complete
// canonical configuration and every critical drift check belong to infrastructure.
func validateLifecycleStream(config natsgo.StreamConfig) error {
	if config.Name != LifecycleStream || config.Retention != natsgo.LimitsPolicy || config.Storage != natsgo.FileStorage {
		return fmt.Errorf("%s requires limits retention and file storage; repair through infrastructure", LifecycleStream)
	}
	if (config.MaxAge != 0 && config.MaxAge < 7*24*time.Hour) || config.Duplicates < 10*time.Minute {
		return fmt.Errorf("%s requires at least seven days retention and ten minutes deduplication; repair through infrastructure", LifecycleStream)
	}
	subjects := make(map[string]bool, len(config.Subjects))
	for _, subject := range config.Subjects {
		if strings.ContainsAny(subject, "*>") || strings.HasPrefix(subject, "comment.realtime.") {
			return fmt.Errorf("%s contains broad or ephemeral subject %q; repair through infrastructure", LifecycleStream, subject)
		}
		subjects[subject] = true
	}
	for _, required := range lifecycleSubjects {
		if !subjects[required] {
			return fmt.Errorf("%s is missing durable subject %q; provision through infrastructure", LifecycleStream, required)
		}
	}
	return nil
}

func (c *Client) PublishLifecycle(ctx context.Context, event domain.OutboxEvent) error {
	message, err := lifecycleMessage(event)
	if err != nil {
		return err
	}
	js, err := c.jetStream()
	if err != nil {
		return err
	}
	if _, err := js.PublishMsg(message, natsgo.Context(ctx)); err != nil {
		return fmt.Errorf("publish lifecycle event: %w", err)
	}
	return nil
}

func lifecycleMessage(event domain.OutboxEvent) (*natsgo.Msg, error) {
	if err := event.Validate(); err != nil {
		return nil, err
	}
	message := natsgo.NewMsg(string(event.Subject))
	message.Data = append([]byte(nil), event.Payload...)
	message.Header.Set(natsgo.MsgIdHdr, event.ID.String())
	return message, nil
}

func (c *Client) PublishTyping(ctx context.Context, threadID string, payload []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if c == nil || c.Conn == nil {
		return errors.New("NATS connection is not configured")
	}
	return c.Conn.Publish("comment.realtime.typing."+threadID, payload)
}

func (c *Client) jetStream() (natsgo.JetStreamContext, error) {
	if c == nil || c.Conn == nil {
		return nil, errors.New("NATS connection is not configured")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.js != nil {
		return c.js, nil
	}
	js, err := c.Conn.JetStream()
	if err != nil {
		return nil, err
	}
	c.js = js
	return js, nil
}

var _ realtimeuc.LifecyclePublisher = (*Client)(nil)
