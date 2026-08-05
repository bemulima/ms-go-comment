package realtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
)

const defaultOutboxLease = 30 * time.Second

type LifecyclePublisher interface {
	PublishLifecycle(context.Context, domain.OutboxEvent) error
}

type Dispatcher struct {
	Outbox    repository.OutboxRepository
	Publisher LifecyclePublisher
	Now       func() time.Time
	Lease     time.Duration
}

type DispatchResult struct {
	Published int
	Failed    int
}

func (d Dispatcher) Process(ctx context.Context, limit int) (DispatchResult, error) {
	if limit < 1 {
		return DispatchResult{}, fmt.Errorf("%w: dispatcher limit must be positive", domain.ErrValidation)
	}
	if d.Publisher == nil {
		return DispatchResult{}, errors.New("lifecycle publisher is not configured")
	}
	now := d.now()
	events, err := d.Outbox.ClaimPending(ctx, now, now.Add(d.lease()), limit)
	if err != nil {
		return DispatchResult{}, err
	}
	result := DispatchResult{}
	var deliveryErrors []error
	for _, event := range events {
		if err := d.Publisher.PublishLifecycle(ctx, event); err != nil {
			next := d.now().Add(outboxRetryDelay(event.Attempts + 1))
			if markErr := d.Outbox.MarkFailed(ctx, event.ID, next, boundedError(err)); markErr != nil {
				deliveryErrors = append(deliveryErrors, markErr)
			}
			result.Failed++
			continue
		}
		if err := d.Outbox.MarkPublished(ctx, event.ID, d.now()); err != nil && !errors.Is(err, domain.ErrNotFound) {
			deliveryErrors = append(deliveryErrors, err)
			continue
		}
		result.Published++
	}
	return result, errors.Join(deliveryErrors...)
}

func (d Dispatcher) lease() time.Duration {
	if d.Lease > 0 {
		return d.Lease
	}
	return defaultOutboxLease
}

func (d Dispatcher) now() time.Time {
	if d.Now != nil {
		return d.Now().UTC()
	}
	return time.Now().UTC()
}

func outboxRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := attempt - 1
	if shift > 6 {
		shift = 6
	}
	delay := 5 * time.Second * time.Duration(1<<shift)
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}

func boundedError(err error) string {
	const maxBytes = 2048
	value := err.Error()
	if len(value) > maxBytes {
		return value[:maxBytes]
	}
	return value
}
