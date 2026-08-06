package postgres

import (
	"context"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

const accessGrantColumns = `grant_hash, issuer, user_id, space_id, resource_type,
resource_id, permissions, expires_at, created_at`

type AccessGrantRepository struct {
	Pool *pgxpool.Pool
}

func (r AccessGrantRepository) Store(ctx context.Context, item domain.AccessGrant) error {
	_, err := runner(ctx, r.Pool).Exec(ctx, `INSERT INTO comment_access_grant (
grant_hash, issuer, user_id, space_id, resource_type, resource_id, permissions,
expires_at, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, item.GrantHash, item.Issuer,
		item.UserID, item.SpaceID, item.Resource.Type, item.Resource.ID,
		item.Permissions, item.ExpiresAt, item.CreatedAt)
	return mapError(err)
}

func (r AccessGrantRepository) Resolve(ctx context.Context, hash []byte, now time.Time) (domain.AccessGrant, error) {
	return scanAccessGrant(runner(ctx, r.Pool).QueryRow(ctx, `SELECT `+accessGrantColumns+`
FROM comment_access_grant WHERE grant_hash=$1 AND expires_at>$2`, hash, now))
}

func (r AccessGrantRepository) DeleteExpired(ctx context.Context, now time.Time, limit int) (int, error) {
	command, err := runner(ctx, r.Pool).Exec(ctx, `WITH expired AS (
    SELECT grant_hash FROM comment_access_grant
    WHERE expires_at <= $1 ORDER BY expires_at, grant_hash
    FOR UPDATE SKIP LOCKED LIMIT $2
)
DELETE FROM comment_access_grant item USING expired
WHERE item.grant_hash=expired.grant_hash`, now, limit)
	if err != nil {
		return 0, mapError(err)
	}
	return int(command.RowsAffected()), nil
}

func scanAccessGrant(row scanner) (domain.AccessGrant, error) {
	var item domain.AccessGrant
	err := row.Scan(&item.GrantHash, &item.Issuer, &item.UserID, &item.SpaceID,
		&item.Resource.Type, &item.Resource.ID, &item.Permissions, &item.ExpiresAt, &item.CreatedAt)
	return item, mapError(err)
}

var _ repository.AccessGrantRepository = (*AccessGrantRepository)(nil)
