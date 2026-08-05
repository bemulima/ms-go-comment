package postgres

import (
	"context"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RealtimeTicketRepository struct {
	Pool *pgxpool.Pool
}

func (r RealtimeTicketRepository) Store(ctx context.Context, item domain.RealtimeTicket) error {
	_, err := runner(ctx, r.Pool).Exec(ctx, `INSERT INTO comment_ws_ticket (
ticket_hash, user_id, thread_id, permissions, requested_last_sequence, expires_at, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7)`, item.TicketHash, item.UserID, item.ThreadID,
		item.Permissions, item.RequestedLastSequence, item.ExpiresAt, item.CreatedAt)
	return mapError(err)
}

func (r RealtimeTicketRepository) Consume(ctx context.Context, ticketHash []byte, now time.Time) (domain.RealtimeTicket, error) {
	return scanRealtimeTicket(runner(ctx, r.Pool).QueryRow(ctx, `DELETE FROM comment_ws_ticket
WHERE ticket_hash=$1 AND expires_at>$2 RETURNING `+realtimeTicketColumns, ticketHash, now))
}

func (r RealtimeTicketRepository) DeleteExpired(ctx context.Context, now time.Time, limit int) (int, error) {
	command, err := runner(ctx, r.Pool).Exec(ctx, `WITH expired AS (
    SELECT ticket_hash FROM comment_ws_ticket WHERE expires_at<=$1
    ORDER BY expires_at, ticket_hash LIMIT $2 FOR UPDATE SKIP LOCKED
)
DELETE FROM comment_ws_ticket item USING expired WHERE item.ticket_hash=expired.ticket_hash`, now, limit)
	if err != nil {
		return 0, mapError(err)
	}
	return int(command.RowsAffected()), nil
}

var _ repository.RealtimeTicketRepository = (*RealtimeTicketRepository)(nil)
