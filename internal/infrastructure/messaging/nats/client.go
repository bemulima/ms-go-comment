package nats

import (
	"context"
	"errors"
	"fmt"
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

func (c *Client) EnsureLifecycleStream(ctx context.Context) error {
	js, err := c.jetStream()
	if err != nil {
		return err
	}
	if _, err := js.StreamInfo(LifecycleStream, natsgo.Context(ctx)); err == nil {
		return nil
	} else if !errors.Is(err, natsgo.ErrStreamNotFound) {
		return fmt.Errorf("inspect lifecycle stream: %w", err)
	}
	_, err = js.AddStream(&natsgo.StreamConfig{
		Name: LifecycleStream, Subjects: append([]string(nil), lifecycleSubjects...),
		Retention: natsgo.LimitsPolicy, Storage: natsgo.FileStorage,
		MaxAge: 7 * 24 * time.Hour, Duplicates: 10 * time.Minute,
	}, natsgo.Context(ctx))
	if err != nil {
		return fmt.Errorf("create lifecycle stream: %w", err)
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
