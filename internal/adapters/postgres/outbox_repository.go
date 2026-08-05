package postgres

import (
	"context"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxRepository struct {
	Pool *pgxpool.Pool
}

func (r OutboxRepository) Add(ctx context.Context, item domain.OutboxEvent) error {
	_, err := runner(ctx, r.Pool).Exec(ctx, `INSERT INTO comment_outbox (
id, aggregate_type, aggregate_id, subject, schema_version, payload, attempts,
next_attempt_at, published_at, last_error, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, item.ID, item.AggregateType,
		item.AggregateID, item.Subject, item.SchemaVersion, item.Payload, item.Attempts,
		item.NextAttemptAt, item.PublishedAt, nullableString(item.LastError), item.CreatedAt)
	return mapError(err)
}

func (r OutboxRepository) ClaimPending(ctx context.Context, now, leaseUntil time.Time, limit int) ([]domain.OutboxEvent, error) {
	rows, err := runner(ctx, r.Pool).Query(ctx, `WITH pending AS (
    SELECT id FROM comment_outbox WHERE published_at IS NULL AND next_attempt_at<=$1
    ORDER BY next_attempt_at, created_at, id LIMIT $3 FOR UPDATE SKIP LOCKED
)
UPDATE comment_outbox item SET next_attempt_at=$2
FROM pending WHERE item.id=pending.id RETURNING
item.id, item.aggregate_type, item.aggregate_id, item.subject, item.schema_version,
item.payload, item.attempts, item.next_attempt_at, item.published_at,
COALESCE(item.last_error, ''), item.created_at`, now, leaseUntil, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.OutboxEvent, 0)
	for rows.Next() {
		item, err := scanOutbox(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r OutboxRepository) MarkPublished(ctx context.Context, eventID uuid.UUID, publishedAt time.Time) error {
	command, err := runner(ctx, r.Pool).Exec(ctx, `UPDATE comment_outbox
SET published_at=$1, last_error=NULL WHERE id=$2 AND published_at IS NULL`, publishedAt, eventID)
	if err != nil {
		return mapError(err)
	}
	if command.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r OutboxRepository) MarkFailed(ctx context.Context, eventID uuid.UUID, nextAttemptAt time.Time, message string) error {
	command, err := runner(ctx, r.Pool).Exec(ctx, `UPDATE comment_outbox
SET attempts=attempts+1, next_attempt_at=$1, last_error=$2
WHERE id=$3 AND published_at IS NULL`, nextAttemptAt, message, eventID)
	if err != nil {
		return mapError(err)
	}
	if command.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

var _ repository.OutboxRepository = (*OutboxRepository)(nil)
